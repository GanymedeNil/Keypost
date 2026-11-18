package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"fulfillment/internal/db"
	"fulfillment/internal/mail"
	"fulfillment/internal/resource"
	"fulfillment/internal/settings"
	"fulfillment/internal/stripe"
)

// Result of processing a Stripe event.
type Result struct {
	Status string // processed | ignored
	Note   string
}

func processed(note string) Result { return Result{Status: "processed", Note: note} }
func ignored(note string) Result   { return Result{Status: "ignored", Note: note} }

func unix(n int64) time.Time { return time.Unix(n, 0).UTC() }

func unixPtr(n int64) *time.Time {
	if n <= 0 {
		return nil
	}
	t := unix(n)
	return &t
}

func unixPtrP(n *int64) *time.Time {
	if n == nil {
		return nil
	}
	return unixPtr(*n)
}

// ---------------------------------------------------------------- lookups

func (s *Svc) resolveStore(sess *stripe.Session, account string) (*db.Store, error) {
	var st db.Store
	if id, err := strconv.ParseInt(sess.Metadata["store_id"], 10, 64); err == nil && id > 0 {
		if s.DB.First(&st, id).Error == nil {
			return &st, nil
		}
	}
	if account != "" && s.DB.Where("stripe_account_id = ?", account).First(&st).Error == nil {
		return &st, nil
	}
	if err := s.DB.Order("id").First(&st).Error; err != nil {
		return nil, fmt.Errorf("no store configured")
	}
	return &st, nil
}

type matched struct {
	Variant db.Variant
	Price   db.Price
	Product db.Product
	Found   bool
}

func (s *Svc) matchLineItem(tx *gorm.DB, li stripe.LineItem) matched {
	var m matched
	if li.Price == nil {
		return m
	}
	// 1. exact Stripe price id
	var p db.Price
	if li.Price.ID != "" && tx.Where("stripe_price_id = ?", li.Price.ID).First(&p).Error == nil {
		if tx.First(&m.Variant, p.VariantID).Error == nil && tx.First(&m.Product, m.Variant.ProductID).Error == nil {
			m.Price, m.Found = p, true
			return m
		}
	}
	// 2. Stripe product id mapped on a variant (each LS variant became its own Stripe product)
	var sp stripe.Product
	li.Price.Product.Decode(&sp)
	prodID := li.Price.Product.ID
	if prodID != "" && tx.Where("stripe_product_id = ?", prodID).Order("id").First(&m.Variant).Error == nil {
		_ = tx.First(&m.Product, m.Variant.ProductID).Error
		_ = tx.Where("variant_id = ?", m.Variant.ID).Order("id DESC").First(&m.Price).Error
		m.Found = true
		return m
	}
	// 3. product metadata (migration tooling / manual tagging)
	for _, k := range []string{"variant_id", "ls_variant_id", "lemonsqueezy_variant_id"} {
		if v := sp.Metadata[k]; v != "" {
			if id, err := strconv.ParseInt(v, 10, 64); err == nil && tx.First(&m.Variant, id).Error == nil {
				_ = tx.First(&m.Product, m.Variant.ProductID).Error
				_ = tx.Where("variant_id = ?", m.Variant.ID).Order("id DESC").First(&m.Price).Error
				m.Found = true
				return m
			}
		}
	}
	// 4. product-level mapping with a single variant
	var prod db.Product
	if prodID != "" && tx.Where("stripe_product_id = ?", prodID).First(&prod).Error == nil {
		var vs []db.Variant
		tx.Where("product_id = ?", prod.ID).Order("sort, id").Limit(2).Find(&vs)
		if len(vs) == 1 {
			m.Variant, m.Product, m.Found = vs[0], prod, true
			_ = tx.Where("variant_id = ?", m.Variant.ID).Order("id DESC").First(&m.Price).Error
		}
	}
	return m
}

// upsertCustomer finds the customer by Stripe id or email (case-insensitive) or creates it.
func (s *Svc) upsertCustomer(tx *gorm.DB, storeID int64, test bool, email, name, stripeID string, addr *stripe.Address) (*db.Customer, error) {
	email = strings.TrimSpace(email)
	var c db.Customer
	found := false
	if stripeID != "" {
		found = tx.Where("store_id = ? AND test_mode = ? AND stripe_customer_id = ?", storeID, test, stripeID).First(&c).Error == nil
	}
	if !found && email != "" {
		found = tx.Where("store_id = ? AND test_mode = ? AND LOWER(email) = ?", storeID, test, strings.ToLower(email)).First(&c).Error == nil
	}
	now := db.Now()
	if found {
		upd := map[string]any{}
		if c.StripeCustomerID == "" && stripeID != "" {
			upd["stripe_customer_id"] = stripeID
			c.StripeCustomerID = stripeID
		}
		if c.Name == "" && name != "" {
			upd["name"] = name
			c.Name = name
		}
		if addr != nil {
			if c.Country == "" && addr.Country != "" {
				upd["country"], c.Country = addr.Country, addr.Country
			}
			if c.City == "" && addr.City != "" {
				upd["city"], c.City = addr.City, addr.City
			}
			if c.Region == "" && addr.State != "" {
				upd["region"], c.Region = addr.State, addr.State
			}
		}
		if len(upd) > 0 {
			upd["updated_at"] = now
			if err := tx.Model(&c).Updates(upd).Error; err != nil {
				return nil, err
			}
		}
		return &c, nil
	}
	if email == "" {
		return nil, errors.New("checkout session has no customer email")
	}
	c = db.Customer{StoreID: storeID, TestMode: test, Email: email, Name: name, Status: "subscribed", StripeCustomerID: stripeID, CreatedAt: now, UpdatedAt: now}
	if addr != nil {
		c.Country, c.City, c.Region = addr.Country, addr.City, addr.State
	}
	if err := tx.Create(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func nextOrderNumber(tx *gorm.DB, storeID int64) int64 {
	var n int64
	tx.Model(&db.Order{}).Where("store_id = ?", storeID).Select("COALESCE(MAX(order_number),0)").Scan(&n)
	return n + 1
}

func cardOf(ch *stripe.Charge, pm *stripe.PaymentMethod) (brand, last4 string) {
	if ch != nil && ch.PaymentMethodDetails != nil && ch.PaymentMethodDetails.Card != nil {
		return strings.ToLower(ch.PaymentMethodDetails.Card.Brand), ch.PaymentMethodDetails.Card.Last4
	}
	if pm != nil && pm.Card != nil {
		return strings.ToLower(pm.Card.Brand), pm.Card.Last4
	}
	return "", ""
}

// ---------------------------------------------------------------- subscription state mapping

type subState struct {
	Status      string
	Cancelled   bool
	EndsAt      *time.Time
	RenewsAt    *time.Time
	TrialEndsAt *time.Time
	Pause       *db.SubscriptionPause
	Anchor      int
}

func mapSubscription(ss stripe.Subscription, now time.Time) subState {
	st := subState{RenewsAt: unixPtr(ss.PeriodEnd()), Anchor: 1}
	if ss.BillingCycleAnchor > 0 {
		st.Anchor = unix(ss.BillingCycleAnchor).Day()
	}
	switch ss.Status {
	case "trialing":
		st.Status = "on_trial"
		st.TrialEndsAt = unixPtrP(ss.TrialEnd)
	case "active":
		st.Status = "active"
	case "past_due", "incomplete":
		st.Status = "past_due"
	case "unpaid":
		st.Status = "unpaid"
	case "paused":
		st.Status = "paused"
	case "canceled", "incomplete_expired":
		st.Status = "expired"
		switch {
		case ss.EndedAt != nil:
			st.EndsAt = unixPtrP(ss.EndedAt)
		case ss.CanceledAt != nil:
			st.EndsAt = unixPtrP(ss.CanceledAt)
		default:
			t := now
			st.EndsAt = &t
		}
		st.Cancelled = true
		return st
	default:
		st.Status = "active"
	}
	if ss.PauseCollection != nil && (st.Status == "active" || st.Status == "on_trial") {
		st.Status = "paused"
	}
	if st.Status == "paused" {
		mode := "free"
		if ss.PauseCollection != nil && ss.PauseCollection.Behavior == "void" {
			mode = "void"
		}
		p := &db.SubscriptionPause{Mode: mode}
		if ss.PauseCollection != nil {
			p.ResumesAt = unixPtrP(ss.PauseCollection.ResumesAt)
		}
		st.Pause = p
	}
	if ss.CancelAtPeriodEnd || ss.CancelAt != nil {
		if st.Status == "active" || st.Status == "on_trial" || st.Status == "paused" {
			st.Status = "cancelled"
		}
		st.Cancelled = true
		if ss.CancelAt != nil {
			st.EndsAt = unixPtrP(ss.CancelAt)
		} else {
			st.EndsAt = unixPtr(ss.PeriodEnd())
		}
	}
	return st
}

func subSnapshot(x db.Subscription) string {
	b, _ := json.Marshal([]any{x.Status, x.Cancelled, x.EndsAt, x.RenewsAt, x.TrialEndsAt, x.Pause.V, x.Quantity, x.PriceID, x.CardBrand, x.CardLastFour, x.StripeCustomerID})
	return string(b)
}

// ---------------------------------------------------------------- checkout fulfilment

type fulfilled struct {
	Store    db.Store
	Customer db.Customer
	Order    db.Order
	Items    []db.OrderItem
	Keys     []db.LicenseKey
	Sub      *db.Subscription
	Invoice  *stripe.Invoice
	Custom   map[string]any
	Unmapped []string
}

// FulfillCheckoutSession turns a paid Stripe Checkout Session into an order,
// subscription and license keys. It is idempotent.
func (s *Svc) FulfillCheckoutSession(ctx context.Context, sessionID, account string) (Result, error) {
	var existing db.Order
	if s.DB.Where("stripe_checkout_session = ?", sessionID).First(&existing).Error == nil {
		return ignored("session already fulfilled as order #" + strconv.FormatInt(existing.OrderNumber, 10)), nil
	}
	cl, err := s.Stripe()
	if err != nil {
		return Result{}, err
	}
	sess, err := cl.RetrieveSession(ctx, sessionID)
	if err != nil {
		return Result{}, fmt.Errorf("retrieve session: %w", err)
	}
	if sess.Mode == "setup" {
		return ignored("setup mode session"), nil
	}
	if sess.PaymentStatus == "unpaid" {
		return ignored("payment pending (waiting for checkout.session.async_payment_succeeded)"), nil
	}
	store, err := s.resolveStore(sess, account)
	if err != nil {
		return Result{}, err
	}

	// optional enrichment (best effort)
	var charge *stripe.Charge
	var pm *stripe.PaymentMethod
	var pi stripe.PaymentIntent
	if sess.PaymentIntent.Decode(&pi) {
		var ch stripe.Charge
		if pi.LatestCharge.Decode(&ch) {
			charge = &ch
		}
		var m stripe.PaymentMethod
		if pi.PaymentMethod.Decode(&m) {
			pm = &m
		}
	}
	var ssub *stripe.Subscription
	if sess.Mode == "subscription" && sess.Subscription.ID != "" {
		var sx stripe.Subscription
		if !sess.Subscription.Decode(&sx) || sx.Status == "" {
			fresh, e := cl.RetrieveSubscription(ctx, sess.Subscription.ID)
			if e != nil {
				return Result{}, fmt.Errorf("retrieve subscription: %w", e)
			}
			sx = *fresh
		}
		ssub = &sx
		var m stripe.PaymentMethod
		if sx.DefaultPaymentMethod.Decode(&m) && pm == nil {
			pm = &m
		}
	}

	var out fulfilled
	test := !sess.Livemode
	out.Custom = s.customDataFor(sess)
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.fulfillTx(tx, store, sess, charge, pm, ssub, test, &out)
	})
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return ignored("session already fulfilled (concurrent delivery)"), nil
		}
		return Result{}, err
	}

	// ---- post-commit: events in the documented order, then email
	ev := func(name, rtype string, rid int64) {
		s.Events.Emit(ctx, Event{Name: name, StoreID: store.ID, TestMode: test, ResourceType: rtype, ResourceID: rid, CustomData: out.Custom})
	}
	ev("order_created", "orders", out.Order.ID)
	if out.Sub != nil {
		ev("subscription_created", "subscriptions", out.Sub.ID)
		if sess.Invoice.ID != "" {
			if inv, e := cl.RetrieveInvoice(ctx, sess.Invoice.ID); e == nil {
				if _, e2 := s.recordInvoice(ctx, cl, inv, *out.Sub, true); e2 != nil {
					s.logf("record initial invoice: %v", e2)
				}
			}
		}
	}
	for _, k := range out.Keys {
		ev("license_key_created", "license-keys", k.ID)
	}
	s.Go(func() { s.SendReceipt(context.Background(), out.Order.ID) })

	note := fmt.Sprintf("order #%d, %d item(s), %d license key(s)", out.Order.OrderNumber, len(out.Items), len(out.Keys))
	if out.Sub != nil {
		note += ", subscription #" + strconv.FormatInt(out.Sub.ID, 10)
	}
	if len(out.Unmapped) > 0 {
		note += "; WARNING unmapped Stripe prices (no variant linked): " + strings.Join(out.Unmapped, ", ")
	}
	return processed(note), nil
}

func (s *Svc) customDataFor(sess *stripe.Session) map[string]any {
	if sess.ClientReferenceID != "" {
		var c db.Checkout
		if s.DB.First(&c, "id = ?", sess.ClientReferenceID).Error == nil {
			if cd, ok := c.Attributes.V["checkout_data"].(map[string]any); ok {
				if custom, ok := cd["custom"].(map[string]any); ok && len(custom) > 0 {
					return custom
				}
			}
			return nil
		}
	}
	if len(sess.Metadata) == 0 {
		return nil
	}
	out := map[string]any{}
	for k, v := range sess.Metadata {
		switch k {
		case "checkout_id", "store_id", "variant_id":
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *Svc) fulfillTx(tx *gorm.DB, store *db.Store, sess *stripe.Session, charge *stripe.Charge, pm *stripe.PaymentMethod, ssub *stripe.Subscription, test bool, out *fulfilled) error {
	now := db.Now()
	email, name := sess.CustomerEmail, ""
	var addr *stripe.Address
	var taxNumber string
	if d := sess.CustomerDetails; d != nil {
		if d.Email != "" {
			email = d.Email
		}
		name, addr = d.Name, d.Address
		if len(d.TaxIDs) > 0 {
			taxNumber = d.TaxIDs[0].Value
		}
	}
	var scust stripe.Customer
	if sess.Customer.Decode(&scust) {
		if email == "" {
			email = scust.Email
		}
		if name == "" {
			name = scust.Name
		}
		if addr == nil {
			addr = scust.Address
		}
	}
	cust, err := s.upsertCustomer(tx, store.ID, test, email, name, sess.Customer.ID, addr)
	if err != nil {
		return err
	}
	out.Store, out.Customer = *store, *cust

	cur := strings.ToUpper(sess.Currency)
	if cur == "" {
		cur = "USD"
	}
	var discount, tax int64
	if td := sess.TotalDetails; td != nil {
		discount, tax = td.AmountDiscount, td.AmountTax
	}
	subtotal, total := sess.AmountSubtotal, sess.AmountTotal
	taxInclusive := tax > 0 && total == subtotal-discount
	brand, last4 := cardOf(charge, pm)
	o := db.Order{
		StoreID: store.ID, CustomerID: cust.ID, Identifier: uuid.NewString(),
		UserName: firstOf(name, cust.Name), UserEmail: firstOf(email, cust.Email),
		Currency: cur, CurrencyRate: s.currencyRate(cur),
		Subtotal: subtotal, DiscountTotal: discount, Tax: tax, Total: total,
		SubtotalUSD: s.toUSD(subtotal, cur), DiscountTotalUSD: s.toUSD(discount, cur), TaxUSD: s.toUSD(tax, cur), TotalUSD: s.toUSD(total, cur),
		TaxRate: "0.00", TaxInclusive: taxInclusive, Status: "paid",
		CardBrand: brand, CardLastFour: last4, CustomData: db.J(out.Custom), TaxNumber: taxNumber,
		StripeCheckoutSession: db.Ptr(sess.ID), StripePaymentIntent: sess.PaymentIntent.ID, StripeCustomerID: sess.Customer.ID,
		Source: "stripe", TestMode: test, CreatedAt: unix(sess.Created), UpdatedAt: now,
	}
	if sess.Created == 0 {
		o.CreatedAt = now
	}
	if addr != nil {
		o.Country, o.City, o.PostalCode = addr.Country, addr.City, addr.PostalCode
	}
	if charge != nil {
		o.StripeChargeID, o.StripeReceiptURL = charge.ID, charge.ReceiptURL
	}
	if sess.Invoice.ID != "" {
		o.StripeInvoiceID = sess.Invoice.ID
	}
	if tax > 0 && subtotal-discount > 0 && !taxInclusive {
		o.TaxRate = fmt.Sprintf("%.2f", float64(tax)*100/float64(subtotal-discount))
		n := "Tax"
		o.TaxName = &n
	}
	for attempt := 0; ; attempt++ {
		o.OrderNumber = nextOrderNumber(tx, store.ID)
		// SAVEPOINT keeps the outer transaction usable after a unique violation on PostgreSQL
		err = tx.Transaction(func(sp *gorm.DB) error { return sp.Create(&o).Error })
		if err == nil {
			break
		}
		if errors.Is(err, gorm.ErrDuplicatedKey) && attempt < 3 && !strings.Contains(err.Error(), "stripe_checkout_session") {
			continue
		}
		return err
	}
	out.Order = o

	var lineItems []stripe.LineItem
	if sess.LineItems != nil {
		lineItems = sess.LineItems.Data
	}
	var firstItem *db.OrderItem
	var firstMatch *matched
	for _, li := range lineItems {
		m := s.matchLineItem(tx, li)
		qty := li.Quantity
		if qty < 1 {
			qty = 1
		}
		it := db.OrderItem{
			OrderID: o.ID, Price: li.AmountSubtotal / int64(qty), Quantity: qty, TestMode: test, CreatedAt: now, UpdatedAt: now,
			ProductName: li.Description, VariantName: "",
		}
		if m.Found {
			it.ProductID, it.VariantID = m.Product.ID, m.Variant.ID
			it.ProductName, it.VariantName = m.Product.Name, m.Variant.Name
			if m.Price.ID != 0 {
				pid := m.Price.ID
				it.PriceID = &pid
			}
		} else {
			pid := ""
			if li.Price != nil {
				pid = li.Price.ID
			}
			out.Unmapped = append(out.Unmapped, pid)
		}
		if err := tx.Create(&it).Error; err != nil {
			return err
		}
		out.Items = append(out.Items, it)
		if firstItem == nil {
			cp := it
			firstItem, firstMatch = &cp, &m
		}
		_ = m
	}

	// subscription
	if ssub != nil && firstItem != nil {
		st := mapSubscription(*ssub, now)
		sub := db.Subscription{
			StoreID: store.ID, CustomerID: cust.ID, OrderID: o.ID, OrderItemID: firstItem.ID, ProductID: firstItem.ProductID,
			VariantID: firstItem.VariantID, PriceID: firstItem.PriceID, ProductName: firstItem.ProductName, VariantName: firstItem.VariantName,
			UserName: o.UserName, UserEmail: o.UserEmail, Status: st.Status, CardBrand: brand, CardLastFour: last4,
			PaymentProcessor: "stripe", Pause: db.J(st.Pause), Cancelled: st.Cancelled, TrialEndsAt: st.TrialEndsAt,
			BillingAnchor: st.Anchor, Quantity: firstItem.Quantity, RenewsAt: st.RenewsAt, EndsAt: st.EndsAt,
			StripeSubscriptionID: db.Ptr(ssub.ID), StripeCustomerID: sess.Customer.ID, CustomData: db.J(out.Custom),
			TestMode: test, CreatedAt: now, UpdatedAt: now,
		}
		if len(ssub.Items.Data) > 0 && ssub.Items.Data[0].Quantity > 0 {
			sub.Quantity = ssub.Items.Data[0].Quantity
		}
		if err := tx.Create(&sub).Error; err != nil {
			return err
		}
		for _, si := range ssub.Items.Data {
			item := db.SubscriptionItem{SubscriptionID: sub.ID, Quantity: maxI(si.Quantity, 1), StripeSubscriptionItem: si.ID, CreatedAt: now, UpdatedAt: now}
			if firstItem.PriceID != nil {
				item.PriceID = *firstItem.PriceID
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
			break // one local item per subscription (LS model)
		}
		out.Sub = &sub
	}

	// license keys: one per order item whose variant issues keys
	for _, it := range out.Items {
		if it.VariantID == 0 {
			continue
		}
		var v db.Variant
		if tx.First(&v, it.VariantID).Error != nil || !v.HasLicenseKeys {
			continue
		}
		var subID *int64
		if out.Sub != nil && it.ID == out.Sub.OrderItemID {
			subID = &out.Sub.ID
		}
		k, err := s.IssueLicenseKey(tx, *store, *cust, o, it, v, subID)
		if err != nil {
			return err
		}
		out.Keys = append(out.Keys, *k)
	}
	_ = firstMatch
	return nil
}

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func maxI(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---------------------------------------------------------------- receipts

// ResendReceipt re-sends the receipt, optionally to another address.
func (s *Svc) ResendReceipt(ctx context.Context, orderID int64, to string) error {
	return s.sendReceipt(ctx, orderID, to)
}

// SendReceipt emails the order receipt (license keys, order link).
func (s *Svc) SendReceipt(ctx context.Context, orderID int64) error {
	return s.sendReceipt(ctx, orderID, "")
}

func (s *Svc) sendReceipt(ctx context.Context, orderID int64, overrideTo string) error {
	var o db.Order
	if err := s.DB.First(&o, orderID).Error; err != nil {
		return err
	}
	var store db.Store
	_ = s.DB.First(&store, o.StoreID).Error
	var items []db.OrderItem
	s.DB.Where("order_id = ?", o.ID).Order("id").Find(&items)
	var keys []db.LicenseKey
	s.DB.Where("order_id = ?", o.ID).Order("id").Find(&keys)
	names := map[int64]string{}
	for _, it := range items {
		names[it.ID] = it.ProductName
	}
	d := mail.ReceiptData{
		StoreName: firstOf(store.Name, "Store"), Intro: store.ReceiptIntro, CustomerName: o.UserName, OrderNumber: o.OrderNumber,
		Total: resource.FormatMoney(o.Total, o.Currency), ReceiptURL: s.URLs.Receipt(o.Identifier), SupportEmail: store.SupportEmail,
	}
	for _, it := range items {
		d.Items = append(d.Items, mail.ReceiptItem{Name: it.ProductName, Variant: it.VariantName, Price: resource.FormatMoney(it.Price*int64(maxI(it.Quantity, 1)), o.Currency)})
	}
	for _, k := range keys {
		d.Keys = append(d.Keys, mail.ReceiptKey{Product: names[k.OrderItemID], Key: k.Key})
	}
	for _, f := range s.FilesForOrder(o.ID) {
		d.Files = append(d.Files, mail.ReceiptFile{Name: f.Name, Size: resource.HumanSize(f.Size), URL: d.ReceiptURL})
	}
	html, text, err := mail.RenderReceipt(d)
	if err != nil {
		return err
	}
	to := firstOf(overrideTo, o.UserEmail)
	subject := store.ReceiptSubject
	if subject == "" {
		subject = fmt.Sprintf("Your order #%d from %s", o.OrderNumber, d.StoreName)
	}
	oid := o.ID
	return s.Mail.Send(ctx, mail.Message{To: to, Subject: subject, HTML: html, Text: text, StoreID: o.StoreID, OrderID: &oid, Template: "receipt", FromName: store.EmailFromName})
}

// FilesForOrder lists the currently published files for every variant on the order.
func (s *Svc) FilesForOrder(orderID int64) []db.File {
	var variantIDs []int64
	s.DB.Model(&db.OrderItem{}).Where("order_id = ? AND variant_id <> 0", orderID).Pluck("variant_id", &variantIDs)
	if len(variantIDs) == 0 {
		return nil
	}
	var files []db.File
	s.DB.Where("variant_id IN ? AND status = ?", variantIDs, "published").Order("variant_id, sort, id").Find(&files)
	return files
}

// InvoiceURLForOrder returns the best hosted receipt link for an order.
func (s *Svc) InvoiceURLForOrder(ctx context.Context, o db.Order) string {
	if o.StripeReceiptURL != "" {
		return o.StripeReceiptURL
	}
	if o.StripeInvoiceID != "" {
		if cl, err := s.Stripe(); err == nil {
			if inv, err := cl.RetrieveInvoice(ctx, o.StripeInvoiceID); err == nil && inv.HostedInvoiceURL != "" {
				return inv.HostedInvoiceURL
			}
		}
	}
	return s.URLs.Receipt(o.Identifier)
}

var _ = settings.StripeSecretKey
