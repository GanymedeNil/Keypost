package importer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"fulfillment/internal/db"
	"fulfillment/internal/service"
)

func pick(m map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}

func runFullImport(t *testing.T) (*env, *fakeLS, db.ImportJob) {
	t.Helper()
	e := newEnv(t)
	f := newFakeLS(t, "lsk_secret_abcd")
	loadFixtures(f)
	job, err := e.r.StartLemonSqueezyAPI("admin@example.com", apiOptions(f))
	j := e.mustFinish(job, err, "completed")
	return e, f, j
}

func TestAPIImportFull(t *testing.T) {
	e, f, j := runFullImport(t)

	// job bookkeeping
	if j.Kind != "lemonsqueezy_api" || j.CreatedBy != "admin@example.com" || j.StartedAt == nil || j.FinishedAt == nil {
		t.Errorf("unexpected job row: %+v", j)
	}
	if got := j.Options.V["api_key"]; got != "****abcd" {
		t.Errorf("api key must be redacted to its last 4 characters, got %v", got)
	}
	raw, _ := json.Marshal(j.Options.V)
	if strings.Contains(string(raw), "secret") {
		t.Errorf("options leak the API key: %s", raw)
	}
	if stage := j.Progress.V["stage"]; stage != "license_key_instances" {
		t.Errorf("final stage = %v", stage)
	}
	if n := progressCounts(j)["orders"]; n != 5 {
		t.Errorf("progress counts orders = %d", n)
	}

	want := map[string]int64{
		"stores": 2, "customers": 3, "products": 3, "variants": 4, "prices": 4, "files": 2, "orders": 5, "order_items": 5,
		"subscriptions": 2, "subscription_items": 3, "subscription_invoices": 2, "license_keys": 4, "license_key_instances": 1,
	}
	if got := e.counts(); !reflect.DeepEqual(got, want) {
		t.Fatalf("row counts = %v, want %v", got, want)
	}

	// original ids and fields survive
	var o db.Order
	if err := e.g.First(&o, 500).Error; err != nil {
		t.Fatal(err)
	}
	if o.Identifier != orderIdent1 || o.OrderNumber != 1 || o.StoreID != 1 || o.CustomerID != 11 || o.Source != "lemonsqueezy" || o.StripeCheckoutSession != nil {
		t.Errorf("order 500 = %+v", o)
	}
	if o.Total != 5880 || o.Tax != 980 || o.TaxName == nil || *o.TaxName != "VAT" || o.TaxRate != "20.00" || o.Status != "paid" {
		t.Errorf("order amounts = %+v", o)
	}
	var refunded db.Order
	e.g.First(&refunded, 502)
	if !refunded.Refunded || refunded.Status != "refunded" || refunded.RefundedAmount != 1500 || refunded.RefundedAt == nil ||
		refunded.AffiliateID == nil || *refunded.AffiliateID != 7 || refunded.ReferralAmount == nil || *refunded.ReferralAmount != 150 {
		t.Errorf("refunded order = %+v", refunded)
	}
	var o501 db.Order
	e.g.First(&o501, 501)
	if o501.CardBrand != "visa" || o501.CardLastFour != "4242" {
		t.Errorf("order 501 should take its card from the subscription: %q %q", o501.CardBrand, o501.CardLastFour)
	}

	var variant db.Variant
	e.g.First(&variant, 200)
	if !variant.HasLicenseKeys || variant.LicenseActivationLimit != 3 || variant.IsLicenseLimitUnlimited || len(variant.Links.V) != 1 {
		t.Errorf("variant 200 = %+v", variant)
	}
	var eur db.Price
	e.g.First(&eur, 310)
	var usd db.Price
	e.g.First(&usd, 300)
	if eur.Currency != "EUR" || usd.Currency != "USD" {
		t.Errorf("price currency should follow the store: %s %s", eur.Currency, usd.Currency)
	}
	var sub db.Subscription
	e.g.First(&sub, 700)
	if sub.StripeSubscriptionID != nil || sub.PriceID == nil || *sub.PriceID != 301 || sub.Quantity != 5 || sub.PaymentProcessor != "stripe" || sub.BillingAnchor != 12 {
		t.Errorf("subscription 700 = %+v", sub)
	}

	// license keys: derived variant / subscription, verbatim key, nil = unlimited
	var k0, k1, k3 db.LicenseKey
	e.g.First(&k0, 1000)
	e.g.First(&k1, 1001)
	e.g.First(&k3, 1003)
	if k0.Key != keyActive || k0.VariantID != 200 || k0.SubscriptionID != nil || k0.ActivationLimit == nil || *k0.ActivationLimit != 3 || k0.InstancesCount != 1 {
		t.Errorf("key 1000 = %+v", k0)
	}
	if k1.VariantID != 201 || k1.SubscriptionID == nil || *k1.SubscriptionID != 700 || k1.ActivationLimit != nil {
		t.Errorf("key 1001 = %+v", k1)
	}
	if k3.SubscriptionID == nil || *k3.SubscriptionID != 701 || k3.ExpiresAt == nil {
		t.Errorf("key 1003 = %+v", k3)
	}

	// files are downloaded into storage
	var file db.File
	e.g.First(&file, 400)
	if file.StorageDriver != "local" || file.StorageKey == "" || file.Size != int64(len(fileBytes1)) || file.ContentType != "application/zip" || file.Identifier != fileIdent1 {
		t.Fatalf("file 400 = %+v", file)
	}
	rc, size, err := e.store.Open(context.Background(), file.StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(rc)
	rc.Close()
	sum := sha256.Sum256(fileBytes1)
	if string(body) != string(fileBytes1) || size != int64(len(fileBytes1)) || file.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("stored file differs from the download")
	}
	for _, a := range f.authSeen {
		if a != "" {
			t.Errorf("the API key must not be sent to download URLs, saw %q", a)
		}
	}

	// the PayPal subscription is imported but flagged
	if !e.logContains(j.ID, "warn", "PayPal") {
		t.Errorf("expected a PayPal warning:\n%s", e.logDump(j.ID))
	}
	if progressWarnings(j) < 1 {
		t.Errorf("warnings = %d", progressWarnings(j))
	}
}

func TestAPIImportRoundTripShape(t *testing.T) {
	e, f, _ := runFullImport(t)
	res := e.svc.Res
	fix := func(resName string, id string) map[string]any {
		o := f.object(resName, id)
		if o == nil {
			t.Fatalf("no fixture %s/%s", resName, id)
		}
		return attrOf(o)
	}
	load := func(model any, id int64) {
		if err := e.g.First(model, id).Error; err != nil {
			t.Fatal(err)
		}
	}

	for _, id := range []int64{500, 501, 502, 503} {
		var o db.Order
		load(&o, id)
		got := attrsOf(t, res.Orders([]db.Order{o})[0])
		assertSubset(t, "order "+o.Identifier, fix("orders", itoa(id)), got, "urls")
	}
	for _, id := range []int64{1000, 1001, 1002, 1003} {
		var k db.LicenseKey
		load(&k, id)
		got := attrsOf(t, res.LicenseKeys([]db.LicenseKey{k})[0])
		assertSubset(t, "license key "+k.Key, fix("license-keys", itoa(id)), got)
		if got["status"] != fix("license-keys", itoa(id))["status"] {
			t.Errorf("license key %d status = %v", id, got["status"])
		}
	}
	for _, id := range []int64{700, 701} {
		var s db.Subscription
		load(&s, id)
		got := attrsOf(t, res.Subscriptions([]db.Subscription{s})[0])
		assertSubset(t, "subscription "+itoa(id), fix("subscriptions", itoa(id)), got, "urls", "status_formatted")
	}
	for _, id := range []int64{900, 901} {
		var inv db.SubscriptionInvoice
		load(&inv, id)
		assertSubset(t, "invoice "+itoa(id), fix("subscription-invoices", itoa(id)), attrsOf(t, res.SubscriptionInvoices([]db.SubscriptionInvoice{inv})[0]))
	}
	var it db.OrderItem
	load(&it, 601)
	assertSubset(t, "order item", fix("order-items", "601"), attrsOf(t, res.OrderItems([]db.OrderItem{it})[0]))
	var si db.SubscriptionItem
	load(&si, 800)
	assertSubset(t, "subscription item", fix("subscription-items", "800"), attrsOf(t, res.SubscriptionItems([]db.SubscriptionItem{si})[0]))
	var inst db.LicenseKeyInstance
	load(&inst, 1100)
	assertSubset(t, "instance", fix("license-key-instances", "1100"), attrsOf(t, res.LicenseKeyInstances([]db.LicenseKeyInstance{inst})[0]))

	var c db.Customer
	load(&c, 11)
	assertSubset(t, "customer", pick(fix("customers", "11"), "store_id", "name", "email", "status", "city", "region", "country", "created_at", "updated_at", "test_mode"),
		attrsOf(t, res.Customers([]db.Customer{c})[0]))
	var p db.Product
	load(&p, 100)
	assertSubset(t, "product", pick(fix("products", "100"), "store_id", "name", "slug", "description", "status", "thumb_url", "large_thumb_url", "created_at", "updated_at", "test_mode"),
		attrsOf(t, res.Products([]db.Product{p})[0]))
	for _, id := range []int64{200, 201} {
		var v db.Variant
		load(&v, id)
		assertSubset(t, "variant "+itoa(id), fix("variants", itoa(id)), attrsOf(t, res.Variants([]db.Variant{v})[0]))
	}
	for _, id := range []int64{300, 301} {
		var pr db.Price
		load(&pr, id)
		assertSubset(t, "price "+itoa(id), fix("prices", itoa(id)), attrsOf(t, res.Prices([]db.Price{pr})[0]))
	}
	var fl db.File
	load(&fl, 400)
	assertSubset(t, "file", pick(fix("files", "400"), "variant_id", "identifier", "name", "extension", "size", "version", "sort", "status", "createdAt", "updatedAt", "test_mode"),
		attrsOf(t, res.Files([]db.File{fl})[0]))
	var st db.Store
	load(&st, 1)
	assertSubset(t, "store", pick(fix("stores", "1"), "name", "slug", "domain", "url", "plan", "country", "currency", "created_at", "updated_at"), attrsOf(t, res.Store(st, false)))
}

func itoa(n int64) string { return normalizeID(n) }

func snapshot(e *env) map[string]any {
	out := map[string]any{}
	add := func(name string, rows any) {
		e.g.Order("id").Find(rows)
		out[name] = normalize(e.t, rows)
	}
	add("stores", &[]db.Store{})
	add("customers", &[]db.Customer{})
	add("products", &[]db.Product{})
	add("variants", &[]db.Variant{})
	add("prices", &[]db.Price{})
	add("files", &[]db.File{})
	add("orders", &[]db.Order{})
	add("order_items", &[]db.OrderItem{})
	add("subscriptions", &[]db.Subscription{})
	add("subscription_items", &[]db.SubscriptionItem{})
	add("subscription_invoices", &[]db.SubscriptionInvoice{})
	add("license_keys", &[]db.LicenseKey{})
	add("license_key_instances", &[]db.LicenseKeyInstance{})
	return out
}

func TestAPIImportIdempotent(t *testing.T) {
	e, f, _ := runFullImport(t)
	before := snapshot(e)
	counts := e.counts()

	job, err := e.r.StartLemonSqueezyAPI("admin", apiOptions(f))
	j := e.mustFinish(job, err, "completed")
	if got := e.counts(); !reflect.DeepEqual(got, counts) {
		t.Fatalf("a second run changed the row counts: %v -> %v", counts, got)
	}
	after := snapshot(e)
	for k := range before {
		if !reflect.DeepEqual(before[k], after[k]) {
			t.Errorf("table %s differs after the second import", k)
		}
	}
	if n := f.downloads("/download/" + fileIdent1); n != 1 {
		t.Errorf("the file was downloaded %d times; an existing object must be kept", n)
	}
	if n := progressCounts(j)["files_kept"]; n != 2 {
		t.Errorf("files_kept = %d", n)
	}
}

func TestAPIImportKeepsLocalState(t *testing.T) {
	e, f, _ := runFullImport(t)

	// state added after the first import (mappings, settings, counters) must survive a re-run
	e.g.Model(&db.Customer{}).Where("id = ?", 11).UpdateColumn("stripe_customer_id", "cus_Keep111")
	e.g.Model(&db.Variant{}).Where("id = ?", 200).UpdateColumn("stripe_product_id", "prod_Keep111")
	e.g.Model(&db.Price{}).Where("id = ?", 300).UpdateColumn("stripe_price_id", "price_Keep111")
	e.g.Model(&db.Subscription{}).Where("id = ?", 700).UpdateColumn("stripe_subscription_id", "sub_Keep111")
	e.g.Model(&db.Store{}).Where("id = ?", 1).UpdateColumn("support_email", "help@example.com")
	e.g.Model(&db.File{}).Where("id = ?", 400).UpdateColumn("downloads_count", 9)
	e.g.Model(&db.Order{}).Where("id = ?", 500).UpdateColumns(map[string]any{"stripe_payment_intent": "pi_Keep111", "card_brand": "mastercard"})

	job, err := e.r.StartLemonSqueezyAPI("admin", apiOptions(f))
	e.mustFinish(job, err, "completed")

	var c db.Customer
	e.g.First(&c, 11)
	var v db.Variant
	e.g.First(&v, 200)
	var p db.Price
	e.g.First(&p, 300)
	var s db.Subscription
	e.g.First(&s, 700)
	var st db.Store
	e.g.First(&st, 1)
	var fl db.File
	e.g.First(&fl, 400)
	var o db.Order
	e.g.First(&o, 500)
	if c.StripeCustomerID != "cus_Keep111" || v.StripeProductID != "prod_Keep111" || p.StripePriceID != "price_Keep111" ||
		s.StripeSubscriptionID == nil || *s.StripeSubscriptionID != "sub_Keep111" || st.SupportEmail != "help@example.com" ||
		fl.DownloadsCount != 9 || o.StripePaymentIntent != "pi_Keep111" || o.CardBrand != "mastercard" {
		t.Errorf("a re-run overwrote local state: customer=%q variant=%q price=%q sub=%v store=%q downloads=%d pi=%q card=%q",
			c.StripeCustomerID, v.StripeProductID, p.StripePriceID, s.StripeSubscriptionID, st.SupportEmail, fl.DownloadsCount, o.StripePaymentIntent, o.CardBrand)
	}
}

func TestLicenseSemanticsAfterImport(t *testing.T) {
	e, _, _ := runFullImport(t)
	ctx := context.Background()
	svc := e.svc

	// limit 3, one imported activation: two more fit, the next is refused
	a1 := svc.ActivateLicense(ctx, keyActive, "second")
	a2 := svc.ActivateLicense(ctx, keyActive, "third")
	if !a1.OK || !a2.OK {
		t.Fatalf("activations should fit: %+v %+v", a1, a2)
	}
	over := svc.ActivateLicense(ctx, keyActive, "fourth")
	if over.OK || over.HTTP != http.StatusBadRequest || over.Err != "This license key has reached the activation limit." || over.Instance != nil {
		t.Fatalf("limit not enforced: %+v", over)
	}

	// the imported instance id is still valid and can be deactivated
	v := svc.ValidateLicense(ctx, keyActive, instIdent)
	if !v.OK || v.Instance == nil || v.Instance.Name != "laptop" || v.Meta == nil || v.Meta.VariantName != "Default" || v.Meta.ProductName != "Pro App" || v.Meta.CustomerEmail != "john@example.com" {
		t.Fatalf("validate imported instance: %+v meta=%+v", v, v.Meta)
	}
	d := svc.DeactivateLicense(ctx, keyActive, instIdent)
	if !d.OK {
		t.Fatalf("deactivate: %+v", d)
	}
	var k db.LicenseKey
	e.g.First(&k, 1000)
	if k.InstancesCount != 2 {
		t.Errorf("instances_count = %d, want 2", k.InstancesCount)
	}
	if again := svc.ActivateLicense(ctx, keyActive, "fifth"); !again.OK {
		t.Errorf("a freed activation must be usable: %+v", again)
	}

	// unlimited (null limit) keys never hit a limit
	for i := 0; i < 8; i++ {
		if r := svc.ActivateLicense(ctx, keyUnlim, "device"); !r.OK {
			t.Fatalf("unlimited key refused activation %d: %+v", i, r)
		}
	}
	if r := svc.ActivateLicense(ctx, keyDisabled, "x"); r.OK || r.Err != "This license key is disabled." {
		t.Errorf("disabled key: %+v", r)
	}
	if r := svc.ActivateLicense(ctx, keyExpired, "x"); r.OK || r.Err != "This license key has expired." {
		t.Errorf("expired key: %+v", r)
	}
	if r := svc.ValidateLicense(ctx, keyExpired, ""); r.OK {
		t.Errorf("expired key validates: %+v", r)
	}
}

func TestAPIImportDryRunWritesNothing(t *testing.T) {
	e := newEnv(t)
	f := newFakeLS(t, "lsk_secret_abcd")
	loadFixtures(f)
	o := apiOptions(f)
	o.DryRun = true
	job, err := e.r.StartLemonSqueezyAPI("admin", o)
	j := e.mustFinish(job, err, "completed")
	for table, n := range e.counts() {
		if n != 0 {
			t.Errorf("dry run wrote %d %s", n, table)
		}
	}
	if n := f.downloads("/download/" + fileIdent1); n != 0 {
		t.Errorf("dry run downloaded files (%d)", n)
	}
	if c := progressCounts(j); c["orders"] != 5 || c["license_keys"] != 4 || c["customers"] != 3 {
		t.Errorf("dry run must still count what it saw: %v", c)
	}
	if j.Progress.V["dry_run"] != true {
		t.Errorf("progress should say dry_run: %v", j.Progress.V)
	}
	if n := e.storedObjects(); n != 0 {
		t.Errorf("dry run stored %d object(s)", n)
	}
}

func TestAPIImportResourceAndStoreFilters(t *testing.T) {
	e := newEnv(t)
	f := newFakeLS(t, "lsk_secret_abcd")
	loadFixtures(f)

	o := apiOptions(f)
	o.StoreIDs = []int64{1}
	job, err := e.r.StartLemonSqueezyAPI("admin", o)
	e.mustFinish(job, err, "completed")
	got := e.counts()
	want := map[string]int64{
		"stores": 1, "customers": 2, "products": 2, "variants": 3, "prices": 3, "files": 2, "orders": 4, "order_items": 4,
		"subscriptions": 2, "subscription_items": 3, "subscription_invoices": 2, "license_keys": 4, "license_key_instances": 1,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("store-scoped import = %v, want %v", got, want)
	}
	sawFilter := false
	for _, r := range f.apiRequests() {
		if strings.HasPrefix(r, "/v1/orders") && strings.Contains(r, "filter%5Bstore_id%5D=1") {
			sawFilter = true
		}
		if strings.HasPrefix(r, "/v1/stores") && strings.Contains(r, "filter") {
			t.Errorf("stores cannot be filtered: %s", r)
		}
	}
	if !sawFilter {
		t.Errorf("orders were not requested with filter[store_id]: %v", f.apiRequests())
	}

	// several stores: one filtered request series per store
	e3 := newEnv(t)
	o3 := apiOptions(f)
	o3.StoreIDs = []int64{1, 2}
	o3.Resources = []string{"customers", "orders"}
	job, err = e3.r.StartLemonSqueezyAPI("admin", o3)
	e3.mustFinish(job, err, "completed")
	if e3.count(&db.Customer{}) != 3 || e3.count(&db.Order{}) != 5 {
		t.Errorf("two-store import: customers=%d orders=%d", e3.count(&db.Customer{}), e3.count(&db.Order{}))
	}

	// resources filter on a fresh database
	e2 := newEnv(t)
	o2 := apiOptions(f)
	o2.Resources = []string{"customers", "license-keys"}
	job, err = e2.r.StartLemonSqueezyAPI("admin", o2)
	e2.mustFinish(job, err, "completed")
	got = e2.counts()
	for table, n := range got {
		switch table {
		case "customers":
			if n != 3 {
				t.Errorf("customers = %d", n)
			}
		case "license_keys":
			if n != 4 {
				t.Errorf("license keys = %d", n)
			}
		case "license_key_instances":
			if n != 1 {
				t.Errorf("instances come with their keys: %d", n)
			}
		default:
			if n != 0 {
				t.Errorf("%s = %d, want nothing", table, n)
			}
		}
	}
	if _, err := e2.r.StartLemonSqueezyAPI("admin", LSOptions{APIKey: "k", BaseURL: f.URL(), Resources: []string{"unicorns"}}); err == nil {
		t.Errorf("an unknown resource must be rejected")
	}
	if _, err := e2.r.StartLemonSqueezyAPI("admin", LSOptions{BaseURL: f.URL()}); err == nil {
		t.Errorf("a missing API key must be rejected")
	}
}

func TestAPIFilesDownloadFailureAndDisabled(t *testing.T) {
	e := newEnv(t)
	f := newFakeLS(t, "lsk_secret_abcd")
	loadFixtures(f)
	f.setFile("/download/"+fileIdent2, nil) // the second file 404s
	job, err := e.r.StartLemonSqueezyAPI("admin", apiOptions(f))
	j := e.mustFinish(job, err, "completed")
	var ok, broken db.File
	e.g.First(&ok, 400)
	e.g.First(&broken, 401)
	if ok.StorageKey == "" || broken.StorageKey != "" || broken.Status != "published" || broken.Identifier != fileIdent2 {
		t.Errorf("files: ok=%+v broken=%+v", ok, broken)
	}
	if !e.logContains(j.ID, "warn", fileIdent2) {
		t.Errorf("the failed download must be logged with the identifier:\n%s", e.logDump(j.ID))
	}

	// re-run with the file available: only the missing one is fetched
	f.setFile("/download/"+fileIdent2, fileBytes2)
	job, err = e.r.StartLemonSqueezyAPI("admin", apiOptions(f))
	e.mustFinish(job, err, "completed")
	e.g.First(&broken, 401)
	if broken.StorageKey == "" || f.downloads("/download/"+fileIdent1) != 1 {
		t.Errorf("re-run should fetch only the missing file: key=%q hits=%d", broken.StorageKey, f.downloads("/download/"+fileIdent1))
	}

	// download disabled: rows are created, nothing is downloaded, a warning remains
	e2 := newEnv(t)
	o := apiOptions(f)
	o.DownloadFiles = false
	hitsBefore := f.downloads("/download/" + fileIdent1)
	job, err = e2.r.StartLemonSqueezyAPI("admin", o)
	j2 := e2.mustFinish(job, err, "completed")
	if e2.count(&db.File{}) != 2 || f.downloads("/download/"+fileIdent1) != hitsBefore || !e2.logContains(j2.ID, "warn", "app.zip") {
		t.Errorf("download_files=false misbehaved")
	}
}

func TestAPIAuthFailureFailsJob(t *testing.T) {
	e := newEnv(t)
	f := newFakeLS(t, "right-key")
	loadFixtures(f)
	o := apiOptions(f)
	o.APIKey = "wrong-key-1234"
	job, err := e.r.StartLemonSqueezyAPI("admin", o)
	j := e.mustFinish(job, err, "failed")
	if !strings.Contains(j.Error, "401") || !strings.Contains(j.Error, "Unauthenticated") {
		t.Errorf("error should explain the rejection: %q", j.Error)
	}
	if e.count(&db.Order{}) != 0 {
		t.Errorf("nothing should be imported")
	}
}

func TestAPIRetriesAndRateLimit(t *testing.T) {
	e := newEnv(t)
	f := newFakeLS(t, "lsk_secret_abcd")
	loadFixtures(f)
	f.setFailures("customers", 429, 500, 502)
	f.setFailures("orders", 429)
	job, err := e.r.StartLemonSqueezyAPI("admin", apiOptions(f))
	e.mustFinish(job, err, "completed")
	if e.count(&db.Customer{}) != 3 || e.count(&db.Order{}) != 5 {
		t.Errorf("retries lost data")
	}
	var sawRetryAfter bool
	for _, d := range e.recordedSleeps() {
		if d == time.Second {
			sawRetryAfter = true
		}
	}
	if !sawRetryAfter {
		t.Errorf("Retry-After: 1 was not honoured: %v", e.recordedSleeps())
	}

	// a persistent 500 eventually fails the job
	e2 := newEnv(t)
	f2 := newFakeLS(t, "lsk_secret_abcd")
	loadFixtures(f2)
	f2.setFailures("products", 500, 500, 500, 500, 500, 500, 500)
	job, err = e2.r.StartLemonSqueezyAPI("admin", apiOptions(f2))
	j := e2.mustFinish(job, err, "failed")
	if !strings.Contains(j.Error, "products") {
		t.Errorf("error should name the resource: %q", j.Error)
	}
	// earlier stages were committed and a re-run continues
	if e2.count(&db.Customer{}) != 3 {
		t.Errorf("customers imported before the failure should be kept")
	}
}

func TestAPIClientStaysUnderRateLimit(t *testing.T) {
	e := newEnv(t)
	var mu sync.Mutex
	var sleeps int
	e.r.rateLimit, e.r.rateWindow = 4, 120*time.Millisecond
	e.r.sleep = func(ctx context.Context, d time.Duration) error {
		mu.Lock()
		sleeps++
		mu.Unlock()
		return sleepCtx(ctx, min(d, 20*time.Millisecond))
	}
	f := newFakeLS(t, "lsk_secret_abcd")
	loadFixtures(f)
	var stamps []time.Time
	f.setHook(func(w http.ResponseWriter, r *http.Request, res string, page int) bool {
		mu.Lock()
		stamps = append(stamps, time.Now())
		mu.Unlock()
		return false
	})
	job, err := e.r.StartLemonSqueezyAPI("admin", apiOptions(f))
	e.mustFinish(job, err, "completed")
	mu.Lock()
	defer mu.Unlock()
	if sleeps == 0 {
		t.Fatalf("the client never waited although it made %d requests with a limit of 4", len(stamps))
	}
	for i := 4; i < len(stamps); i++ {
		if gap := stamps[i].Sub(stamps[i-4]); gap < 60*time.Millisecond {
			t.Fatalf("request %d came %s after request %d: more than 4 requests inside the 120ms window", i, gap, i-4)
		}
	}
}

func TestAPIPaginationFollowsLinks(t *testing.T) {
	e := newEnv(t)
	f := newFakeLS(t, "lsk_secret_abcd")
	var customers []map[string]any
	for i := int64(1); i <= 250; i++ {
		customers = append(customers, jobj("customers", i, map[string]any{"store_id": 1, "name": "C", "email": "c" + itoa(i) + "@example.com", "created_at": tsA, "updated_at": tsA}))
	}
	f.set("customers", customers...)
	o := apiOptions(f)
	o.Resources = []string{"customers"}
	job, err := e.r.StartLemonSqueezyAPI("admin", o)
	j := e.mustFinish(job, err, "completed")
	if n := e.count(&db.Customer{}); n != 250 {
		t.Fatalf("imported %d of 250 customers", n)
	}
	pages := 0
	for _, r := range f.apiRequests() {
		if strings.HasPrefix(r, "/v1/customers") {
			pages++
			if !strings.Contains(r, "page%5Bsize%5D=100") && !strings.Contains(r, "page[size]=100") {
				t.Errorf("page size not requested: %s", r)
			}
		}
	}
	if pages != 3 {
		t.Errorf("requested %d pages, want 3", pages)
	}
	if tot, _ := j.Progress.V["total"].(float64); tot != 250 {
		t.Errorf("progress total = %v", j.Progress.V["total"])
	}
}

func TestAPILinksNextToForeignHostIsRebased(t *testing.T) {
	e := newEnv(t)
	f := newFakeLS(t, "lsk_secret_abcd")
	var customers []map[string]any
	for i := int64(1); i <= 150; i++ {
		customers = append(customers, jobj("customers", i, map[string]any{"store_id": 1, "name": "C", "email": "c" + itoa(i) + "@example.com"}))
	}
	f.set("customers", customers...)
	// rewrite the next link so it points at a host that must never be contacted
	f.setHook(func(w http.ResponseWriter, r *http.Request, res string, page int) bool {
		if page != 1 {
			return false
		}
		doc := map[string]any{
			"meta":  map[string]any{"page": map[string]any{"currentPage": 1, "lastPage": 2, "total": 150}},
			"links": map[string]any{"next": "https://evil.invalid/v1/customers?page%5Bnumber%5D=2&page%5Bsize%5D=100"},
			"data":  customers[50:],
		}
		json.NewEncoder(w).Encode(doc)
		return true
	})
	o := apiOptions(f)
	o.Resources = []string{"customers"}
	job, err := e.r.StartLemonSqueezyAPI("admin", o)
	e.mustFinish(job, err, "completed")
	if n := e.count(&db.Customer{}); n != 150 {
		t.Errorf("customers = %d, want 150 (page 2 must be fetched from the configured host)", n)
	}
}

func TestCancelAndSingleRunningJob(t *testing.T) {
	e := newEnv(t)
	f := newFakeLS(t, "lsk_secret_abcd")
	var customers, orders []map[string]any
	for i := int64(1); i <= 120; i++ {
		customers = append(customers, jobj("customers", i, map[string]any{"store_id": 1, "name": "C", "email": "c" + itoa(i) + "@example.com"}))
	}
	f.set("customers", customers...)
	f.set("orders", orders...)
	reached := make(chan struct{})
	var once sync.Once
	f.setHook(func(w http.ResponseWriter, r *http.Request, res string, page int) bool {
		if res == "customers" && page == 2 {
			once.Do(func() { close(reached) })
			<-r.Context().Done() // hang until the client gives up
			return true
		}
		return false
	})
	o := apiOptions(f)
	o.Resources = []string{"customers"}
	job, err := e.r.StartLemonSqueezyAPI("admin", o)
	if err != nil {
		t.Fatal(err)
	}
	<-reached

	// a second import of the same kind is refused while one runs
	_, err2 := e.r.StartLemonSqueezyAPI("admin", o)
	if se, ok := service.AsError(err2); !ok || se.Status != http.StatusConflict {
		t.Fatalf("second start = %v, want a conflict", err2)
	}
	// ...but other kinds are fine
	if !e.r.Cancel(job.ID) {
		t.Fatalf("Cancel returned false for a running job")
	}
	j := e.mustFinish(job, nil, "cancelled")
	if j.FinishedAt == nil {
		t.Errorf("a cancelled job needs finished_at")
	}
	if e.r.Cancel(job.ID) || e.r.Cancel(99999) {
		t.Errorf("Cancel must report false for finished or unknown jobs")
	}
	// rows read before the cancellation were still written
	if n := e.count(&db.Customer{}); n == 0 {
		t.Errorf("the first page should have been stored before cancelling")
	}
	// and a new job can start afterwards
	f.setHook(nil)
	job, err = e.r.StartLemonSqueezyAPI("admin", o)
	e.mustFinish(job, err, "completed")
	if n := e.count(&db.Customer{}); n != 120 {
		t.Errorf("customers after re-run = %d", n)
	}
}

func TestPanicBecomesFailedJob(t *testing.T) {
	e := newEnv(t)
	job, err := e.r.start("lemonsqueezy_api", "admin", map[string]any{}, nil, func(ctx context.Context, j *jobCtl) error {
		var m map[string]int
		m["boom"] = 1
		return nil
	})
	j := e.mustFinish(job, err, "failed")
	if !strings.Contains(j.Error, "panic") {
		t.Errorf("error = %q", j.Error)
	}
	// the kind is free again
	job, err = e.r.start("lemonsqueezy_api", "admin", map[string]any{}, nil, func(ctx context.Context, j *jobCtl) error { return nil })
	e.mustFinish(job, err, "completed")
}

func TestRecoverInterrupted(t *testing.T) {
	e := newEnv(t)
	now := db.Now()
	stuck := db.ImportJob{Kind: "lemonsqueezy_api", Status: "running", StartedAt: &now}
	done := db.ImportJob{Kind: "lemonsqueezy_api", Status: "completed", StartedAt: &now, FinishedAt: &now}
	e.g.Create(&stuck)
	e.g.Create(&done)
	e.r.RecoverInterrupted()
	var a, b db.ImportJob
	e.g.First(&a, stuck.ID)
	e.g.First(&b, done.ID)
	if a.Status != "failed" || a.Error != "interrupted by restart" || a.FinishedAt == nil {
		t.Errorf("stuck job = %+v", a)
	}
	if b.Status != "completed" {
		t.Errorf("finished job must be untouched: %+v", b)
	}
}

func TestMalformedAPIDataDoesNotPanic(t *testing.T) {
	e := newEnv(t)
	f := newFakeLS(t, "lsk_secret_abcd")
	junk := func(res string) []map[string]any {
		return []map[string]any{
			{"type": res, "id": "1", "attributes": nil},
			{"type": res, "id": "2", "attributes": map[string]any{}},
			{"type": res, "id": "three", "attributes": map[string]any{"name": "x"}},
			{"type": res, "id": "4", "attributes": map[string]any{
				"store_id": "1", "customer_id": "oops", "email": 7, "name": []any{1}, "status": map[string]any{}, "key": nil, "identifier": nil,
				"total": "12.5", "created_at": "not a date", "test_mode": "1", "first_subscription_item": "nope", "pause": 5, "tiers": "x", "links": 3,
				"first_order_item": []any{}, "urls": "u", "activation_limit": "many", "disabled": "yes",
			}},
			{"type": res, "id": "5"},
			{"type": res, "attributes": map[string]any{"name": "no id"}},
		}
	}
	for _, res := range []string{"stores", "customers", "products", "variants", "prices", "files", "orders", "order-items", "subscriptions",
		"subscription-items", "subscription-invoices", "license-keys", "license-key-instances"} {
		f.set(res, junk(res)...)
	}
	job, err := e.r.StartLemonSqueezyAPI("admin", apiOptions(f))
	j := e.mustFinish(job, err, "completed")
	if progressCounts(j)["invalid"] == 0 && progressWarnings(j) == 0 {
		t.Errorf("garbage should produce warnings: %v", j.Progress.V)
	}
}

func TestGarbageResponseFailsJobCleanly(t *testing.T) {
	e := newEnv(t)
	f := newFakeLS(t, "lsk_secret_abcd")
	f.setHook(func(w http.ResponseWriter, r *http.Request, res string, page int) bool {
		w.Write([]byte("<html>gateway</html>"))
		return true
	})
	o := apiOptions(f)
	o.Resources = []string{"customers"}
	job, err := e.r.StartLemonSqueezyAPI("admin", o)
	j := e.mustFinish(job, err, "failed")
	if !strings.Contains(j.Error, "customers") {
		t.Errorf("error = %q", j.Error)
	}
}

func TestDuplicateCustomersAreMerged(t *testing.T) {
	e := newEnv(t)
	f := newFakeLS(t, "lsk_secret_abcd")
	cust := func(id int64, email string) map[string]any {
		return jobj("customers", id, map[string]any{"store_id": 1, "name": "Dup " + itoa(id), "email": email, "status": "subscribed", "created_at": tsA, "updated_at": tsA})
	}
	// the API lists newest first, so the higher id arrives first
	f.set("customers", cust(20, "dup@example.com"), cust(21, "DUP@example.com"), cust(22, "other@example.com"))
	f.set("orders",
		jobj("orders", 1, map[string]any{"store_id": 1, "customer_id": 21, "identifier": "id-1", "order_number": 1, "user_email": "DUP@example.com", "status": "paid", "total": 100, "created_at": tsA, "updated_at": tsA}),
		jobj("orders", 2, map[string]any{"store_id": 1, "customer_id": 22, "identifier": "id-2", "order_number": 2, "user_email": "other@example.com", "status": "paid", "total": 100, "created_at": tsA, "updated_at": tsA}),
	)
	f.set("subscriptions", jobj("subscriptions", 5, map[string]any{"store_id": 1, "customer_id": 21, "order_id": 1, "status": "active", "user_email": "DUP@example.com", "created_at": tsA, "updated_at": tsA}))
	f.set("license-keys", jobj("license-keys", 9, map[string]any{"store_id": 1, "customer_id": 21, "order_id": 1, "key": "kkk", "created_at": tsA, "updated_at": tsA}))
	o := apiOptions(f)
	o.Resources = []string{"customers", "orders", "subscriptions", "license_keys"}
	job, err := e.r.StartLemonSqueezyAPI("admin", o)
	j := e.mustFinish(job, err, "completed")
	if n := e.count(&db.Customer{}); n != 2 {
		t.Fatalf("customers = %d, want 2 (duplicate merged)", n)
	}
	var gone int64
	e.g.Model(&db.Customer{}).Where("id = ?", 21).Count(&gone)
	if gone != 0 {
		t.Errorf("duplicate customer 21 must not be stored")
	}
	var order db.Order
	var sub db.Subscription
	var key db.LicenseKey
	e.g.First(&order, 1)
	e.g.First(&sub, 5)
	e.g.First(&key, 9)
	if order.CustomerID != 20 || sub.CustomerID != 20 || key.CustomerID != 20 {
		t.Errorf("references must be re-pointed to #20: order=%d sub=%d key=%d", order.CustomerID, sub.CustomerID, key.CustomerID)
	}
	if !e.logContains(j.ID, "warn", "customer #21") {
		t.Errorf("merge must be logged:\n%s", e.logDump(j.ID))
	}
	// idempotent
	job, err = e.r.StartLemonSqueezyAPI("admin", o)
	e.mustFinish(job, err, "completed")
	if n := e.count(&db.Customer{}); n != 2 {
		t.Errorf("customers after re-run = %d", n)
	}
}

func TestStoreSlugClashIsRenamed(t *testing.T) {
	e := newEnv(t)
	e.g.Create(&db.Store{ID: 99, Name: "Local", Slug: "my-store"})
	f := newFakeLS(t, "lsk_secret_abcd")
	loadFixtures(f)
	o := apiOptions(f)
	o.Resources = []string{"stores"}
	job, err := e.r.StartLemonSqueezyAPI("admin", o)
	j := e.mustFinish(job, err, "completed")
	var s db.Store
	e.g.First(&s, 1)
	if s.Slug != "my-store-1" || e.count(&db.Store{}) != 3 || !e.logContains(j.ID, "warn", "my-store") {
		t.Errorf("slug clash: %+v", s)
	}
}

func TestOrderIDCollisionRowsAreIsolated(t *testing.T) {
	e := newEnv(t)
	// a local order that owns the identifier of LS order 500 under another id
	e.g.Create(&db.Order{ID: 7, StoreID: 1, CustomerID: 1, Identifier: orderIdent1, OrderNumber: 50})
	f := newFakeLS(t, "lsk_secret_abcd")
	loadFixtures(f)
	o := apiOptions(f)
	o.Resources = []string{"orders"}
	job, err := e.r.StartLemonSqueezyAPI("admin", o)
	j := e.mustFinish(job, err, "completed")
	// 4 of 5 LS orders (store 1 and 2) fit; the clashing one is reported, not fatal
	if n := e.count(&db.Order{}); n != 5 {
		t.Errorf("orders = %d, want 4 imported + 1 local", n)
	}
	if !e.logContains(j.ID, "error", "orders #500") || progressCounts(j)["failed"] != 1 {
		t.Errorf("the clashing row must be logged as an error:\n%s", e.logDump(j.ID))
	}
}

func TestOrdersResolveMergedCustomersByEmailOnPartialRun(t *testing.T) {
	e := newEnv(t)
	f := newFakeLS(t, "lsk_secret_abcd")
	cust := func(id int64, email string) map[string]any {
		return jobj("customers", id, map[string]any{"store_id": 1, "name": "Dup", "email": email, "created_at": tsA, "updated_at": tsA})
	}
	f.set("customers", cust(20, "dup@example.com"), cust(21, "dup@example.com"))
	f.set("orders", jobj("orders", 1, map[string]any{"store_id": 1, "customer_id": 21, "identifier": "id-1", "order_number": 1, "user_email": "dup@example.com", "status": "paid", "created_at": tsA, "updated_at": tsA}))
	o := apiOptions(f)
	o.Resources = []string{"customers"}
	job, err := e.r.StartLemonSqueezyAPI("admin", o)
	e.mustFinish(job, err, "completed")

	// a later run of only the orders no longer knows about the merge: the e-mail finds the surviving customer
	o.Resources = []string{"orders"}
	job, err = e.r.StartLemonSqueezyAPI("admin", o)
	j := e.mustFinish(job, err, "completed")
	var order db.Order
	e.g.First(&order, 1)
	if order.CustomerID != 20 || !e.logContains(j.ID, "warn", "customer #21") {
		t.Errorf("order customer = %d\n%s", order.CustomerID, e.logDump(j.ID))
	}
}

func TestReimportKeepsNewerLocalChangesAndTakesNewerLemonSqueezyOnes(t *testing.T) {
	e, f, _ := runFullImport(t)
	ctx := context.Background()

	// local activity after the import: an activation and a customer update bump updated_at
	if r := e.svc.ActivateLicense(ctx, keyActive, "new machine"); !r.OK {
		t.Fatalf("activate: %+v", r)
	}
	e.g.Model(&db.Customer{}).Where("id = ?", 12).Updates(map[string]any{"name": "Jane Local"})

	// ...and Lemon Squeezy changed a different order since (newer updated_at)
	o := f.object("orders", "501")
	attrOf(o)["user_name"], attrOf(o)["updated_at"] = "Jane Changed At LS", "2099-01-01T00:00:00.000000Z"

	job, err := e.r.StartLemonSqueezyAPI("admin", apiOptions(f))
	e.mustFinish(job, err, "completed")

	var k db.LicenseKey
	e.g.First(&k, 1000)
	if k.InstancesCount != 2 {
		t.Errorf("a re-run must not revert a newer local activation count: %d", k.InstancesCount)
	}
	var c db.Customer
	e.g.First(&c, 12)
	if c.Name != "Jane Local" {
		t.Errorf("a re-run must not revert a newer local edit: %q", c.Name)
	}
	var ord db.Order
	e.g.First(&ord, 501)
	if ord.UserName != "Jane Changed At LS" {
		t.Errorf("newer Lemon Squeezy data must be picked up: %q", ord.UserName)
	}
}

func TestImportOverwritesThePlaceholderDefaultStore(t *testing.T) {
	e := newEnv(t)
	if _, err := db.EnsureDefaultStore(e.g, "http://localhost:3000"); err != nil {
		t.Fatal(err)
	}
	f := newFakeLS(t, "lsk_secret_abcd")
	loadFixtures(f)
	o := apiOptions(f)
	o.Resources = []string{"stores"}
	job, err := e.r.StartLemonSqueezyAPI("admin", o)
	e.mustFinish(job, err, "completed")
	var s db.Store
	e.g.First(&s, 1)
	if s.Slug != "my-store" || s.Name != "My Store" || s.Domain != "my-store.lemonsqueezy.com" || s.Plan != "fresh" {
		t.Errorf("store 1 = %+v", s)
	}
}
