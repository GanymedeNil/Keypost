package admin

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"fulfillment/internal/db"
	"fulfillment/internal/resource"
	"fulfillment/internal/service"
)

// ---------------------------------------------------------------- customers

func (h *Handler) customerDTOs(cs []db.Customer) []map[string]any {
	ids := make([]int64, len(cs))
	for i, c := range cs {
		ids[i] = c.ID
	}
	stats := h.S.Res.CustomerStats(ids)
	count := func(model any, col string) map[int64]int {
		out := map[int64]int{}
		if len(ids) == 0 {
			return out
		}
		type row struct {
			K int64
			N int
		}
		var rows []row
		h.S.DB.Model(model).Select(col+" AS k, COUNT(*) AS n").Where(col+" IN ?", ids).Group(col).Scan(&rows)
		for _, r := range rows {
			out[r.K] = r.N
		}
		return out
	}
	oc, sc, kc := count(&db.Order{}, "customer_id"), count(&db.Subscription{}, "customer_id"), count(&db.LicenseKey{}, "customer_id")
	out := make([]map[string]any, len(cs))
	for i, c := range cs {
		st := stats[c.ID]
		out[i] = map[string]any{
			"id": c.ID, "store_id": c.StoreID, "name": c.Name, "email": c.Email, "status": c.Status, "city": c.City, "region": c.Region,
			"country": c.Country, "stripe_customer_id": c.StripeCustomerID, "total_revenue": st.Revenue, "mrr": st.MRR,
			"orders_count": oc[c.ID], "subscriptions_count": sc[c.ID], "license_keys_count": kc[c.ID], "test_mode": c.TestMode,
			"created_at": ts(c.CreatedAt), "updated_at": ts(c.UpdatedAt),
		}
	}
	return out
}

func (h *Handler) listCustomers(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	q := h.S.DB.Model(&db.Customer{}).Where("test_mode = ?", p.TestMode)
	if p.StoreID > 0 {
		q = q.Where("store_id = ?", p.StoreID)
	}
	if c := r.URL.Query().Get("country"); c != "" {
		q = q.Where("country = ?", strings.ToUpper(c))
	}
	if p.Q != "" {
		q = q.Where("(LOWER(name) LIKE ? ESCAPE '\\' OR LOWER(email) LIKE ? ESCAPE '\\')", like(p.Q), like(p.Q))
	}
	q = applySort(q, p.Sort, map[string]string{"name": "name", "email": "email", "created_at": "created_at", "id": "id"}, "id DESC")
	var rows []db.Customer
	meta, err := paginate(q, p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.respondList(w, h.customerDTOs(rows), meta)
}

func (h *Handler) getCustomer(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var c db.Customer
	if err := h.S.DB.First(&c, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	dto := h.customerDTOs([]db.Customer{c})[0]
	var os []db.Order
	h.S.DB.Where("customer_id = ?", id).Order("id DESC").Limit(100).Find(&os)
	var ss []db.Subscription
	h.S.DB.Where("customer_id = ?", id).Order("id DESC").Limit(100).Find(&ss)
	var ks []db.LicenseKey
	h.S.DB.Where("customer_id = ?", id).Order("id DESC").Limit(100).Find(&ks)
	dto["orders"] = h.orderDTOs(os)
	dto["subscriptions"] = h.subscriptionDTOs(ss)
	dto["license_keys"] = h.keyDTOs(ks)
	h.ok(w, dto)
}

func (h *Handler) createCustomer(w http.ResponseWriter, r *http.Request, u *db.User) {
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	sid, _ := b.int("store_id")
	if sid == 0 {
		var st db.Store
		if h.S.DB.Order("id").First(&st).Error == nil {
			sid = st.ID
		}
	}
	name, _ := b.str("name")
	email, _ := b.str("email")
	tm, _ := b.boolean("test_mode")
	c, err := h.S.CreateCustomer(r.Context(), service.CustomerInput{StoreID: sid, TestMode: tm, Name: name, Email: email})
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "customer.create", "customer", c.ID, nil)
	h.created(w, h.customerDTOs([]db.Customer{*c})[0])
}

func (h *Handler) updateCustomer(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	var p service.CustomerPatch
	opt := func(k string) *string {
		if v, ok := b.str(k); ok {
			return &v
		}
		if b.isNull(k) { // null clears the field
			empty := ""
			return &empty
		}
		return nil
	}
	p.Name, p.Email, p.City, p.Region, p.Country, p.StripeCustomerID = opt("name"), opt("email"), opt("city"), opt("region"), opt("country"), opt("stripe_customer_id")
	c, err := h.S.UpdateCustomer(r.Context(), id, p)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "customer.update", "customer", c.ID, nil)
	h.ok(w, h.customerDTOs([]db.Customer{*c})[0])
}

func (h *Handler) portalLink(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	url, err := h.S.PortalURL(r.Context(), id, h.S.URLs.Base+"/admin/customers/"+strconv.FormatInt(id, 10))
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.ok(w, map[string]any{"url": url})
}

// ---------------------------------------------------------------- orders

func (h *Handler) orderDTOs(os []db.Order) []map[string]any {
	ids := make([]int64, len(os))
	for i, o := range os {
		ids[i] = o.ID
	}
	first := map[int64]db.OrderItem{}
	if len(ids) > 0 {
		var its []db.OrderItem
		h.S.DB.Where("order_id IN ?", ids).Order("id").Find(&its)
		for _, it := range its {
			if _, ok := first[it.OrderID]; !ok {
				first[it.OrderID] = it
			}
		}
	}
	out := make([]map[string]any, len(os))
	for i, o := range os {
		var fi any
		if it, ok := first[o.ID]; ok {
			fi = map[string]any{"product_name": it.ProductName, "variant_name": it.VariantName, "price": it.Price, "quantity": it.Quantity}
		}
		out[i] = map[string]any{
			"id": o.ID, "store_id": o.StoreID, "customer_id": o.CustomerID, "identifier": o.Identifier, "order_number": o.OrderNumber,
			"user_name": o.UserName, "user_email": o.UserEmail, "currency": o.Currency, "currency_rate": o.CurrencyRate,
			"subtotal": o.Subtotal, "setup_fee": o.SetupFee, "discount_total": o.DiscountTotal, "tax": o.Tax, "total": o.Total,
			"refunded_amount": o.RefundedAmount, "subtotal_usd": o.SubtotalUSD, "discount_total_usd": o.DiscountTotalUSD, "tax_usd": o.TaxUSD,
			"total_usd": o.TotalUSD, "refunded_amount_usd": o.RefundedAmountUSD, "tax_name": o.TaxName, "tax_rate": o.TaxRate,
			"tax_inclusive": o.TaxInclusive, "status": o.Status, "refunded": o.Refunded, "refunded_at": tsp(o.RefundedAt),
			"affiliate_id": o.AffiliateID, "referral_amount": o.ReferralAmount, "country": o.Country, "city": o.City, "postal_code": o.PostalCode,
			"card_brand": o.CardBrand, "card_last_four": o.CardLastFour, "custom_data": o.CustomData.V,
			"stripe_checkout_session": derefStr(o.StripeCheckoutSession), "stripe_payment_intent": o.StripePaymentIntent,
			"stripe_invoice_id": o.StripeInvoiceID, "stripe_customer_id": o.StripeCustomerID, "source": o.Source, "test_mode": o.TestMode,
			"receipt_url": h.S.URLs.Receipt(o.Identifier), "first_item": fi, "created_at": ts(o.CreatedAt), "updated_at": ts(o.UpdatedAt),
		}
	}
	return out
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (h *Handler) orderQuery(r *http.Request, p page) *gorm.DB {
	q := h.S.DB.Model(&db.Order{}).Where("test_mode = ?", p.TestMode)
	if p.StoreID > 0 {
		q = q.Where("store_id = ?", p.StoreID)
	}
	if s := r.URL.Query().Get("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	if cid := qInt(r, "customer_id"); cid > 0 {
		q = q.Where("customer_id = ?", cid)
	}
	if t, _ := parseRFC3339(r.URL.Query().Get("created_from")); t != nil {
		q = q.Where("created_at >= ?", *t)
	}
	if t, _ := parseRFC3339(r.URL.Query().Get("created_to")); t != nil {
		q = q.Where("created_at <= ?", *t)
	}
	if p.Q != "" {
		cond := "(LOWER(user_email) LIKE ? ESCAPE '\\' OR LOWER(user_name) LIKE ? ESCAPE '\\' OR identifier = ?"
		args := []any{like(p.Q), like(p.Q), strings.TrimSpace(p.Q)}
		if n, err := strconv.ParseInt(strings.TrimPrefix(p.Q, "#"), 10, 64); err == nil {
			cond += " OR order_number = ?"
			args = append(args, n)
		}
		q = q.Where(cond+")", args...)
	}
	return q
}

var orderSorts = map[string]string{"created_at": "created_at", "total": "total_usd", "order_number": "order_number", "id": "id", "status": "status"}

func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	q := applySort(h.orderQuery(r, p), p.Sort, orderSorts, "id DESC")
	var rows []db.Order
	meta, err := paginate(q, p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.respondList(w, h.orderDTOs(rows), meta)
}

func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var o db.Order
	if err := h.S.DB.First(&o, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	dto := h.orderDTOs([]db.Order{o})[0]
	var items []db.OrderItem
	h.S.DB.Where("order_id = ?", id).Order("id").Find(&items)
	itemDTO := make([]map[string]any, len(items))
	for i, it := range items {
		itemDTO[i] = map[string]any{"id": it.ID, "order_id": it.OrderID, "product_id": it.ProductID, "variant_id": it.VariantID, "price_id": it.PriceID,
			"product_name": it.ProductName, "variant_name": it.VariantName, "price": it.Price, "quantity": it.Quantity}
	}
	var ks []db.LicenseKey
	h.S.DB.Where("order_id = ?", id).Order("id").Find(&ks)
	var ss []db.Subscription
	h.S.DB.Where("order_id = ?", id).Order("id").Find(&ss)
	var ems []db.EmailLog
	h.S.DB.Where("order_id = ?", id).Order("id DESC").Limit(20).Find(&ems)
	emails := make([]map[string]any, len(ems))
	for i, e := range ems {
		emails[i] = emailDTO(e)
	}
	dto["items"] = itemDTO
	dto["license_keys"] = h.keyDTOs(ks)
	dto["subscriptions"] = h.subscriptionDTOs(ss)
	dto["files"] = h.fileDTOs(h.S.FilesForOrder(id))
	dto["emails"] = emails
	dto["webhook_deliveries"] = h.orderDeliveries(o)
	h.ok(w, dto)
}

func (h *Handler) orderDeliveries(o db.Order) []map[string]any {
	var ds []db.WebhookDelivery
	h.S.DB.Where("resource_type = ? AND resource_id = ?", "orders", strconv.FormatInt(o.ID, 10)).Order("id DESC").Limit(20).Find(&ds)
	return h.deliveryDTOs(ds, false)
}

func (h *Handler) refundOrder(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var in struct {
		Amount *int64 `json:"amount"`
		Reason string `json:"reason"`
	}
	if r.ContentLength != 0 && !h.decode(w, r, &in) {
		return
	}
	o, err := h.S.RefundOrder(r.Context(), id, in.Amount, in.Reason)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "order.refund", "order", id, map[string]any{"amount": in.Amount})
	h.ok(w, h.orderDTOs([]db.Order{*o})[0])
}

func (h *Handler) resendReceipt(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	var in struct {
		Email string `json:"email"`
	}
	if r.ContentLength != 0 {
		_ = h.decode(w, r, &in)
	}
	var o db.Order
	if err := h.S.DB.First(&o, id).Error; err != nil {
		h.failErr(w, err)
		return
	}
	if err := h.S.ResendReceipt(r.Context(), id, strings.TrimSpace(in.Email)); err != nil {
		h.fail(w, http.StatusBadGateway, "internal", "Could not send the email: "+err.Error(), nil)
		return
	}
	h.audit(r, u, "order.resend_receipt", "order", id, nil)
	noContent(w)
}

func (h *Handler) exportOrders(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="orders.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"identifier", "order_number", "product_name", "variant_name", "user_name", "user_email", "city", "country", "postal_code",
		"card_brand", "card_last_four", "currency", "currency_rate", "subtotal", "discount_total", "tax", "total", "tax_name", "tax_rate", "tax_number", "date_utc"})
	q := applySort(h.orderQuery(r, p), p.Sort, orderSorts, "id")
	var batch []db.Order
	err := q.FindInBatches(&batch, 500, func(tx *gorm.DB, _ int) error {
		ids := make([]int64, len(batch))
		for i, o := range batch {
			ids[i] = o.ID
		}
		first := map[int64]db.OrderItem{}
		var its []db.OrderItem
		h.S.DB.Where("order_id IN ?", ids).Order("id").Find(&its)
		for _, it := range its {
			if _, ok := first[it.OrderID]; !ok {
				first[it.OrderID] = it
			}
		}
		for _, o := range batch {
			it := first[o.ID]
			tn := ""
			if o.TaxName != nil {
				tn = *o.TaxName
			}
			_ = cw.Write([]string{o.Identifier, strconv.FormatInt(o.OrderNumber, 10), it.ProductName, it.VariantName, o.UserName, o.UserEmail, o.City, o.Country, o.PostalCode,
				o.CardBrand, o.CardLastFour, o.Currency, o.CurrencyRate, strconv.FormatInt(o.SubtotalUSD, 10), strconv.FormatInt(o.DiscountTotalUSD, 10),
				strconv.FormatInt(o.TaxUSD, 10), strconv.FormatInt(o.TotalUSD, 10), tn, o.TaxRate, o.TaxNumber, o.CreatedAt.UTC().Format("2006-01-02 15:04:05")})
		}
		return nil
	}).Error
	cw.Flush()
	if err != nil {
		h.S.Log.Error("export orders", "err", err)
	}
}

// ---------------------------------------------------------------- subscriptions

func (h *Handler) subscriptionDTOs(ss []db.Subscription) []map[string]any {
	out := make([]map[string]any, len(ss))
	for i, s := range ss {
		out[i] = map[string]any{
			"id": s.ID, "store_id": s.StoreID, "customer_id": s.CustomerID, "order_id": s.OrderID, "order_item_id": s.OrderItemID,
			"product_id": s.ProductID, "variant_id": s.VariantID, "price_id": s.PriceID, "product_name": s.ProductName, "variant_name": s.VariantName,
			"user_name": s.UserName, "user_email": s.UserEmail, "status": s.Status, "status_formatted": resource.FormatStatus(s.Status),
			"card_brand": s.CardBrand, "card_last_four": s.CardLastFour, "payment_processor": s.PaymentProcessor, "pause": s.Pause.V,
			"cancelled": s.Cancelled, "trial_ends_at": tsp(s.TrialEndsAt), "billing_anchor": s.BillingAnchor, "quantity": s.Quantity,
			"renews_at": tsp(s.RenewsAt), "ends_at": tsp(s.EndsAt), "stripe_subscription_id": derefStr(s.StripeSubscriptionID),
			"stripe_customer_id": s.StripeCustomerID, "test_mode": s.TestMode, "created_at": ts(s.CreatedAt), "updated_at": ts(s.UpdatedAt),
		}
	}
	return out
}

func invoiceDTOs(is []db.SubscriptionInvoice) []map[string]any {
	out := make([]map[string]any, len(is))
	for i, v := range is {
		out[i] = map[string]any{
			"id": v.ID, "subscription_id": v.SubscriptionID, "customer_id": v.CustomerID, "user_name": v.UserName, "user_email": v.UserEmail,
			"billing_reason": v.BillingReason, "card_brand": v.CardBrand, "card_last_four": v.CardLastFour, "currency": v.Currency, "status": v.Status,
			"refunded": v.Refunded, "refunded_at": tsp(v.RefundedAt), "subtotal": v.Subtotal, "discount_total": v.DiscountTotal, "tax": v.Tax,
			"total": v.Total, "refunded_amount": v.RefundedAmount, "invoice_url": v.InvoiceURL, "stripe_invoice_id": derefStr(v.StripeInvoiceID),
			"created_at": ts(v.CreatedAt),
		}
	}
	return out
}

func (h *Handler) listSubscriptions(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	q := h.S.DB.Model(&db.Subscription{}).Where("test_mode = ?", p.TestMode)
	if p.StoreID > 0 {
		q = q.Where("store_id = ?", p.StoreID)
	}
	if s := r.URL.Query().Get("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	if cid := qInt(r, "customer_id"); cid > 0 {
		q = q.Where("customer_id = ?", cid)
	}
	if pid := qInt(r, "product_id"); pid > 0 {
		q = q.Where("product_id = ?", pid)
	}
	if p.Q != "" {
		q = q.Where("(LOWER(user_email) LIKE ? ESCAPE '\\' OR LOWER(user_name) LIKE ? ESCAPE '\\' OR LOWER(product_name) LIKE ? ESCAPE '\\')", like(p.Q), like(p.Q), like(p.Q))
	}
	q = applySort(q, p.Sort, map[string]string{"created_at": "created_at", "renews_at": "renews_at", "status": "status", "id": "id"}, "id DESC")
	var rows []db.Subscription
	meta, err := paginate(q, p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.respondList(w, h.subscriptionDTOs(rows), meta)
}

func (h *Handler) subscriptionDetail(id int64) (map[string]any, error) {
	var s db.Subscription
	if err := h.S.DB.First(&s, id).Error; err != nil {
		return nil, err
	}
	dto := h.subscriptionDTOs([]db.Subscription{s})[0]
	var inv []db.SubscriptionInvoice
	h.S.DB.Where("subscription_id = ?", id).Order("id DESC").Find(&inv)
	var ks []db.LicenseKey
	h.S.DB.Where("subscription_id = ?", id).Order("id").Find(&ks)
	dto["invoices"] = invoiceDTOs(inv)
	dto["license_keys"] = h.keyDTOs(ks)
	return dto, nil
}

func (h *Handler) getSubscription(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	dto, err := h.subscriptionDetail(id)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.ok(w, dto)
}

func (h *Handler) subAction(w http.ResponseWriter, r *http.Request, u *db.User, action string, fn func(id int64) (*db.Subscription, error)) {
	id, _ := pathID(r)
	if _, err := fn(id); err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "subscription."+action, "subscription", id, nil)
	dto, err := h.subscriptionDetail(id)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.ok(w, dto)
}

func (h *Handler) cancelSubscription(w http.ResponseWriter, r *http.Request, u *db.User) {
	h.subAction(w, r, u, "cancel", func(id int64) (*db.Subscription, error) { return h.S.CancelSubscription(r.Context(), id) })
}
func (h *Handler) resumeSubscription(w http.ResponseWriter, r *http.Request, u *db.User) {
	h.subAction(w, r, u, "resume", func(id int64) (*db.Subscription, error) { return h.S.ResumeSubscription(r.Context(), id) })
}
func (h *Handler) syncSubscription(w http.ResponseWriter, r *http.Request, u *db.User) {
	h.subAction(w, r, u, "sync", func(id int64) (*db.Subscription, error) { return h.S.SyncSubscription(r.Context(), id) })
}
func (h *Handler) linkSubscription(w http.ResponseWriter, r *http.Request, u *db.User) {
	var in struct {
		StripeSubscriptionID string `json:"stripe_subscription_id"`
	}
	if !h.decode(w, r, &in) {
		return
	}
	h.subAction(w, r, u, "link", func(id int64) (*db.Subscription, error) {
		return h.S.LinkStripeSubscription(r.Context(), id, in.StripeSubscriptionID)
	})
}

func (h *Handler) listInvoices(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	q := h.S.DB.Model(&db.SubscriptionInvoice{}).Where("test_mode = ?", p.TestMode)
	if sid := qInt(r, "subscription_id"); sid > 0 {
		q = q.Where("subscription_id = ?", sid)
	}
	if s := r.URL.Query().Get("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	q = applySort(q, p.Sort, map[string]string{"created_at": "created_at", "id": "id", "total": "total"}, "id DESC")
	var rows []db.SubscriptionInvoice
	meta, err := paginate(q, p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.respondList(w, invoiceDTOs(rows), meta)
}

// ---------------------------------------------------------------- license keys

func (h *Handler) keyDTOs(ks []db.LicenseKey) []map[string]any {
	vids := make([]int64, 0, len(ks))
	for _, k := range ks {
		vids = append(vids, k.VariantID)
	}
	vn, pn := h.variantProductNames(vids)
	now := time.Now()
	out := make([]map[string]any, len(ks))
	for i := range ks {
		k := ks[i]
		st := resource.LicenseStatus(&k, now)
		out[i] = map[string]any{
			"id": k.ID, "store_id": k.StoreID, "customer_id": k.CustomerID, "order_id": k.OrderID, "order_item_id": k.OrderItemID,
			"product_id": k.ProductID, "variant_id": k.VariantID, "subscription_id": k.SubscriptionID, "product_name": pn[k.VariantID],
			"variant_name": vn[k.VariantID], "user_name": k.UserName, "user_email": k.UserEmail, "key": k.Key, "key_short": resource.KeyShort(k.Key),
			"activation_limit": k.ActivationLimit, "instances_count": k.InstancesCount, "disabled": k.Disabled, "status": st,
			"status_formatted": resource.FormatStatus(st), "expires_at": tsp(k.ExpiresAt), "test_mode": k.TestMode,
			"created_at": ts(k.CreatedAt), "updated_at": ts(k.UpdatedAt),
		}
	}
	return out
}

func instanceDTOs(is []db.LicenseKeyInstance) []map[string]any {
	out := make([]map[string]any, len(is))
	for i, it := range is {
		out[i] = map[string]any{"id": it.ID, "license_key_id": it.LicenseKeyID, "identifier": it.Identifier, "name": it.Name, "created_at": ts(it.CreatedAt), "updated_at": ts(it.UpdatedAt)}
	}
	return out
}

func (h *Handler) listKeys(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	q := h.S.DB.Model(&db.LicenseKey{}).Where("test_mode = ?", p.TestMode)
	if p.StoreID > 0 {
		q = q.Where("store_id = ?", p.StoreID)
	}
	now := time.Now().UTC()
	switch r.URL.Query().Get("status") {
	case "disabled":
		q = q.Where("disabled = ?", true)
	case "expired":
		q = q.Where("disabled = ? AND expires_at IS NOT NULL AND expires_at <= ?", false, now)
	case "active":
		q = q.Where("disabled = ? AND instances_count > 0 AND (expires_at IS NULL OR expires_at > ?)", false, now)
	case "inactive":
		q = q.Where("disabled = ? AND instances_count = 0 AND (expires_at IS NULL OR expires_at > ?)", false, now)
	}
	for _, f := range []string{"order_id", "customer_id", "product_id", "variant_id"} {
		if v := qInt(r, f); v > 0 {
			q = q.Where(f+" = ?", v)
		}
	}
	if p.Q != "" {
		q = q.Where("(LOWER(\"key\") LIKE ? ESCAPE '\\' OR LOWER(user_email) LIKE ? ESCAPE '\\' OR LOWER(user_name) LIKE ? ESCAPE '\\')", like(p.Q), like(p.Q), like(p.Q))
	}
	q = applySort(q, p.Sort, map[string]string{"created_at": "created_at", "expires_at": "expires_at", "id": "id", "instances_count": "instances_count"}, "id DESC")
	var rows []db.LicenseKey
	meta, err := paginate(q, p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.respondList(w, h.keyDTOs(rows), meta)
}

func (h *Handler) keyDetail(id int64) (map[string]any, error) {
	var k db.LicenseKey
	if err := h.S.DB.First(&k, id).Error; err != nil {
		return nil, err
	}
	dto := h.keyDTOs([]db.LicenseKey{k})[0]
	var is []db.LicenseKeyInstance
	h.S.DB.Where("license_key_id = ?", id).Order("id").Find(&is)
	dto["instances"] = instanceDTOs(is)
	return dto, nil
}

func (h *Handler) getKey(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	dto, err := h.keyDetail(id)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.ok(w, dto)
}

func (h *Handler) updateKey(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	var p service.LicensePatch
	if b.has("activation_limit") {
		p.ActivationLimitSet = true
		if n, ok := b.int("activation_limit"); ok {
			v := int(n)
			p.ActivationLimit = &v
		}
	}
	if b.has("expires_at") {
		p.ExpiresAtSet = true
		if s, ok := b.str("expires_at"); ok && s != "" {
			t, err := parseRFC3339(s)
			if err != nil {
				h.invalid(w, "expires_at", "Invalid date-time.")
				return
			}
			p.ExpiresAt = t
		}
	}
	if d, ok := b.boolean("disabled"); ok {
		p.Disabled = &d
	}
	if _, err := h.S.PatchLicenseKey(r.Context(), id, p); err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "license_key.update", "license_key", id, b)
	dto, _ := h.keyDetail(id)
	h.ok(w, dto)
}

func (h *Handler) createKey(w http.ResponseWriter, r *http.Request, u *db.User) {
	b, ok := h.readBody(w, r)
	if !ok {
		return
	}
	vid, _ := b.int("variant_id")
	var v db.Variant
	if err := h.S.DB.First(&v, vid).Error; err != nil {
		h.invalid(w, "variant_id", "The selected variant is invalid.")
		return
	}
	var prod db.Product
	h.S.DB.First(&prod, v.ProductID)
	var store db.Store
	h.S.DB.First(&store, prod.StoreID)

	var cust db.Customer
	var ord db.Order
	var itemID int64
	email, _ := b.str("user_email")
	name, _ := b.str("user_name")
	if oid, ok := b.int("order_id"); ok && oid > 0 {
		if err := h.S.DB.First(&ord, oid).Error; err != nil {
			h.invalid(w, "order_id", "The selected order is invalid.")
			return
		}
		h.S.DB.First(&cust, ord.CustomerID)
		var it db.OrderItem
		if h.S.DB.Where("order_id = ? AND variant_id = ?", oid, vid).Order("id").First(&it).Error == nil {
			itemID = it.ID
		}
		store.ID = ord.StoreID
	} else if cid, ok := b.int("customer_id"); ok && cid > 0 {
		if err := h.S.DB.First(&cust, cid).Error; err != nil {
			h.invalid(w, "customer_id", "The selected customer is invalid.")
			return
		}
	} else if email != "" {
		c, err := h.S.CreateCustomer(r.Context(), service.CustomerInput{StoreID: store.ID, TestMode: v.TestMode, Name: firstNonEmpty(name, email), Email: email})
		if err != nil {
			// existing customer → reuse
			if h.S.DB.Where("store_id = ? AND test_mode = ? AND LOWER(email) = ?", store.ID, v.TestMode, strings.ToLower(email)).First(&cust).Error != nil {
				h.failErr(w, err)
				return
			}
		} else {
			cust = *c
		}
	} else {
		h.invalid(w, "user_email", "Provide an order_id, customer_id or user_email.")
		return
	}
	now := db.Now()
	k := db.LicenseKey{
		StoreID: store.ID, CustomerID: cust.ID, OrderID: ord.ID, OrderItemID: itemID, ProductID: prod.ID, VariantID: v.ID,
		UserName: firstNonEmpty(name, cust.Name), UserEmail: firstNonEmpty(email, cust.Email), Key: service.NewLicenseKeyString(), TestMode: v.TestMode, CreatedAt: now, UpdatedAt: now,
	}
	if !v.IsLicenseLimitUnlimited {
		l := v.LicenseActivationLimit
		k.ActivationLimit = &l
	}
	if !v.IsLicenseLengthUnlimited && v.LicenseLengthValue > 0 {
		e := service.AddLicenseLength(now, v.LicenseLengthValue, v.LicenseLengthUnit)
		k.ExpiresAt = &e
	}
	if b.has("activation_limit") {
		if n, ok := b.int("activation_limit"); ok {
			l := int(n)
			k.ActivationLimit = &l
		} else {
			k.ActivationLimit = nil
		}
	}
	if b.has("expires_at") {
		if s, ok := b.str("expires_at"); ok && s != "" {
			t, err := parseRFC3339(s)
			if err != nil {
				h.invalid(w, "expires_at", "Invalid date-time.")
				return
			}
			k.ExpiresAt = t
		} else {
			k.ExpiresAt = nil
		}
	}
	if err := h.S.DB.Create(&k).Error; err != nil {
		h.failErr(w, err)
		return
	}
	h.S.Events.Emit(r.Context(), service.Event{Name: "license_key_created", StoreID: k.StoreID, TestMode: k.TestMode, ResourceType: "license-keys", ResourceID: k.ID})
	h.audit(r, u, "license_key.create", "license_key", k.ID, nil)
	dto, _ := h.keyDetail(k.ID)
	h.created(w, dto)
}

func (h *Handler) regenerateKey(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	err := h.S.DB.Transaction(func(tx *gorm.DB) error {
		var k db.LicenseKey
		if err := tx.First(&k, id).Error; err != nil {
			return err
		}
		if err := tx.Where("license_key_id = ?", id).Delete(&db.LicenseKeyInstance{}).Error; err != nil {
			return err
		}
		return tx.Model(&k).Updates(map[string]any{"key": uuid.NewString(), "instances_count": 0, "updated_at": db.Now()}).Error
	})
	if err != nil {
		h.failErr(w, err)
		return
	}
	var k db.LicenseKey
	h.S.DB.First(&k, id)
	h.S.Events.Emit(r.Context(), service.Event{Name: "license_key_updated", StoreID: k.StoreID, TestMode: k.TestMode, ResourceType: "license-keys", ResourceID: k.ID})
	h.audit(r, u, "license_key.regenerate", "license_key", id, nil)
	dto, _ := h.keyDetail(id)
	h.ok(w, dto)
}

func (h *Handler) listInstances(w http.ResponseWriter, r *http.Request, u *db.User) {
	p := parsePage(r)
	q := h.S.DB.Model(&db.LicenseKeyInstance{})
	if kid := qInt(r, "license_key_id"); kid > 0 {
		q = q.Where("license_key_id = ?", kid)
	}
	q = applySort(q, p.Sort, map[string]string{"created_at": "created_at", "id": "id"}, "id")
	var rows []db.LicenseKeyInstance
	meta, err := paginate(q, p, &rows)
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.respondList(w, instanceDTOs(rows), meta)
}

func (h *Handler) deleteInstance(w http.ResponseWriter, r *http.Request, u *db.User) {
	id, _ := pathID(r)
	err := h.S.DB.Transaction(func(tx *gorm.DB) error {
		var it db.LicenseKeyInstance
		if err := tx.First(&it, id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&it).Error; err != nil {
			return err
		}
		var n int64
		tx.Model(&db.LicenseKeyInstance{}).Where("license_key_id = ?", it.LicenseKeyID).Count(&n)
		return tx.Model(&db.LicenseKey{}).Where("id = ?", it.LicenseKeyID).Updates(map[string]any{"instances_count": n, "updated_at": db.Now()}).Error
	})
	if err != nil {
		h.failErr(w, err)
		return
	}
	h.audit(r, u, "license_key.instance_delete", "license_key_instance", id, nil)
	noContent(w)
}
