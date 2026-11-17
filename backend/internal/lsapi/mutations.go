package lsapi

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"fulfillment/internal/db"
	"fulfillment/internal/resource"
	"fulfillment/internal/service"
)

func (a *API) usersMe(w http.ResponseWriter, r *http.Request, au *Auth) {
	res := a.S.Res.User(au.User)
	write(w, http.StatusOK, singleDoc{JSONAPI: v10, Links: &resource.SelfLink{Self: a.S.URLs.API("/users/me")}, Data: res})
}

// ---------------------------------------------------------------- customers

func (a *API) createCustomer(w http.ResponseWriter, r *http.Request, au *Auth) {
	d, err := readDoc(r)
	if err != nil {
		a.failErr(w, err)
		return
	}
	storeID, ok := d.relID("store")
	if !ok {
		a.failErr(w, service.Invalid("/data/relationships/store", "The store relationship is required."))
		return
	}
	name, _ := d.Attrs.str("name")
	email, _ := d.Attrs.str("email")
	city, _ := d.Attrs.str("city")
	region, _ := d.Attrs.str("region")
	country, _ := d.Attrs.str("country")
	c, err := a.S.CreateCustomer(r.Context(), service.CustomerInput{
		StoreID: storeID, TestMode: au.TestMode, Name: name, Email: email, City: city, Region: region, Country: country,
	})
	if err != nil {
		a.failErr(w, err)
		return
	}
	respondOne(a, au, customerSpec, w, r, *c, http.StatusCreated)
}

func (a *API) updateCustomer(w http.ResponseWriter, r *http.Request, au *Auth) {
	id, ok := pathID(r)
	if !ok {
		fail(w, http.StatusNotFound, "Not Found", "The requested resource could not be found.", "")
		return
	}
	if _, err := loadOne(a, customerSpec, r, au, id); err != nil {
		a.failErr(w, err)
		return
	}
	d, err := readDoc(r)
	if err != nil {
		a.failErr(w, err)
		return
	}
	p := service.CustomerPatch{}
	if v, ok := d.Attrs.str("name"); ok {
		p.Name = &v
	}
	if v, ok := d.Attrs.str("email"); ok {
		p.Email = &v
	}
	if v, ok := d.Attrs.str("city"); ok {
		p.City = &v
	}
	if v, ok := d.Attrs.str("region"); ok {
		p.Region = &v
	}
	if v, ok := d.Attrs.str("country"); ok {
		p.Country = &v
	}
	if v, ok := d.Attrs.str("status"); ok {
		p.Status = &v
	}
	c, err := a.S.UpdateCustomer(r.Context(), id, p)
	if err != nil {
		a.failErr(w, err)
		return
	}
	respondOne(a, au, customerSpec, w, r, *c, http.StatusOK)
}

// ---------------------------------------------------------------- orders

func (a *API) refundOrder(w http.ResponseWriter, r *http.Request, au *Auth) {
	id, ok := pathID(r)
	if !ok {
		fail(w, http.StatusNotFound, "Not Found", "The requested resource could not be found.", "")
		return
	}
	if _, err := loadOne(a, orderSpec, r, au, id); err != nil {
		a.failErr(w, err)
		return
	}
	var amount *int64
	if r.ContentLength != 0 {
		if d, err := readDoc(r); err == nil {
			if n, ok := d.Attrs.num("amount"); ok {
				v := int64(n)
				amount = &v
			}
		}
	}
	o, err := a.S.RefundOrder(r.Context(), id, amount, "")
	if err != nil {
		a.failErr(w, err)
		return
	}
	respondOne(a, au, orderSpec, w, r, *o, http.StatusOK)
}

func (a *API) generateInvoice(w http.ResponseWriter, r *http.Request, au *Auth) {
	id, ok := pathID(r)
	if !ok {
		fail(w, http.StatusNotFound, "Not Found", "The requested resource could not be found.", "")
		return
	}
	o, err := loadOne(a, orderSpec, r, au, id)
	if err != nil {
		a.failErr(w, err)
		return
	}
	// Stripe produces receipts/invoices itself; point at the hosted receipt.
	target := a.S.InvoiceURLForOrder(r.Context(), *o)
	write(w, http.StatusCreated, map[string]any{
		"meta": map[string]any{"urls": map[string]any{"download_invoice": target}},
	})
}

// ---------------------------------------------------------------- subscriptions

func (a *API) cancelSubscription(w http.ResponseWriter, r *http.Request, au *Auth) {
	id, ok := pathID(r)
	if !ok {
		fail(w, http.StatusNotFound, "Not Found", "The requested resource could not be found.", "")
		return
	}
	if _, err := loadOne(a, subscriptionSpec, r, au, id); err != nil {
		a.failErr(w, err)
		return
	}
	s, err := a.S.CancelSubscription(r.Context(), id)
	if err != nil {
		a.failErr(w, err)
		return
	}
	respondOne(a, au, subscriptionSpec, w, r, *s, http.StatusOK)
}

func (a *API) updateSubscription(w http.ResponseWriter, r *http.Request, au *Auth) {
	id, ok := pathID(r)
	if !ok {
		fail(w, http.StatusNotFound, "Not Found", "The requested resource could not be found.", "")
		return
	}
	cur, err := loadOne(a, subscriptionSpec, r, au, id)
	if err != nil {
		a.failErr(w, err)
		return
	}
	d, err := readDoc(r)
	if err != nil {
		a.failErr(w, err)
		return
	}
	for _, unsupported := range []string{"variant_id", "product_id", "price_id", "billing_anchor", "invoice_immediately", "disable_prorations"} {
		if d.Attrs.has(unsupported) && unsupported != "billing_anchor" {
			a.failErr(w, service.Invalid("/data/attributes/"+unsupported, "Plan changes are handled by Stripe. Use the customer portal instead."))
			return
		}
	}
	sub := *cur
	if c, ok := d.Attrs.boolean("cancelled"); ok {
		var s *db.Subscription
		if c {
			s, err = a.S.CancelSubscription(r.Context(), id)
		} else {
			s, err = a.S.ResumeSubscription(r.Context(), id)
		}
		if err != nil {
			a.failErr(w, err)
			return
		}
		sub = *s
	}
	if d.Attrs.has("pause") {
		if raw, ok := d.Attrs["pause"].(map[string]any); ok && raw != nil {
			mode, _ := raw["mode"].(string)
			var resumes *time.Time
			if rs, ok := raw["resumes_at"].(string); ok && rs != "" {
				t, perr := parseTime(rs)
				if perr != nil {
					a.failErr(w, service.Invalid("/data/attributes/pause/resumes_at", perr.Error()))
					return
				}
				resumes = t
			}
			s, err := a.S.PauseSubscription(r.Context(), id, mode, resumes)
			if err != nil {
				a.failErr(w, err)
				return
			}
			sub = *s
		} else if d.Attrs["pause"] == nil {
			s, err := a.S.UnpauseSubscription(r.Context(), id)
			if err != nil {
				a.failErr(w, err)
				return
			}
			sub = *s
		}
	}
	respondOne(a, au, subscriptionSpec, w, r, sub, http.StatusOK)
}

func (a *API) refundSubscriptionInvoice(w http.ResponseWriter, r *http.Request, au *Auth) {
	id, ok := pathID(r)
	if !ok {
		fail(w, http.StatusNotFound, "Not Found", "The requested resource could not be found.", "")
		return
	}
	if _, err := loadOne(a, subInvoiceSpec, r, au, id); err != nil {
		a.failErr(w, err)
		return
	}
	var amount *int64
	if r.ContentLength != 0 {
		if d, err := readDoc(r); err == nil {
			if n, ok := d.Attrs.num("amount"); ok {
				v := int64(n)
				amount = &v
			}
		}
	}
	inv, err := a.S.RefundSubscriptionInvoice(r.Context(), id, amount)
	if err != nil {
		a.failErr(w, err)
		return
	}
	respondOne(a, au, subInvoiceSpec, w, r, *inv, http.StatusOK)
}

// ---------------------------------------------------------------- license keys

func (a *API) updateLicenseKey(w http.ResponseWriter, r *http.Request, au *Auth) {
	id, ok := pathID(r)
	if !ok {
		fail(w, http.StatusNotFound, "Not Found", "The requested resource could not be found.", "")
		return
	}
	if _, err := loadOne(a, licenseKeySpec, r, au, id); err != nil {
		a.failErr(w, err)
		return
	}
	d, err := readDoc(r)
	if err != nil {
		a.failErr(w, err)
		return
	}
	var p service.LicensePatch
	if d.Attrs.has("activation_limit") {
		p.ActivationLimitSet = true
		if n, ok := d.Attrs.num("activation_limit"); ok {
			v := int(n)
			p.ActivationLimit = &v
		}
	}
	if d.Attrs.has("expires_at") {
		p.ExpiresAtSet = true
		if s, ok := d.Attrs.str("expires_at"); ok && s != "" {
			t, perr := parseTime(s)
			if perr != nil {
				a.failErr(w, service.Invalid("/data/attributes/expires_at", perr.Error()))
				return
			}
			p.ExpiresAt = t
		}
	}
	if b, ok := d.Attrs.boolean("disabled"); ok {
		p.Disabled = &b
	}
	k, err := a.S.PatchLicenseKey(r.Context(), id, p)
	if err != nil {
		a.failErr(w, err)
		return
	}
	respondOne(a, au, licenseKeySpec, w, r, *k, http.StatusOK)
}

// ---------------------------------------------------------------- webhooks

func validateWebhookAttrs(d *docIn, creating bool) (rawURL string, events []string, secret string, err *service.Error) {
	if s, ok := d.Attrs.str("url"); ok {
		u, perr := url.Parse(s)
		if perr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "", nil, "", service.Invalid("/data/attributes/url", "The url must be a valid URL.")
		}
		rawURL = s
	} else if creating {
		return "", nil, "", service.Invalid("/data/attributes/url", "The url field is required.")
	}
	if raw, ok := d.Attrs["events"].([]any); ok {
		for _, e := range raw {
			name, _ := e.(string)
			if !service.IsWebhookEvent(name) {
				return "", nil, "", service.Invalid("/data/attributes/events", "The selected events are invalid: "+name)
			}
			events = append(events, name)
		}
		if len(events) == 0 {
			return "", nil, "", service.Invalid("/data/attributes/events", "The events field is required.")
		}
	} else if creating {
		return "", nil, "", service.Invalid("/data/attributes/events", "The events field is required.")
	}
	if s, ok := d.Attrs.str("secret"); ok {
		if len(s) < 6 || len(s) > 40 {
			return "", nil, "", service.Invalid("/data/attributes/secret", "The secret must be between 6 and 40 characters.")
		}
		secret = s
	} else if creating {
		return "", nil, "", service.Invalid("/data/attributes/secret", "The secret field is required.")
	}
	return
}

func (a *API) createWebhook(w http.ResponseWriter, r *http.Request, au *Auth) {
	d, err := readDoc(r)
	if err != nil {
		a.failErr(w, err)
		return
	}
	storeID, ok := d.relID("store")
	if !ok {
		a.failErr(w, service.Invalid("/data/relationships/store", "The store relationship is required."))
		return
	}
	var st db.Store
	if err := a.S.DB.First(&st, storeID).Error; err != nil {
		a.failErr(w, service.Invalid("/data/relationships/store", "The selected store is invalid."))
		return
	}
	u, events, secret, verr := validateWebhookAttrs(d, true)
	if verr != nil {
		a.failErr(w, verr)
		return
	}
	test := au.TestMode
	if b, ok := d.Attrs.boolean("test_mode"); ok {
		test = b
	}
	if test != au.TestMode {
		a.failErr(w, service.Invalid("/data/attributes/test_mode", "The API key mode does not match test_mode."))
		return
	}
	now := db.Now()
	wh := db.Webhook{StoreID: storeID, URL: u, Events: db.J(events), Secret: db.Secret(secret), TestMode: test, CreatedAt: now, UpdatedAt: now}
	if err := a.S.DB.Create(&wh).Error; err != nil {
		a.failErr(w, err)
		return
	}
	respondOne(a, au, webhookSpec, w, r, wh, http.StatusCreated)
}

func (a *API) updateWebhook(w http.ResponseWriter, r *http.Request, au *Auth) {
	id, ok := pathID(r)
	if !ok {
		fail(w, http.StatusNotFound, "Not Found", "The requested resource could not be found.", "")
		return
	}
	wh, err := loadOne(a, webhookSpec, r, au, id)
	if err != nil {
		a.failErr(w, err)
		return
	}
	d, err := readDoc(r)
	if err != nil {
		a.failErr(w, err)
		return
	}
	u, events, secret, verr := validateWebhookAttrs(d, false)
	if verr != nil {
		a.failErr(w, verr)
		return
	}
	if u != "" {
		wh.URL = u
	}
	if events != nil {
		wh.Events = db.J(events)
	}
	if secret != "" {
		wh.Secret = db.Secret(secret)
	}
	wh.UpdatedAt = db.Now()
	if err := a.S.DB.Save(wh).Error; err != nil {
		a.failErr(w, err)
		return
	}
	respondOne(a, au, webhookSpec, w, r, *wh, http.StatusOK)
}

func (a *API) deleteWebhook(w http.ResponseWriter, r *http.Request, au *Auth) {
	id, ok := pathID(r)
	if !ok {
		fail(w, http.StatusNotFound, "Not Found", "The requested resource could not be found.", "")
		return
	}
	wh, err := loadOne(a, webhookSpec, r, au, id)
	if err != nil {
		a.failErr(w, err)
		return
	}
	if err := a.S.DB.Delete(wh).Error; err != nil {
		a.failErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- checkouts

type checkoutAttrs struct {
	StoreID         int64          `json:"store_id"`
	VariantID       int64          `json:"variant_id"`
	CustomPrice     *int64         `json:"custom_price"`
	ProductOptions  map[string]any `json:"product_options"`
	CheckoutOptions map[string]any `json:"checkout_options"`
	CheckoutData    map[string]any `json:"checkout_data"`
	Preview         any            `json:"preview"`
	ExpiresAt       *string        `json:"expires_at"`
	CreatedAt       string         `json:"created_at"`
	UpdatedAt       string         `json:"updated_at"`
	TestMode        bool           `json:"test_mode"`
	URL             string         `json:"url"`
}

func (a *API) checkoutDoc(c db.Checkout) singleDoc {
	m := c.Attributes.V
	asMap := func(k string) map[string]any {
		if v, ok := m[k].(map[string]any); ok {
			return v
		}
		return map[string]any{}
	}
	var cp *int64
	if n, ok := m["custom_price"].(float64); ok {
		v := int64(n)
		cp = &v
	}
	self := &resource.SelfLink{Self: a.S.URLs.API("/checkouts/" + c.ID)}
	return singleDoc{JSONAPI: v10, Links: self, Data: resource.Resource{
		Type: "checkouts", ID: c.ID, Links: self,
		Attributes: checkoutAttrs{
			StoreID: c.StoreID, VariantID: c.VariantID, CustomPrice: cp, ProductOptions: asMap("product_options"),
			CheckoutOptions: asMap("checkout_options"), CheckoutData: asMap("checkout_data"), Preview: m["preview"],
			ExpiresAt: resource.TSP(c.ExpiresAt), CreatedAt: resource.TS(c.CreatedAt), UpdatedAt: resource.TS(c.UpdatedAt),
			TestMode: c.TestMode, URL: c.URL,
		},
	}}
}

func (a *API) createCheckout(w http.ResponseWriter, r *http.Request, au *Auth) {
	d, err := readDoc(r)
	if err != nil {
		a.failErr(w, err)
		return
	}
	storeID, ok := d.relID("store")
	if !ok {
		a.failErr(w, service.Invalid("/data/relationships/store", "The store relationship is required."))
		return
	}
	variantID, ok := d.relID("variant")
	if !ok {
		a.failErr(w, service.Invalid("/data/relationships/variant", "The variant relationship is required."))
		return
	}
	in := service.CheckoutInput{StoreID: storeID, VariantID: variantID, TestMode: au.TestMode, Raw: d.Attrs, Quantity: 1}
	if n, ok := d.Attrs.num("custom_price"); ok {
		v := int64(n)
		in.CustomPrice = &v
	}
	if s, ok := d.Attrs.str("expires_at"); ok && s != "" {
		t, perr := parseTime(s)
		if perr != nil {
			a.failErr(w, service.Invalid("/data/attributes/expires_at", perr.Error()))
			return
		}
		in.ExpiresAt = t
	}
	if po, ok := d.Attrs["product_options"].(map[string]any); ok {
		in.RedirectURL, _ = po["redirect_url"].(string)
	}
	if cd, ok := d.Attrs["checkout_data"].(map[string]any); ok {
		in.Email, _ = cd["email"].(string)
		in.Name, _ = cd["name"].(string)
		in.TaxNumber, _ = cd["tax_number"].(string)
		in.DiscountCode, _ = cd["discount_code"].(string)
		if ba, ok := cd["billing_address"].(map[string]any); ok {
			in.Country, _ = ba["country"].(string)
			in.Zip, _ = ba["zip"].(string)
		}
		if custom, ok := cd["custom"].(map[string]any); ok {
			in.Custom = custom
		}
		if vq, ok := cd["variant_quantities"].([]any); ok {
			for _, e := range vq {
				if m, ok := e.(map[string]any); ok {
					if id, _ := m["variant_id"].(float64); int64(id) == variantID {
						if q, ok := m["quantity"].(float64); ok && q >= 1 {
							in.Quantity = int(q)
						}
					}
				}
			}
		}
	}
	c, err := a.S.CreateCheckout(r.Context(), in)
	if err != nil {
		a.failErr(w, err)
		return
	}
	write(w, http.StatusCreated, a.checkoutDoc(*c))
}

func (a *API) getCheckout(w http.ResponseWriter, r *http.Request, au *Auth) {
	var c db.Checkout
	if err := a.S.DB.Where("id = ? AND test_mode = ?", strings.TrimSpace(r.PathValue("id")), au.TestMode).First(&c).Error; err != nil {
		fail(w, http.StatusNotFound, "Not Found", "The requested resource could not be found.", "")
		return
	}
	write(w, http.StatusOK, a.checkoutDoc(c))
}

func (a *API) listCheckouts(w http.ResponseWriter, r *http.Request, au *Auth) {
	q := a.S.DB.Model(&db.Checkout{}).Where("test_mode = ?", au.TestMode)
	if v := r.URL.Query().Get("filter[store_id]"); v != "" {
		q = q.Where("store_id = ?", v)
	}
	if v := r.URL.Query().Get("filter[variant_id]"); v != "" {
		q = q.Where("variant_id = ?", v)
	}
	number, size := pageParams(r)
	var total int64
	q.Count(&total)
	var rows []db.Checkout
	q.Order("created_at DESC").Offset((number - 1) * size).Limit(size).Find(&rows)
	data := make([]resource.Resource, len(rows))
	for i, c := range rows {
		data[i] = a.checkoutDoc(c).Data
	}
	last := int((total + int64(size) - 1) / int64(size))
	if last < 1 {
		last = 1
	}
	base := a.S.URLs.Base + r.URL.Path + "?"
	links := map[string]string{"first": base + pageLink(r, 1, size, "-createdAt"), "last": base + pageLink(r, last, size, "-createdAt")}
	from, to := 0, 0
	if len(rows) > 0 {
		from, to = (number-1)*size+1, (number-1)*size+len(rows)
	}
	write(w, http.StatusOK, listDoc{
		Meta:    map[string]any{"page": pageMeta{CurrentPage: number, From: from, LastPage: last, PerPage: size, To: to, Total: int(total)}},
		JSONAPI: v10, Links: links, Data: data,
	})
}
