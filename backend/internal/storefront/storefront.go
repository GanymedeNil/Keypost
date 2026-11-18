// Package storefront serves everything the buyer touches: signed file
// downloads, checkout redirects, the Stripe billing portal hand-off, the
// Stripe webhook endpoint and the customer "My orders" JSON API.
package storefront

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"fulfillment/internal/crypt"
	"fulfillment/internal/db"
	"fulfillment/internal/httpx"
	"fulfillment/internal/mail"
	"fulfillment/internal/ratelimit"
	"fulfillment/internal/resource"
	"fulfillment/internal/service"
	"fulfillment/internal/settings"
	"fulfillment/internal/storage"
	"fulfillment/internal/stripe"
)

type Handler struct {
	S       *service.Svc
	limiter *ratelimit.Limiter
}

func New(s *service.Svc) *Handler { return &Handler{S: s, limiter: ratelimit.New(time.Minute)} }

const customerCookie = "df_customer"

func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /webhooks/stripe", h.stripeWebhook)
	mux.HandleFunc("GET /download/{identifier}", h.download)
	mux.HandleFunc("GET /checkout/buy/{slug}", h.buy)
	mux.HandleFunc("GET /checkout/success", h.checkoutSuccess)
	mux.HandleFunc("GET /billing", h.billing)

	mux.HandleFunc("POST /storefront-api/auth/magic-link", h.csrf(h.magicLink))
	mux.HandleFunc("POST /storefront-api/auth/verify", h.csrf(h.verify))
	mux.HandleFunc("POST /storefront-api/auth/logout", h.csrf(h.logout))
	mux.HandleFunc("GET /storefront-api/me/orders", h.myOrders)
	mux.HandleFunc("GET /storefront-api/orders/{identifier}", h.orderDetail)
	mux.HandleFunc("GET /storefront-api/billing-url", h.billingURL)
}

func (h *Handler) csrf(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Requested-With") != "df-admin" {
			apiErr(w, http.StatusForbidden, "forbidden", "Missing X-Requested-With header.")
			return
		}
		next(w, r)
	}
}

func apiErr(w http.ResponseWriter, status int, code, msg string) {
	httpx.JSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": msg}})
}

func (h *Handler) ip(r *http.Request) string { return httpx.ClientIP(r, h.S.Cfg.TrustProxyHeaders) }

// ---------------------------------------------------------------- stripe webhook

func (h *Handler) stripeWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}
	secrets := h.S.Settings.List(settings.StripeWebhookSecret)
	if err := stripe.VerifySignature(body, r.Header.Get("Stripe-Signature"), secrets, 5*time.Minute, time.Now()); err != nil {
		h.S.Log.Warn("stripe webhook rejected", "err", err, "ip", h.ip(r))
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}
	if err := h.S.IngestStripeEvent(r.Context(), body); err != nil {
		http.Error(w, "processing failed, will retry", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"received":true}`))
}

// ---------------------------------------------------------------- downloads

func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	ident := r.PathValue("identifier")
	q := r.URL.Query()
	expires, _ := strconv.ParseInt(q.Get("expires"), 10, 64)
	orderID, _ := strconv.ParseInt(q.Get("order"), 10, 64)
	if !h.S.URLs.VerifyDownload(ident, expires, orderID, q.Get("signature"), time.Now()) {
		textPage(w, http.StatusForbidden, "Link expired", "This download link is invalid or has expired. Open your order page again to get a fresh link.")
		return
	}
	var f db.File
	if err := h.S.DB.Where("identifier = ?", ident).First(&f).Error; err != nil || (f.Status != "published" && orderID != 0) {
		textPage(w, http.StatusNotFound, "File not found", "This file is no longer available.")
		return
	}
	if orderID != 0 && !h.orderMayDownload(orderID, f) {
		textPage(w, http.StatusForbidden, "Access denied", "This order no longer has access to this file (refunded or subscription ended).")
		return
	}
	ip := h.ip(r)
	limit := h.S.Settings.Int(settings.DownloadLimitPerDay, 10)
	if limit > 0 {
		var n int64
		h.S.DB.Model(&db.DownloadLog{}).Where("file_id = ? AND ip = ? AND created_at > ?", f.ID, ip, db.Now().Add(-24*time.Hour)).Count(&n)
		if int(n) >= limit {
			w.Header().Set("Retry-After", "3600")
			textPage(w, http.StatusTooManyRequests, "Download limit reached", fmt.Sprintf("This file can be downloaded %d times per day from one network. Please try again later.", limit))
			return
		}
	}
	rec := db.DownloadLog{FileID: f.ID, IP: ip, UserAgent: trunc(r.UserAgent(), 255), CreatedAt: db.Now()}
	if orderID != 0 {
		rec.OrderID = &orderID
	}
	h.S.DB.Create(&rec)
	h.S.DB.Model(&db.File{}).Where("id = ?", f.ID).UpdateColumn("downloads_count", gorm.Expr("downloads_count + 1"))
	Serve(r.Context(), h.S, w, r, f)
}

// Serve streams (or redirects to) a stored file.
func Serve(ctx context.Context, s *service.Svc, w http.ResponseWriter, r *http.Request, f db.File) {
	if u, err := s.Storage.PresignGet(ctx, f.StorageKey, f.Name, 5*time.Minute); err == nil && u != "" {
		http.Redirect(w, r, u, http.StatusFound)
		return
	}
	rc, size, err := s.Storage.Open(ctx, f.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			textPage(w, http.StatusNotFound, "File missing", "The file is registered but missing from storage. Please contact the seller.")
			return
		}
		textPage(w, http.StatusInternalServerError, "Error", "Could not open the file.")
		return
	}
	defer rc.Close()
	ct := f.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, asciiName(f.Name), url.PathEscape(f.Name)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	if rs, ok := rc.(io.ReadSeeker); ok {
		http.ServeContent(w, r, f.Name, time.Time{}, rs)
		return
	}
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	_, _ = io.Copy(w, rc)
}

func asciiName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 32 || r > 126 || r == '"' || r == '\\' {
			b.WriteByte('_')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// orderMayDownload checks refund/subscription state for the order that owns the link.
func (h *Handler) orderMayDownload(orderID int64, f db.File) bool {
	var o db.Order
	if h.S.DB.First(&o, orderID).Error != nil {
		return false
	}
	switch o.Status {
	case "paid", "partial_refund":
	default:
		return false
	}
	var n int64
	h.S.DB.Model(&db.OrderItem{}).Where("order_id = ? AND variant_id = ?", orderID, f.VariantID).Count(&n)
	if n == 0 {
		return false
	}
	var expired int64
	h.S.DB.Model(&db.Subscription{}).Where("order_id = ? AND variant_id = ? AND status IN ?", orderID, f.VariantID, []string{"expired", "unpaid"}).Count(&expired)
	return expired == 0
}

// ---------------------------------------------------------------- checkout + billing

func (h *Handler) buy(w http.ResponseWriter, r *http.Request) {
	target, err := h.S.BuyNow(r.Context(), r.PathValue("slug"))
	if err != nil {
		e, _ := service.AsError(err)
		status, msg := http.StatusBadGateway, err.Error()
		if e != nil {
			status, msg = e.Status, e.Detail
		}
		textPage(w, status, "Checkout unavailable", msg)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (h *Handler) checkoutSuccess(w http.ResponseWriter, r *http.Request) {
	sid := r.URL.Query().Get("session_id")
	if sid == "" || !strings.HasPrefix(sid, "cs_") {
		http.Redirect(w, r, "/my-orders", http.StatusSeeOther)
		return
	}
	target, _ := h.S.CheckoutLanding(r.Context(), sid, r.URL.Query().Get("checkout"))
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (h *Handler) billing(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cid, _ := strconv.ParseInt(q.Get("customer"), 10, 64)
	sid, _ := strconv.ParseInt(q.Get("subscription"), 10, 64)
	exp, _ := strconv.ParseInt(q.Get("expires"), 10, 64)
	if cid == 0 || !h.S.URLs.VerifyPortal(cid, sid, exp, q.Get("signature"), time.Now()) {
		textPage(w, http.StatusForbidden, "Link expired", "This billing link is invalid or has expired. Open your order page again to get a new one.")
		return
	}
	target, err := h.S.PortalURL(r.Context(), cid, h.S.URLs.Base+"/my-orders")
	if err != nil {
		textPage(w, http.StatusBadGateway, "Billing portal unavailable", err.Error())
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func textPage(w http.ResponseWriter, status int, title, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title>
<style>body{font:16px/1.5 -apple-system,Segoe UI,Helvetica,Arial,sans-serif;background:#f5f5f7;color:#1d1d1f;display:grid;place-items:center;min-height:100vh;margin:0}main{background:#fff;border-radius:14px;padding:32px;max-width:440px;margin:16px;box-shadow:0 1px 3px rgba(0,0,0,.08)}h1{font-size:20px;margin:0 0 8px}p{color:#555;margin:0}a{color:#0071e3}</style></head>
<body><main><h1>%s</h1><p>%s</p><p style="margin-top:16px"><a href="/my-orders">Go to my orders</a></p></main></body></html>`, html.EscapeString(title), html.EscapeString(title), html.EscapeString(msg))
}

// ---------------------------------------------------------------- customer session

func (h *Handler) setCustomerCookie(w http.ResponseWriter, email string) {
	exp := time.Now().Add(30 * 24 * time.Hour).Unix()
	enc := base64.RawURLEncoding.EncodeToString([]byte(email))
	val := fmt.Sprintf("%s.%d.%s", enc, exp, crypt.MAC("customer", enc, strconv.FormatInt(exp, 10)))
	http.SetCookie(w, &http.Cookie{Name: customerCookie, Value: val, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(h.S.Cfg.PublicURL, "https://"), SameSite: http.SameSiteLaxMode, Expires: time.Unix(exp, 0)})
}

func (h *Handler) customerEmail(r *http.Request) (string, bool) {
	c, err := r.Cookie(customerCookie)
	if err != nil {
		return "", false
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 3 {
		return "", false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || exp < time.Now().Unix() || !crypt.MACEqual(parts[2], crypt.MAC("customer", parts[0], parts[1])) {
		return "", false
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	return string(b), true
}

func (h *Handler) magicLink(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if err := httpx.ReadJSON(r, &in, 4096); err != nil {
		apiErr(w, http.StatusBadRequest, "validation_failed", "Invalid request body.")
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if ok, _, _ := h.limiter.Allow("ml-ip:"+h.ip(r), 10); !ok {
		apiErr(w, http.StatusTooManyRequests, "rate_limited", "Too many requests. Try again in a minute.")
		return
	}
	if email == "" || !strings.Contains(email, "@") {
		apiErr(w, http.StatusUnprocessableEntity, "validation_failed", "Enter a valid email address.")
		return
	}
	if ok, _, _ := h.limiter.Allow("ml-mail:"+email, 3); ok {
		var last db.Order
		found := h.S.DB.Where("LOWER(user_email) = ? AND test_mode IN ?", email, h.visibleModes()).Order("id DESC").Limit(1).Find(&last).RowsAffected > 0
		if found {
			token := crypt.RandomToken(32)
			now := db.Now()
			h.S.DB.Create(&db.CustomerLoginToken{TokenHash: crypt.SHA256Hex(token), Email: email, ExpiresAt: now.Add(15 * time.Minute), CreatedAt: now})
			var store db.Store // the store the customer bought from, so the e-mail names the right shop
			if h.S.DB.First(&store, last.StoreID).Error != nil {
				h.S.DB.Order("id").First(&store)
			}
			link := h.S.URLs.MagicLink(token)
			h.S.Go(func() {
				html, text, err := mail.RenderMagicLink(mail.MagicLinkData{StoreName: firstNonEmpty(store.Name, "Your store"), AvatarURL: store.AvatarURL, Link: link})
				if err == nil {
					_ = h.S.Mail.Send(context.Background(), mail.Message{To: email, Subject: "Your sign-in link", HTML: html, Text: text, StoreID: store.ID, Template: "magic_link", FromName: store.EmailFromName})
				}
			})
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// visibleModes: customers see live data. On a server running with a Stripe test key (an
// evaluation or staging instance) everything is test data, so test-mode rows are shown too;
// otherwise nobody could sign in to My Orders there.
func (h *Handler) visibleModes() []bool {
	if h.S.StripeTestMode() {
		return []bool{false, true}
	}
	return []bool{false}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if err := httpx.ReadJSON(r, &in, 4096); err != nil || in.Token == "" {
		apiErr(w, http.StatusBadRequest, "invalid_token", "The sign-in link is invalid or has expired.")
		return
	}
	if ok, _, _ := h.limiter.Allow("verify:"+h.ip(r), 20); !ok {
		apiErr(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts.")
		return
	}
	now := db.Now()
	res := h.S.DB.Model(&db.CustomerLoginToken{}).Where("token_hash = ? AND used_at IS NULL AND expires_at > ?", crypt.SHA256Hex(in.Token), now).Update("used_at", now)
	if res.Error != nil || res.RowsAffected == 0 {
		apiErr(w, http.StatusBadRequest, "invalid_token", "The sign-in link is invalid or has expired.")
		return
	}
	var t db.CustomerLoginToken
	h.S.DB.Where("token_hash = ?", crypt.SHA256Hex(in.Token)).First(&t)
	h.setCustomerCookie(w, t.Email)
	httpx.JSON(w, http.StatusOK, map[string]any{"email": t.Email})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: customerCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- orders

type orderBrief struct {
	Identifier    string `json:"identifier"`
	OrderNumber   int64  `json:"order_number"`
	CreatedAt     string `json:"created_at"`
	Status        string `json:"status"`
	TotalFormated string `json:"total_formatted"`
	Currency      string `json:"currency"`
	Items         []struct {
		ProductName string `json:"product_name"`
		VariantName string `json:"variant_name"`
	} `json:"items"`
	LicenseKeyCount int `json:"license_key_count"`
	FileCount       int `json:"file_count"`
}

func (h *Handler) myOrders(w http.ResponseWriter, r *http.Request) {
	email, ok := h.customerEmail(r)
	if !ok {
		apiErr(w, http.StatusUnauthorized, "unauthenticated", "Sign in to view your orders.")
		return
	}
	var orders []db.Order
	h.S.DB.Where("LOWER(user_email) = ? AND test_mode IN ?", strings.ToLower(email), h.visibleModes()).Order("id DESC").Limit(200).Find(&orders)
	ids := make([]int64, len(orders))
	for i, o := range orders {
		ids[i] = o.ID
	}
	items := map[int64][]db.OrderItem{}
	keyCount := map[int64]int{}
	if len(ids) > 0 {
		var its []db.OrderItem
		h.S.DB.Where("order_id IN ?", ids).Order("id").Find(&its)
		for _, it := range its {
			items[it.OrderID] = append(items[it.OrderID], it)
		}
		type kc struct {
			OrderID int64
			N       int
		}
		var kcs []kc
		h.S.DB.Model(&db.LicenseKey{}).Select("order_id, COUNT(*) AS n").Where("order_id IN ?", ids).Group("order_id").Scan(&kcs)
		for _, k := range kcs {
			keyCount[k.OrderID] = k.N
		}
	}
	variantFiles := map[int64]int{}
	var vids []int64
	for _, its := range items {
		for _, it := range its {
			vids = append(vids, it.VariantID)
		}
	}
	if len(vids) > 0 {
		type fc struct {
			VariantID int64
			N         int
		}
		var fcs []fc
		h.S.DB.Model(&db.File{}).Select("variant_id, COUNT(*) AS n").Where("variant_id IN ? AND status = ?", vids, "published").Group("variant_id").Scan(&fcs)
		for _, f := range fcs {
			variantFiles[f.VariantID] = f.N
		}
	}
	out := make([]orderBrief, 0, len(orders))
	for _, o := range orders {
		b := orderBrief{Identifier: o.Identifier, OrderNumber: o.OrderNumber, CreatedAt: resource.TS(o.CreatedAt), Status: o.Status,
			TotalFormated: resource.FormatMoney(o.Total, o.Currency), Currency: o.Currency, LicenseKeyCount: keyCount[o.ID]}
		for _, it := range items[o.ID] {
			b.Items = append(b.Items, struct {
				ProductName string `json:"product_name"`
				VariantName string `json:"variant_name"`
			}{it.ProductName, it.VariantName})
			b.FileCount += variantFiles[it.VariantID]
		}
		out = append(out, b)
	}
	var subs []db.Subscription
	h.S.DB.Where("LOWER(user_email) = ? AND test_mode IN ?", strings.ToLower(email), h.visibleModes()).Order("id DESC").Limit(100).Find(&subs)
	subOut := make([]map[string]any, 0, len(subs))
	for _, s := range subs {
		subOut = append(subOut, map[string]any{
			"id": s.ID, "status": s.Status, "status_formatted": resource.FormatStatus(s.Status), "product_name": s.ProductName,
			"variant_name": s.VariantName, "renews_at": resource.TSP(s.RenewsAt), "ends_at": resource.TSP(s.EndsAt),
			"billing_url": h.S.URLs.Portal(s.CustomerID, s.ID, time.Now(), false),
		})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"email": email, "orders": out, "subscriptions": subOut})
}

func (h *Handler) orderDetail(w http.ResponseWriter, r *http.Request) {
	ident := r.PathValue("identifier")
	var o db.Order
	if err := h.S.DB.Where("identifier = ?", ident).First(&o).Error; err != nil {
		apiErr(w, http.StatusNotFound, "not_found", "Order not found.")
		return
	}
	allowed := h.S.URLs.VerifyReceipt(ident, r.URL.Query().Get("signature"))
	if !allowed {
		if email, ok := h.customerEmail(r); ok && strings.EqualFold(email, o.UserEmail) {
			allowed = true
		}
	}
	if !allowed {
		apiErr(w, http.StatusUnauthorized, "unauthenticated", "Sign in or use the link from your receipt email.")
		return
	}
	httpx.JSON(w, http.StatusOK, h.buildOrderDetail(o))
}

func (h *Handler) buildOrderDetail(o db.Order) map[string]any {
	now := time.Now()
	var items []db.OrderItem
	h.S.DB.Where("order_id = ?", o.ID).Order("id").Find(&items)
	var keys []db.LicenseKey
	h.S.DB.Where("order_id = ?", o.ID).Order("id").Find(&keys)
	var subs []db.Subscription
	h.S.DB.Where("order_id = ?", o.ID).Order("id").Find(&subs)

	itemOut := make([]map[string]any, 0, len(items))
	names := map[int64]db.OrderItem{}
	variantIDs := []int64{}
	for _, it := range items {
		names[it.ID] = it
		variantIDs = append(variantIDs, it.VariantID)
		itemOut = append(itemOut, map[string]any{"product_name": it.ProductName, "variant_name": it.VariantName, "quantity": it.Quantity, "price_formatted": resource.FormatMoney(it.Price, o.Currency)})
	}
	keyOut := make([]map[string]any, 0, len(keys))
	firstKey := ""
	for _, k := range keys {
		if firstKey == "" {
			firstKey = k.Key
		}
		st := resource.LicenseStatus(&k, now)
		it := names[k.OrderItemID]
		keyOut = append(keyOut, map[string]any{
			"key": k.Key, "status": st, "status_formatted": resource.FormatStatus(st), "activation_limit": k.ActivationLimit,
			"instances_count": k.InstancesCount, "expires_at": resource.TSP(k.ExpiresAt), "product_name": it.ProductName, "variant_name": it.VariantName,
		})
	}
	hasAccess := (o.Status == "paid" || o.Status == "partial_refund")
	for _, s := range subs {
		if s.Status == "expired" || s.Status == "unpaid" {
			hasAccess = false
		}
	}
	files := []map[string]any{}
	links := []map[string]any{}
	if hasAccess && len(variantIDs) > 0 {
		var fs []db.File
		h.S.DB.Where("variant_id IN ? AND status = ?", variantIDs, "published").Order("variant_id, sort, id").Find(&fs)
		prodByVariant := map[int64]db.OrderItem{}
		for _, it := range items {
			prodByVariant[it.VariantID] = it
		}
		for _, f := range fs {
			it := prodByVariant[f.VariantID]
			files = append(files, map[string]any{
				"id": f.ID, "name": f.Name, "extension": f.Extension, "size_formatted": resource.HumanSize(f.Size), "version": f.Version,
				"product_name": it.ProductName, "variant_name": it.VariantName, "download_url": h.S.URLs.Download(f.Identifier, o.ID, now, false),
			})
		}
		var vs []db.Variant
		h.S.DB.Where("id IN ?", variantIDs).Find(&vs)
		rep := strings.NewReplacer("[license_key]", url.QueryEscape(firstKey), "[order_id]", strconv.FormatInt(o.ID, 10), "[order_identifier]", o.Identifier,
			"[email]", url.QueryEscape(o.UserEmail), "[name]", url.QueryEscape(o.UserName), "[total]", url.QueryEscape(resource.FormatMoney(o.Total, o.Currency)))
		for _, v := range vs {
			for _, l := range v.Links.V {
				links = append(links, map[string]any{"title": l.Title, "url": rep.Replace(l.URL)})
			}
		}
	}
	subOut := make([]map[string]any, 0, len(subs))
	for _, s := range subs {
		subOut = append(subOut, map[string]any{"id": s.ID, "status": s.Status, "status_formatted": resource.FormatStatus(s.Status), "product_name": s.ProductName, "renews_at": resource.TSP(s.RenewsAt), "ends_at": resource.TSP(s.EndsAt)})
	}
	var billing any
	if len(subs) > 0 {
		billing = h.S.URLs.Portal(o.CustomerID, subs[0].ID, now, false)
	}
	invoiceURL := o.StripeReceiptURL
	if invoiceURL == "" && len(subs) > 0 {
		var inv db.SubscriptionInvoice
		if h.S.DB.Where("subscription_id = ? AND invoice_url <> ''", subs[0].ID).Order("id DESC").First(&inv).Error == nil {
			invoiceURL = inv.InvoiceURL
		}
	}
	var invAny any
	if invoiceURL != "" {
		invAny = invoiceURL
	}
	return map[string]any{
		"identifier": o.Identifier, "order_number": o.OrderNumber, "status": o.Status, "status_formatted": resource.FormatStatus(o.Status), "created_at": resource.TS(o.CreatedAt),
		"user_name": o.UserName, "user_email": o.UserEmail, "currency": o.Currency,
		"subtotal_formatted": resource.FormatMoney(o.Subtotal, o.Currency), "discount_total_formatted": resource.FormatMoney(o.DiscountTotal, o.Currency),
		"tax_formatted": resource.FormatMoney(o.Tax, o.Currency), "total_formatted": resource.FormatMoney(o.Total, o.Currency), "refunded": o.Refunded,
		"card_brand": o.CardBrand, "card_last_four": o.CardLastFour,
		"items": itemOut, "license_keys": keyOut, "files": files, "links": links, "subscriptions": subOut, "billing_url": billing, "invoice_url": invAny,
	}
}

func (h *Handler) billingURL(w http.ResponseWriter, r *http.Request) {
	email, ok := h.customerEmail(r)
	if !ok {
		apiErr(w, http.StatusUnauthorized, "unauthenticated", "Sign in first.")
		return
	}
	var c db.Customer
	if err := h.S.DB.Where("LOWER(email) = ? AND test_mode IN ?", strings.ToLower(email), h.visibleModes()).Order("(stripe_customer_id = '') ASC, id").First(&c).Error; err != nil {
		apiErr(w, http.StatusNotFound, "not_found", "No billing profile found for this email.")
		return
	}
	u, err := h.S.PortalURL(r.Context(), c.ID, h.S.URLs.Base+"/my-orders")
	if err != nil {
		if e, ok := service.AsError(err); ok {
			apiErr(w, e.Status, e.Code, e.Detail)
			return
		}
		apiErr(w, http.StatusBadGateway, "stripe_error", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"url": u})
}

var _ = os.Stat
