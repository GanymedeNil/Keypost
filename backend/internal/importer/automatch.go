package importer

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"fulfillment/internal/db"
	"fulfillment/internal/service"
	"fulfillment/internal/stripe"
)

// matcher links local rows to Stripe objects through the Stripe API. Whatever
// cannot be matched unambiguously is logged (with the Lemon Squeezy id and the
// e-mail or name) so it can be fixed by hand.
type matcher struct {
	r  *Runner
	j  *jobCtl
	g  *gorm.DB
	cl *stripe.Client
	// requests counts Stripe calls so the first one is not delayed.
	requests int
}

func (r *Runner) StartAutoMatch(createdBy string, o MatchOptions) (*db.ImportJob, error) {
	if !o.Customers && !o.Products && !o.Subscriptions {
		return nil, service.Invalid("", "Select at least one of customers, products or subscriptions to match.")
	}
	cl, err := r.svc.Stripe()
	if err != nil {
		return nil, service.Bad(err.Error())
	}
	opts := map[string]any{"customers": o.Customers, "products": o.Products, "subscriptions": o.Subscriptions}
	return r.start(kindAutoMatch, createdBy, opts, nil, func(ctx context.Context, j *jobCtl) error {
		m := &matcher{r: r, j: j, g: r.svc.DB, cl: cl}
		if o.Customers {
			if err := m.customers(ctx); err != nil {
				return err
			}
		}
		if o.Products {
			if err := m.products(ctx); err != nil {
				return err
			}
		}
		if o.Subscriptions {
			if err := m.subscriptions(ctx); err != nil {
				return err
			}
		}
		j.info("auto-match finished")
		return nil
	})
}

// pace keeps the Stripe request rate polite (about 10 requests per second).
func (m *matcher) pace(ctx context.Context) error {
	m.requests++
	if m.requests == 1 {
		return ctx.Err()
	}
	if m.r.sleep == nil {
		return sleepCtx(ctx, m.r.matchDelay)
	}
	return m.r.sleep(ctx, m.r.matchDelay)
}

// fatalStripe reports errors that make continuing pointless.
func fatalStripe(err error) bool {
	var se *stripe.Error
	if errors.As(err, &se) {
		return se.Status == 401 || se.Status == 403
	}
	return errors.Is(err, stripe.ErrNotConfigured) || isCtxErr(err)
}

// ---------------------------------------------------------------- customers

func (m *matcher) customers(ctx context.Context) error {
	m.j.setStage("customers")
	var total int64
	m.g.Model(&db.Customer{}).Where(noStripeCustomer).Count(&total)
	m.j.setTotal(int(total))
	return eachBatch(m.g, noStripeCustomer, func(c db.Customer) int64 { return c.ID }, func(cs []db.Customer) error {
		for _, c := range cs {
			if err := m.matchCustomer(ctx, c); err != nil {
				return err
			}
			m.j.advance(1)
		}
		return nil
	})
}

func (m *matcher) matchCustomer(ctx context.Context, c db.Customer) error {
	if err := m.pace(ctx); err != nil {
		return err
	}
	found, err := m.cl.ListCustomersByEmail(ctx, c.Email)
	if err != nil {
		if fatalStripe(err) {
			return fmt.Errorf("customers: %w", err)
		}
		m.j.count("customers_errors", 1)
		m.j.warn("customer #%d (%s): Stripe lookup failed: %v", c.ID, c.Email, err)
		return nil
	}
	var cands []stripe.Customer
	for _, f := range found {
		if !f.Deleted && f.ID != "" {
			cands = append(cands, f)
		}
	}
	switch len(cands) {
	case 0:
		m.j.count("customers_unmatched", 1)
		m.j.warn("customer #%d (%s, store #%d): no Stripe customer with this e-mail", c.ID, c.Email, c.StoreID)
		return nil
	case 1:
	default:
		oldest := cands[0]
		for _, x := range cands[1:] {
			if x.Created != 0 && (oldest.Created == 0 || x.Created <= oldest.Created) {
				oldest = x
			}
		}
		ids := make([]string, len(cands))
		for i, x := range cands {
			ids[i] = x.ID
		}
		m.j.count("customers_ambiguous", 1)
		m.j.warn("customer #%d (%s): %d Stripe customers share this e-mail (%s); linked the oldest, %s", c.ID, c.Email, len(cands), strings.Join(ids, ", "), oldest.ID)
		cands = []stripe.Customer{oldest}
	}
	err = m.g.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&db.Customer{}).Where("id = ?", c.ID).Updates(map[string]any{"stripe_customer_id": cands[0].ID, "updated_at": db.Now()}).Error; err != nil {
			return err
		}
		return tx.Model(&db.Subscription{}).Where("customer_id = ? AND (stripe_customer_id IS NULL OR stripe_customer_id = '')", c.ID).UpdateColumn("stripe_customer_id", cands[0].ID).Error
	})
	if err != nil {
		return err
	}
	m.j.count("customers_linked", 1)
	return nil
}

// ---------------------------------------------------------------- products & prices

// uniqueIDs removes duplicates and ids that are claimed by another local row.
func unclaimed(ids []string, claimed map[string]int64, self int64) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if owner, taken := claimed[id]; taken && owner != self {
			continue
		}
		out = append(out, id)
	}
	return out
}

func (m *matcher) products(ctx context.Context) error {
	m.j.setStage("products")
	if err := m.pace(ctx); err != nil {
		return err
	}
	sprods, err := m.cl.ListProducts(ctx, true)
	if err != nil {
		return fmt.Errorf("list Stripe products: %w", err)
	}
	if err := m.pace(ctx); err != nil {
		return err
	}
	sprices, err := m.cl.ListPrices(ctx, true)
	if err != nil {
		return fmt.Errorf("list Stripe prices: %w", err)
	}
	m.j.info("Stripe has %d active product(s) and %d active price(s)", len(sprods), len(sprices))

	byMeta, byName := map[string][]string{}, map[string][]string{}
	for _, p := range sprods {
		for _, key := range []string{"variant_id", "ls_variant_id"} {
			if v := strings.TrimSpace(p.Metadata[key]); v != "" {
				byMeta[v] = append(byMeta[v], p.ID)
			}
		}
		byName[strings.ToLower(strings.TrimSpace(p.Name))] = append(byName[strings.ToLower(strings.TrimSpace(p.Name))], p.ID)
	}
	claimed := map[string]int64{}
	var linked []db.Variant
	m.g.Select("id", "stripe_product_id").Where("stripe_product_id IS NOT NULL AND stripe_product_id <> ''").Find(&linked)
	for _, v := range linked {
		claimed[v.StripeProductID] = v.ID
	}
	siblings := map[int64]int{}
	var counts []struct {
		ProductID int64
		N         int
	}
	m.g.Model(&db.Variant{}).Select("product_id, COUNT(*) AS n").Group("product_id").Scan(&counts)
	for _, c := range counts {
		siblings[c.ProductID] = c.N
	}

	var total int64
	m.g.Model(&db.Variant{}).Where(noStripeProduct).Count(&total)
	m.j.setTotal(int(total))
	err = eachBatch(m.g, noStripeProduct, func(v db.Variant) int64 { return v.ID }, func(vs []db.Variant) error {
		pids := make([]int64, len(vs))
		for i, v := range vs {
			pids[i] = v.ProductID
		}
		products := productsByID(m.g, pids)
		for _, v := range vs {
			if err := ctx.Err(); err != nil {
				return err
			}
			p := products[v.ProductID]
			ids, why := matchVariantName(v, p, siblings[v.ProductID] == 1, byMeta, byName, claimed)
			m.j.advance(1)
			switch {
			case len(ids) == 1:
				if err := m.linkVariant(v, p, ids[0], siblings[v.ProductID] == 1); err != nil {
					return err
				}
				claimed[ids[0]] = v.ID
				m.j.count("variants_linked", 1)
			case len(ids) > 1:
				m.j.count("variants_ambiguous", 1)
				m.j.warn("variant #%d (%s - %s, store #%d): %d Stripe products match by %s (%s); link it manually", v.ID, p.Name, v.Name, p.StoreID, len(ids), why, strings.Join(ids, ", "))
			default:
				m.j.count("variants_unmatched", 1)
				m.j.warn("variant #%d (%s - %s, store #%d): no Stripe product found", v.ID, p.Name, v.Name, p.StoreID)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return m.prices(ctx, sprods, sprices)
}

// matchVariantName looks for the Stripe product of a variant: metadata first,
// then the name forms in decreasing specificity. The first rule that yields
// candidates decides; several candidates are reported as ambiguous.
func matchVariantName(v db.Variant, p db.Product, onlyVariant bool, byMeta, byName map[string][]string, claimed map[string]int64) ([]string, string) {
	if c := unclaimed(byMeta[strconv.FormatInt(v.ID, 10)], claimed, v.ID); len(c) > 0 {
		return c, "metadata"
	}
	vn, pn := strings.TrimSpace(v.Name), strings.TrimSpace(p.Name)
	forms := []string{}
	if pn != "" {
		forms = append(forms, pn+" - "+vn, pn+" ("+vn+")", pn+" — "+vn)
	}
	forms = append(forms, vn)
	if onlyVariant && pn != "" {
		forms = append(forms, pn)
	}
	for _, f := range forms {
		if c := unclaimed(byName[strings.ToLower(f)], claimed, v.ID); len(c) > 0 {
			return c, fmt.Sprintf("name %q", f)
		}
	}
	return nil, ""
}

func (m *matcher) linkVariant(v db.Variant, p db.Product, stripeID string, onlyVariant bool) error {
	return m.g.Transaction(func(tx *gorm.DB) error {
		now := db.Now()
		if err := tx.Model(&db.Variant{}).Where("id = ?", v.ID).Updates(map[string]any{"stripe_product_id": stripeID, "updated_at": now}).Error; err != nil {
			return err
		}
		if onlyVariant && p.ID != 0 && p.StripeProductID == "" {
			return tx.Model(&db.Product{}).Where("id = ?", p.ID).Updates(map[string]any{"stripe_product_id": stripeID, "updated_at": now}).Error
		}
		return nil
	})
}

// priceMatches compares a local price with a Stripe price.
func priceMatches(sp stripe.Price, lp db.Price) bool {
	if sp.UnitAmount == nil || *sp.UnitAmount != lp.UnitPrice {
		return false
	}
	if sp.Currency != "" && lp.Currency != "" && !strings.EqualFold(sp.Currency, lp.Currency) {
		return false
	}
	// Only a subscription price is recurring. Lemon Squeezy keeps the renewal
	// interval on a price that was switched to one-time, so the category decides.
	if lp.Category != "subscription" || lp.RenewalIntervalUnit == nil {
		return sp.Recurring == nil
	}
	if sp.Recurring == nil || sp.Recurring.Interval != *lp.RenewalIntervalUnit {
		return false
	}
	want := 1
	if lp.RenewalIntervalQuantity != nil && *lp.RenewalIntervalQuantity > 0 {
		want = *lp.RenewalIntervalQuantity
	}
	have := sp.Recurring.IntervalCount
	if have == 0 {
		have = 1
	}
	return have == want
}

func (m *matcher) prices(ctx context.Context, sprods []stripe.Product, sprices []stripe.Price) error {
	defaults := map[string]string{} // stripe product -> default price
	for _, p := range sprods {
		if p.DefaultPrice.ID != "" {
			defaults[p.ID] = p.DefaultPrice.ID
		}
	}
	byProduct := map[string][]stripe.Price{}
	for _, sp := range sprices {
		byProduct[sp.Product.ID] = append(byProduct[sp.Product.ID], sp)
	}
	claimed := map[string]int64{}
	var linked []db.Price
	m.g.Select("id", "stripe_price_id").Where("stripe_price_id IS NOT NULL AND stripe_price_id <> ''").Find(&linked)
	for _, p := range linked {
		claimed[p.StripePriceID] = p.ID
	}
	latest := map[int64]int64{}
	var maxes []struct {
		VariantID int64
		MaxID     int64
	}
	m.g.Model(&db.Price{}).Select("variant_id, MAX(id) AS max_id").Group("variant_id").Scan(&maxes)
	for _, x := range maxes {
		latest[x.VariantID] = x.MaxID
	}

	m.j.setStage("prices")
	var total int64
	m.g.Model(&db.Price{}).Where(noStripePrice).Count(&total)
	m.j.setTotal(int(total))
	return eachBatch(m.g, noStripePrice, func(p db.Price) int64 { return p.ID }, func(ps []db.Price) error {
		vids := make([]int64, len(ps))
		for i, p := range ps {
			vids[i] = p.VariantID
		}
		variants := variantsByID(m.g, vids)
		for _, lp := range ps {
			if err := ctx.Err(); err != nil {
				return err
			}
			m.j.advance(1)
			v := variants[lp.VariantID]
			isLatest := latest[lp.VariantID] == lp.ID
			if v.StripeProductID == "" {
				if isLatest {
					m.j.count("prices_unmatched", 1)
					m.j.warn("price #%d (variant #%d %s, %d %s): the variant has no Stripe product yet", lp.ID, lp.VariantID, v.Name, lp.UnitPrice, lp.Currency)
				}
				continue
			}
			var cands []string
			for _, sp := range byProduct[v.StripeProductID] {
				if _, taken := claimed[sp.ID]; !taken && priceMatches(sp, lp) {
					cands = append(cands, sp.ID)
				}
			}
			if len(cands) == 0 {
				if isLatest {
					m.j.count("prices_unmatched", 1)
					m.j.warn("price #%d (variant #%d %s, %d %s): no active Stripe price of %s has this amount and interval", lp.ID, lp.VariantID, v.Name, lp.UnitPrice, lp.Currency, v.StripeProductID)
				}
				continue
			}
			choice := cands[0]
			if len(cands) > 1 {
				for _, c := range cands {
					if c == defaults[v.StripeProductID] {
						choice = c
					}
				}
				m.j.warn("price #%d (variant #%d %s): %d Stripe prices match (%s); linked %s", lp.ID, lp.VariantID, v.Name, len(cands), strings.Join(cands, ", "), choice)
			}
			if err := m.g.Model(&db.Price{}).Where("id = ?", lp.ID).Updates(map[string]any{"stripe_price_id": choice, "updated_at": db.Now()}).Error; err != nil {
				return err
			}
			claimed[choice] = lp.ID
			m.j.count("prices_linked", 1)
		}
		return nil
	})
}

// ---------------------------------------------------------------- subscriptions

func (m *matcher) subscriptions(ctx context.Context) error {
	m.j.setStage("subscriptions")
	used := map[string]bool{}
	var linked []db.Subscription
	m.g.Select("id", "stripe_subscription_id").Where("stripe_subscription_id IS NOT NULL AND stripe_subscription_id <> ''").Find(&linked)
	for _, s := range linked {
		used[*s.StripeSubscriptionID] = true
	}
	var total int64
	m.g.Model(&db.Subscription{}).Where(noStripeSub).Count(&total)
	m.j.setTotal(int(total))
	cache := map[string][]stripe.Subscription{}
	return eachBatch(m.g, noStripeSub, func(s db.Subscription) int64 { return s.ID }, func(ss []db.Subscription) error {
		for _, s := range ss {
			if err := ctx.Err(); err != nil {
				return err
			}
			m.j.advance(1)
			if err := m.matchSubscription(ctx, s, used, cache); err != nil {
				return err
			}
		}
		return nil
	})
}

func (m *matcher) matchSubscription(ctx context.Context, s db.Subscription, used map[string]bool, cache map[string][]stripe.Subscription) error {
	who := fmt.Sprintf("subscription #%d (%s, %s - %s)", s.ID, s.UserEmail, s.ProductName, s.VariantName)
	if s.PaymentProcessor == "paypal" {
		m.j.count("subscriptions_paypal", 1)
		m.j.warn("%s is a PayPal subscription and cannot be linked to Stripe", who)
		return nil
	}
	var custs []db.Customer
	m.g.Select("id", "stripe_customer_id").Where("id = ?", s.CustomerID).Limit(1).Find(&custs)
	if len(custs) == 0 || custs[0].StripeCustomerID == "" {
		m.j.count("subscriptions_unmatched", 1)
		m.j.warn("%s: its customer #%d is not linked to a Stripe customer yet (match or map customers first)", who, s.CustomerID)
		return nil
	}
	cus := custs[0].StripeCustomerID

	priceID, productID := "", ""
	if s.PriceID != nil {
		var ps []db.Price
		m.g.Select("id", "stripe_price_id").Where("id = ?", *s.PriceID).Limit(1).Find(&ps)
		if len(ps) > 0 {
			priceID = ps[0].StripePriceID
		}
	}
	vs := variantsByID(m.g, []int64{s.VariantID})
	productID = vs[s.VariantID].StripeProductID
	if priceID == "" && productID == "" {
		m.j.count("subscriptions_unmatched", 1)
		m.j.warn("%s: neither its price nor its variant is linked to Stripe yet (match or map products first)", who)
		return nil
	}

	subs, ok := cache[cus]
	if !ok {
		if err := m.pace(ctx); err != nil {
			return err
		}
		var err error
		subs, err = m.cl.ListSubscriptions(ctx, cus)
		if err != nil {
			if fatalStripe(err) {
				return fmt.Errorf("subscriptions: %w", err)
			}
			m.j.count("subscriptions_errors", 1)
			m.j.warn("%s: could not list the Stripe subscriptions of %s: %v", who, cus, err)
			return nil
		}
		cache[cus] = subs
	}
	var cands []stripe.Subscription
	for _, ss := range subs {
		if used[ss.ID] || ss.Status == "canceled" || ss.Status == "incomplete_expired" {
			continue
		}
		for _, it := range ss.Items.Data {
			if it.Price == nil {
				continue
			}
			if (priceID != "" && it.Price.ID == priceID) || (productID != "" && it.Price.Product.ID == productID) {
				cands = append(cands, ss)
				break
			}
		}
	}
	switch len(cands) {
	case 0:
		m.j.count("subscriptions_unmatched", 1)
		m.j.warn("%s: no open Stripe subscription of %s uses its price or product", who, cus)
		return nil
	case 1:
	default:
		ids := make([]string, len(cands))
		for i, c := range cands {
			ids[i] = c.ID
		}
		m.j.count("subscriptions_ambiguous", 1)
		m.j.warn("%s: %d Stripe subscriptions of %s match (%s); link it manually", who, len(cands), cus, strings.Join(ids, ", "))
		return nil
	}
	sid := cands[0].ID
	err := m.g.Model(&db.Subscription{}).Where("id = ?", s.ID).Updates(map[string]any{"stripe_subscription_id": sid, "stripe_customer_id": cus, "updated_at": db.Now()}).Error
	if err != nil {
		return err
	}
	used[sid] = true
	m.j.count("subscriptions_linked", 1)
	if err := m.pace(ctx); err != nil {
		return err
	}
	if _, err := m.r.svc.SyncSubscription(ctx, s.ID); err != nil {
		if isCtxErr(err) {
			return err
		}
		m.j.warn("%s: linked to %s but the Stripe sync failed: %v", who, sid, err)
	}
	return nil
}
