package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"fulfillment/internal/db"
	"fulfillment/internal/settings"
	"fulfillment/internal/stripe"
)

var ErrUnknownSubscription = errors.New("subscription is not known locally")

// ---------------------------------------------------------------- applying Stripe subscription state

// applySubscription reconciles the local subscription with a Stripe
// subscription object and emits the matching lifecycle events.
func (s *Svc) applySubscription(ctx context.Context, ss *stripe.Subscription) (*db.Subscription, error) {
	var sub db.Subscription
	buf := &eventBuf{}
	changed := false
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("stripe_subscription_id = ?", ss.ID).First(&sub).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUnknownSubscription
			}
			return err
		}
		now := db.Now()
		old := sub
		before := subSnapshot(sub)
		st := mapSubscription(*ss, now)
		sub.Status, sub.Cancelled, sub.EndsAt, sub.RenewsAt, sub.TrialEndsAt = st.Status, st.Cancelled, st.EndsAt, st.RenewsAt, st.TrialEndsAt
		sub.Pause = db.J(st.Pause)
		if st.Anchor > 0 {
			sub.BillingAnchor = st.Anchor
		}
		if ss.Customer.ID != "" {
			sub.StripeCustomerID = ss.Customer.ID
		}
		if len(ss.Items.Data) > 0 {
			it := ss.Items.Data[0]
			if it.Quantity > 0 {
				sub.Quantity = it.Quantity
			}
			if it.Price != nil && it.Price.ID != "" {
				var p db.Price
				if tx.Where("stripe_price_id = ?", it.Price.ID).First(&p).Error == nil && (sub.PriceID == nil || *sub.PriceID != p.ID) {
					var v db.Variant
					var pr db.Product
					if tx.First(&v, p.VariantID).Error == nil && tx.First(&pr, v.ProductID).Error == nil {
						pid := p.ID
						sub.PriceID, sub.VariantID, sub.ProductID = &pid, v.ID, pr.ID
						sub.VariantName, sub.ProductName = v.Name, pr.Name
						tx.Model(&db.SubscriptionItem{}).Where("subscription_id = ?", sub.ID).Updates(map[string]any{"price_id": p.ID, "quantity": sub.Quantity, "updated_at": now})
					}
				}
			}
		}
		var pm stripe.PaymentMethod
		if ss.DefaultPaymentMethod.Decode(&pm) && pm.Card != nil {
			sub.CardBrand, sub.CardLastFour = strings.ToLower(pm.Card.Brand), pm.Card.Last4
		}
		if subSnapshot(sub) == before {
			return nil
		}
		changed = true
		sub.UpdatedAt = now
		if err := tx.Save(&sub).Error; err != nil {
			return err
		}
		keys, err := s.SyncSubscriptionLicenses(tx, sub)
		if err != nil {
			return err
		}
		ev := func(name string) {
			buf.add(Event{Name: name, StoreID: sub.StoreID, TestMode: sub.TestMode, ResourceType: "subscriptions", ResourceID: sub.ID, CustomData: sub.CustomData.V})
		}
		switch {
		case sub.Status == "cancelled" && old.Status != "cancelled":
			ev("subscription_cancelled")
		case old.Status == "cancelled" && sub.Status != "cancelled" && sub.Status != "expired":
			ev("subscription_resumed")
		}
		if sub.Status == "expired" && old.Status != "expired" {
			ev("subscription_expired")
		}
		if sub.Status == "paused" && old.Status != "paused" {
			ev("subscription_paused")
		}
		if old.Status == "paused" && sub.Status != "paused" {
			ev("subscription_unpaused")
		}
		ev("subscription_updated")
		for _, k := range keys {
			buf.add(Event{Name: "license_key_updated", StoreID: k.StoreID, TestMode: k.TestMode, ResourceType: "license-keys", ResourceID: k.ID, CustomData: sub.CustomData.V})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if changed {
		s.flush(ctx, buf)
	}
	return &sub, nil
}

// RefreshSubscription pulls the subscription from Stripe and applies it.
func (s *Svc) RefreshSubscription(ctx context.Context, stripeSubID string) (*db.Subscription, error) {
	cl, err := s.Stripe()
	if err != nil {
		return nil, err
	}
	ss, err := cl.RetrieveSubscription(ctx, stripeSubID)
	if err != nil {
		return nil, stripeErr(err)
	}
	return s.applySubscription(ctx, ss)
}

// ---------------------------------------------------------------- invoices

type invoiceOutcome struct {
	Rec        db.SubscriptionInvoice
	Created    bool
	BecamePaid bool
	WasFailed  bool
	Skipped    bool
}

func billingReason(r string) string {
	switch r {
	case "subscription_create":
		return "initial"
	case "subscription_update":
		return "updated"
	default:
		return "renewal"
	}
}

// recordInvoice upserts the local invoice for a Stripe invoice. When emit is
// true the payment_success / payment_recovered events are fired.
func (s *Svc) recordInvoice(ctx context.Context, cl *stripe.Client, inv *stripe.Invoice, sub db.Subscription, emit bool) (*invoiceOutcome, error) {
	if inv.Total == 0 && inv.AmountPaid == 0 && inv.Status == "paid" {
		return &invoiceOutcome{Skipped: true}, nil // trial / free invoices carry no payment
	}
	cur := strings.ToUpper(inv.Currency)
	if cur == "" {
		cur = "USD"
	}
	status := "pending"
	switch inv.Status {
	case "paid":
		status = "paid"
	case "void":
		status = "void"
	}
	piID, chargeID := inv.PaymentIntentID(), inv.ChargeID()
	var ch *stripe.Charge
	if chargeID == "" && piID != "" {
		if pi, err := cl.RetrievePaymentIntent(ctx, piID); err == nil {
			chargeID = pi.LatestCharge.ID
		}
	}
	if chargeID != "" {
		if c, err := cl.RetrieveCharge(ctx, chargeID); err == nil {
			ch = c
			if piID == "" {
				piID = c.PaymentIntent.ID
			}
		}
	}
	brand, last4 := cardOf(ch, nil)
	if brand == "" {
		brand, last4 = sub.CardBrand, sub.CardLastFour
	}
	tax, disc := inv.TaxAmount(), inv.DiscountAmount()
	out := &invoiceOutcome{}
	buf := &eventBuf{}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rec db.SubscriptionInvoice
		err := lockForUpdate(tx).Where("stripe_invoice_id = ?", inv.ID).First(&rec).Error
		now := db.Now()
		exists := err == nil
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		prev := rec.Status
		if !exists {
			rec = db.SubscriptionInvoice{
				StoreID: sub.StoreID, SubscriptionID: sub.ID, CustomerID: sub.CustomerID, UserName: sub.UserName, UserEmail: sub.UserEmail,
				BillingReason: billingReason(inv.BillingReason), Currency: cur, CurrencyRate: s.currencyRate(cur),
				StripeInvoiceID: db.Ptr(inv.ID), TestMode: sub.TestMode, CreatedAt: unix(inv.Created), UpdatedAt: now,
			}
			if inv.Created == 0 {
				rec.CreatedAt = now
			}
		}
		rec.Status = status
		rec.CardBrand, rec.CardLastFour = brand, last4
		rec.Subtotal, rec.DiscountTotal, rec.Tax, rec.Total = inv.Subtotal, disc, tax, inv.Total
		rec.SubtotalUSD, rec.DiscountTotalUSD, rec.TaxUSD, rec.TotalUSD = s.toUSD(inv.Subtotal, cur), s.toUSD(disc, cur), s.toUSD(tax, cur), s.toUSD(inv.Total, cur)
		rec.InvoiceURL = inv.HostedInvoiceURL
		if piID != "" {
			rec.StripePaymentIntent = piID
		}
		if chargeID != "" {
			rec.StripeChargeID = chargeID
		}
		rec.UpdatedAt = now
		if exists {
			if err := tx.Save(&rec).Error; err != nil {
				return err
			}
		} else if err := tx.Create(&rec).Error; err != nil {
			return err
		}
		out.Rec, out.Created = rec, !exists
		out.WasFailed = exists && prev == "pending"
		out.BecamePaid = status == "paid" && prev != "paid"
		if emit && out.BecamePaid {
			buf.add(Event{Name: "subscription_payment_success", StoreID: rec.StoreID, TestMode: rec.TestMode, ResourceType: "subscription-invoices", ResourceID: rec.ID, CustomData: sub.CustomData.V})
			if out.WasFailed || sub.Status == "past_due" || sub.Status == "unpaid" {
				buf.add(Event{Name: "subscription_payment_recovered", StoreID: rec.StoreID, TestMode: rec.TestMode, ResourceType: "subscription-invoices", ResourceID: rec.ID, CustomData: sub.CustomData.V})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.flush(ctx, buf)
	return out, nil
}

func (s *Svc) subByStripeID(id string) (*db.Subscription, bool) {
	if id == "" {
		return nil, false
	}
	var sub db.Subscription
	if err := s.DB.Where("stripe_subscription_id = ?", id).First(&sub).Error; err != nil {
		return nil, false
	}
	return &sub, true
}

func (s *Svc) handleInvoicePaid(ctx context.Context, invID string) (Result, error) {
	cl, err := s.Stripe()
	if err != nil {
		return Result{}, err
	}
	inv, err := cl.RetrieveInvoice(ctx, invID)
	if err != nil {
		return Result{}, fmt.Errorf("retrieve invoice: %w", err)
	}
	subID := inv.SubscriptionID()
	if subID == "" {
		return ignored("invoice is not tied to a subscription"), nil
	}
	sub, ok := s.subByStripeID(subID)
	if !ok {
		if inv.BillingReason == "subscription_create" {
			return ignored("initial invoice is recorded during checkout fulfilment"), nil
		}
		return ignored("unknown subscription " + subID + " (import / map it, then replay this event)"), nil
	}
	out, err := s.recordInvoice(ctx, cl, inv, *sub, true)
	if err != nil {
		return Result{}, err
	}
	if out.Skipped {
		return ignored("zero-amount invoice"), nil
	}
	if _, err := s.RefreshSubscription(ctx, subID); err != nil && !errors.Is(err, ErrUnknownSubscription) {
		return Result{}, err
	}
	return processed(fmt.Sprintf("invoice %s recorded (%s)", invID, out.Rec.Status)), nil
}

func (s *Svc) handleInvoiceFailed(ctx context.Context, invID string) (Result, error) {
	cl, err := s.Stripe()
	if err != nil {
		return Result{}, err
	}
	inv, err := cl.RetrieveInvoice(ctx, invID)
	if err != nil {
		return Result{}, fmt.Errorf("retrieve invoice: %w", err)
	}
	sub, ok := s.subByStripeID(inv.SubscriptionID())
	if !ok {
		return ignored("not a known subscription invoice"), nil
	}
	out, err := s.recordInvoice(ctx, cl, inv, *sub, false)
	if err != nil {
		return Result{}, err
	}
	if !out.Skipped {
		s.Events.Emit(ctx, Event{Name: "subscription_payment_failed", StoreID: sub.StoreID, TestMode: sub.TestMode, ResourceType: "subscription-invoices", ResourceID: out.Rec.ID, CustomData: sub.CustomData.V})
	}
	if _, err := s.RefreshSubscription(ctx, inv.SubscriptionID()); err != nil && !errors.Is(err, ErrUnknownSubscription) {
		return Result{}, err
	}
	return processed("payment failure recorded"), nil
}

func (s *Svc) handleSubscriptionEvent(ctx context.Context, subID string) (Result, error) {
	if _, ok := s.subByStripeID(subID); !ok {
		return ignored("unknown subscription " + subID + " (import / map it, then replay this event)"), nil
	}
	if _, err := s.RefreshSubscription(ctx, subID); err != nil {
		return Result{}, err
	}
	return processed("subscription synchronised"), nil
}

// ---------------------------------------------------------------- refunds (webhook driven)

func (s *Svc) handleChargeRefunded(ctx context.Context, chargeID string) (Result, error) {
	cl, err := s.Stripe()
	if err != nil {
		return Result{}, err
	}
	ch, err := cl.RetrieveCharge(ctx, chargeID)
	if err != nil {
		return Result{}, fmt.Errorf("retrieve charge: %w", err)
	}
	piID := ch.PaymentIntent.ID
	var note []string

	var o db.Order
	q := s.DB.Where("stripe_charge_id = ?", ch.ID)
	if piID != "" {
		q = s.DB.Where("stripe_charge_id = ? OR stripe_payment_intent = ?", ch.ID, piID)
	}
	if q.First(&o).Error == nil {
		if s.applyOrderRefund(ctx, o.ID, ch.AmountRefunded) {
			note = append(note, fmt.Sprintf("order #%d refunded %d", o.OrderNumber, ch.AmountRefunded))
		}
	}
	var inv db.SubscriptionInvoice
	q2 := s.DB.Where("stripe_charge_id = ?", ch.ID)
	if piID != "" {
		q2 = s.DB.Where("stripe_charge_id = ? OR stripe_payment_intent = ?", ch.ID, piID)
	}
	if q2.First(&inv).Error == nil {
		if s.applyInvoiceRefund(ctx, inv.ID, ch.AmountRefunded) {
			note = append(note, fmt.Sprintf("invoice #%d refunded %d", inv.ID, ch.AmountRefunded))
		}
		if inv.BillingReason == "initial" {
			var sub db.Subscription
			if s.DB.First(&sub, inv.SubscriptionID).Error == nil && sub.OrderID != 0 {
				if s.applyOrderRefund(ctx, sub.OrderID, ch.AmountRefunded) {
					note = append(note, "initial order refunded")
				}
			}
		}
	}
	if len(note) == 0 {
		return ignored("no matching order or invoice (or refund already applied)"), nil
	}
	return processed(strings.Join(note, "; ")), nil
}

func capRefund(total, amt int64) int64 {
	if amt > total {
		return total
	}
	if amt < 0 {
		return 0
	}
	return amt
}

// applyOrderRefund sets the cumulative refunded amount. It returns whether
// anything changed, so repeated deliveries are no-ops.
func (s *Svc) applyOrderRefund(ctx context.Context, orderID, refunded int64) bool {
	var o db.Order
	buf := &eventBuf{}
	changed := false
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&o, orderID).Error; err != nil {
			return err
		}
		refunded = capRefund(o.Total, refunded)
		if refunded == o.RefundedAmount {
			return nil
		}
		changed = true
		now := db.Now()
		o.RefundedAmount = refunded
		o.RefundedAmountUSD = s.toUSD(refunded, o.Currency)
		switch {
		case refunded == 0:
			o.Status, o.Refunded, o.RefundedAt = "paid", false, nil
		case refunded >= o.Total:
			o.Status, o.Refunded, o.RefundedAt = "refunded", true, &now
		default:
			o.Status, o.Refunded, o.RefundedAt = "partial_refund", false, nil
		}
		o.UpdatedAt = now
		if err := tx.Save(&o).Error; err != nil {
			return err
		}
		buf.add(Event{Name: "order_refunded", StoreID: o.StoreID, TestMode: o.TestMode, ResourceType: "orders", ResourceID: o.ID, CustomData: o.CustomData.V})
		if o.Refunded && s.Settings.Bool(settings.LicenseDisableOnRefund) {
			var keys []db.LicenseKey
			if err := tx.Where("order_id = ? AND disabled = ?", o.ID, false).Find(&keys).Error; err != nil {
				return err
			}
			for _, k := range keys {
				if err := tx.Model(&k).Updates(map[string]any{"disabled": true, "updated_at": now}).Error; err != nil {
					return err
				}
				buf.add(Event{Name: "license_key_updated", StoreID: k.StoreID, TestMode: k.TestMode, ResourceType: "license-keys", ResourceID: k.ID, CustomData: o.CustomData.V})
			}
		}
		return nil
	})
	if err != nil {
		s.logf("apply order refund: %v", err)
		return false
	}
	if changed {
		s.flush(ctx, buf)
	}
	return changed
}

func (s *Svc) applyInvoiceRefund(ctx context.Context, invoiceID, refunded int64) bool {
	var inv db.SubscriptionInvoice
	buf := &eventBuf{}
	changed := false
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&inv, invoiceID).Error; err != nil {
			return err
		}
		refunded = capRefund(inv.Total, refunded)
		if refunded == inv.RefundedAmount {
			return nil
		}
		changed = true
		now := db.Now()
		inv.RefundedAmount = refunded
		inv.RefundedAmountUSD = s.toUSD(refunded, inv.Currency)
		switch {
		case refunded == 0:
			inv.Status, inv.Refunded, inv.RefundedAt = "paid", false, nil
		case refunded >= inv.Total:
			inv.Status, inv.Refunded, inv.RefundedAt = "refunded", true, &now
		default:
			inv.Status, inv.Refunded, inv.RefundedAt = "partial_refund", false, nil
		}
		inv.UpdatedAt = now
		if err := tx.Save(&inv).Error; err != nil {
			return err
		}
		buf.add(Event{Name: "subscription_payment_refunded", StoreID: inv.StoreID, TestMode: inv.TestMode, ResourceType: "subscription-invoices", ResourceID: inv.ID})
		return nil
	})
	if err != nil {
		s.logf("apply invoice refund: %v", err)
		return false
	}
	if changed {
		s.flush(ctx, buf)
	}
	return changed
}

// ---------------------------------------------------------------- customer.updated

func (s *Svc) handleCustomerUpdated(ctx context.Context, sc stripe.Customer) (Result, error) {
	var cs []db.Customer
	s.DB.Where("stripe_customer_id = ?", sc.ID).Find(&cs)
	if len(cs) == 0 {
		return ignored("customer not known locally"), nil
	}
	n := 0
	for _, c := range cs {
		upd := map[string]any{}
		if sc.Name != "" && sc.Name != c.Name {
			upd["name"] = sc.Name
		}
		if sc.Email != "" && !strings.EqualFold(sc.Email, c.Email) {
			var clash int64
			s.DB.Model(&db.Customer{}).Where("store_id = ? AND test_mode = ? AND LOWER(email) = ? AND id <> ?", c.StoreID, c.TestMode, strings.ToLower(sc.Email), c.ID).Count(&clash)
			if clash == 0 {
				upd["email"] = sc.Email
			}
		}
		if a := sc.Address; a != nil {
			if a.Country != "" && a.Country != c.Country {
				upd["country"] = a.Country
			}
			if a.City != "" && a.City != c.City {
				upd["city"] = a.City
			}
			if a.State != "" && a.State != c.Region {
				upd["region"] = a.State
			}
		}
		if len(upd) == 0 {
			continue
		}
		upd["updated_at"] = db.Now()
		if err := s.DB.Model(&c).Updates(upd).Error; err != nil {
			return Result{}, err
		}
		s.Events.Emit(ctx, Event{Name: "customer_updated", StoreID: c.StoreID, TestMode: c.TestMode, ResourceType: "customers", ResourceID: c.ID})
		n++
	}
	if n == 0 {
		return ignored("no customer fields changed"), nil
	}
	return processed(fmt.Sprintf("%d customer(s) updated", n)), nil
}
