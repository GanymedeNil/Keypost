package service

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"fulfillment/internal/db"
	"fulfillment/internal/settings"
	"fulfillment/internal/stripe"
)

type CheckoutInput struct {
	StoreID      int64
	VariantID    int64
	TestMode     bool
	Email, Name  string
	Country, Zip string
	TaxNumber    string
	DiscountCode string
	Custom       map[string]any
	CustomPrice  *int64
	Quantity     int
	RedirectURL  string
	ExpiresAt    *time.Time
	Raw          map[string]any // attributes echoed back by GET /v1/checkouts/{id}
}

// CreateCheckout creates a Stripe Checkout Session for a variant and stores
// a Lemon Squeezy shaped checkout record that points at it.
func (s *Svc) CreateCheckout(ctx context.Context, in CheckoutInput) (*db.Checkout, error) {
	var store db.Store
	if err := s.DB.First(&store, in.StoreID).Error; err != nil {
		return nil, Invalid("/data/relationships/store", "The selected store is invalid.")
	}
	var v db.Variant
	if err := s.DB.First(&v, in.VariantID).Error; err != nil {
		return nil, Invalid("/data/relationships/variant", "The selected variant is invalid.")
	}
	var prod db.Product
	if err := s.DB.First(&prod, v.ProductID).Error; err != nil || prod.StoreID != store.ID {
		return nil, Invalid("/data/relationships/variant", "The selected variant does not belong to this store.")
	}
	if v.TestMode != in.TestMode {
		return nil, Invalid("/data/attributes/test_mode", "The variant belongs to a different mode (test/live).")
	}
	var price db.Price
	if err := s.DB.Where("variant_id = ?", v.ID).Order("id DESC").First(&price).Error; err != nil { // the latest price record is the current one
		return nil, Invalid("/data/relationships/variant", "This variant has no price.")
	}
	if price.StripePriceID == "" && in.CustomPrice == nil {
		return nil, Invalid("/data/relationships/variant", "This variant is not linked to a Stripe price yet (Products → variant → Stripe).")
	}
	cl, err := s.Stripe()
	if err != nil {
		return nil, stripeErr(err)
	}

	id := uuid.NewString()
	mode := "payment"
	if price.Category == "subscription" {
		mode = "subscription"
	}
	qty := in.Quantity
	if qty < 1 {
		qty = 1
	}
	success := s.URLs.Base + "/checkout/success?session_id={CHECKOUT_SESSION_ID}&checkout=" + id
	cancel := s.Settings.Get(settings.CheckoutCancelURL)
	if cancel == "" {
		cancel = firstOf(store.URL, s.URLs.Base)
	}

	p := stripe.P().Set("mode", mode).Set("success_url", success).Set("cancel_url", cancel).Set("client_reference_id", id).
		Set("metadata[checkout_id]", id).Set("metadata[store_id]", strconv.FormatInt(store.ID, 10)).Set("metadata[variant_id]", strconv.FormatInt(v.ID, 10))
	if mode == "subscription" {
		p.Set("subscription_data[metadata][checkout_id]", id).Set("subscription_data[metadata][store_id]", strconv.FormatInt(store.ID, 10))
	}
	if in.CustomPrice != nil {
		cur := strings.ToLower(firstOf(price.Currency, store.Currency))
		p.Set("line_items[0][price_data][currency]", cur).Int("line_items[0][price_data][unit_amount]", *in.CustomPrice)
		if v.StripeProductID != "" {
			p.Set("line_items[0][price_data][product]", v.StripeProductID)
		} else if prod.StripeProductID != "" {
			p.Set("line_items[0][price_data][product]", prod.StripeProductID)
		} else {
			p.Set("line_items[0][price_data][product_data][name]", prod.Name+" — "+v.Name)
		}
		if mode == "subscription" && price.RenewalIntervalUnit != nil {
			p.Set("line_items[0][price_data][recurring][interval]", *price.RenewalIntervalUnit)
			if price.RenewalIntervalQuantity != nil {
				p.Int("line_items[0][price_data][recurring][interval_count]", int64(*price.RenewalIntervalQuantity))
			}
		}
	} else {
		p.Set("line_items[0][price]", price.StripePriceID)
	}
	p.Int("line_items[0][quantity]", int64(qty))

	if in.Email != "" {
		var c db.Customer
		if s.DB.Where("store_id = ? AND test_mode = ? AND LOWER(email) = ? AND stripe_customer_id <> ''", store.ID, in.TestMode, strings.ToLower(in.Email)).First(&c).Error == nil {
			p.Set("customer", c.StripeCustomerID)
		} else {
			p.Set("customer_email", in.Email)
		}
	}
	if in.DiscountCode != "" {
		p.Bool("allow_promotion_codes", true)
	}
	if in.ExpiresAt != nil {
		p.Int("expires_at", in.ExpiresAt.Unix())
	}
	managed := s.Settings.Bool(settings.StripeManagedPayments)
	if managed {
		p.Bool("managed_payments[enabled]", true)
	}
	sess, err := cl.CreateSession(ctx, p, "checkout-"+id)
	if err != nil && !managed && asksForManagedPaymentsOptOut(err) {
		// Some accounts have Managed Payments switched on by default, which needs a product tax code
		// on every product; this service does not use it unless the setting says so, so opt out for
		// this session. (Sent only on that answer: accounts without the feature do not know the field.)
		p.Bool("managed_payments[enabled]", false)
		sess, err = cl.CreateSession(ctx, p, "checkout-"+id+"-unmanaged")
	}
	if err != nil {
		return nil, stripeErr(err)
	}
	now := db.Now()
	co := db.Checkout{
		ID: id, StoreID: store.ID, VariantID: v.ID, StripeSessionID: sess.ID, URL: sess.URL, RedirectURL: in.RedirectURL,
		Attributes: db.J(in.Raw), ExpiresAt: in.ExpiresAt, TestMode: in.TestMode, CreatedAt: now, UpdatedAt: now,
	}
	if co.Attributes.V == nil {
		co.Attributes = db.J(map[string]any{})
	}
	if sess.ExpiresAt > 0 && co.ExpiresAt == nil {
		co.ExpiresAt = unixPtr(sess.ExpiresAt)
	}
	if err := s.DB.WithContext(ctx).Create(&co).Error; err != nil {
		return nil, err
	}
	return &co, nil
}

// BuyNow creates a checkout for a published variant by slug (the product's buy_now_url).
func (s *Svc) BuyNow(ctx context.Context, variantSlug string) (string, error) {
	var v db.Variant
	if err := s.DB.Where("slug = ?", variantSlug).First(&v).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", NotFound("Product")
		}
		return "", err
	}
	var p db.Product
	if err := s.DB.First(&p, v.ProductID).Error; err != nil {
		return "", NotFound("Product")
	}
	if p.Status != "published" || v.Status == "draft" {
		return "", NotFound("Product")
	}
	co, err := s.CreateCheckout(ctx, CheckoutInput{StoreID: p.StoreID, VariantID: v.ID, TestMode: v.TestMode, Quantity: 1})
	if err != nil {
		return "", err
	}
	return co.URL, nil
}

// CheckoutLanding resolves where to send the buyer after paying: the signed
// order page (or the merchant's redirect_url with link variables filled in).
// It waits briefly for the Stripe webhook to fulfil the order.
func (s *Svc) CheckoutLanding(ctx context.Context, sessionID, checkoutID string) (string, bool) {
	deadline := time.Now().Add(10 * time.Second)
	for {
		var o db.Order
		if s.DB.Where("stripe_checkout_session = ?", sessionID).First(&o).Error == nil {
			target := s.URLs.Receipt(o.Identifier)
			var co db.Checkout
			if checkoutID != "" && s.DB.First(&co, "id = ?", checkoutID).Error == nil && co.RedirectURL != "" {
				target = s.fillLinkVariables(co.RedirectURL, o)
			}
			return target, true
		}
		// try fulfilling inline so the buyer is not blocked on webhook latency
		if time.Until(deadline) > 6*time.Second {
			_, _ = s.FulfillCheckoutSession(ctx, sessionID, "")
			continue
		}
		if time.Now().After(deadline) {
			return s.URLs.Base + "/my-orders?pending=1", false
		}
		select {
		case <-ctx.Done():
			return s.URLs.Base + "/my-orders?pending=1", false
		case <-time.After(1 * time.Second):
		}
	}
}

// fillLinkVariables substitutes Lemon Squeezy style [variables] in a URL.
func (s *Svc) fillLinkVariables(raw string, o db.Order) string {
	key := ""
	var k db.LicenseKey
	if s.DB.Where("order_id = ?", o.ID).Order("id").First(&k).Error == nil {
		key = k.Key
	}
	total := ""
	if o.Total > 0 {
		total = formatTotal(o)
	}
	r := strings.NewReplacer(
		"[license_key]", url.QueryEscape(key), "[order_id]", strconv.FormatInt(o.ID, 10), "[order_identifier]", o.Identifier,
		"[email]", url.QueryEscape(o.UserEmail), "[name]", url.QueryEscape(o.UserName), "[total]", url.QueryEscape(total),
	)
	return r.Replace(raw)
}

func formatTotal(o db.Order) string {
	return strconv.FormatFloat(float64(o.Total)/100, 'f', 2, 64) + " " + o.Currency
}

// asksForManagedPaymentsOptOut recognises Stripe's "pass managed_payments[enabled]=false" answer.
func asksForManagedPaymentsOptOut(err error) bool {
	var se *stripe.Error
	return errors.As(err, &se) && strings.Contains(se.Message, "managed_payments[enabled]=false")
}
