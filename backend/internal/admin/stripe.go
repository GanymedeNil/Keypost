package admin

import (
	"context"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"fulfillment/internal/db"
	"fulfillment/internal/service"
	"fulfillment/internal/settings"
	"fulfillment/internal/stripe"
	"fulfillment/internal/webhooks"
)

var _ = webhooks.New

func (h *Handler) webhookURL() string { return h.S.Cfg.PublicURL + "/webhooks/stripe" }

func (h *Handler) stripeStatus(w http.ResponseWriter, r *http.Request, u *db.User) {
	out := map[string]any{
		"configured": false, "livemode": false, "account": nil,
		"webhook_secret_set": h.S.Settings.Get(settings.StripeWebhookSecret) != "",
		"webhook_url":        h.webhookURL(), "required_events": service.RequiredStripeEvents,
	}
	cl, err := h.S.Stripe()
	if err != nil {
		h.ok(w, out)
		return
	}
	out["configured"] = true
	out["livemode"] = !h.S.StripeTestMode()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	a, err := cl.Account(ctx)
	if err != nil {
		out["error"] = err.Error()
		h.ok(w, out)
		return
	}
	out["account"] = map[string]any{"id": a.ID, "business_name": a.DisplayName(), "country": a.Country, "email": a.Email}
	h.ok(w, out)
}

// stripeWebhookEndpoint creates (or refreshes) the Stripe webhook endpoint for this installation.
func (h *Handler) stripeWebhookEndpoint(w http.ResponseWriter, r *http.Request, u *db.User) {
	cl, err := h.S.Stripe()
	if err != nil {
		h.fail(w, http.StatusUnprocessableEntity, "stripe_error", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	target := h.webhookURL()
	list, err := cl.ListWebhookEndpoints(ctx)
	if err != nil {
		h.failErr(w, service.Upstream(err.Error()))
		return
	}
	haveSecret := h.S.Settings.Get(settings.StripeWebhookSecret) != ""
	var existing *stripe.WebhookEndpoint
	for i := range list {
		if list[i].URL == target {
			existing = &list[i]
			break
		}
	}
	created, recreated := false, false
	var ep *stripe.WebhookEndpoint
	switch {
	case existing != nil && haveSecret:
		ep, err = cl.UpdateWebhookEndpoint(ctx, existing.ID, service.RequiredStripeEvents)
	default:
		if existing != nil { // the signing secret of an existing endpoint can't be read back → recreate it
			if derr := cl.DeleteWebhookEndpoint(ctx, existing.ID); derr != nil {
				h.failErr(w, service.Upstream(derr.Error()))
				return
			}
			recreated = true
		}
		ep, err = cl.CreateWebhookEndpoint(ctx, target, service.RequiredStripeEvents)
		created = err == nil
	}
	if err != nil {
		h.failErr(w, service.Upstream(err.Error()))
		return
	}
	if ep.Secret != "" {
		if err := h.S.Settings.Set(settings.StripeWebhookSecret, ep.Secret); err != nil {
			h.failErr(w, err)
			return
		}
	}
	h.audit(r, u, "stripe.webhook_endpoint", "stripe", ep.ID, map[string]any{"created": created, "recreated": recreated})
	h.ok(w, map[string]any{"id": ep.ID, "url": ep.URL, "events": ep.EnabledEvents, "created": created, "recreated": recreated})
}

func (h *Handler) stripeCatalog(w http.ResponseWriter, r *http.Request, u *db.User) {
	cl, err := h.S.Stripe()
	if err != nil {
		h.fail(w, http.StatusUnprocessableEntity, "stripe_error", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	prods, err := cl.ListProducts(ctx, true)
	if err != nil {
		h.failErr(w, service.Upstream(err.Error()))
		return
	}
	prices, err := cl.ListPrices(ctx, true)
	if err != nil {
		h.failErr(w, service.Upstream(err.Error()))
		return
	}
	byProduct := map[string][]map[string]any{}
	for _, pr := range prices {
		var rec any
		if pr.Recurring != nil {
			rec = map[string]any{"interval": pr.Recurring.Interval, "interval_count": pr.Recurring.IntervalCount}
		}
		byProduct[pr.Product.ID] = append(byProduct[pr.Product.ID], map[string]any{"id": pr.ID, "unit_amount": pr.UnitAmount, "currency": pr.Currency, "recurring": rec, "active": pr.Active})
	}
	var vs []db.Variant
	h.S.DB.Where("stripe_product_id <> ''").Find(&vs)
	mapped := map[string]int64{}
	for _, v := range vs {
		mapped[v.StripeProductID] = v.ID
	}
	out := make([]map[string]any, 0, len(prods))
	for _, p := range prods {
		pr := byProduct[p.ID]
		if pr == nil {
			pr = []map[string]any{}
		}
		var mv any
		if id, ok := mapped[p.ID]; ok {
			mv = id
		}
		md := p.Metadata
		if md == nil {
			md = map[string]string{}
		}
		out = append(out, map[string]any{"id": p.ID, "name": p.Name, "active": p.Active, "description": p.Description, "metadata": md,
			"default_price": nilStr(p.DefaultPrice.ID), "prices": pr, "mapped_variant_id": mv})
	}
	h.ok(w, out)
}

func (h *Handler) stripeCatalogImport(w http.ResponseWriter, r *http.Request, u *db.User) {
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	sid, _ := b.int("store_id")
	var st db.Store
	if sid == 0 {
		h.S.DB.Order("id").First(&st)
	} else if err := h.S.DB.First(&st, sid).Error; err != nil {
		h.invalid(w, "store_id", "The selected store is invalid.")
		return
	}
	if st.ID == 0 {
		h.invalid(w, "store_id", "Create a store first.")
		return
	}
	wanted := map[string]bool{}
	if ids, ok := b.strList("stripe_product_ids"); ok {
		for _, id := range ids {
			wanted[id] = true
		}
	}
	publish, _ := b.boolean("publish")
	cl, err := h.S.Stripe()
	if err != nil {
		h.fail(w, http.StatusUnprocessableEntity, "stripe_error", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	prods, err := cl.ListProducts(ctx, true)
	if err != nil {
		h.failErr(w, service.Upstream(err.Error()))
		return
	}
	prices, err := cl.ListPrices(ctx, true)
	if err != nil {
		h.failErr(w, service.Upstream(err.Error()))
		return
	}
	test := h.S.StripeTestMode()
	counts := map[string]int{"products": 0, "variants": 0, "prices": 0, "skipped": 0}
	status := "draft"
	if publish {
		status = "published"
	}
	for _, sp := range prods {
		if len(wanted) > 0 && !wanted[sp.ID] {
			continue
		}
		var already int64
		h.S.DB.Model(&db.Variant{}).Where("stripe_product_id = ?", sp.ID).Count(&already)
		var already2 int64
		h.S.DB.Model(&db.Product{}).Where("stripe_product_id = ?", sp.ID).Count(&already2)
		if already+already2 > 0 {
			counts["skipped"]++
			continue
		}
		now := db.Now()
		err := h.S.DB.Transaction(func(tx *gorm.DB) error {
			p := db.Product{StoreID: st.ID, Name: sp.Name, Slug: slugify(sp.Name), Description: sp.Description, Status: status, StripeProductID: sp.ID, TestMode: test, CreatedAt: now, UpdatedAt: now}
			if len(sp.Images) > 0 {
				p.ThumbURL, p.LargeThumbURL = sp.Images[0], sp.Images[0]
			}
			if err := tx.Create(&p).Error; err != nil {
				return err
			}
			v := db.Variant{ProductID: p.ID, Name: "Default", Slug: newSlug(), Status: "published", StripeProductID: sp.ID, LicenseActivationLimit: 1, LicenseLengthValue: 1, LicenseLengthUnit: "years", IsLicenseLengthUnlimited: true, TestMode: test, Links: db.J([]db.VariantLink{}), CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&v).Error; err != nil {
				return err
			}
			counts["products"]++
			counts["variants"]++
			for _, sprice := range prices {
				if sprice.Product.ID != sp.ID || sprice.UnitAmount == nil {
					continue
				}
				pr := db.Price{VariantID: v.ID, Category: "one_time", Scheme: "standard", UnitPrice: *sprice.UnitAmount, PackageSize: 1, TaxCode: "eservice",
					Currency: strings.ToUpper(sprice.Currency), StripePriceID: sprice.ID, Tiers: db.J([]db.PriceTier{}), CreatedAt: now, UpdatedAt: now}
				if sprice.Recurring != nil {
					pr.Category = "subscription"
					unit, qty := sprice.Recurring.Interval, sprice.Recurring.IntervalCount
					if qty == 0 {
						qty = 1
					}
					pr.RenewalIntervalUnit, pr.RenewalIntervalQuantity = &unit, &qty
					if sprice.Recurring.TrialPeriodDays != nil && *sprice.Recurring.TrialPeriodDays > 0 {
						tu, tq := "day", *sprice.Recurring.TrialPeriodDays
						pr.TrialIntervalUnit, pr.TrialIntervalQuantity = &tu, &tq
					}
				}
				if err := tx.Create(&pr).Error; err != nil {
					return err
				}
				counts["prices"]++
			}
			return nil
		})
		if err != nil {
			h.failErr(w, err)
			return
		}
	}
	h.audit(r, u, "stripe.catalog_import", "stripe", "", map[string]any{"counts": counts})
	h.ok(w, counts)
}

func (h *Handler) linkVariant(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var v db.Variant
	if err := h.S.DB.First(&v, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	prodID, _ := b.str("stripe_product_id")
	priceID, _ := b.str("stripe_price_id")
	if prodID != "" && !strings.HasPrefix(prodID, "prod_") {
		h.invalid(w, "stripe_product_id", "Must look like prod_…")
		return
	}
	if priceID != "" && !strings.HasPrefix(priceID, "price_") {
		h.invalid(w, "stripe_price_id", "Must look like price_…")
		return
	}
	if prodID == "" && priceID == "" {
		h.invalid(w, "stripe_price_id", "Provide a Stripe product id and/or price id.")
		return
	}
	err := h.S.DB.Transaction(func(tx *gorm.DB) error {
		if prodID != "" {
			if err := tx.Model(&v).Updates(map[string]any{"stripe_product_id": prodID, "updated_at": db.Now()}).Error; err != nil {
				return err
			}
		}
		if priceID != "" {
			var pr db.Price
			if err := tx.Where("variant_id = ?", v.ID).Order("id DESC").First(&pr).Error; err != nil {
				return fieldErr{"stripe_price_id", "This variant has no price to link; add a price first."}
			}
			return tx.Model(&pr).Updates(map[string]any{"stripe_price_id": priceID, "updated_at": db.Now()}).Error
		}
		return nil
	})
	if fe, ok := err.(fieldErr); ok {
		h.invalid(w, fe.field, fe.msg)
		return
	}
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "variant.link_stripe", "variant", id, map[string]any{"product": prodID, "price": priceID})
	h.S.DB.First(&v, id)
	h.ok(w, h.variantDTOs([]db.Variant{v}, true)[0])
}

func (h *Handler) unlinkVariant(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var v db.Variant
	if err := h.S.DB.First(&v, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.S.DB.Model(&v).Updates(map[string]any{"stripe_product_id": "", "updated_at": db.Now()})
	h.S.DB.Model(&db.Price{}).Where("variant_id = ?", id).Update("stripe_price_id", "")
	h.audit(r, u, "variant.unlink_stripe", "variant", id, nil)
	noContent(w)
}
