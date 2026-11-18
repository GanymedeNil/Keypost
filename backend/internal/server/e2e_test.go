package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"fulfillment/internal/auth"
	"fulfillment/internal/config"
	"fulfillment/internal/crypt"
	"fulfillment/internal/db"
	"fulfillment/internal/server"
	"fulfillment/internal/service"
	"fulfillment/internal/storage"
	"fulfillment/internal/stripe"
)

const webhookSecret = "whsec_test_secret"

// ---------------------------------------------------------------- fake Stripe

type fakeStripe struct {
	mu      sync.Mutex
	objects map[string]string // "GET /v1/..." → JSON
	posts   []url.Values
	srv     *httptest.Server
}

func newFakeStripe(t *testing.T) *fakeStripe {
	f := &fakeStripe{objects: map[string]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Method == "POST" {
			_ = r.ParseForm()
			f.posts = append(f.posts, r.PostForm)
		}
		key := r.Method + " " + r.URL.Path
		if body, ok := f.objects[key]; ok {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, body)
			return
		}
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"error":{"type":"invalid_request_error","message":"No such object: `+key+`"}}`)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeStripe) set(method, path, body string) {
	f.mu.Lock()
	f.objects[method+" "+path] = body
	f.mu.Unlock()
}

func (f *fakeStripe) lastPost() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.posts) == 0 {
		return nil
	}
	return f.posts[len(f.posts)-1]
}

// ---------------------------------------------------------------- merchant webhook receiver

type received struct {
	Event string
	Sig   string
	Body  []byte
}

type receiver struct {
	mu  sync.Mutex
	got []received
	srv *httptest.Server
}

func newReceiver(t *testing.T) *receiver {
	r := &receiver{}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.got = append(r.got, received{Event: req.Header.Get("X-Event-Name"), Sig: req.Header.Get("X-Signature"), Body: b})
		r.mu.Unlock()
		w.WriteHeader(200)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *receiver) events() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, g := range r.got {
		out = append(out, g.Event)
	}
	return out
}

func (r *receiver) waitFor(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		c := len(r.got)
		r.mu.Unlock()
		if c >= n {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("expected %d webhook deliveries, got %v", n, r.events())
}

// ---------------------------------------------------------------- harness

type env struct {
	t      *testing.T
	app    *server.App
	stripe *fakeStripe
	hook   *receiver
	apiKey string
	sent   []string
	mu     sync.Mutex
}

func newEnv(t *testing.T) *env {
	t.Helper()
	fs := newFakeStripe(t)
	e := &env{t: t, stripe: fs, hook: newReceiver(t)}
	dbURL := "sqlite::memory:"
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" { // run the same suite against PostgreSQL
		dbURL = u
		raw, err := db.Open(u, false)
		if err != nil {
			t.Fatal(err)
		}
		raw.Exec("DROP SCHEMA public CASCADE")
		raw.Exec("CREATE SCHEMA public")
		if sqlDB, err := raw.DB(); err == nil {
			sqlDB.Close()
		}
	}
	cfg := &config.Config{
		Env: "development", ListenAddr: ":0", PublicURL: "http://pay.test", AppSecret: "test-secret-0123456789abcdef",
		DatabaseURL: dbURL, DataDir: t.TempDir(), StorageDriver: "local",
		Stripe:           config.StripeConfig{SecretKey: "sk_test_x", WebhookSecret: webhookSecret, APIVersion: "2025-09-30.clover", APIBase: fs.srv.URL, LoginMode: "allowlist", LoginAutoRole: "viewer", OAuthClientID: "ca_test_123", OAuthAuthBase: fs.srv.URL, OAuthRevokeAfter: true},
		SMTP:             config.SMTPConfig{Host: "smtp.test", From: "Shop <shop@example.com>", Port: 587, Security: "none"},
		LicenseRateLimit: 1000, APIRateLimit: 1000, DownloadLimitDay: 3, DownloadURLTTL: time.Hour, SessionTTL: time.Hour,
		DisableKeysRefund: true, WebhookMaxAttempts: 2, AdminEmail: "owner@example.com", AdminPassword: "correct horse battery", AdminName: "Owner",
	}
	st, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	app, err := server.New(cfg, log, st)
	if err != nil {
		t.Fatal(err)
	}
	app.Svc.Mail.Transport = func(from string, to []string, raw []byte) error {
		e.mu.Lock()
		e.sent = append(e.sent, string(raw))
		e.mu.Unlock()
		return nil
	}
	e.app = app
	if sqlDB, err := app.DB.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	ctx, cancel := context.WithCancel(context.Background())
	go app.Dispatcher.Run(ctx)
	t.Cleanup(func() { cancel(); service.WaitBackground() })

	var owner db.User
	app.DB.First(&owner)
	_, tok, err := auth.NewAPIKey(app.DB, owner.ID, "test", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	e.apiKey = tok
	return e
}

func (e *env) do(method, path string, hdr map[string]string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	e.app.Handler.ServeHTTP(rr, req)
	return rr
}

func (e *env) api(method, path string, body any) *httptest.ResponseRecorder {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	return e.do(method, path, map[string]string{
		"Authorization": "Bearer " + e.apiKey, "Accept": "application/vnd.api+json", "Content-Type": "application/vnd.api+json",
	}, rd)
}

func (e *env) form(path string, kv map[string]string) (*httptest.ResponseRecorder, map[string]any) {
	v := url.Values{}
	for k, x := range kv {
		v.Set(k, x)
	}
	rr := e.do("POST", path, map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"}, strings.NewReader(v.Encode()))
	var out map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	return rr, out
}

func (e *env) stripeEvent(typ string, object map[string]any) *httptest.ResponseRecorder {
	ev := map[string]any{"id": "evt_" + crypt.RandomHex(8), "object": "event", "type": typ, "livemode": false, "created": time.Now().Unix(), "data": map[string]any{"object": object}}
	return e.postEvent(ev)
}

func (e *env) postEvent(ev map[string]any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(ev)
	return e.do("POST", "/webhooks/stripe", map[string]string{"Stripe-Signature": stripe.SignPayload(b, webhookSecret, time.Now())}, bytes.NewReader(b))
}

func decode(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatalf("bad json (%d): %s", rr.Code, rr.Body.String())
	}
	return m
}

func sub(m map[string]any, path ...string) any {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[p]
	}
	return cur
}

// seedCatalog creates a store product with two variants: a perpetual licence (limit 2) and a subscription.
func (e *env) seedCatalog() (oneTime, subscription db.Variant) {
	g := e.app.DB
	var st db.Store
	g.First(&st)
	now := db.Now()
	p := db.Product{StoreID: st.ID, Name: "Pro App", Slug: "pro-app", Status: "published", TestMode: true, CreatedAt: now, UpdatedAt: now}
	g.Create(&p)
	oneTime = db.Variant{ProductID: p.ID, Name: "Lifetime", Slug: "var-lifetime", Status: "published", HasLicenseKeys: true, LicenseActivationLimit: 2, IsLicenseLengthUnlimited: true, TestMode: true, Links: db.J([]db.VariantLink{{Title: "Docs", URL: "https://docs.example.com/?k=[license_key]"}}), CreatedAt: now, UpdatedAt: now}
	g.Create(&oneTime)
	g.Create(&db.Price{VariantID: oneTime.ID, Category: "one_time", Scheme: "standard", UnitPrice: 999, PackageSize: 1, TaxCode: "eservice", Currency: "USD", StripePriceID: "price_once", CreatedAt: now, UpdatedAt: now})
	subscription = db.Variant{ProductID: p.ID, Name: "Monthly", Slug: "var-monthly", Status: "published", HasLicenseKeys: true, IsLicenseLimitUnlimited: true, IsLicenseLengthUnlimited: true, TestMode: true, CreatedAt: now, UpdatedAt: now}
	g.Create(&subscription)
	unit, qty := "month", 1
	g.Create(&db.Price{VariantID: subscription.ID, Category: "subscription", Scheme: "standard", UnitPrice: 500, PackageSize: 1, TaxCode: "saas", Currency: "USD", RenewalIntervalUnit: &unit, RenewalIntervalQuantity: &qty, StripePriceID: "price_monthly", CreatedAt: now, UpdatedAt: now})
	g.Create(&db.Webhook{StoreID: st.ID, URL: e.hook.srv.URL, Events: db.J(service.WebhookEvents), Secret: "hooksecret", TestMode: true, CreatedAt: now, UpdatedAt: now})
	return
}

func sessionJSON(id, mode, priceID, email string, total, tax int64, extra string) string {
	return fmt.Sprintf(`{"id":%q,"mode":%q,"payment_status":"paid","status":"complete","currency":"usd","amount_subtotal":%d,"amount_total":%d,
"total_details":{"amount_discount":0,"amount_tax":%d},"customer":"cus_1","customer_details":{"email":%q,"name":"Buyer One","address":{"country":"US","postal_code":"94107","city":"SF"}},
"client_reference_id":"","metadata":{},"livemode":false,"created":%d,%s
"line_items":{"data":[{"id":"li_1","description":"Pro App","quantity":1,"amount_subtotal":%d,"amount_total":%d,"currency":"usd","price":{"id":%q,"product":{"id":"prod_1","name":"Pro App","metadata":{}}}}]}}`,
		id, mode, total-tax, total, tax, email, time.Now().Unix(), extra, total-tax, total, priceID)
}

// ---------------------------------------------------------------- tests

func TestLicenseAPIAndOneTimeFulfilment(t *testing.T) {
	e := newEnv(t)
	once, _ := e.seedCatalog()

	e.stripe.set("GET", "/v1/checkout/sessions/cs_test_1", sessionJSON("cs_test_1", "payment", "price_once", "buyer@example.com", 1199, 200,
		`"payment_intent":{"id":"pi_1","latest_charge":{"id":"ch_1","receipt_url":"https://pay.stripe.com/r/1","payment_method_details":{"type":"card","card":{"brand":"visa","last4":"4242"}}}},`))

	rr := e.stripeEvent("checkout.session.completed", map[string]any{"id": "cs_test_1"})
	if rr.Code != 200 {
		t.Fatalf("webhook status %d: %s", rr.Code, rr.Body.String())
	}
	// duplicate delivery of the same session via another event id must not create a second order
	if rr := e.stripeEvent("checkout.session.completed", map[string]any{"id": "cs_test_1"}); rr.Code != 200 {
		t.Fatalf("dup status %d", rr.Code)
	}
	var orders []db.Order
	e.app.DB.Find(&orders)
	if len(orders) != 1 {
		t.Fatalf("want 1 order, got %d", len(orders))
	}
	o := orders[0]
	if o.Total != 1199 || o.Tax != 200 || o.Subtotal != 999 || o.Status != "paid" || o.CardLastFour != "4242" || o.OrderNumber != 1 {
		t.Fatalf("order fields wrong: %+v", o)
	}
	var keys []db.LicenseKey
	e.app.DB.Find(&keys)
	if len(keys) != 1 || keys[0].VariantID != once.ID || keys[0].ActivationLimit == nil || *keys[0].ActivationLimit != 2 {
		t.Fatalf("license key wrong: %+v", keys)
	}
	key := keys[0].Key

	// outgoing webhooks: LS event order + signature
	e.hook.waitFor(t, 2)
	if got := e.hook.events(); got[0] != "order_created" || got[1] != "license_key_created" {
		t.Fatalf("event order = %v", got)
	}
	g0 := e.hook.got[0]
	if g0.Sig != crypt.HMACHex("hooksecret", g0.Body) {
		t.Fatalf("bad X-Signature")
	}
	var payload map[string]any
	_ = json.Unmarshal(g0.Body, &payload)
	if sub(payload, "meta", "event_name") != "order_created" || sub(payload, "data", "type") != "orders" || sub(payload, "data", "attributes", "total_formatted") != "$11.99" {
		t.Fatalf("payload shape: %s", g0.Body)
	}

	// License API: activate x2, third hits the limit
	rr1, a1 := e.form("/v1/licenses/activate", map[string]string{"license_key": key, "instance_name": "laptop"})
	if rr1.Code != 200 || a1["activated"] != true || sub(a1, "license_key", "status") != "active" || sub(a1, "instance", "name") != "laptop" {
		t.Fatalf("activate: %d %s", rr1.Code, rr1.Body.String())
	}
	inst1 := sub(a1, "instance", "id").(string)
	if sub(a1, "meta", "customer_email") != "buyer@example.com" || sub(a1, "meta", "product_name") != "Pro App" || sub(a1, "meta", "variant_name") != "Lifetime" {
		t.Fatalf("meta: %s", rr1.Body.String())
	}
	if rr, _ := e.form("/v1/licenses/activate", map[string]string{"license_key": key, "instance_name": "desktop"}); rr.Code != 200 {
		t.Fatalf("2nd activate %d", rr.Code)
	}
	rr3, a3 := e.form("/v1/licenses/activate", map[string]string{"license_key": key, "instance_name": "phone"})
	if rr3.Code != 400 || a3["activated"] != false || a3["error"] != "This license key has reached the activation limit." {
		t.Fatalf("limit: %d %s", rr3.Code, rr3.Body.String())
	}
	if _, has := a3["instance"]; has || sub(a3, "license_key", "activation_usage") != float64(2) {
		t.Fatalf("limit response shape: %s", rr3.Body.String())
	}

	// validate with and without instance
	_, v1 := e.form("/v1/licenses/validate", map[string]string{"license_key": key, "instance_id": inst1})
	if v1["valid"] != true || sub(v1, "instance", "id") != inst1 {
		t.Fatalf("validate: %v", v1)
	}
	_, v2 := e.form("/v1/licenses/validate", map[string]string{"license_key": key})
	if v2["valid"] != true || v2["instance"] != nil {
		t.Fatalf("validate no instance: %v", v2)
	}
	if rr, v := e.form("/v1/licenses/validate", map[string]string{"license_key": "nope"}); rr.Code != 404 || v["valid"] != false || v["error"] != "license_key not found." {
		t.Fatalf("unknown key: %d %v", rr.Code, v)
	}
	if rr, _ := e.form("/v1/licenses/validate", map[string]string{}); rr.Code != 422 {
		t.Fatalf("missing key should be 422, got %d", rr.Code)
	}

	// deactivate frees a slot
	_, d1 := e.form("/v1/licenses/deactivate", map[string]string{"license_key": key, "instance_id": inst1})
	if d1["deactivated"] != true || sub(d1, "license_key", "activation_usage") != float64(1) {
		t.Fatalf("deactivate: %v", d1)
	}
	if rr, _ := e.form("/v1/licenses/activate", map[string]string{"license_key": key, "instance_name": "phone"}); rr.Code != 200 {
		t.Fatalf("activate after deactivate: %d", rr.Code)
	}

	// JSON:API: list, filter, include, pagination, errors
	rl := e.api("GET", "/v1/license-keys?filter[status]=active&page[size]=1&include=license-key-instances", nil)
	l := decode(t, rl)
	if rl.Code != 200 || sub(l, "meta", "page", "total") != float64(1) || sub(l, "jsonapi", "version") != "1.0" {
		t.Fatalf("list keys: %d %s", rl.Code, rl.Body.String())
	}
	if inc, _ := l["included"].([]any); len(inc) != 2 {
		t.Fatalf("included instances = %d, want 2", len(inc))
	}
	if !strings.Contains(rl.Header().Get("Content-Type"), "application/vnd.api+json") || rl.Header().Get("X-Ratelimit-Limit") == "" {
		t.Fatalf("headers: %v", rl.Header())
	}
	ro := e.api("GET", fmt.Sprintf("/v1/orders/%d?include=order-items,license-keys", o.ID), nil)
	od := decode(t, ro)
	if sub(od, "data", "attributes", "first_order_item", "variant_name") != "Lifetime" || sub(od, "data", "attributes", "urls", "receipt") == nil || len(od["included"].([]any)) != 2 {
		t.Fatalf("order doc: %s", ro.Body.String())
	}
	if rr := e.api("GET", fmt.Sprintf("/v1/orders/%d/customer", o.ID), nil); rr.Code != 200 || sub(decode(t, rr), "data", "attributes", "email") != "buyer@example.com" {
		t.Fatalf("nested customer: %d %s", rr.Code, rr.Body.String())
	}
	if rr := e.api("GET", "/v1/license-keys/9999", nil); rr.Code != 404 || sub(decode(t, rr), "errors") == nil {
		t.Fatalf("404 shape: %s", rr.Body.String())
	}
	unauth := e.do("GET", "/v1/orders", map[string]string{"Accept": "application/vnd.api+json"}, nil)
	if unauth.Code != 401 || sub(decode(t, unauth), "errors") == nil {
		t.Fatalf("unauth: %d %s", unauth.Code, unauth.Body.String())
	}

	// PATCH license key via API (disable) → validate says disabled
	rp := e.api("PATCH", fmt.Sprintf("/v1/license-keys/%d", keys[0].ID), map[string]any{"data": map[string]any{"type": "license-keys", "id": fmt.Sprint(keys[0].ID), "attributes": map[string]any{"disabled": true, "activation_limit": nil}}})
	if rp.Code != 200 || sub(decode(t, rp), "data", "attributes", "status") != "disabled" {
		t.Fatalf("patch key: %d %s", rp.Code, rp.Body.String())
	}
	if _, v := e.form("/v1/licenses/validate", map[string]string{"license_key": key}); v["valid"] != false || v["error"] != "This license key is disabled." {
		t.Fatalf("disabled validate: %v", v)
	}

	// refund through Stripe → full refund
	e.stripe.set("POST", "/v1/refunds", `{"id":"re_1","status":"succeeded","amount":1199,"currency":"usd"}`)
	e.stripe.set("GET", "/v1/charges/ch_1", `{"id":"ch_1","payment_intent":"pi_1","amount":1199,"amount_refunded":1199,"refunded":true,"currency":"usd"}`)
	rf := e.api("POST", fmt.Sprintf("/v1/orders/%d/refund", o.ID), map[string]any{"data": map[string]any{"type": "orders", "id": fmt.Sprint(o.ID)}})
	if rf.Code != 200 || sub(decode(t, rf), "data", "attributes", "status") != "refunded" {
		t.Fatalf("refund: %d %s", rf.Code, rf.Body.String())
	}
	if lp := e.stripe.lastPost(); lp.Get("payment_intent") != "pi_1" || lp.Get("amount") != "1199" {
		t.Fatalf("stripe refund params: %v", lp)
	}
	// the later charge.refunded webhook is a no-op (state already applied) and must not re-emit order_refunded
	before := len(e.hook.events())
	e.stripeEvent("charge.refunded", map[string]any{"id": "ch_1"})
	time.Sleep(150 * time.Millisecond)
	count := 0
	for _, ev := range e.hook.events() {
		if ev == "order_refunded" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("order_refunded emitted %d times (events before=%d: %v)", count, before, e.hook.events())
	}
}

func TestSubscriptionLifecycle(t *testing.T) {
	e := newEnv(t)
	_, monthly := e.seedCatalog()
	end := time.Now().Add(30 * 24 * time.Hour).Unix()

	subJSON := func(status string, extra string) string {
		return fmt.Sprintf(`{"id":"sub_1","status":%q,"customer":"cus_1","cancel_at_period_end":false,"billing_cycle_anchor":%d,"livemode":false,"created":%d,
"items":{"data":[{"id":"si_1","quantity":1,"current_period_end":%d,"price":{"id":"price_monthly"}}]},
"default_payment_method":{"id":"pm_1","type":"card","card":{"brand":"mastercard","last4":"4444"}}%s}`, status, time.Now().Unix(), time.Now().Unix(), end, extra)
	}
	invJSON := func(id, reason, status string) string {
		return fmt.Sprintf(`{"id":%q,"status":%q,"billing_reason":%q,"currency":"usd","subtotal":500,"total":500,"amount_paid":500,"customer":"cus_1","created":%d,
"parent":{"subscription_details":{"subscription":"sub_1"}},"payments":{"data":[{"payment":{"payment_intent":"pi_sub","charge":"ch_sub"}}]},"hosted_invoice_url":"https://invoice.stripe.com/i/%s"}`, id, status, reason, time.Now().Unix(), id)
	}
	e.stripe.set("GET", "/v1/checkout/sessions/cs_sub_1", sessionJSON("cs_sub_1", "subscription", "price_monthly", "sub@example.com", 500, 0,
		`"subscription":{"id":"sub_1"},"invoice":"in_1",`))
	e.stripe.set("GET", "/v1/subscriptions/sub_1", subJSON("active", ""))
	e.stripe.set("GET", "/v1/invoices/in_1", invJSON("in_1", "subscription_create", "paid"))
	e.stripe.set("GET", "/v1/charges/ch_sub", `{"id":"ch_sub","payment_intent":"pi_sub","amount":500,"amount_refunded":0,"currency":"usd","payment_method_details":{"type":"card","card":{"brand":"mastercard","last4":"4444"}}}`)

	if rr := e.stripeEvent("checkout.session.completed", map[string]any{"id": "cs_sub_1"}); rr.Code != 200 {
		t.Fatalf("fulfil: %d %s", rr.Code, rr.Body.String())
	}
	e.hook.waitFor(t, 4)
	want := []string{"order_created", "subscription_created", "subscription_payment_success", "license_key_created"}
	for i, w := range want {
		if e.hook.events()[i] != w {
			t.Fatalf("event order = %v, want prefix %v", e.hook.events(), want)
		}
	}
	var sb db.Subscription
	e.app.DB.First(&sb)
	if sb.Status != "active" || sb.VariantID != monthly.ID || sb.CardLastFour != "4444" || sb.RenewsAt == nil || derefS(sb.StripeSubscriptionID) != "sub_1" {
		t.Fatalf("subscription: %+v", sb)
	}
	var key db.LicenseKey
	e.app.DB.First(&key)
	if key.SubscriptionID == nil || key.ExpiresAt != nil || key.ActivationLimit != nil {
		t.Fatalf("subscription key should be perpetual & unlimited while active: %+v", key)
	}

	// renewal
	e.stripe.set("GET", "/v1/invoices/in_2", invJSON("in_2", "subscription_cycle", "paid"))
	if rr := e.stripeEvent("invoice.paid", map[string]any{"id": "in_2"}); rr.Code != 200 {
		t.Fatalf("renewal: %d", rr.Code)
	}
	var invs []db.SubscriptionInvoice
	e.app.DB.Order("id").Find(&invs)
	if len(invs) != 2 || invs[1].BillingReason != "renewal" || invs[1].Total != 500 {
		t.Fatalf("invoices: %+v", invs)
	}
	// invoice.paid for the initial invoice again is idempotent
	e.stripeEvent("invoice.paid", map[string]any{"id": "in_1"})
	var n int64
	e.app.DB.Model(&db.SubscriptionInvoice{}).Count(&n)
	if n != 2 {
		t.Fatalf("invoice dedupe failed: %d", n)
	}

	// payment failure → past_due
	e.stripe.set("GET", "/v1/invoices/in_3", invJSON("in_3", "subscription_cycle", "open"))
	e.stripe.set("GET", "/v1/subscriptions/sub_1", subJSON("past_due", ""))
	e.stripeEvent("invoice.payment_failed", map[string]any{"id": "in_3"})
	e.app.DB.First(&sb)
	if sb.Status != "past_due" {
		t.Fatalf("want past_due, got %s", sb.Status)
	}

	// customer cancels at period end → cancelled with grace period, key expires at ends_at
	e.stripe.set("GET", "/v1/subscriptions/sub_1", subJSON("active", fmt.Sprintf(`,"cancel_at_period_end":true,"cancel_at":%d`, end)))
	e.stripeEvent("customer.subscription.updated", map[string]any{"id": "sub_1"})
	e.app.DB.First(&sb)
	e.app.DB.First(&key)
	if sb.Status != "cancelled" || !sb.Cancelled || sb.EndsAt == nil || key.ExpiresAt == nil {
		t.Fatalf("cancelled state: sub=%+v key.expires=%v", sb, key.ExpiresAt)
	}

	// subscription deleted → expired, licence expired immediately → License API refuses
	e.stripe.set("GET", "/v1/subscriptions/sub_1", fmt.Sprintf(`{"id":"sub_1","status":"canceled","customer":"cus_1","ended_at":%d,"items":{"data":[]}}`, time.Now().Add(-time.Minute).Unix()))
	e.stripeEvent("customer.subscription.deleted", map[string]any{"id": "sub_1"})
	e.app.DB.First(&sb)
	if sb.Status != "expired" {
		t.Fatalf("want expired, got %s", sb.Status)
	}
	if _, v := e.form("/v1/licenses/validate", map[string]string{"license_key": key.Key}); v["valid"] != false || sub(v, "license_key", "status") != "expired" {
		t.Fatalf("expired validate: %v", v)
	}
	if rr, a := e.form("/v1/licenses/activate", map[string]string{"license_key": key.Key, "instance_name": "desktop"}); rr.Code != 400 || a["activated"] != false {
		t.Fatalf("expired activate: %d %v", rr.Code, a)
	}
	evs := strings.Join(e.hook.events(), ",")
	for _, w := range []string{"subscription_payment_failed", "subscription_cancelled", "subscription_expired", "subscription_updated"} {
		if !strings.Contains(evs, w) {
			// deliveries are asynchronous: wait a little and re-check once
			time.Sleep(300 * time.Millisecond)
			evs = strings.Join(e.hook.events(), ",")
			if !strings.Contains(evs, w) {
				t.Fatalf("missing %s in %s", w, evs)
			}
		}
	}
}

func derefS(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func TestStripeWebhookSecurityAndUnknownEvents(t *testing.T) {
	e := newEnv(t)
	body := []byte(`{"id":"evt_x","type":"checkout.session.completed","data":{"object":{"id":"cs_x"}}}`)
	if rr := e.do("POST", "/webhooks/stripe", map[string]string{"Stripe-Signature": "t=1,v1=bad"}, bytes.NewReader(body)); rr.Code != 400 {
		t.Fatalf("bad signature should be 400, got %d", rr.Code)
	}
	old := stripe.SignPayload(body, webhookSecret, time.Now().Add(-time.Hour))
	if rr := e.do("POST", "/webhooks/stripe", map[string]string{"Stripe-Signature": old}, bytes.NewReader(body)); rr.Code != 400 {
		t.Fatalf("stale timestamp should be 400, got %d", rr.Code)
	}
	if rr := e.stripeEvent("product.created", map[string]any{"id": "prod_1"}); rr.Code != 200 {
		t.Fatalf("unhandled events are acknowledged: %d", rr.Code)
	}
	// a failing handler (unknown session) returns 500 so Stripe retries, and the event is recorded as failed
	rr := e.stripeEvent("checkout.session.completed", map[string]any{"id": "cs_missing"})
	if rr.Code != 500 {
		t.Fatalf("failed fulfilment should be 500, got %d", rr.Code)
	}
	var failed int64
	e.app.DB.Model(&db.StripeEvent{}).Where("status = ?", "failed").Count(&failed)
	if failed != 1 {
		t.Fatalf("failed events = %d", failed)
	}
}

func TestAdminAPI(t *testing.T) {
	e := newEnv(t)
	e.seedCatalog()
	hdr := map[string]string{"Content-Type": "application/json", "X-Requested-With": "df-admin"}

	if rr := e.do("GET", "/admin-api/me", nil, nil); rr.Code != 401 {
		t.Fatalf("me without session: %d", rr.Code)
	}
	cfg := decode(t, e.do("GET", "/admin-api/auth/config", nil, nil))
	if cfg["password_login"] != true || cfg["setup_required"] != false {
		t.Fatalf("config: %v", cfg)
	}
	// CSRF header is required on writes
	bad := e.do("POST", "/admin-api/auth/login", map[string]string{"Content-Type": "application/json"}, strings.NewReader(`{"email":"owner@example.com","password":"x"}`))
	if bad.Code != 403 {
		t.Fatalf("login without csrf header: %d", bad.Code)
	}
	if rr := e.do("POST", "/admin-api/auth/login", hdr, strings.NewReader(`{"email":"owner@example.com","password":"wrong"}`)); rr.Code != 401 {
		t.Fatalf("wrong password: %d", rr.Code)
	}
	rr := e.do("POST", "/admin-api/auth/login", hdr, strings.NewReader(`{"email":"owner@example.com","password":"correct horse battery"}`))
	if rr.Code != 200 {
		t.Fatalf("login: %d %s", rr.Code, rr.Body.String())
	}
	cookie := rr.Result().Cookies()[0].String()
	authed := func(method, path string, body any) *httptest.ResponseRecorder {
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		return e.do(method, path, map[string]string{"Cookie": cookie, "Content-Type": "application/json", "X-Requested-With": "df-admin"}, rd)
	}
	if me := decode(t, authed("GET", "/admin-api/me", nil)); me["email"] != "owner@example.com" || me["role"] != "owner" {
		t.Fatalf("me: %v", me)
	}

	// catalogue CRUD
	pr := authed("POST", "/admin-api/products", map[string]any{"name": "New Product", "status": "published", "test_mode": true})
	if pr.Code != 201 {
		t.Fatalf("create product: %d %s", pr.Code, pr.Body.String())
	}
	pid := int64(decode(t, pr)["id"].(float64))
	vr := authed("POST", "/admin-api/variants", map[string]any{"product_id": pid, "name": "Std", "has_license_keys": true, "license_activation_limit": 3,
		"price": map[string]any{"category": "one_time", "unit_price": 1500, "currency": "USD"}, "links": []any{map[string]any{"title": "Docs", "url": "https://x.test"}}})
	if vr.Code != 201 {
		t.Fatalf("create variant: %d %s", vr.Code, vr.Body.String())
	}
	v := decode(t, vr)
	vid := int64(v["id"].(float64))
	if len(v["prices"].([]any)) != 1 || v["buy_now_url"] == "" {
		t.Fatalf("variant dto: %v", v)
	}
	if rr := authed("POST", "/admin-api/variants", map[string]any{"product_id": pid, "name": "Bad", "status": "nope"}); rr.Code != 422 {
		t.Fatalf("variant validation: %d", rr.Code)
	}

	// file upload → listed → admin download
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "app v1.zip")
	_, _ = fw.Write([]byte("PK-binary-content"))
	_ = mw.WriteField("version", "1.0.0")
	_ = mw.Close()
	up := e.do("POST", fmt.Sprintf("/admin-api/variants/%d/files", vid), map[string]string{"Cookie": cookie, "Content-Type": mw.FormDataContentType(), "X-Requested-With": "df-admin"}, &buf)
	if up.Code != 201 {
		t.Fatalf("upload: %d %s", up.Code, up.Body.String())
	}
	f := decode(t, up)
	if f["size"] != float64(17) || f["version"] != "1.0.0" || f["extension"] != "zip" || f["status"] != "published" {
		t.Fatalf("file dto: %v", f)
	}
	dl := authed("GET", fmt.Sprintf("/admin-api/files/%d/download", int64(f["id"].(float64))), nil)
	if dl.Code != 200 || dl.Body.String() != "PK-binary-content" || !strings.Contains(dl.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("admin download: %d %q", dl.Code, dl.Body.String())
	}

	// manual licence key → shows in list + dashboard
	kr := authed("POST", "/admin-api/license-keys", map[string]any{"variant_id": vid, "user_email": "manual@example.com", "user_name": "Manual"})
	if kr.Code != 201 {
		t.Fatalf("create key: %d %s", kr.Code, kr.Body.String())
	}
	if list := decode(t, authed("GET", "/admin-api/license-keys?test_mode=true&q=manual", nil)); sub(list, "meta", "total") != float64(1) {
		t.Fatalf("key list: %v", list)
	}
	if db := decode(t, authed("GET", "/admin-api/dashboard?test_mode=true&days=7", nil)); len(db["series"].([]any)) != 7 || sub(db, "totals", "license_keys") != float64(1) {
		t.Fatalf("dashboard: %v", db)
	}

	// settings never leak secrets; owner can update
	st := decode(t, authed("GET", "/admin-api/settings", nil))
	if sub(st, "stripe", "secret_key", "set") != true || strings.Contains(fmt.Sprint(st), "sk_test_x") {
		t.Fatalf("settings leak: %v", st["stripe"])
	}
	if rr := authed("PUT", "/admin-api/settings", map[string]any{"smtp": map[string]any{"host": "smtp.example.org", "port": 2525}, "license": map[string]any{"disable_on_refund": false}}); rr.Code != 200 {
		t.Fatalf("put settings: %d %s", rr.Code, rr.Body.String())
	}
	if e.app.Svc.Settings.Get("smtp.host") != "smtp.example.org" {
		t.Fatalf("setting not stored")
	}

	// API key creation returns the token once and it works against /v1
	ak := decode(t, authed("POST", "/admin-api/api-keys", map[string]any{"name": "ci", "test_mode": true}))
	tok, _ := ak["token"].(string)
	if !strings.HasPrefix(tok, "dfk_test_") {
		t.Fatalf("api key: %v", ak)
	}
	req := httptest.NewRequest("GET", "/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	e.app.Handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("new api key: %d", rec.Code)
	}

	// viewer role is read-only
	hash, _ := auth.HashPassword("viewer-password-1")
	e.app.DB.Create(&db.User{Email: "viewer@example.com", Role: "viewer", PasswordHash: hash, CreatedAt: db.Now(), UpdatedAt: db.Now()})
	vl := e.do("POST", "/admin-api/auth/login", hdr, strings.NewReader(`{"email":"viewer@example.com","password":"viewer-password-1"}`))
	vc := vl.Result().Cookies()[0].String()
	if rr := e.do("POST", "/admin-api/products", map[string]string{"Cookie": vc, "Content-Type": "application/json", "X-Requested-With": "df-admin"}, strings.NewReader(`{"name":"x"}`)); rr.Code != 403 {
		t.Fatalf("viewer write: %d", rr.Code)
	}
	if rr := e.do("GET", "/admin-api/products", map[string]string{"Cookie": vc}, nil); rr.Code != 200 {
		t.Fatalf("viewer read: %d", rr.Code)
	}
	// viewers may maintain their own profile but nothing else
	vh := map[string]string{"Cookie": vc, "Content-Type": "application/json", "X-Requested-With": "df-admin"}
	if rr := e.do("PATCH", "/admin-api/me", vh, strings.NewReader(`{"name":"Vera Viewer"}`)); rr.Code != 200 || decode(t, rr)["name"] != "Vera Viewer" {
		t.Fatalf("viewer PATCH /me: %d %s", rr.Code, rr.Body.String())
	}
	if rr := e.do("POST", "/admin-api/me/password", vh, strings.NewReader(`{"current_password":"viewer-password-1","new_password":"another-password-2"}`)); rr.Code != 204 {
		t.Fatalf("viewer change password: %d %s", rr.Code, rr.Body.String())
	}
	// admins cannot mint API keys or manage users (owner only)
	ahash, _ := auth.HashPassword("admin-password-1")
	e.app.DB.Create(&db.User{Email: "admin2@example.com", Role: "admin", PasswordHash: ahash, CreatedAt: db.Now(), UpdatedAt: db.Now()})
	al := e.do("POST", "/admin-api/auth/login", hdr, strings.NewReader(`{"email":"admin2@example.com","password":"admin-password-1"}`))
	ah := map[string]string{"Cookie": al.Result().Cookies()[0].String(), "Content-Type": "application/json", "X-Requested-With": "df-admin"}
	if rr := e.do("POST", "/admin-api/api-keys", ah, strings.NewReader(`{"name":"nope"}`)); rr.Code != 403 {
		t.Fatalf("admin creating api key: %d", rr.Code)
	}
	if rr := e.do("GET", "/admin-api/users", ah, nil); rr.Code != 403 {
		t.Fatalf("admin listing users: %d", rr.Code)
	}
	if rr := e.do("GET", "/admin-api/api-keys", ah, nil); rr.Code != 200 {
		t.Fatalf("admin reading api keys: %d", rr.Code)
	}
	// null clears the Stripe customer id; uploads keep the file extension when the display name has none
	cust := db.Customer{StoreID: 1, TestMode: true, Email: "clear@example.com", Name: "Clear", StripeCustomerID: "cus_x", Status: "subscribed", CreatedAt: db.Now(), UpdatedAt: db.Now()}
	e.app.DB.Create(&cust)
	if rr := authed("PATCH", fmt.Sprintf("/admin-api/customers/%d", cust.ID), map[string]any{"stripe_customer_id": nil}); rr.Code != 200 || decode(t, rr)["stripe_customer_id"] != "" {
		t.Fatalf("null clear: %d %s", rr.Code, rr.Body.String())
	}
	var buf2 bytes.Buffer
	mw2 := multipart.NewWriter(&buf2)
	fw2, _ := mw2.CreateFormFile("file", "installer.dmg")
	_, _ = fw2.Write([]byte("x"))
	_ = mw2.WriteField("name", "Installer")
	_ = mw2.Close()
	up2 := e.do("POST", fmt.Sprintf("/admin-api/variants/%d/files", vid), map[string]string{"Cookie": cookie, "Content-Type": mw2.FormDataContentType(), "X-Requested-With": "df-admin"}, &buf2)
	if f2 := decode(t, up2); up2.Code != 201 || f2["name"] != "Installer.dmg" || f2["extension"] != "dmg" {
		t.Fatalf("upload name/extension: %d %s", up2.Code, up2.Body.String())
	}
	if rr := authed("POST", "/admin-api/auth/logout", nil); rr.Code != 204 {
		t.Fatalf("logout: %d", rr.Code)
	}
	if rr := authed("GET", "/admin-api/me", nil); rr.Code != 401 {
		t.Fatalf("session should be revoked: %d", rr.Code)
	}
}

// The License API answers like the real Lemon Squeezy one (verified against
// it with a test-mode key): Laravel-style 422 validation errors, 404s that carry
// nothing but the flag and the error, "instance_id not found.", "is expired."
func TestLicenseAPIErrorShapesMatchLemonSqueezy(t *testing.T) {
	e := newEnv(t)
	oneTime, _ := e.seedCatalog()
	g := e.app.DB
	var st db.Store
	g.First(&st)
	limit := 2
	key := db.LicenseKey{StoreID: st.ID, ProductID: oneTime.ProductID, VariantID: oneTime.ID, Key: "11111111-2222-3333-4444-555555555555", ActivationLimit: &limit, TestMode: true, CreatedAt: db.Now(), UpdatedAt: db.Now()}
	g.Create(&key)
	post := func(path string, kv map[string]string) (int, map[string]any) {
		rr, v := e.form("/v1/licenses/"+path, kv)
		return rr.Code, v
	}
	keys := func(v map[string]any) string {
		var ks []string
		for k := range v {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		return strings.Join(ks, ",")
	}

	// validation: 422, {"message","errors"}, nothing else; the message names the first failing field and counts the rest
	for _, c := range []struct {
		path string
		form map[string]string
		msg  string
	}{
		{"activate", map[string]string{}, "The license key field is required. (and 1 more error)"},
		{"activate", map[string]string{"license_key": key.Key}, "The instance name field is required."},
		{"activate", map[string]string{"license_key": key.Key, "instance_name": "ab"}, "The instance name field must be at least 3 characters."},
		{"activate", map[string]string{"license_key": key.Key, "instance_name": strings.Repeat("a", 256)}, "The instance name field must not be greater than 255 characters."},
		{"deactivate", map[string]string{"license_key": key.Key}, "The instance id field is required."},
		{"deactivate", map[string]string{}, "The license key field is required. (and 1 more error)"},
		{"validate", map[string]string{}, "The license key field is required."},
		{"validate", map[string]string{"license_key": "  "}, "The license key field is required."},
	} {
		code, v := post(c.path, c.form)
		if code != 422 || v["message"] != c.msg || keys(v) != "errors,message" {
			t.Errorf("%s %v: %d %v", c.path, c.form, code, v)
		}
	}
	if _, v := post("activate", map[string]string{"license_key": "no-such-key", "instance_name": "ab"}); v["errors"] == nil {
		t.Errorf("validation comes before the key lookup: %v", v)
	}

	// unknown key / unknown instance: 404 with the flag and the error only
	unknown := "99999999-9999-9999-9999-999999999999"
	for _, c := range []struct {
		path, flag string
		form       map[string]string
		err        string
	}{
		{"activate", "activated", map[string]string{"license_key": "nope", "instance_name": "abc"}, "license_key not found."},
		{"validate", "valid", map[string]string{"license_key": "nope"}, "license_key not found."},
		{"deactivate", "deactivated", map[string]string{"license_key": "nope", "instance_id": unknown}, "license_key not found."},
		{"validate", "valid", map[string]string{"license_key": key.Key, "instance_id": unknown}, "instance_id not found."},
		{"deactivate", "deactivated", map[string]string{"license_key": key.Key, "instance_id": unknown}, "instance_id not found."},
	} {
		code, v := post(c.path, c.form)
		if code != 404 || v[c.flag] != false || v["error"] != c.err || keys(v) != keys(map[string]any{c.flag: 0, "error": 0}) {
			t.Errorf("%s %v: %d %v", c.path, c.form, code, v)
		}
	}

	// an empty instance_id means "no instance"; test_mode is part of the license_key object
	code, v := post("validate", map[string]string{"license_key": key.Key, "instance_id": ""})
	if code != 200 || v["valid"] != true || v["instance"] != nil || sub(v, "license_key", "test_mode") != true || keys(v) != "error,instance,license_key,meta,valid" {
		t.Errorf("validate: %d %v", code, v)
	}
	_, a := post("activate", map[string]string{"license_key": key.Key, "instance_name": "laptop"})
	inst, _ := sub(a, "instance", "id").(string)
	if a["activated"] != true || inst == "" || keys(a) != "activated,error,instance,license_key,meta" {
		t.Fatalf("activate: %v", a)
	}
	// limit reached: license_key and meta, but no instance
	post("activate", map[string]string{"license_key": key.Key, "instance_name": "phone"})
	code, v = post("activate", map[string]string{"license_key": key.Key, "instance_name": "tablet"})
	if code != 400 || v["error"] != "This license key has reached the activation limit." || keys(v) != "activated,error,license_key,meta" {
		t.Errorf("limit: %d %v", code, v)
	}

	// disabled / expired: the instance is looked up first, then the state is reported with the instance
	g.Model(&key).Update("disabled", true)
	if code, v := post("validate", map[string]string{"license_key": key.Key, "instance_id": unknown}); code != 404 || v["error"] != "instance_id not found." {
		t.Errorf("disabled + unknown instance: %d %v", code, v)
	}
	code, v = post("validate", map[string]string{"license_key": key.Key, "instance_id": inst})
	if code != 400 || v["valid"] != false || v["error"] != "This license key is disabled." || sub(v, "instance", "id") != inst || sub(v, "license_key", "status") != "disabled" {
		t.Errorf("disabled: %d %v", code, v)
	}
	g.Model(&key).Updates(map[string]any{"disabled": false, "expires_at": time.Now().Add(-time.Hour)})
	for _, c := range []struct{ path, flag string }{{"validate", "valid"}, {"activate", "activated"}} {
		form := map[string]string{"license_key": key.Key, "instance_id": inst, "instance_name": "desk"}
		if code, v := post(c.path, form); code != 400 || v[c.flag] != false || v["error"] != "This license key is expired." {
			t.Errorf("expired %s: %d %v", c.path, code, v)
		}
	}
}

// Shapes that only showed up when a real Lemon Squeezy account was imported and
// compared field by field with its API.
func TestJSONAPIShapesMatchRealLemonSqueezyData(t *testing.T) {
	e := newEnv(t)
	oneTime, _ := e.seedCatalog()
	g := e.app.DB
	var st db.Store
	g.First(&st)
	now := db.Now()

	// price history: editing a variant's price adds a record, and the variant follows the latest one
	// (here a one-time price that still carries a renewal interval from an earlier subscription setup)
	year, one := "year", 1
	latest := db.Price{VariantID: oneTime.ID, Category: "one_time", Scheme: "standard", UnitPrice: 1999, PackageSize: 1, TaxCode: "eservice", Currency: "USD", RenewalIntervalUnit: &year, RenewalIntervalQuantity: &one, CreatedAt: now, UpdatedAt: now}
	g.Create(&latest)
	v := decode(t, e.api("GET", "/v1/variants/"+strconv.FormatInt(oneTime.ID, 10), nil))
	if sub(v, "data", "attributes", "price") != float64(1999) || sub(v, "data", "attributes", "is_subscription") != false ||
		sub(v, "data", "attributes", "interval") != "year" || sub(v, "data", "attributes", "interval_count") != float64(1) {
		t.Errorf("variant attributes: %v", sub(v, "data", "attributes"))
	}
	pr := decode(t, e.api("GET", "/v1/products/"+strconv.FormatInt(oneTime.ProductID, 10), nil))
	if sub(pr, "data", "attributes", "price") != float64(500) { // the cheaper subscription variant is still the "from" price
		t.Errorf("product price: %v", sub(pr, "data", "attributes", "price"))
	}

	// one-time prices: tiers and setup_fee_enabled are null, not [] / false
	p := decode(t, e.api("GET", "/v1/prices/"+strconv.FormatInt(latest.ID, 10), nil))
	attrs, _ := sub(p, "data", "attributes").(map[string]any)
	if v, present := attrs["tiers"]; !present || v != nil {
		t.Errorf("tiers = %v (present %v)", v, present)
	}
	if v, present := attrs["setup_fee_enabled"]; !present || v != nil {
		t.Errorf("setup_fee_enabled = %v (present %v)", v, present)
	}

	// test-mode orders never count towards store or customer totals
	cust := db.Customer{StoreID: st.ID, Email: "buyer@example.com", Name: "Buyer", TestMode: true, CreatedAt: now, UpdatedAt: now}
	g.Create(&cust)
	g.Create(&db.Order{StoreID: st.ID, CustomerID: cust.ID, Identifier: "test-order", OrderNumber: 1, Status: "paid", Total: 1999, TotalUSD: 1999, Currency: "USD", TestMode: true, Source: "stripe", CreatedAt: now, UpdatedAt: now})
	s := decode(t, e.api("GET", "/v1/stores/"+strconv.FormatInt(st.ID, 10), nil))
	if sub(s, "data", "attributes", "total_sales") != float64(0) || sub(s, "data", "attributes", "total_revenue") != float64(0) {
		t.Errorf("store totals: %v", sub(s, "data", "attributes"))
	}
	c := decode(t, e.api("GET", "/v1/customers/"+strconv.FormatInt(cust.ID, 10), nil))
	if sub(c, "data", "attributes", "total_revenue_currency") != float64(0) {
		t.Errorf("customer revenue: %v", sub(c, "data", "attributes", "total_revenue_currency"))
	}
	g.Create(&db.Order{StoreID: st.ID, CustomerID: cust.ID, Identifier: "live-order", OrderNumber: 2, Status: "paid", Total: 500, TotalUSD: 500, Currency: "USD", Source: "stripe", CreatedAt: now, UpdatedAt: now})
	s = decode(t, e.api("GET", "/v1/stores/"+strconv.FormatInt(st.ID, 10), nil))
	c = decode(t, e.api("GET", "/v1/customers/"+strconv.FormatInt(cust.ID, 10), nil))
	if sub(s, "data", "attributes", "total_sales") != float64(1) || sub(s, "data", "attributes", "total_revenue") != float64(500) ||
		sub(c, "data", "attributes", "total_revenue_currency") != float64(500) {
		t.Errorf("live totals: store %v customer %v", sub(s, "data", "attributes"), sub(c, "data", "attributes", "total_revenue_currency"))
	}

	// first_order_item carries price_id and no deleted_at
	priceID := latest.ID
	it := db.OrderItem{OrderID: 1, ProductID: oneTime.ProductID, VariantID: oneTime.ID, PriceID: &priceID, ProductName: "Pro App", VariantName: "Lifetime", Price: 1999, Quantity: 1, TestMode: true, CreatedAt: now, UpdatedAt: now}
	g.Create(&it)
	o := decode(t, e.api("GET", "/v1/orders/1", nil))
	foi, _ := sub(o, "data", "attributes", "first_order_item").(map[string]any)
	if _, has := foi["deleted_at"]; has || foi["price_id"] != float64(priceID) {
		t.Errorf("first_order_item = %v", foi)
	}
}

func TestAdminImportRejectsUnknownConflictPolicy(t *testing.T) {
	e := newEnv(t)
	hdr := map[string]string{"Content-Type": "application/json", "X-Requested-With": "df-admin"}
	rr := e.do("POST", "/admin-api/auth/login", hdr, strings.NewReader(`{"email":"owner@example.com","password":"correct horse battery"}`))
	if rr.Code != 200 {
		t.Fatalf("login: %d %s", rr.Code, rr.Body.String())
	}
	hdr["Cookie"] = rr.Result().Cookies()[0].String()
	// validated before anything is started or sent to Lemon Squeezy
	bad := e.do("POST", "/admin-api/imports/lemonsqueezy", hdr, strings.NewReader(`{"api_key":"lsk_test_1234","on_conflict":"skip"}`))
	if bad.Code != 422 || !strings.Contains(bad.Body.String(), "on_conflict") {
		t.Fatalf("unknown policy: %d %s", bad.Code, bad.Body.String())
	}
}

func TestStorefrontDownloadsAndMagicLink(t *testing.T) {
	e := newEnv(t)
	once, _ := e.seedCatalog()
	// published file + draft file
	key := storage.NewKey("tool.zip")
	size, sha, err := e.app.Svc.Storage.Put(context.Background(), key, strings.NewReader("ZIPDATA"), "application/zip")
	if err != nil {
		t.Fatal(err)
	}
	now := db.Now()
	file := db.File{VariantID: once.ID, Identifier: "11111111-1111-1111-1111-111111111111", Name: "tool.zip", Extension: "zip", Size: size, SHA256: sha, Version: "2.0", Sort: 1, Status: "published", StorageDriver: "local", StorageKey: key, TestMode: true, CreatedAt: now, UpdatedAt: now}
	e.app.DB.Create(&file)
	e.app.DB.Create(&db.File{VariantID: once.ID, Identifier: "22222222-2222-2222-2222-222222222222", Name: "draft.zip", Status: "draft", StorageKey: key, TestMode: true, CreatedAt: now, UpdatedAt: now})

	e.stripe.set("GET", "/v1/checkout/sessions/cs_live_1", strings.Replace(sessionJSON("cs_live_1", "payment", "price_once", "Buyer@Example.com", 999, 0,
		`"payment_intent":{"id":"pi_9"},`), `"livemode":false`, `"livemode":true`, 1))
	// session is live → customer/order are live-mode; the harness key is test-mode so create the order as live
	rr := e.stripeEvent("checkout.session.completed", map[string]any{"id": "cs_live_1"})
	if rr.Code != 200 {
		t.Fatalf("fulfil: %d %s", rr.Code, rr.Body.String())
	}
	var o db.Order
	e.app.DB.First(&o)
	if o.TestMode {
		t.Fatalf("order should be live mode")
	}
	e.app.DB.Model(&db.Variant{}).Where("id = ?", once.ID).Update("test_mode", true) // unchanged catalogue
	// the live session maps to the same (test-flagged) catalogue price; license key issued
	var k db.LicenseKey
	if e.app.DB.First(&k).Error != nil {
		t.Fatal("no licence key")
	}

	sig := e.app.Svc.URLs.ReceiptSignature(o.Identifier)
	od := e.do("GET", "/storefront-api/orders/"+o.Identifier+"?signature="+sig, nil, nil)
	if od.Code != 200 {
		t.Fatalf("order detail: %d %s", od.Code, od.Body.String())
	}
	d := decode(t, od)
	files := d["files"].([]any)
	if len(files) != 1 || sub(files[0].(map[string]any), "name") != "tool.zip" {
		t.Fatalf("only published files are exposed: %v", files)
	}
	if lk := d["license_keys"].([]any); len(lk) != 1 || sub(lk[0].(map[string]any), "key") != k.Key {
		t.Fatalf("license keys: %v", d["license_keys"])
	}
	if links := d["links"].([]any); len(links) != 1 || !strings.Contains(sub(links[0].(map[string]any), "url").(string), k.Key) {
		t.Fatalf("link variable not substituted: %v", d["links"])
	}
	if rr := e.do("GET", "/storefront-api/orders/"+o.Identifier+"?signature=bad", nil, nil); rr.Code != 401 {
		t.Fatalf("bad signature: %d", rr.Code)
	}

	dlURL := sub(files[0].(map[string]any), "download_url").(string)
	got := e.do("GET", dlURL, nil, nil)
	if got.Code != 200 || got.Body.String() != "ZIPDATA" {
		t.Fatalf("download: %d %q", got.Code, got.Body.String())
	}
	// tampered signature and per-day limit (3)
	if rr := e.do("GET", strings.Replace(dlURL, "signature=", "signature=00", 1), nil, nil); rr.Code != 403 {
		t.Fatalf("tampered download: %d", rr.Code)
	}
	e.do("GET", dlURL, nil, nil)
	e.do("GET", dlURL, nil, nil)
	if rr := e.do("GET", dlURL, nil, nil); rr.Code != 429 {
		t.Fatalf("download limit should trigger 429, got %d", rr.Code)
	}
	// refunded orders lose access to files
	e.app.DB.Model(&db.Order{}).Where("id = ?", o.ID).Update("status", "refunded")
	if rr := e.do("GET", dlURL, map[string]string{"X-Forwarded-For": "1.2.3.4"}, nil); rr.Code != 403 && rr.Code != 429 {
		t.Fatalf("refunded download: %d", rr.Code)
	}
	e.app.DB.Model(&db.Order{}).Where("id = ?", o.ID).Update("status", "paid")

	// magic link login (case-insensitive email) → cookie → My Orders
	csrf := map[string]string{"Content-Type": "application/json", "X-Requested-With": "df-admin"}
	if rr := e.do("POST", "/storefront-api/auth/magic-link", csrf, strings.NewReader(`{"email":"buyer@example.com"}`)); rr.Code != 204 {
		t.Fatalf("magic link: %d", rr.Code)
	}
	if rr := e.do("POST", "/storefront-api/auth/magic-link", csrf, strings.NewReader(`{"email":"stranger@example.com"}`)); rr.Code != 204 {
		t.Fatalf("magic link unknown email must also be 204: %d", rr.Code)
	}
	var token string
	for i := 0; i < 100 && token == ""; i++ {
		service.WaitBackground()
		e.mu.Lock()
		for _, raw := range e.sent {
			if i := strings.Index(raw, "/my-orders/verify?token="); i >= 0 {
				rest := raw[i+len("/my-orders/verify?token="):]
				if j := strings.IndexAny(rest, "\"\r\n &"); j > 0 {
					token = rest[:j]
				}
			}
		}
		e.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	if token == "" {
		t.Fatalf("no magic link email captured (%d mails)", len(e.sent))
	}
	vr := e.do("POST", "/storefront-api/auth/verify", csrf, strings.NewReader(fmt.Sprintf(`{"token":%q}`, token)))
	if vr.Code != 200 || len(vr.Result().Cookies()) == 0 {
		t.Fatalf("verify: %d %s", vr.Code, vr.Body.String())
	}
	if rr := e.do("POST", "/storefront-api/auth/verify", csrf, strings.NewReader(fmt.Sprintf(`{"token":%q}`, token))); rr.Code != 400 {
		t.Fatalf("token must be single use: %d", rr.Code)
	}
	cookie := vr.Result().Cookies()[0].String()
	mo := e.do("GET", "/storefront-api/me/orders", map[string]string{"Cookie": cookie}, nil)
	md := decode(t, mo)
	if mo.Code != 200 || len(md["orders"].([]any)) != 1 || sub(md["orders"].([]any)[0].(map[string]any), "license_key_count") != float64(1) {
		t.Fatalf("my orders: %d %s", mo.Code, mo.Body.String())
	}
	if rr := e.do("GET", "/storefront-api/me/orders", nil, nil); rr.Code != 401 {
		t.Fatalf("my orders w/o cookie: %d", rr.Code)
	}
	// order detail also works with the session cookie
	if rr := e.do("GET", "/storefront-api/orders/"+o.Identifier, map[string]string{"Cookie": cookie}, nil); rr.Code != 200 {
		t.Fatalf("order via cookie: %d", rr.Code)
	}
	// receipt email was sent with the licence key
	service.WaitBackground()
	found := false
	e.mu.Lock()
	for _, raw := range e.sent {
		if strings.Contains(raw, k.Key) {
			found = true
		}
	}
	e.mu.Unlock()
	if !found {
		t.Fatalf("receipt email with licence key not sent")
	}
}

func TestCheckoutAndBuyNow(t *testing.T) {
	e := newEnv(t)
	once, _ := e.seedCatalog()
	e.stripe.set("POST", "/v1/checkout/sessions", `{"id":"cs_test_new","url":"https://checkout.stripe.com/c/pay/cs_test_new","expires_at":1999999999}`)
	var st db.Store
	e.app.DB.First(&st)
	body := map[string]any{"data": map[string]any{"type": "checkouts",
		"attributes":    map[string]any{"checkout_data": map[string]any{"email": "x@example.com", "custom": map[string]any{"user_id": "42"}}, "product_options": map[string]any{"redirect_url": "https://app.test/welcome?k=[license_key]"}, "test_mode": true},
		"relationships": map[string]any{"store": map[string]any{"data": map[string]any{"type": "stores", "id": fmt.Sprint(st.ID)}}, "variant": map[string]any{"data": map[string]any{"type": "variants", "id": fmt.Sprint(once.ID)}}}}}
	rr := e.api("POST", "/v1/checkouts", body)
	if rr.Code != 201 {
		t.Fatalf("create checkout: %d %s", rr.Code, rr.Body.String())
	}
	d := decode(t, rr)
	if sub(d, "data", "attributes", "url") != "https://checkout.stripe.com/c/pay/cs_test_new" || sub(d, "data", "type") != "checkouts" {
		t.Fatalf("checkout doc: %s", rr.Body.String())
	}
	lp := e.stripe.lastPost()
	if lp.Get("mode") != "payment" || lp.Get("line_items[0][price]") != "price_once" || lp.Get("customer_email") != "x@example.com" || lp.Get("client_reference_id") == "" {
		t.Fatalf("stripe session params: %v", lp)
	}
	// custom data round-trips into the order and the webhook payload
	e.stripe.set("GET", "/v1/checkout/sessions/cs_test_new", strings.Replace(sessionJSON("cs_test_new", "payment", "price_once", "x@example.com", 999, 0, ""), `"client_reference_id":""`, fmt.Sprintf(`"client_reference_id":%q`, sub(d, "data", "id")), 1))
	e.stripeEvent("checkout.session.completed", map[string]any{"id": "cs_test_new"})
	e.hook.waitFor(t, 2)
	var payload map[string]any
	_ = json.Unmarshal(e.hook.got[0].Body, &payload)
	if sub(payload, "meta", "custom_data", "user_id") != "42" {
		t.Fatalf("custom_data missing in webhook: %s", e.hook.got[0].Body)
	}
	// success landing redirects to the signed order page, or the merchant redirect_url with variables filled
	land := e.do("GET", "/checkout/success?session_id=cs_test_new&checkout="+fmt.Sprint(sub(d, "data", "id")), nil, nil)
	loc := land.Header().Get("Location")
	if land.Code != 303 || !strings.HasPrefix(loc, "https://app.test/welcome?k=") || strings.Contains(loc, "[license_key]") {
		t.Fatalf("landing: %d %s", land.Code, loc)
	}
	// buy-now link creates a session and redirects to Stripe
	buy := e.do("GET", "/checkout/buy/"+once.Slug, nil, nil)
	if buy.Code != 303 || !strings.HasPrefix(buy.Header().Get("Location"), "https://checkout.stripe.com/") {
		t.Fatalf("buy now: %d %s", buy.Code, buy.Header().Get("Location"))
	}
}

func cookieValue(rr *httptest.ResponseRecorder, name string) string {
	for _, c := range rr.Result().Cookies() {
		if c.Name == name {
			return c.Name + "=" + c.Value
		}
	}
	return ""
}

func TestSignInWithStripe(t *testing.T) {
	e := newEnv(t)
	csrf := map[string]string{"Content-Type": "application/json", "X-Requested-With": "df-admin"}
	if cfg := decode(t, e.do("GET", "/admin-api/auth/config", nil, nil)); cfg["stripe_login"] != true {
		t.Fatalf("stripe_login should be enabled: %v", cfg)
	}

	start := func(mode string, cookie string) (loc *url.URL, oauthCookie string) {
		h := map[string]string{}
		if cookie != "" {
			h["Cookie"] = cookie
		}
		rr := e.do("GET", "/admin-api/auth/stripe/start?mode="+mode, h, nil)
		if rr.Code != 302 {
			t.Fatalf("start %s: %d %s", mode, rr.Code, rr.Body.String())
		}
		u, _ := url.Parse(rr.Header().Get("Location"))
		return u, cookieValue(rr, "df_oauth")
	}
	callback := func(query, cookie string) *httptest.ResponseRecorder {
		h := map[string]string{}
		if cookie != "" {
			h["Cookie"] = cookie
		}
		return e.do("GET", "/admin-api/auth/stripe/callback?"+query, h, nil)
	}
	token := func(acct string) {
		e.stripe.set("POST", "/oauth/token", fmt.Sprintf(`{"stripe_user_id":%q,"scope":"read_write","livemode":false,"token_type":"bearer"}`, acct))
		e.stripe.set("POST", "/oauth/deauthorize", fmt.Sprintf(`{"stripe_user_id":%q}`, acct))
		e.stripe.set("GET", "/v1/account", fmt.Sprintf(`{"id":%q,"email":"merchant@example.com","country":"US","business_profile":{"name":"Acme Inc"}}`, acct))
	}

	// 1) authorize redirect carries client_id, scope, redirect_uri and a state bound to a signed cookie
	loc, oc := start("login", "")
	q := loc.Query()
	if loc.Path != "/oauth/authorize" || q.Get("client_id") != "ca_test_123" || q.Get("scope") != "read_write" || q.Get("response_type") != "code" ||
		q.Get("redirect_uri") != "http://pay.test/admin-api/auth/stripe/callback" || q.Get("state") == "" || oc == "" {
		t.Fatalf("authorize url: %s (cookie %q)", loc, oc)
	}
	state := q.Get("state")

	// 2) an account that is neither linked nor allow-listed is refused (no session, no user created)
	token("acct_stranger")
	rr := callback("code=ac_1&state="+state, oc)
	if rr.Code != 302 || rr.Header().Get("Location") != "http://pay.test/admin/login?error=stripe_not_allowed" || cookieValue(rr, "df_admin") != "" {
		t.Fatalf("stranger: %d %s", rr.Code, rr.Header().Get("Location"))
	}
	var users int64
	e.app.DB.Model(&db.User{}).Count(&users)
	if users != 1 {
		t.Fatalf("no user may be created for a refused account (users=%d)", users)
	}

	// 3) CSRF protections: wrong state / missing cookie / user denied
	loc, oc = start("login", "")
	if rr := callback("code=x&state=WRONG", oc); rr.Header().Get("Location") != "http://pay.test/admin/login?error=stripe_state" {
		t.Fatalf("state mismatch: %s", rr.Header().Get("Location"))
	}
	if rr := callback("code=x&state="+loc.Query().Get("state"), ""); rr.Header().Get("Location") != "http://pay.test/admin/login?error=stripe_state" {
		t.Fatalf("missing cookie: %s", rr.Header().Get("Location"))
	}
	loc, oc = start("login", "")
	if rr := callback("error=access_denied&state="+loc.Query().Get("state"), oc); rr.Header().Get("Location") != "http://pay.test/admin/login?error=stripe_denied" {
		t.Fatalf("denied: %s", rr.Header().Get("Location"))
	}

	// 4) allow-listed account → user provisioned (admin, an owner already exists), session started, access revoked again
	if err := e.app.Svc.Settings.Set("stripe.login_accounts", "acct_ok"); err != nil {
		t.Fatal(err)
	}
	token("acct_ok")
	loc, oc = start("login", "")
	rr = callback("code=ac_2&state="+loc.Query().Get("state"), oc)
	if rr.Code != 302 || rr.Header().Get("Location") != "http://pay.test/admin" {
		t.Fatalf("allow-listed login: %d %s", rr.Code, rr.Header().Get("Location"))
	}
	sess := cookieValue(rr, "df_admin")
	if sess == "" {
		t.Fatal("no session cookie")
	}
	me := decode(t, e.do("GET", "/admin-api/me", map[string]string{"Cookie": sess}, nil))
	if me["stripe_account_id"] != "acct_ok" || me["role"] != "admin" || me["email"] != "merchant@example.com" || me["stripe_account_label"] != "Acme Inc" {
		t.Fatalf("provisioned user: %v", me)
	}
	service.WaitBackground()
	var revoked bool
	e.stripe.mu.Lock()
	for _, p := range e.stripe.posts {
		if p.Get("stripe_user_id") == "acct_ok" && p.Get("client_id") == "ca_test_123" {
			revoked = true
		}
	}
	e.stripe.mu.Unlock()
	if !revoked {
		t.Fatal("platform access must be revoked (oauth/deauthorize) right after login")
	}
	// the same account signs in again as the same user
	loc, oc = start("login", "")
	if rr := callback("code=ac_3&state="+loc.Query().Get("state"), oc); cookieValue(rr, "df_admin") == "" {
		t.Fatal("second login failed")
	}
	e.app.DB.Model(&db.User{}).Count(&users)
	if users != 2 {
		t.Fatalf("users = %d, want 2", users)
	}

	// 5) link an account to the signed-in owner, then sign in with it as the owner
	owner := e.do("POST", "/admin-api/auth/login", csrf, strings.NewReader(`{"email":"owner@example.com","password":"correct horse battery"}`))
	ownerCookie := cookieValue(owner, "df_admin")
	if rr := e.do("GET", "/admin-api/auth/stripe/start?mode=link", nil, nil); rr.Header().Get("Location") != "http://pay.test/admin/login?error=unauthenticated" {
		t.Fatalf("link without session: %s", rr.Header().Get("Location"))
	}
	token("acct_link")
	loc, oc = start("link", ownerCookie)
	rr = callback("code=ac_4&state="+loc.Query().Get("state"), oc)
	if rr.Header().Get("Location") != "http://pay.test/admin/settings/profile?linked=1" {
		t.Fatalf("link callback: %s", rr.Header().Get("Location"))
	}
	if me := decode(t, e.do("GET", "/admin-api/me", map[string]string{"Cookie": ownerCookie}, nil)); me["stripe_account_id"] != "acct_link" {
		t.Fatalf("owner not linked: %v", me)
	}
	loc, oc = start("login", "")
	rr = callback("code=ac_5&state="+loc.Query().Get("state"), oc)
	if me := decode(t, e.do("GET", "/admin-api/me", map[string]string{"Cookie": cookieValue(rr, "df_admin")}, nil)); me["email"] != "owner@example.com" || me["role"] != "owner" {
		t.Fatalf("linked account must sign in as the owner: %v", me)
	}
	// unlink needs a password on the account (owner has one)
	if rr := e.do("DELETE", "/admin-api/me/stripe", map[string]string{"Cookie": ownerCookie, "X-Requested-With": "df-admin"}, nil); rr.Code != 204 {
		t.Fatalf("unlink: %d", rr.Code)
	}
	// a disabled user cannot sign in via Stripe
	e.app.DB.Model(&db.User{}).Where("stripe_account_id = ?", "acct_ok").Update("disabled", true)
	token("acct_ok")
	loc, oc = start("login", "")
	if rr := callback("code=ac_6&state="+loc.Query().Get("state"), oc); rr.Header().Get("Location") != "http://pay.test/admin/login?error=disabled" {
		t.Fatalf("disabled: %s", rr.Header().Get("Location"))
	}
}
