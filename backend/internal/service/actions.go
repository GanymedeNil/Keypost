package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"fulfillment/internal/db"
	"fulfillment/internal/stripe"
)

// ---------------------------------------------------------------- customers

type CustomerInput struct {
	StoreID                            int64
	TestMode                           bool
	Name, Email, City, Region, Country string
}

type CustomerPatch struct {
	Name, Email, City, Region, Country, Status, StripeCustomerID *string
}

func validEmail(e string) bool {
	at := strings.LastIndex(e, "@")
	return at > 0 && at < len(e)-3 && strings.Contains(e[at:], ".") && !strings.ContainsAny(e, " \t\r\n")
}

func (s *Svc) CreateCustomer(ctx context.Context, in CustomerInput) (*db.Customer, error) {
	var st db.Store
	if err := s.DB.First(&st, in.StoreID).Error; err != nil {
		return nil, Invalid("/data/relationships/store", "The selected store is invalid.")
	}
	if strings.TrimSpace(in.Name) == "" {
		return nil, Invalid("/data/attributes/name", "The name field is required.")
	}
	in.Email = strings.TrimSpace(in.Email)
	if !validEmail(in.Email) {
		return nil, Invalid("/data/attributes/email", "The email must be a valid email address.")
	}
	var n int64
	s.DB.Model(&db.Customer{}).Where("store_id = ? AND test_mode = ? AND LOWER(email) = ?", in.StoreID, in.TestMode, strings.ToLower(in.Email)).Count(&n)
	if n > 0 {
		return nil, Invalid("/data/attributes/email", "The email has already been taken.")
	}
	now := db.Now()
	c := db.Customer{StoreID: in.StoreID, TestMode: in.TestMode, Name: in.Name, Email: in.Email, Status: "subscribed", City: in.City, Region: in.Region, Country: strings.ToUpper(in.Country), CreatedAt: now, UpdatedAt: now}
	if err := s.DB.WithContext(ctx).Create(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Svc) UpdateCustomer(ctx context.Context, id int64, p CustomerPatch) (*db.Customer, error) {
	var c db.Customer
	if err := s.DB.WithContext(ctx).First(&c, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, NotFound("Customer")
		}
		return nil, err
	}
	upd := map[string]any{"updated_at": db.Now()}
	if p.Name != nil {
		upd["name"] = *p.Name
	}
	if p.Email != nil {
		e := strings.TrimSpace(*p.Email)
		if !validEmail(e) {
			return nil, Invalid("/data/attributes/email", "The email must be a valid email address.")
		}
		var n int64
		s.DB.Model(&db.Customer{}).Where("store_id = ? AND test_mode = ? AND LOWER(email) = ? AND id <> ?", c.StoreID, c.TestMode, strings.ToLower(e), c.ID).Count(&n)
		if n > 0 {
			return nil, Invalid("/data/attributes/email", "The email has already been taken.")
		}
		upd["email"] = e
	}
	if p.City != nil {
		upd["city"] = *p.City
	}
	if p.Region != nil {
		upd["region"] = *p.Region
	}
	if p.Country != nil {
		upd["country"] = strings.ToUpper(*p.Country)
	}
	if p.Status != nil {
		switch *p.Status {
		case "subscribed", "unsubscribed", "archived", "requires_verification", "invalid_email", "bounced":
			upd["status"] = *p.Status
		default:
			return nil, Invalid("/data/attributes/status", "The selected status is invalid.")
		}
	}
	if p.StripeCustomerID != nil {
		upd["stripe_customer_id"] = *p.StripeCustomerID
	}
	if err := s.DB.WithContext(ctx).Model(&c).Updates(upd).Error; err != nil {
		return nil, err
	}
	if err := s.DB.First(&c, id).Error; err != nil {
		return nil, err
	}
	s.Events.Emit(ctx, Event{Name: "customer_updated", StoreID: c.StoreID, TestMode: c.TestMode, ResourceType: "customers", ResourceID: c.ID})
	return &c, nil
}

// PortalURL creates a Stripe Billing Portal session for the customer.
func (s *Svc) PortalURL(ctx context.Context, customerID int64, returnURL string) (string, error) {
	var c db.Customer
	if err := s.DB.First(&c, customerID).Error; err != nil {
		return "", NotFound("Customer")
	}
	cl, err := s.Stripe()
	if err != nil {
		return "", stripeErr(err)
	}
	if c.StripeCustomerID == "" {
		list, err := cl.ListCustomersByEmail(ctx, c.Email)
		if err != nil {
			return "", stripeErr(err)
		}
		if len(list) == 0 {
			return "", Invalid("", "This customer has no Stripe customer yet (no Stripe payment or mapping).")
		}
		c.StripeCustomerID = list[0].ID
		s.DB.Model(&c).Update("stripe_customer_id", c.StripeCustomerID)
	}
	if returnURL == "" {
		returnURL = s.URLs.Base + "/my-orders"
	}
	ps, err := cl.CreatePortalSession(ctx, c.StripeCustomerID, returnURL)
	if err != nil {
		return "", stripeErr(err)
	}
	return ps.URL, nil
}

// ---------------------------------------------------------------- subscriptions (actions)

func (s *Svc) stripeSubFor(id int64) (*db.Subscription, string, error) {
	var sub db.Subscription
	if err := s.DB.First(&sub, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", NotFound("Subscription")
		}
		return nil, "", err
	}
	if sub.StripeSubscriptionID == nil || *sub.StripeSubscriptionID == "" {
		return nil, "", Invalid("", "This subscription is not linked to a Stripe subscription yet. Link it first (Imports → mapping).")
	}
	return &sub, *sub.StripeSubscriptionID, nil
}

func (s *Svc) updateStripeSub(ctx context.Context, id int64, p *stripe.Params) (*db.Subscription, error) {
	sub, sid, err := s.stripeSubFor(id)
	if err != nil {
		return nil, err
	}
	cl, err := s.Stripe()
	if err != nil {
		return nil, stripeErr(err)
	}
	ss, err := cl.UpdateSubscription(ctx, sid, p)
	if err != nil {
		return nil, stripeErr(err)
	}
	if _, err := s.applySubscription(ctx, ss); err != nil {
		return nil, err
	}
	s.DB.First(sub, id)
	return sub, nil
}

// CancelSubscription cancels at the end of the paid period (grace period).
func (s *Svc) CancelSubscription(ctx context.Context, id int64) (*db.Subscription, error) {
	return s.updateStripeSub(ctx, id, stripe.P().Bool("cancel_at_period_end", true))
}

func (s *Svc) ResumeSubscription(ctx context.Context, id int64) (*db.Subscription, error) {
	var sub db.Subscription
	if err := s.DB.First(&sub, id).Error; err != nil {
		return nil, NotFound("Subscription")
	}
	if sub.Status == "expired" {
		return nil, Invalid("", "An expired subscription cannot be resumed; the customer must subscribe again.")
	}
	return s.updateStripeSub(ctx, id, stripe.P().Bool("cancel_at_period_end", false).Set("cancel_at", ""))
}

func (s *Svc) PauseSubscription(ctx context.Context, id int64, mode string, resumesAt *time.Time) (*db.Subscription, error) {
	behavior := "mark_uncollectible"
	switch mode {
	case "void":
		behavior = "void"
	case "free", "":
	default:
		return nil, Invalid("/data/attributes/pause/mode", "The pause mode must be void or free.")
	}
	p := stripe.P().Set("pause_collection[behavior]", behavior)
	if resumesAt != nil {
		p.Int("pause_collection[resumes_at]", resumesAt.Unix())
	}
	return s.updateStripeSub(ctx, id, p)
}

func (s *Svc) UnpauseSubscription(ctx context.Context, id int64) (*db.Subscription, error) {
	return s.updateStripeSub(ctx, id, stripe.P().Set("pause_collection", ""))
}

// SyncSubscription refetches the subscription from Stripe.
func (s *Svc) SyncSubscription(ctx context.Context, id int64) (*db.Subscription, error) {
	sub, sid, err := s.stripeSubFor(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.RefreshSubscription(ctx, sid); err != nil {
		return nil, err
	}
	s.DB.First(sub, id)
	return sub, nil
}

// LinkStripeSubscription attaches an existing Stripe subscription (migration).
func (s *Svc) LinkStripeSubscription(ctx context.Context, id int64, stripeSubID string) (*db.Subscription, error) {
	stripeSubID = strings.TrimSpace(stripeSubID)
	if !strings.HasPrefix(stripeSubID, "sub_") {
		return nil, Invalid("stripe_subscription_id", "Must look like sub_…")
	}
	var clash int64
	s.DB.Model(&db.Subscription{}).Where("stripe_subscription_id = ? AND id <> ?", stripeSubID, id).Count(&clash)
	if clash > 0 {
		return nil, Conflict("That Stripe subscription is already linked to another subscription.")
	}
	var sub db.Subscription
	if err := s.DB.First(&sub, id).Error; err != nil {
		return nil, NotFound("Subscription")
	}
	if err := s.DB.Model(&sub).Updates(map[string]any{"stripe_subscription_id": stripeSubID, "updated_at": db.Now()}).Error; err != nil {
		return nil, err
	}
	return s.SyncSubscription(ctx, id)
}

// ---------------------------------------------------------------- refunds (actions)

func reasonOK(r string) bool {
	return r == "duplicate" || r == "fraudulent" || r == "requested_by_customer"
}

// paymentRefs finds the PaymentIntent / Charge id to refund for an order.
func (s *Svc) paymentRefsForOrder(ctx context.Context, o db.Order) (pi, charge string) {
	if o.StripePaymentIntent != "" || o.StripeChargeID != "" {
		return o.StripePaymentIntent, o.StripeChargeID
	}
	var inv db.SubscriptionInvoice
	if s.DB.Joins("JOIN subscriptions ON subscriptions.id = subscription_invoices.subscription_id").
		Where("subscriptions.order_id = ? AND subscription_invoices.billing_reason = ?", o.ID, "initial").First(&inv).Error == nil {
		if inv.StripePaymentIntent != "" || inv.StripeChargeID != "" {
			return inv.StripePaymentIntent, inv.StripeChargeID
		}
	}
	if o.StripeInvoiceID != "" {
		if cl, err := s.Stripe(); err == nil {
			if si, err := cl.RetrieveInvoice(ctx, o.StripeInvoiceID); err == nil {
				return si.PaymentIntentID(), si.ChargeID()
			}
		}
	}
	return "", ""
}

func (s *Svc) createRefund(ctx context.Context, pi, charge string, amount int64, idem, reason string) (*stripe.Refund, error) {
	cl, err := s.Stripe()
	if err != nil {
		return nil, stripeErr(err)
	}
	p := stripe.P().Int("amount", amount)
	switch {
	case pi != "":
		p.Set("payment_intent", pi)
	case charge != "":
		p.Set("charge", charge)
	default:
		return nil, Invalid("", "No Stripe payment is attached to this record, so it cannot be refunded here.")
	}
	if reasonOK(reason) {
		p.Set("reason", reason)
	}
	r, err := cl.CreateRefund(ctx, p, idem)
	if err != nil {
		return nil, stripeErr(err)
	}
	return r, nil
}

// RefundOrder refunds (part of) an order through Stripe and updates the order.
func (s *Svc) RefundOrder(ctx context.Context, orderID int64, amount *int64, reason string) (*db.Order, error) {
	var o db.Order
	if err := s.DB.WithContext(ctx).First(&o, orderID).Error; err != nil {
		return nil, NotFound("Order")
	}
	remaining := o.Total - o.RefundedAmount
	if remaining <= 0 {
		return nil, Invalid("", "This order has already been fully refunded.")
	}
	amt := remaining
	if amount != nil {
		amt = *amount
	}
	if amt <= 0 || amt > remaining {
		return nil, Invalid("/data/attributes/amount", fmt.Sprintf("The amount must be between 1 and %d.", remaining))
	}
	pi, ch := s.paymentRefsForOrder(ctx, o)
	r, err := s.createRefund(ctx, pi, ch, amt, "refund-order-"+strconv.FormatInt(o.ID, 10)+"-"+strconv.FormatInt(o.RefundedAmount, 10)+"-"+strconv.FormatInt(amt, 10), reason)
	if err != nil {
		return nil, err
	}
	s.applyOrderRefund(ctx, o.ID, o.RefundedAmount+r.Amount)
	if err := s.DB.First(&o, orderID).Error; err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *Svc) RefundSubscriptionInvoice(ctx context.Context, id int64, amount *int64) (*db.SubscriptionInvoice, error) {
	var inv db.SubscriptionInvoice
	if err := s.DB.WithContext(ctx).First(&inv, id).Error; err != nil {
		return nil, NotFound("Subscription invoice")
	}
	remaining := inv.Total - inv.RefundedAmount
	if remaining <= 0 {
		return nil, Invalid("", "This invoice has already been fully refunded.")
	}
	amt := remaining
	if amount != nil {
		amt = *amount
	}
	if amt <= 0 || amt > remaining {
		return nil, Invalid("/data/attributes/amount", fmt.Sprintf("The amount must be between 1 and %d.", remaining))
	}
	r, err := s.createRefund(ctx, inv.StripePaymentIntent, inv.StripeChargeID, amt, "refund-invoice-"+strconv.FormatInt(inv.ID, 10)+"-"+strconv.FormatInt(inv.RefundedAmount, 10)+"-"+strconv.FormatInt(amt, 10), "")
	if err != nil {
		return nil, err
	}
	s.applyInvoiceRefund(ctx, inv.ID, inv.RefundedAmount+r.Amount)
	if inv.BillingReason == "initial" {
		var sub db.Subscription
		if s.DB.First(&sub, inv.SubscriptionID).Error == nil && sub.OrderID != 0 {
			var o db.Order
			if s.DB.First(&o, sub.OrderID).Error == nil {
				s.applyOrderRefund(ctx, o.ID, o.RefundedAmount+r.Amount)
			}
		}
	}
	if err := s.DB.First(&inv, id).Error; err != nil {
		return nil, err
	}
	return &inv, nil
}
