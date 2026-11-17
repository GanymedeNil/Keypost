package importer

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"fulfillment/internal/db"
	"fulfillment/internal/service"
)

type zipEntry struct {
	name string
	data []byte
}

func writeZip(t *testing.T, entries []zipEntry) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "export.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(e.data)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return p
}

func jsonBytes(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func ndjson(t *testing.T, objs []map[string]any) []byte {
	var b strings.Builder
	for _, o := range objs {
		b.Write(jsonBytes(t, o))
		b.WriteString("\n")
	}
	return []byte(b.String())
}

// flatten turns JSON:API objects into plain objects (attributes at top level).
func flatten(objs []map[string]any) []map[string]any {
	out := make([]map[string]any, len(objs))
	for i, o := range objs {
		m := map[string]any{"id": o["id"]}
		for k, v := range attrOf(o) {
			m[k] = v
		}
		out[i] = m
	}
	return out
}

func TestArchiveImportMixedFormats(t *testing.T) {
	src := newFakeLS(t, "unused")
	loadFixtures(src)
	d := src.data // not served: only used as a source of JSON
	e := newEnv(t)

	entries := []zipEntry{
		{"stores.json", jsonBytes(t, map[string]any{"data": d["stores"]})},                                        // JSON:API document
		{"export/customers.json", jsonBytes(t, map[string]any{"meta": map[string]any{}, "data": d["customers"]})}, // nested folder
		{"products.json", jsonBytes(t, d["products"])},                                                            // bare array
		{"variants.ndjson", ndjson(t, d["variants"])},                                                             // NDJSON
		{"prices.json", jsonBytes(t, flatten(d["prices"]))},                                                       // flat objects
		{"files.json", jsonBytes(t, map[string]any{"data": d["files"]})},
		{"files/" + fileIdent1, fileBytes1}, // the binary of file 400
		{"orders-1.json", append([]byte("\xef\xbb\xbf"), jsonBytes(t, map[string]any{"data": d["orders"][:2]})...)},
		{"orders-2.json", jsonBytes(t, map[string]any{"data": d["orders"][2:]})},
		{"order_items.json", jsonBytes(t, d["order-items"])},
		{"subscriptions.json", jsonBytes(t, map[string]any{"data": d["subscriptions"]})},
		{"subscription-items.json", jsonBytes(t, d["subscription-items"])},
		{"subscription-invoices.ndjson", ndjson(t, d["subscription-invoices"])},
		{"license-keys.json", jsonBytes(t, map[string]any{"data": d["license-keys"]})},
		{"license_key_instances.json", jsonBytes(t, d["license-key-instances"])},
		{"__MACOSX/._orders-1.json", []byte("junk")},
		{"README.txt", []byte("hello")},
		{"../evil.json", []byte(`[]`)},
		{"/abs/orders.json", []byte(`[]`)},
	}
	zipPath := writeZip(t, entries)
	job, err := e.r.StartLemonSqueezyArchive("admin", zipPath, ArchiveOptions{})
	j := e.mustFinish(job, err, "completed")

	want := map[string]int64{
		"stores": 2, "customers": 3, "products": 3, "variants": 4, "prices": 4, "files": 2, "orders": 5, "order_items": 5,
		"subscriptions": 2, "subscription_items": 3, "subscription_invoices": 2, "license_keys": 4, "license_key_instances": 1,
	}
	if got := e.counts(); !reflect.DeepEqual(got, want) {
		t.Fatalf("row counts = %v, want %v\n%s", got, want, e.logDump(j.ID))
	}
	if _, err := os.Stat(zipPath); !os.IsNotExist(err) {
		t.Errorf("the runner must delete the uploaded archive (stat err = %v)", err)
	}
	if j.Kind != "lemonsqueezy_files" {
		t.Errorf("kind = %s", j.Kind)
	}

	// same mapping as the API importer: ids, license keys, derived links
	var k db.LicenseKey
	e.g.First(&k, 1001)
	if k.Key != keyUnlim || k.VariantID != 201 || k.SubscriptionID == nil || *k.SubscriptionID != 700 || k.ActivationLimit != nil {
		t.Errorf("key 1001 = %+v", k)
	}
	var p db.Price
	e.g.First(&p, 310)
	if p.Currency != "EUR" || p.UnitPrice != 2500 {
		t.Errorf("price 310 = %+v", p)
	}
	var o db.Order
	e.g.First(&o, 500)
	if o.Identifier != orderIdent1 || o.Source != "lemonsqueezy" || o.Total != 5880 {
		t.Errorf("order 500 = %+v", o)
	}

	// the bundled binary was stored; the other file has none and download is off
	var f1, f2 db.File
	e.g.First(&f1, 400)
	e.g.First(&f2, 401)
	rc, _, err := e.store.Open(context.Background(), f1.StorageKey)
	if err != nil || f1.Size != int64(len(fileBytes1)) {
		t.Fatalf("file 400 not stored: %+v err=%v", f1, err)
	}
	rc.Close()
	if f2.StorageKey != "" || !e.logContains(j.ID, "warn", fileIdent2) {
		t.Errorf("file 401 should be a warning without data: %+v\n%s", f2, e.logDump(j.ID))
	}
	if n := len(src.apiRequests()) + src.downloads("/download/"+fileIdent2); n != 0 {
		t.Errorf("download_files is off: %d requests were made", n)
	}
	// unsafe entries were ignored, loudly
	if !e.logContains(j.ID, "warn", "../evil.json") || !e.logContains(j.ID, "warn", "/abs/orders.json") {
		t.Errorf("unsafe entries must be reported:\n%s", e.logDump(j.ID))
	}

	// a second import of the same archive changes nothing
	before := snapshot(e)
	zip2 := writeZip(t, entries)
	job, err = e.r.StartLemonSqueezyArchive("admin", zip2, ArchiveOptions{})
	e.mustFinish(job, err, "completed")
	if after := snapshot(e); !reflect.DeepEqual(before, after) {
		t.Errorf("re-importing the archive changed data")
	}
}

func TestArchiveDownloadsMissingBinaryWhenAllowed(t *testing.T) {
	src := newFakeLS(t, "unused")
	loadFixtures(src)
	e := newEnv(t)
	zipPath := writeZip(t, []zipEntry{{"files.json", jsonBytes(t, map[string]any{"data": src.data["files"]})}})
	job, err := e.r.StartLemonSqueezyArchive("admin", zipPath, ArchiveOptions{DownloadFiles: true})
	e.mustFinish(job, err, "completed")
	var f db.File
	e.g.First(&f, 401)
	if f.StorageKey == "" || f.Size != int64(len(fileBytes2)) || src.downloads("/download/"+fileIdent2) != 1 {
		t.Errorf("file should have been fetched from its download_url: %+v", f)
	}
}

func TestArchiveGuards(t *testing.T) {
	oldEntries, oldBytes := maxZipEntries, maxEntryBytes
	t.Cleanup(func() { maxZipEntries, maxEntryBytes = oldEntries, oldBytes })

	t.Run("too many entries", func(t *testing.T) {
		maxZipEntries = 3
		defer func() { maxZipEntries = oldEntries }()
		e := newEnv(t)
		zp := writeZip(t, []zipEntry{{"a.txt", nil}, {"b.txt", nil}, {"c.txt", nil}, {"d.txt", nil}})
		job, err := e.r.StartLemonSqueezyArchive("admin", zp, ArchiveOptions{})
		j := e.mustFinish(job, err, "failed")
		if !strings.Contains(j.Error, "entries") {
			t.Errorf("error = %q", j.Error)
		}
		if _, err := os.Stat(zp); !os.IsNotExist(err) {
			t.Errorf("archive not removed after a failed import")
		}
	})

	t.Run("oversized entries are skipped", func(t *testing.T) {
		maxEntryBytes = 300
		defer func() { maxEntryBytes = oldBytes }()
		e := newEnv(t)
		big := jsonBytes(t, []map[string]any{jobj("customers", 1, map[string]any{"email": "big@example.com", "store_id": 1, "name": strings.Repeat("x", 400)})})
		small := jsonBytes(t, []map[string]any{jobj("products", 2, map[string]any{"store_id": 1, "name": "ok"})})
		job, err := e.r.StartLemonSqueezyArchive("admin", writeZip(t, []zipEntry{{"customers.json", big}, {"products.json", small}}), ArchiveOptions{})
		j := e.mustFinish(job, err, "completed")
		if e.count(&db.Customer{}) != 0 || e.count(&db.Product{}) != 1 || !e.logContains(j.ID, "warn", "customers.json") {
			t.Errorf("limit not applied:\n%s", e.logDump(j.ID))
		}
	})

	t.Run("not a zip", func(t *testing.T) {
		e := newEnv(t)
		p := filepath.Join(t.TempDir(), "x.zip")
		os.WriteFile(p, []byte("this is not a zip file"), 0o600)
		job, err := e.r.StartLemonSqueezyArchive("admin", p, ArchiveOptions{})
		j := e.mustFinish(job, err, "failed")
		if !strings.Contains(j.Error, "ZIP") {
			t.Errorf("error = %q", j.Error)
		}
	})

	t.Run("corrupt json keeps the good files", func(t *testing.T) {
		e := newEnv(t)
		good := jsonBytes(t, []map[string]any{jobj("products", 2, map[string]any{"store_id": 1, "name": "ok"})})
		job, err := e.r.StartLemonSqueezyArchive("admin", writeZip(t, []zipEntry{{"customers.json", []byte(`{"data":[{"id":"1","attributes":{"email":"a@b.c"}},{"id":`)}, {"products.json", good}}), ArchiveOptions{})
		j := e.mustFinish(job, err, "completed")
		if e.count(&db.Product{}) != 1 || e.count(&db.Customer{}) != 1 || !e.logContains(j.ID, "error", "customers.json") {
			t.Errorf("corrupt input handling:\n%s", e.logDump(j.ID))
		}
	})
}

func TestArchiveDryRun(t *testing.T) {
	src := newFakeLS(t, "unused")
	loadFixtures(src)
	e := newEnv(t)
	zp := writeZip(t, []zipEntry{
		{"orders.json", jsonBytes(t, src.data["orders"])},
		{"files.json", jsonBytes(t, src.data["files"])},
		{"files/" + fileIdent1, fileBytes1},
	})
	job, err := e.r.StartLemonSqueezyArchive("admin", zp, ArchiveOptions{DryRun: true})
	j := e.mustFinish(job, err, "completed")
	for table, n := range e.counts() {
		if n != 0 {
			t.Errorf("dry run wrote %s = %d", table, n)
		}
	}
	if progressCounts(j)["orders"] != 5 || e.storedObjects() != 0 {
		t.Errorf("counts=%v stored=%d", progressCounts(j), e.storedObjects())
	}
}

func TestArchiveCleanupWhenBusy(t *testing.T) {
	e := newEnv(t)
	release := make(chan struct{})
	job, err := e.r.start("lemonsqueezy_files", "admin", map[string]any{}, nil, func(ctx context.Context, j *jobCtl) error {
		<-release
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	zp := writeZip(t, []zipEntry{{"orders.json", []byte(`[]`)}})
	_, err = e.r.StartLemonSqueezyArchive("admin", zp, ArchiveOptions{})
	if se, ok := service.AsError(err); !ok || se.Status != http.StatusConflict {
		t.Fatalf("err = %v", err)
	}
	if _, serr := os.Stat(zp); !os.IsNotExist(serr) {
		t.Errorf("the runner owns the file even when the job cannot start")
	}
	close(release)
	e.mustFinish(job, nil, "completed")
}

const ordersCSV = "\ufeffidentifier,order_number,product_name,variant_name,user_name,user_email,city,country,postal_code,card_brand,card_last_four,currency,currency_rate,subtotal,discount_total,tax,total,tax_name,tax_rate,tax_number,date_utc\r\n" +
	"104e18a2-d755-4d4b-80c4-a6c1dcbe1c10,1,Pro App,Default,John Doe,john@example.com,Berlin,DE,10115,visa,4242,USD,1,4900,0,980,5880,VAT,20.00,DE123456,2024-01-02 03:04:05\r\n" +
	"2d6e8f1a-4b7c-4e9d-8a1b-3c5d7e9f1a2b,2,Mystery Box,Default,Jane Roe,jane@example.com,,US,,,,EUR,0.92,1000,0,0,1000,,0.00,,2024-02-03T04:05:06Z\r\n" +
	"7c1c2f5e-8f3d-4d0e-a5c9-1b2a3c4d5e6f,3,Pro App,Default,John Doe,JOHN@example.com,Berlin,DE,10115,mastercard,5555,USD,1,4900,0,0,4900,,0.00,,2024-03-04 05:06:07\r\n"

func TestArchiveOrdersCSVFallback(t *testing.T) {
	e := newEnv(t)
	e.g.Create(&db.Store{ID: 5, Name: "S", Slug: "s", Currency: "USD"})
	e.g.Create(&db.Product{ID: 100, StoreID: 5, Name: "Pro App", Status: "published"})
	e.g.Create(&db.Variant{ID: 200, ProductID: 100, Name: "Default", Slug: "v200"})

	run := func() db.ImportJob {
		zp := writeZip(t, []zipEntry{{"orders.csv", []byte(ordersCSV)}})
		job, err := e.r.StartLemonSqueezyArchive("admin", zp, ArchiveOptions{})
		return e.mustFinish(job, err, "completed")
	}
	j := run()

	if n := e.count(&db.Customer{}); n != 2 {
		t.Fatalf("customers = %d, want 2 (john appears twice)", n)
	}
	if n := e.count(&db.Order{}); n != 3 {
		t.Fatalf("orders = %d", n)
	}
	var o1, o2 db.Order
	e.g.Where("identifier = ?", "104e18a2-d755-4d4b-80c4-a6c1dcbe1c10").First(&o1)
	e.g.Where("identifier = ?", "2d6e8f1a-4b7c-4e9d-8a1b-3c5d7e9f1a2b").First(&o2)
	if o1.StoreID != 5 || o1.OrderNumber != 1 || o1.UserEmail != "john@example.com" || o1.Total != 5880 || o1.Tax != 980 || o1.TotalUSD != 5880 ||
		o1.TaxName == nil || *o1.TaxName != "VAT" || o1.TaxRate != "20.00" || o1.TaxNumber != "DE123456" || o1.CardBrand != "visa" || o1.CardLastFour != "4242" ||
		o1.City != "Berlin" || o1.Country != "DE" || o1.PostalCode != "10115" || o1.Status != "paid" || o1.Source != "lemonsqueezy" || o1.CustomerID == 0 {
		t.Errorf("order 1 = %+v", o1)
	}
	if o1.CreatedAt.UTC().Format("2006-01-02 15:04:05") != "2024-01-02 03:04:05" {
		t.Errorf("created_at = %s", o1.CreatedAt)
	}
	if o2.Currency != "EUR" || o2.CurrencyRate != "0.92" || o2.TotalUSD != 1000 || o2.Total != 920 || o2.CreatedAt.UTC().Format("2006-01-02") != "2024-02-03" {
		t.Errorf("order 2 = %+v", o2)
	}
	var o3 db.Order
	e.g.Where("order_number = ?", 3).First(&o3)
	if o3.CustomerID != o1.CustomerID {
		t.Errorf("the same e-mail must map to one customer: %d vs %d", o3.CustomerID, o1.CustomerID)
	}

	var items []db.OrderItem
	e.g.Order("order_id").Find(&items)
	if len(items) != 3 {
		t.Fatalf("order items = %d", len(items))
	}
	byOrder := map[int64]db.OrderItem{}
	for _, it := range items {
		byOrder[it.OrderID] = it
	}
	if it := byOrder[o1.ID]; it.VariantID != 200 || it.ProductID != 100 || it.ProductName != "Pro App" || it.Price != 4900 || it.Quantity != 1 {
		t.Errorf("matched item = %+v", it)
	}
	if it := byOrder[o2.ID]; it.VariantID != 0 || it.ProductName != "Mystery Box" || it.Price != 920 {
		t.Errorf("unmatched item = %+v", it)
	}
	if !e.logContains(j.ID, "warn", "Mystery Box") {
		t.Errorf("an unmatched product must be reported:\n%s", e.logDump(j.ID))
	}

	// idempotent: same ids, no new rows
	before := snapshot(e)
	run()
	if after := snapshot(e); !reflect.DeepEqual(before, after) {
		t.Errorf("re-importing orders.csv changed data")
	}
}

func TestArchiveOrdersJSONWinsOverCSV(t *testing.T) {
	src := newFakeLS(t, "unused")
	loadFixtures(src)
	e := newEnv(t)
	zp := writeZip(t, []zipEntry{{"orders.json", jsonBytes(t, src.data["orders"])}, {"orders.csv", []byte(ordersCSV)}})
	job, err := e.r.StartLemonSqueezyArchive("admin", zp, ArchiveOptions{})
	e.mustFinish(job, err, "completed")
	if e.count(&db.Order{}) != 5 || e.count(&db.Customer{}) != 0 {
		t.Errorf("orders.json must take precedence over orders.csv")
	}
}

func TestResourceOfNaming(t *testing.T) {
	cases := map[string]string{
		"orders.json": "orders", "orders-1.json": "orders", "orders_2.ndjson": "orders", "orders2.json": "orders", "Orders.JSON": "orders",
		"license-keys.json": "license_keys", "license_keys_page3.json": "license_keys", "license-key-instances.json": "license_key_instances",
		"order-items.json": "order_items", "subscription-invoices-1.json": "subscription_invoices", "subscription_items.jsonl": "subscription_items",
		"stores.json": "stores", "files.json": "files",
	}
	for name, want := range cases {
		if got, ok := resourceOf(name); !ok || got != want {
			t.Errorf("resourceOf(%q) = %q,%v want %q", name, got, ok, want)
		}
	}
	for _, name := range []string{"orderz.json", "ordersx.json", "notes.json", "readme.json"} {
		if got, ok := resourceOf(name); ok {
			t.Errorf("resourceOf(%q) = %q, want no match", name, got)
		}
	}
}

func TestNaturalOrder(t *testing.T) {
	names := []string{"orders-10.json", "orders-2.json", "orders-1.json"}
	for i := range names {
		for j := i + 1; j < len(names); j++ {
			if naturalLess(names[j], names[i]) {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	if !reflect.DeepEqual(names, []string{"orders-1.json", "orders-2.json", "orders-10.json"}) {
		t.Errorf("order = %v", names)
	}
}

func TestEntryNameRejectsUnsafePaths(t *testing.T) {
	for _, bad := range []string{"../x", "a/../../x", "/etc/passwd", "C:/x", `..\x`, "a/../b/../../c"} {
		if _, ok := entryName(bad); ok {
			t.Errorf("%q must be rejected", bad)
		}
	}
	if n, ok := entryName("a/b/c.json"); !ok || n != "a/b/c.json" {
		t.Errorf("good path rejected: %q %v", n, ok)
	}
	if !errors.Is(errEntryTooLarge, errEntryTooLarge) {
		t.Fatal("sanity")
	}
}

func TestArchiveBinaryNameForms(t *testing.T) {
	src := newFakeLS(t, "unused")
	loadFixtures(src)
	for name, entry := range map[string]string{
		"exact":                "files/" + fileIdent1,
		"with extension":       "files/" + fileIdent1 + ".zip",
		"identifier directory": "files/" + fileIdent1 + "/app.zip",
		"identifier-prefixed":  "export/files/" + fileIdent1 + "-app.zip",
		"upper case":           "files/" + strings.ToUpper(fileIdent1),
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			zp := writeZip(t, []zipEntry{
				{"files.json", jsonBytes(t, map[string]any{"data": src.data["files"][:1]})},
				{entry, fileBytes1},
				{"files/unrelated.bin", []byte("zzz")},
			})
			job, err := e.r.StartLemonSqueezyArchive("admin", zp, ArchiveOptions{})
			e.mustFinish(job, err, "completed")
			var f db.File
			e.g.First(&f, 400)
			if f.StorageKey == "" || f.Size != int64(len(fileBytes1)) {
				t.Errorf("binary %q was not matched: %+v", entry, f)
			}
		})
	}
}
