// Package admin implements the back-office JSON API under /admin-api.
// The contract lives in docs/ADMIN_API.md.
package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"fulfillment/internal/auth"
	"fulfillment/internal/db"
	"fulfillment/internal/httpx"
	"fulfillment/internal/importer"
	"fulfillment/internal/ratelimit"
	"fulfillment/internal/service"
	"fulfillment/internal/webhooks"
)

const sessionCookie = "df_admin"

type Handler struct {
	S       *service.Svc
	D       *webhooks.Dispatcher
	Imp     *importer.Runner
	limiter *ratelimit.Limiter
}

func New(s *service.Svc, d *webhooks.Dispatcher, imp *importer.Runner) *Handler {
	return &Handler{S: s, D: d, Imp: imp, limiter: ratelimit.New(time.Minute)}
}

type handler func(w http.ResponseWriter, r *http.Request, u *db.User)

// ---------------------------------------------------------------- responses

func (h *Handler) fail(w http.ResponseWriter, status int, code, msg string, fields map[string]string) {
	e := map[string]any{"code": code, "message": msg}
	if len(fields) > 0 {
		e["fields"] = fields
	}
	httpx.JSON(w, status, map[string]any{"error": e})
}

func (h *Handler) failErr(w http.ResponseWriter, err error) {
	if e, ok := service.AsError(err); ok {
		h.fail(w, e.Status, e.Code, e.Detail, e.Fields)
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		h.fail(w, http.StatusNotFound, "not_found", "Not found.", nil)
		return
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		h.fail(w, http.StatusConflict, "conflict", "A record with the same unique value already exists.", nil)
		return
	}
	h.S.Log.Error("admin error", "err", err)
	h.fail(w, http.StatusInternalServerError, "internal", "Internal server error.", nil)
}

func (h *Handler) invalid(w http.ResponseWriter, field, msg string) {
	f := map[string]string{}
	if field != "" {
		f[field] = msg
	}
	h.fail(w, http.StatusUnprocessableEntity, "validation_failed", msg, f)
}

func (h *Handler) ok(w http.ResponseWriter, v any)      { httpx.JSON(w, http.StatusOK, v) }
func (h *Handler) created(w http.ResponseWriter, v any) { httpx.JSON(w, http.StatusCreated, v) }
func noContent(w http.ResponseWriter)                   { w.WriteHeader(http.StatusNoContent) }

func (h *Handler) decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := httpx.ReadJSON(r, v, 2<<20); err != nil {
		h.fail(w, http.StatusBadRequest, "bad_request", "Invalid JSON body: "+err.Error(), nil)
		return false
	}
	return true
}

// ---------------------------------------------------------------- middleware

type level int

const (
	lvlRead  level = iota // any signed-in user
	lvlWrite              // owner or admin
	lvlOwner              // owner only
	lvlSelf               // any signed-in user, even for writes (own profile)
)

func (h *Handler) secureCookie() bool { return strings.HasPrefix(h.S.Cfg.PublicURL, "https://") }

func (h *Handler) ip(r *http.Request) string { return httpx.ClientIP(r, h.S.Cfg.TrustProxyHeaders) }

func (h *Handler) currentUser(r *http.Request) (*db.User, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, false
	}
	u, err := auth.SessionUser(h.S.DB, c.Value)
	if err != nil {
		return nil, false
	}
	return u, true
}

func (h *Handler) guard(l level, next handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := h.currentUser(r)
		if !ok {
			h.fail(w, http.StatusUnauthorized, "unauthenticated", "Sign in required.", nil)
			return
		}
		mutating := r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions
		if mutating && r.Header.Get("X-Requested-With") != "df-admin" {
			h.fail(w, http.StatusForbidden, "forbidden", "Missing X-Requested-With header.", nil)
			return
		}
		if l != lvlSelf && (mutating || l >= lvlWrite) && u.Role == auth.RoleViewer {
			h.fail(w, http.StatusForbidden, "forbidden", "Your role is read-only.", nil)
			return
		}
		if l == lvlOwner && u.Role != auth.RoleOwner {
			h.fail(w, http.StatusForbidden, "forbidden", "Only owners can do this.", nil)
			return
		}
		next(w, r, u)
	}
}

// open registers a route that needs no session but still enforces CSRF on writes.
func (h *Handler) open(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Header.Get("X-Requested-With") != "df-admin" {
			h.fail(w, http.StatusForbidden, "forbidden", "Missing X-Requested-With header.", nil)
			return
		}
		next(w, r)
	}
}

func (h *Handler) audit(r *http.Request, u *db.User, action, resType string, resID any, details map[string]any) {
	rec := db.AuditLog{Action: action, ResourceType: resType, ResourceID: toStr(resID), Details: db.J(details), IP: h.ip(r), CreatedAt: db.Now()}
	if u != nil {
		id := u.ID
		rec.UserID, rec.UserEmail = &id, u.Email
	}
	h.S.DB.Create(&rec)
}

func toStr(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// ---------------------------------------------------------------- query helpers

type page struct {
	Page, PerPage int
	Sort          string
	Q             string
	TestMode      bool
	StoreID       int64
}

func parsePage(r *http.Request) page {
	q := r.URL.Query()
	p := page{Page: 1, PerPage: 25, Sort: q.Get("sort"), Q: strings.TrimSpace(q.Get("q"))}
	if n, err := strconv.Atoi(q.Get("page")); err == nil && n > 0 {
		p.Page = n
	}
	if n, err := strconv.Atoi(q.Get("per_page")); err == nil && n > 0 {
		p.PerPage = n
	}
	if p.PerPage > 100 {
		p.PerPage = 100
	}
	p.TestMode = q.Get("test_mode") == "true" || q.Get("test_mode") == "1"
	p.StoreID, _ = strconv.ParseInt(q.Get("store_id"), 10, 64)
	return p
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func qInt(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(r.URL.Query().Get(name), 10, 64)
	return n
}

// applySort orders q by the sort query using an allow-list (public name → column).
func applySort(q *gorm.DB, sort string, allowed map[string]string, def string) *gorm.DB {
	applied := false
	for _, part := range strings.Split(sort, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		dir := "ASC"
		if strings.HasPrefix(part, "-") {
			dir, part = "DESC", part[1:]
		}
		if col, ok := allowed[part]; ok {
			q = q.Order(col + " " + dir)
			applied = true
		}
	}
	if !applied {
		q = q.Order(def)
	}
	return q
}

type listMeta struct {
	Page     int   `json:"page"`
	PerPage  int   `json:"per_page"`
	Total    int64 `json:"total"`
	LastPage int   `json:"last_page"`
}

// paginate counts, pages and loads rows into dest, returning the meta block.
func paginate(q *gorm.DB, p page, dest any) (listMeta, error) {
	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return listMeta{}, err
	}
	if err := q.Offset((p.Page - 1) * p.PerPage).Limit(p.PerPage).Find(dest).Error; err != nil {
		return listMeta{}, err
	}
	last := int((total + int64(p.PerPage) - 1) / int64(p.PerPage))
	if last < 1 {
		last = 1
	}
	return listMeta{Page: p.Page, PerPage: p.PerPage, Total: total, LastPage: last}, nil
}

func (h *Handler) respondList(w http.ResponseWriter, data any, meta listMeta) {
	httpx.JSON(w, http.StatusOK, map[string]any{"data": data, "meta": meta})
}

// like builds a case-insensitive LIKE pattern.
func like(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), "%", `\%`), "_", `\_`)
	return "%" + strings.ToLower(s) + "%"
}

// ---------------------------------------------------------------- time/format helpers

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func tsp(t *time.Time) any {
	if t == nil {
		return nil
	}
	return ts(*t)
}

func nilStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func parseRFC3339(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	for _, l := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(l, s); err == nil {
			t = t.UTC()
			return &t, nil
		}
	}
	return nil, errors.New("invalid date-time")
}

// ---------------------------------------------------------------- routes

func (h *Handler) Routes(mux *http.ServeMux) {
	const p = "/admin-api"
	rd := func(pat string, f handler) { mux.HandleFunc(pat, h.guard(lvlRead, f)) }
	wr := func(pat string, f handler) { mux.HandleFunc(pat, h.guard(lvlWrite, f)) }
	ow := func(pat string, f handler) { mux.HandleFunc(pat, h.guard(lvlOwner, f)) }
	self := func(pat string, f handler) { mux.HandleFunc(pat, h.guard(lvlSelf, f)) }
	m := func(method, path string) string { return method + " " + p + path }

	// auth
	mux.HandleFunc(m("GET", "/auth/config"), h.authConfig)
	mux.HandleFunc(m("POST", "/auth/setup"), h.open(h.setup))
	mux.HandleFunc(m("POST", "/auth/login"), h.open(h.login))
	mux.HandleFunc(m("POST", "/auth/logout"), h.open(h.logout))
	mux.HandleFunc(m("GET", "/auth/stripe/start"), h.stripeStart)
	mux.HandleFunc(m("GET", "/auth/stripe/callback"), h.stripeCallback)
	rd(m("GET", "/me"), h.me)
	self(m("PATCH", "/me"), h.updateMe)
	self(m("POST", "/me/password"), h.changePassword)
	self(m("DELETE", "/me/stripe"), h.unlinkStripe)

	rd(m("GET", "/dashboard"), h.dashboard)

	// catalogue
	rd(m("GET", "/stores"), h.listStores)
	wr(m("POST", "/stores"), h.createStore)
	rd(m("GET", "/stores/{id}"), h.getStore)
	wr(m("PATCH", "/stores/{id}"), h.updateStore)
	rd(m("GET", "/products"), h.listProducts)
	wr(m("POST", "/products"), h.createProduct)
	rd(m("GET", "/products/{id}"), h.getProduct)
	wr(m("PATCH", "/products/{id}"), h.updateProduct)
	wr(m("DELETE", "/products/{id}"), h.deleteProduct)
	rd(m("GET", "/variants"), h.listVariants)
	wr(m("POST", "/variants"), h.createVariant)
	rd(m("GET", "/variants/{id}"), h.getVariant)
	wr(m("PATCH", "/variants/{id}"), h.updateVariant)
	wr(m("DELETE", "/variants/{id}"), h.deleteVariant)
	rd(m("GET", "/prices"), h.listPrices)
	wr(m("POST", "/prices"), h.createPrice)
	rd(m("GET", "/prices/{id}"), h.getPrice)
	wr(m("PATCH", "/prices/{id}"), h.updatePrice)
	wr(m("DELETE", "/prices/{id}"), h.deletePrice)
	rd(m("GET", "/files"), h.listFiles)
	rd(m("GET", "/files/{id}"), h.getFile)
	wr(m("PATCH", "/files/{id}"), h.updateFile)
	wr(m("DELETE", "/files/{id}"), h.deleteFile)
	wr(m("POST", "/variants/{id}/files"), h.uploadFile)
	rd(m("GET", "/files/{id}/download"), h.downloadFile)

	// commerce
	rd(m("GET", "/customers"), h.listCustomers)
	wr(m("POST", "/customers"), h.createCustomer)
	rd(m("GET", "/customers/{id}"), h.getCustomer)
	wr(m("PATCH", "/customers/{id}"), h.updateCustomer)
	wr(m("POST", "/customers/{id}/portal-link"), h.portalLink)
	rd(m("GET", "/orders"), h.listOrders)
	rd(m("GET", "/orders/export.csv"), h.exportOrders)
	rd(m("GET", "/orders/{id}"), h.getOrder)
	wr(m("POST", "/orders/{id}/refund"), h.refundOrder)
	wr(m("POST", "/orders/{id}/resend-receipt"), h.resendReceipt)
	rd(m("GET", "/subscriptions"), h.listSubscriptions)
	rd(m("GET", "/subscriptions/{id}"), h.getSubscription)
	wr(m("POST", "/subscriptions/{id}/cancel"), h.cancelSubscription)
	wr(m("POST", "/subscriptions/{id}/resume"), h.resumeSubscription)
	wr(m("POST", "/subscriptions/{id}/sync"), h.syncSubscription)
	wr(m("POST", "/subscriptions/{id}/link-stripe"), h.linkSubscription)
	rd(m("GET", "/subscription-invoices"), h.listInvoices)
	rd(m("GET", "/license-keys"), h.listKeys)
	wr(m("POST", "/license-keys"), h.createKey)
	rd(m("GET", "/license-keys/{id}"), h.getKey)
	wr(m("PATCH", "/license-keys/{id}"), h.updateKey)
	wr(m("POST", "/license-keys/{id}/regenerate"), h.regenerateKey)
	rd(m("GET", "/license-key-instances"), h.listInstances)
	wr(m("DELETE", "/license-key-instances/{id}"), h.deleteInstance)

	// integrations
	rd(m("GET", "/webhook-events"), h.webhookEvents)
	rd(m("GET", "/webhooks"), h.listWebhooks)
	wr(m("POST", "/webhooks"), h.createWebhook)
	rd(m("GET", "/webhooks/{id}"), h.getWebhook)
	wr(m("PATCH", "/webhooks/{id}"), h.updateWebhook)
	wr(m("DELETE", "/webhooks/{id}"), h.deleteWebhook)
	wr(m("POST", "/webhooks/{id}/test"), h.testWebhook)
	rd(m("GET", "/webhook-deliveries"), h.listDeliveries)
	rd(m("GET", "/webhook-deliveries/{id}"), h.getDelivery)
	wr(m("POST", "/webhook-deliveries/{id}/redeliver"), h.redeliver)
	rd(m("GET", "/stripe-events"), h.listStripeEvents)
	rd(m("GET", "/stripe-events/{id}"), h.getStripeEvent)
	wr(m("POST", "/stripe-events/{id}/replay"), h.replayStripeEvent)
	wr(m("GET", "/api-keys"), h.listAPIKeys)
	ow(m("POST", "/api-keys"), h.createAPIKey)
	ow(m("DELETE", "/api-keys/{id}"), h.revokeAPIKey)
	ow(m("GET", "/users"), h.listUsers)
	ow(m("POST", "/users"), h.createUser)
	ow(m("PATCH", "/users/{id}"), h.updateUser)
	ow(m("DELETE", "/users/{id}"), h.deleteUser)
	rd(m("GET", "/audit-logs"), h.listAudit)
	rd(m("GET", "/email-logs"), h.listEmailLogs)
	wr(m("POST", "/email-logs/test"), h.testEmail)

	// stripe + settings
	rd(m("GET", "/stripe/status"), h.stripeStatus)
	wr(m("POST", "/stripe/webhook-endpoint"), h.stripeWebhookEndpoint)
	rd(m("GET", "/stripe/catalog"), h.stripeCatalog)
	wr(m("POST", "/stripe/catalog/import"), h.stripeCatalogImport)
	wr(m("POST", "/variants/{id}/link-stripe"), h.linkVariant)
	wr(m("DELETE", "/variants/{id}/link-stripe"), h.unlinkVariant)
	wr(m("GET", "/settings"), h.getSettings)
	ow(m("PUT", "/settings"), h.putSettings)
	wr(m("POST", "/settings/smtp/test"), h.testEmail)

	// imports
	rd(m("GET", "/imports"), h.listImports)
	rd(m("GET", "/imports/mapping-report.csv"), h.mappingReport)
	rd(m("GET", "/imports/{id}"), h.getImport)
	rd(m("GET", "/imports/{id}/logs"), h.importLogs)
	wr(m("POST", "/imports/{id}/cancel"), h.cancelImport)
	ow(m("POST", "/imports/lemonsqueezy"), h.importLS)
	ow(m("POST", "/imports/lemonsqueezy/archive"), h.importLSArchive)
	wr(m("POST", "/imports/stripe-mapping"), h.importMapping)
	wr(m("POST", "/imports/stripe-auto-match"), h.importAutoMatch)

	// unknown admin routes → JSON 404 instead of an HTML page
	mux.HandleFunc(p+"/", func(w http.ResponseWriter, r *http.Request) {
		h.fail(w, http.StatusNotFound, "not_found", "Unknown endpoint.", nil)
	})
}
