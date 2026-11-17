package importer

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"fulfillment/internal/config"
	"fulfillment/internal/db"
)

type fakeStripe struct {
	t   *testing.T
	srv *httptest.Server

	mu        sync.Mutex
	customers map[string][]map[string]any
	products  []map[string]any
	prices    []map[string]any
	subs      map[string][]map[string]any
	retrieve  map[string]map[string]any
	status    int // answer every request with this status when set
	requests  []string
	subsPage  int // page size of /v1/subscriptions (0 = everything at once)
}

func newFakeStripe(t *testing.T) *fakeStripe {
	f := &fakeStripe{t: t, customers: map[string][]map[string]any{}, subs: map[string][]map[string]any{}, retrieve: map[string]map[string]any{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeStripe) opt() envOpt {
	return func(c *config.Config) {
		c.Stripe.SecretKey = "sk_test_importer"
		c.Stripe.APIBase = f.srv.URL
	}
}

func (f *fakeStripe) calls(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}

func (f *fakeStripe) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.URL.RequestURI())
	status := f.status
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if status != 0 {
		w.WriteHeader(status)
		w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"nope"}}`))
		return
	}
	list := func(data any) {
		json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data, "has_more": false})
	}
	switch {
	case r.URL.Path == "/v1/customers":
		email := r.URL.Query().Get("email")
		if email == "err@example.com" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"bad email"}}`))
			return
		}
		list(f.customers[email])
	case r.URL.Path == "/v1/products":
		list(f.products)
	case r.URL.Path == "/v1/prices":
		list(f.prices)
	case r.URL.Path == "/v1/subscriptions":
		all := f.subs[r.URL.Query().Get("customer")]
		if f.subsPage == 0 {
			list(all)
			return
		}
		start := 0
		if after := r.URL.Query().Get("starting_after"); after != "" {
			for i, s := range all {
				if s["id"] == after {
					start = i + 1
				}
			}
		}
		end := min(start+f.subsPage, len(all))
		json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": all[start:end], "has_more": end < len(all)})
	case strings.HasPrefix(r.URL.Path, "/v1/subscriptions/"):
		if s, ok := f.retrieve[strings.TrimPrefix(r.URL.Path, "/v1/subscriptions/")]; ok {
			json.NewEncoder(w).Encode(s)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"message":"No such subscription"}}`))
	default:
		http.NotFound(w, r)
	}
}

func stripeProduct(id, name string, meta map[string]string, defaultPrice string) map[string]any {
	return map[string]any{"id": id, "name": name, "active": true, "metadata": meta, "default_price": nilIfEmpty(defaultPrice)}
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func stripePrice(id, product string, amount int64, interval string) map[string]any {
	p := map[string]any{"id": id, "product": product, "active": true, "currency": "usd", "unit_amount": amount, "type": "one_time", "recurring": nil}
	if interval != "" {
		p["type"] = "recurring"
		p["recurring"] = map[string]any{"interval": interval, "interval_count": 1}
	}
	return p
}

func stripeSub(id, customer, priceID, productID string, qty int) map[string]any {
	future := time.Now().Add(30 * 24 * time.Hour).Unix()
	return map[string]any{
		"id": id, "status": "active", "customer": customer, "cancel_at_period_end": false, "billing_cycle_anchor": future - 86400,
		"current_period_end": future, "metadata": map[string]any{},
		"items": map[string]any{"data": []any{map[string]any{"id": "si_" + id, "quantity": qty, "current_period_end": future,
			"price": map[string]any{"id": priceID, "product": productID}}}},
	}
}

func seedMatchData(e *env) {
	e.g.Create(&db.Store{ID: 1, Name: "S", Slug: "s", Currency: "USD"})
	for _, c := range []db.Customer{
		{ID: 11, StoreID: 1, Email: "john@example.com", Name: "John"},
		{ID: 12, StoreID: 1, Email: "jane@example.com", Name: "Jane"},
		{ID: 13, StoreID: 1, Email: "none@example.com", Name: "Nobody"},
		{ID: 14, StoreID: 1, Email: "linked@example.com", Name: "Linked", StripeCustomerID: "cus_Already1"},
		{ID: 15, StoreID: 1, Email: "err@example.com", Name: "Broken"},
	} {
		e.g.Create(&c)
	}
	for _, p := range []db.Product{{ID: 100, StoreID: 1, Name: "Pro App"}, {ID: 101, StoreID: 1, Name: "Course"}, {ID: 102, StoreID: 1, Name: "Ghost"}, {ID: 103, StoreID: 1, Name: "Twin"}} {
		e.g.Create(&p)
	}
	for _, v := range []db.Variant{
		{ID: 200, ProductID: 100, Name: "Default", Slug: "v200"}, {ID: 201, ProductID: 100, Name: "Monthly", Slug: "v201"},
		{ID: 202, ProductID: 101, Name: "Default", Slug: "v202"}, {ID: 203, ProductID: 102, Name: "Default", Slug: "v203"},
		{ID: 204, ProductID: 103, Name: "Twin Variant", Slug: "v204"},
	} {
		e.g.Create(&v)
	}
	month, one := "month", 1
	for _, p := range []db.Price{
		{ID: 299, VariantID: 200, UnitPrice: 3900, Currency: "USD"}, // superseded price
		{ID: 300, VariantID: 200, UnitPrice: 4900, Currency: "USD"},
		{ID: 301, VariantID: 201, Category: "subscription", UnitPrice: 999, Currency: "USD", RenewalIntervalUnit: &month, RenewalIntervalQuantity: &one},
		{ID: 302, VariantID: 202, UnitPrice: 1500, Currency: "USD"},
		{ID: 303, VariantID: 203, UnitPrice: 700, Currency: "USD"},
	} {
		e.g.Create(&p)
	}
	pid := int64(301)
	for _, s := range []db.Subscription{
		{ID: 700, StoreID: 1, CustomerID: 11, VariantID: 201, PriceID: &pid, Status: "active", UserEmail: "john@example.com", ProductName: "Pro App", VariantName: "Monthly"},
		{ID: 701, StoreID: 1, CustomerID: 11, VariantID: 201, PriceID: &pid, Status: "active", UserEmail: "john@example.com", PaymentProcessor: "paypal"},
		{ID: 702, StoreID: 1, CustomerID: 12, VariantID: 201, PriceID: &pid, Status: "active", UserEmail: "jane@example.com"},
		{ID: 703, StoreID: 1, CustomerID: 13, VariantID: 201, PriceID: &pid, Status: "active", UserEmail: "none@example.com"},
		{ID: 704, StoreID: 1, CustomerID: 11, VariantID: 201, PriceID: &pid, Status: "expired", UserEmail: "john@example.com"},
	} {
		e.g.Create(&s)
	}
}

func stripeFixture(f *fakeStripe) {
	f.customers["john@example.com"] = []map[string]any{{"id": "cus_John", "email": "john@example.com", "created": 1000}}
	f.customers["jane@example.com"] = []map[string]any{
		{"id": "cus_JaneNew", "email": "jane@example.com", "created": 2000}, {"id": "cus_JaneOld", "email": "jane@example.com", "created": 1000},
	}
	f.products = []map[string]any{
		stripeProduct("prod_PA", "Pro App - Default", nil, ""),
		stripeProduct("prod_PM", "Whatever", map[string]string{"ls_variant_id": "201"}, ""),
		stripeProduct("prod_C", "course", nil, "price_C2"), // case-insensitive, sole variant of its product
		stripeProduct("prod_T1", "Twin Variant", nil, ""),
		stripeProduct("prod_T2", "Twin Variant", nil, ""),
	}
	f.prices = []map[string]any{
		stripePrice("price_A1", "prod_PA", 4900, ""), stripePrice("price_A2", "prod_PA", 5000, ""),
		stripePrice("price_M1", "prod_PM", 999, "month"), stripePrice("price_M2", "prod_PM", 999, "year"),
		stripePrice("price_C1", "prod_C", 1500, ""), stripePrice("price_C2", "prod_C", 1500, ""),
	}
	f.subs["cus_John"] = []map[string]any{stripeSub("sub_J1", "cus_John", "price_M1", "prod_PM", 3)}
	f.subs["cus_JaneOld"] = []map[string]any{stripeSub("sub_Ja1", "cus_JaneOld", "price_M1", "prod_PM", 1), stripeSub("sub_Ja2", "cus_JaneOld", "price_M1", "prod_PM", 1)}
	f.retrieve["sub_J1"] = stripeSub("sub_J1", "cus_John", "price_M1", "prod_PM", 3)
}

func TestAutoMatchEverything(t *testing.T) {
	fs := newFakeStripe(t)
	stripeFixture(fs)
	e := newEnv(t, fs.opt())
	seedMatchData(e)

	job, err := e.r.StartAutoMatch("admin", MatchOptions{Customers: true, Products: true, Subscriptions: true})
	j := e.mustFinish(job, err, "completed")
	if j.Kind != "stripe_auto_match" {
		t.Errorf("kind = %s", j.Kind)
	}

	cust := func(id int64) string { return e.customerStripe(id) }
	if cust(11) != "cus_John" || cust(12) != "cus_JaneOld" || cust(13) != "" || cust(14) != "cus_Already1" || cust(15) != "" {
		t.Errorf("customers: %q %q %q %q %q", cust(11), cust(12), cust(13), cust(14), cust(15))
	}
	for _, want := range []string{"customer #12", "cus_JaneNew", "cus_JaneOld", "customer #13 (none@example.com", "customer #15", "Stripe lookup failed"} {
		if !e.logContains(j.ID, "warn", want) {
			t.Errorf("missing warning containing %q:\n%s", want, e.logDump(j.ID))
		}
	}

	variant := func(id int64) string {
		var v db.Variant
		e.g.First(&v, id)
		return v.StripeProductID
	}
	if variant(200) != "prod_PA" || variant(201) != "prod_PM" || variant(202) != "prod_C" || variant(203) != "" || variant(204) != "" {
		t.Errorf("variants: %q %q %q %q %q", variant(200), variant(201), variant(202), variant(203), variant(204))
	}
	var sole db.Product
	e.g.First(&sole, 101)
	if sole.StripeProductID != "prod_C" {
		t.Errorf("the product of a single-variant product follows its variant: %q", sole.StripeProductID)
	}
	var multi db.Product
	e.g.First(&multi, 100)
	if multi.StripeProductID != "" {
		t.Errorf("a multi-variant product must not be linked: %q", multi.StripeProductID)
	}
	for _, want := range []string{"variant #203 (Ghost - Default", "variant #204", "prod_T1", "prod_T2"} {
		if !e.logContains(j.ID, "warn", want) {
			t.Errorf("missing warning containing %q:\n%s", want, e.logDump(j.ID))
		}
	}

	price := func(id int64) string {
		var p db.Price
		e.g.First(&p, id)
		return p.StripePriceID
	}
	if price(300) != "price_A1" || price(301) != "price_M1" || price(302) != "price_C2" || price(303) != "" || price(299) != "" {
		t.Errorf("prices: 300=%q 301=%q 302=%q 303=%q 299=%q", price(300), price(301), price(302), price(303), price(299))
	}
	if e.logContains(j.ID, "warn", "price #299") {
		t.Errorf("a superseded price must not produce a warning")
	}
	if !e.logContains(j.ID, "warn", "price #303") {
		t.Errorf("the unmatched current price must be reported")
	}

	sub := func(id int64) db.Subscription {
		var s db.Subscription
		e.g.First(&s, id)
		return s
	}
	s700 := sub(700)
	if s700.StripeSubscriptionID == nil || *s700.StripeSubscriptionID != "sub_J1" || s700.StripeCustomerID != "cus_John" {
		t.Fatalf("subscription 700 = %+v", s700)
	}
	if s700.Quantity != 3 || s700.RenewsAt == nil {
		t.Errorf("the linked subscription should have been synchronised from Stripe: qty=%d renews=%v", s700.Quantity, s700.RenewsAt)
	}
	for _, id := range []int64{701, 702, 703, 704} {
		if s := sub(id); s.StripeSubscriptionID != nil {
			t.Errorf("subscription %d must stay unlinked, got %v", id, *s.StripeSubscriptionID)
		}
	}
	for _, want := range []string{"subscription #701", "PayPal", "subscription #702 (jane@example.com", "sub_Ja1", "sub_Ja2", "subscription #703 (none@example.com"} {
		if !e.logContains(j.ID, "warn", want) {
			t.Errorf("missing warning containing %q:\n%s", want, e.logDump(j.ID))
		}
	}
	if e.logContains(j.ID, "", "subscription #704") {
		t.Errorf("expired subscriptions are not matched")
	}

	// politeness: every request but the first was preceded by a pause
	pauses := 0
	for _, d := range e.recordedSleeps() {
		if d == 100*time.Millisecond {
			pauses++
		}
	}
	if total := fs.calls("/v1/"); pauses < total-1 {
		t.Errorf("%d Stripe requests but only %d pauses", total, pauses)
	}
	c := progressCounts(j)
	if c["customers_linked"] != 2 || c["customers_unmatched"] != 1 || c["customers_ambiguous"] != 1 || c["variants_linked"] != 3 || c["prices_linked"] != 3 || c["subscriptions_linked"] != 1 {
		t.Errorf("counts = %v", c)
	}
}

func TestAutoMatchRespectsOptionsAndIsRepeatable(t *testing.T) {
	fs := newFakeStripe(t)
	stripeFixture(fs)
	e := newEnv(t, fs.opt())
	seedMatchData(e)

	job, err := e.r.StartAutoMatch("admin", MatchOptions{Customers: true})
	e.mustFinish(job, err, "completed")
	if fs.calls("/v1/products") != 0 || fs.calls("/v1/prices") != 0 || fs.calls("/v1/subscriptions") != 0 {
		t.Errorf("only customers were requested: %v", fs.requests)
	}
	var v db.Variant
	e.g.First(&v, 200)
	if v.StripeProductID != "" {
		t.Errorf("products must not be touched")
	}

	// a second run only looks at what is still unlinked
	before := fs.calls("/v1/customers")
	job, err = e.r.StartAutoMatch("admin", MatchOptions{Customers: true})
	e.mustFinish(job, err, "completed")
	if got := fs.calls("/v1/customers") - before; got != 2 { // only 13 (no match) and 15 (error) are still unlinked
		t.Errorf("second run queried %d customers, want 2 (13 and 15)", got)
	}

	if _, err := e.r.StartAutoMatch("admin", MatchOptions{}); err == nil {
		t.Errorf("selecting nothing must be rejected")
	}
}

func TestAutoMatchStripeNotConfigured(t *testing.T) {
	e := newEnv(t)
	if _, err := e.r.StartAutoMatch("admin", MatchOptions{Customers: true}); err == nil {
		t.Errorf("a missing Stripe key must be reported when starting")
	}
}

func TestAutoMatchStripeAuthFailureFailsJob(t *testing.T) {
	fs := newFakeStripe(t)
	fs.status = http.StatusUnauthorized
	e := newEnv(t, fs.opt())
	seedMatchData(e)
	job, err := e.r.StartAutoMatch("admin", MatchOptions{Customers: true})
	j := e.mustFinish(job, err, "failed")
	if !strings.Contains(j.Error, "customers") {
		t.Errorf("error = %q", j.Error)
	}
	if fs.calls("/v1/customers") != 1 {
		t.Errorf("an authentication failure must stop the run immediately, made %d calls", fs.calls("/v1/customers"))
	}
}

func TestMappingReport(t *testing.T) {
	e, _, _ := runFullImport(t)
	// give one of each a Stripe id: they must disappear from the report
	e.g.Model(&db.Customer{}).Where("id = ?", 11).UpdateColumn("stripe_customer_id", "cus_Abc12345")
	e.g.Model(&db.Variant{}).Where("id = ?", 200).UpdateColumn("stripe_product_id", "prod_Abc12345")
	e.g.Model(&db.Price{}).Where("id = ?", 300).UpdateColumn("stripe_price_id", "price_Abc12345")
	e.g.Model(&db.Subscription{}).Where("id = ?", 700).UpdateColumn("stripe_subscription_id", "sub_Abc12345")
	e.g.Create(&db.Subscription{ID: 799, StoreID: 1, CustomerID: 11, Status: "expired", UserEmail: "gone@example.com"})
	e.g.Model(&db.Customer{}).Where("id = ?", 12).UpdateColumn("name", `Roe, "Jane"`)

	var out strings.Builder
	if err := e.r.MappingReport(&out); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(out.String())).ReadAll()
	if err != nil {
		t.Fatalf("the report must be valid CSV: %v\n%s", err, out.String())
	}
	if strings.Join(rows[0], ",") != "type,id,name_or_email,store_id,detail" {
		t.Errorf("header = %v", rows[0])
	}
	got := map[string][]string{}
	for _, r := range rows[1:] {
		if len(r) != 5 {
			t.Fatalf("row %v has %d fields", r, len(r))
		}
		got[r[0]+":"+r[1]] = r
	}
	wantPresent := []string{"customer:12", "customer:21", "variant:201", "variant:202", "variant:210", "price:301", "price:302", "price:310", "subscription:701"}
	wantAbsent := []string{"customer:11", "variant:200", "price:300", "subscription:700", "subscription:799"}
	for _, k := range wantPresent {
		if _, ok := got[k]; !ok {
			t.Errorf("report lacks %s\n%s", k, out.String())
		}
	}
	for _, k := range wantAbsent {
		if _, ok := got[k]; ok {
			t.Errorf("report must not list %s", k)
		}
	}
	if r := got["customer:12"]; r[2] != "jane@example.com" || r[3] != "1" || !strings.Contains(r[4], `Roe, "Jane"`) {
		t.Errorf("customer row = %v", r)
	}
	if r := got["variant:201"]; r[2] != "Pro App - Monthly" || r[3] != "1" {
		t.Errorf("variant row = %v", r)
	}
	if r := got["variant:210"]; r[3] != "2" {
		t.Errorf("variant of the second store = %v", r)
	}
	if r := got["subscription:701"]; r[2] != "john@example.com" || !strings.Contains(r[4], "paypal") {
		t.Errorf("subscription row = %v", r)
	}
	if r := got["price:301"]; !strings.Contains(r[4], "999 USD") || !strings.Contains(r[4], "every 1 month") {
		t.Errorf("price row = %v", r)
	}
}

func TestMappingReportEmptyDatabase(t *testing.T) {
	e := newEnv(t)
	var out strings.Builder
	if err := e.r.MappingReport(&out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "type,id,name_or_email,store_id,detail" {
		t.Errorf("report = %q", out.String())
	}
}

func TestListSubscriptionsPaginatesAndAsksForAllStatuses(t *testing.T) {
	fs := newFakeStripe(t)
	for i := 0; i < 5; i++ {
		fs.subs["cus_Many"] = append(fs.subs["cus_Many"], stripeSub("sub_"+itoa(int64(i)), "cus_Many", "price_M1", "prod_PM", 1))
	}
	fs.subsPage = 2
	e := newEnv(t, fs.opt())
	cl, err := e.svc.Stripe()
	if err != nil {
		t.Fatal(err)
	}
	subs, err := cl.ListSubscriptions(t.Context(), "cus_Many")
	if err != nil || len(subs) != 5 || subs[4].ID != "sub_4" {
		t.Fatalf("subs = %d err = %v", len(subs), err)
	}
	if fs.calls("/v1/subscriptions?customer=cus_Many&limit=100&status=all") != 1 || fs.calls("/v1/subscriptions") != 3 {
		t.Errorf("requests = %v", fs.requests)
	}
}
