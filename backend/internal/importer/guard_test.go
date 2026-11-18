package importer

import (
	"strings"
	"testing"
	"time"

	"fulfillment/internal/db"
	"fulfillment/internal/service"
)

// localRow is a row created locally (by a Stripe checkout, by hand ...) under
// an id that a Lemon Squeezy row of the fixtures also uses.
type localRow struct {
	resource string
	id       int64
	create   func(e *env)
	// intact reports whether the local row is still what create made.
	intact func(e *env) bool
}

func localRows() []localRow {
	return []localRow{
		{"stores", 1,
			func(e *env) { e.g.Create(&db.Store{ID: 1, Name: "Local", Slug: "local-shop", Currency: "USD"}) },
			func(e *env) bool { var s db.Store; e.g.First(&s, 1); return s.Slug == "local-shop" }},
		{"customers", 11,
			func(e *env) { e.g.Create(&db.Customer{ID: 11, StoreID: 1, Email: "local@example.com", Name: "Local"}) },
			func(e *env) bool { var c db.Customer; e.g.First(&c, 11); return c.Email == "local@example.com" }},
		{"products", 100,
			func(e *env) { e.g.Create(&db.Product{ID: 100, StoreID: 1, Name: "Local product", Status: "published"}) },
			func(e *env) bool { var p db.Product; e.g.First(&p, 100); return p.Name == "Local product" }},
		{"variants", 200,
			func(e *env) {
				e.g.Create(&db.Variant{ID: 200, ProductID: 1, Name: "Local variant", Slug: "local-variant"})
			},
			func(e *env) bool { var v db.Variant; e.g.First(&v, 200); return v.Name == "Local variant" }},
		{"prices", 300,
			func(e *env) {
				e.g.Create(&db.Price{ID: 300, VariantID: 1, Category: "one_time", UnitPrice: 1234, Currency: "USD"})
			},
			func(e *env) bool { var p db.Price; e.g.First(&p, 300); return p.UnitPrice == 1234 }},
		{"files", 400,
			func(e *env) {
				e.g.Create(&db.File{ID: 400, VariantID: 1, Identifier: "local-file", Name: "local.zip", Status: "published"})
			},
			func(e *env) bool { var f db.File; e.g.First(&f, 400); return f.Identifier == "local-file" }},
		{"orders", 500,
			func(e *env) {
				e.g.Create(&db.Order{ID: 500, StoreID: 1, Identifier: "local-stripe-order", OrderNumber: 77, Total: 123, Source: "stripe"})
			},
			func(e *env) bool {
				var o db.Order
				e.g.First(&o, 500)
				return o.Identifier == "local-stripe-order" && o.Total == 123
			}},
		{"order_items", 600,
			func(e *env) {
				e.g.Create(&db.OrderItem{ID: 600, OrderID: 1, ProductName: "Local item", Price: 99, Quantity: 1})
			},
			func(e *env) bool { var i db.OrderItem; e.g.First(&i, 600); return i.ProductName == "Local item" }},
		{"subscriptions", 700,
			func(e *env) {
				e.g.Create(&db.Subscription{ID: 700, StoreID: 1, UserEmail: "local@example.com", Status: "active", PaymentProcessor: "stripe", Quantity: 1})
			},
			func(e *env) bool {
				var s db.Subscription
				e.g.First(&s, 700)
				return s.UserEmail == "local@example.com"
			}},
		{"subscription_items", 800,
			func(e *env) { e.g.Create(&db.SubscriptionItem{ID: 800, SubscriptionID: 1, PriceID: 1, Quantity: 9}) },
			func(e *env) bool { var i db.SubscriptionItem; e.g.First(&i, 800); return i.Quantity == 9 }},
		{"subscription_invoices", 900,
			func(e *env) {
				e.g.Create(&db.SubscriptionInvoice{ID: 900, StoreID: 1, SubscriptionID: 1, UserEmail: "local@example.com", Status: "paid", Total: 4242})
			},
			func(e *env) bool { var i db.SubscriptionInvoice; e.g.First(&i, 900); return i.Total == 4242 }},
		{"license_keys", 1000,
			func(e *env) {
				e.g.Create(&db.LicenseKey{ID: 1000, StoreID: 1, Key: "local-key-0001", UserEmail: "local@example.com"})
			},
			func(e *env) bool { var k db.LicenseKey; e.g.First(&k, 1000); return k.Key == "local-key-0001" }},
		{"license_key_instances", 1100,
			func(e *env) {
				e.g.Create(&db.LicenseKeyInstance{ID: 1100, LicenseKeyID: 1, Identifier: "local-instance", Name: "local"})
			},
			func(e *env) bool {
				var i db.LicenseKeyInstance
				e.g.First(&i, 1100)
				return i.Identifier == "local-instance"
			}},
	}
}

func tableOf(e *env, resource string) int64 { return e.counts()[resource] }

func TestIDConflictStopsTheImportWithoutOverwriting(t *testing.T) {
	for _, lr := range localRows() {
		t.Run(lr.resource, func(t *testing.T) {
			e := newEnv(t)
			lr.create(e)
			f := newFakeLS(t, "lsk_secret_abcd")
			loadFixtures(f)
			o := apiOptions(f)
			o.Resources = []string{lr.resource}
			job, err := e.r.StartLemonSqueezyAPI("admin", o)
			j := e.mustFinish(job, err, "failed")

			if !strings.Contains(j.Error, "already belong to different rows") || !strings.Contains(j.Error, "#"+itoa(lr.id)) {
				t.Errorf("error = %q", j.Error)
			}
			if !e.logContains(j.ID, "error", lr.resource+" #"+itoa(lr.id)+": the id already belongs to a different local row") {
				t.Errorf("the clash must be listed:\n%s", e.logDump(j.ID))
			}
			if !lr.intact(e) {
				t.Errorf("the local %s row was overwritten", lr.resource)
			}
			if lr.resource == "files" {
				// files are written one by one, so a file read before the clash is valid and stays;
				// what must not happen is downloading the bytes of the clashing one
				if n := f.downloads("/download/" + fileIdent1); n != 0 {
					t.Errorf("the clashing file was downloaded %d time(s)", n)
				}
			} else {
				if n := tableOf(e, lr.resource); n != 1 {
					t.Errorf("%s = %d, want only the local row (the conflicting batch must not be written)", lr.resource, n)
				}
				if e.storedObjects() != 0 {
					t.Errorf("a conflicting import must not download files: %d objects stored", e.storedObjects())
				}
			}
			if progressCounts(j)["conflicts"] != 1 {
				t.Errorf("conflicts = %d", progressCounts(j)["conflicts"])
			}
		})
	}
}

func TestIDConflictKeepsEarlierStagesAndStopsLaterOnes(t *testing.T) {
	e := newEnv(t)
	e.g.Create(&db.Product{ID: 100, StoreID: 1, Name: "Local product", Status: "published"})
	f := newFakeLS(t, "lsk_secret_abcd")
	loadFixtures(f)
	job, err := e.r.StartLemonSqueezyAPI("admin", apiOptions(f))
	j := e.mustFinish(job, err, "failed")

	c := e.counts()
	if c["stores"] != 2 || c["customers"] != 3 {
		t.Errorf("stages before the conflict are valid data and stay: %v", c)
	}
	if c["products"] != 1 || c["variants"] != 0 || c["orders"] != 0 || c["license_keys"] != 0 {
		t.Errorf("nothing from the conflicting stage on may be written (rows would be re-parented to the local product): %v", c)
	}
	if j.Progress.V["stage"] != "products" {
		t.Errorf("stopped in stage %v", j.Progress.V["stage"])
	}
}

func TestIDConflictIgnoresTheSameRowEvenIfCreatedAtDiffers(t *testing.T) {
	e := newEnv(t)
	// same natural keys as LS rows 11 / 500 / 1000 / 1100 / 400 / store 1, other created_at
	e.g.Create(&db.Customer{ID: 11, StoreID: 1, Email: "JOHN@example.com", Name: "Old"})
	e.g.Create(&db.Order{ID: 500, StoreID: 1, Identifier: orderIdent1, OrderNumber: 1, Source: "lemonsqueezy"})
	e.g.Create(&db.LicenseKey{ID: 1000, StoreID: 1, Key: keyActive})
	e.g.Create(&db.LicenseKeyInstance{ID: 1100, LicenseKeyID: 1000, Identifier: instIdent, Name: "laptop"})
	e.g.Create(&db.Store{ID: 1, Name: "Placeholder", Slug: "my-store", Currency: "USD"})
	f := newFakeLS(t, "lsk_secret_abcd")
	loadFixtures(f)
	job, err := e.r.StartLemonSqueezyAPI("admin", apiOptions(f))
	j := e.mustFinish(job, err, "completed")
	if n := progressCounts(j)["conflicts"]; n != 0 {
		t.Errorf("rows with the same natural key are the same row, conflicts = %d\n%s", n, e.logDump(j.ID))
	}
}

func TestIDConflictDryRunListsEverythingAndWritesNothing(t *testing.T) {
	e := newEnv(t)
	e.g.Create(&db.Product{ID: 100, StoreID: 1, Name: "Local product", Status: "published"})
	e.g.Create(&db.Order{ID: 500, StoreID: 1, Identifier: "local-stripe-order", OrderNumber: 77, Source: "stripe"})
	before := e.counts()
	f := newFakeLS(t, "lsk_secret_abcd")
	loadFixtures(f)
	o := apiOptions(f)
	o.DryRun = true
	job, err := e.r.StartLemonSqueezyAPI("admin", o)
	j := e.mustFinish(job, err, "failed")

	if !strings.Contains(j.Error, "dry run found 2 id conflict") {
		t.Errorf("error = %q", j.Error)
	}
	for _, want := range []string{"products #100", "orders #500"} {
		if !e.logContains(j.ID, "error", want) {
			t.Errorf("the dry run must list %q:\n%s", want, e.logDump(j.ID))
		}
	}
	if progressCounts(j)["conflicts"] != 2 {
		t.Errorf("conflicts = %v", progressCounts(j))
	}
	after := e.counts()
	for table, n := range after {
		if n != before[table] {
			t.Errorf("dry run wrote %s: %d -> %d", table, before[table], n)
		}
	}
}

func TestIDConflictDryRunWithoutConflictsStillCompletes(t *testing.T) {
	e := newEnv(t)
	f := newFakeLS(t, "lsk_secret_abcd")
	loadFixtures(f)
	o := apiOptions(f)
	o.DryRun = true
	job, err := e.r.StartLemonSqueezyAPI("admin", o)
	e.mustFinish(job, err, "completed")
}

func TestIDConflictOverwritePolicyReplacesTheLocalRows(t *testing.T) {
	e := newEnv(t)
	// older than the Lemon Squeezy row, so the timestamp rule lets the overwrite through
	e.g.Create(&db.Product{ID: 100, StoreID: 1, Name: "Disposable test product", Status: "draft", CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)})
	f := newFakeLS(t, "lsk_secret_abcd")
	loadFixtures(f)
	o := apiOptions(f)
	o.OnConflict = ConflictOverwrite
	job, err := e.r.StartLemonSqueezyAPI("admin", o)
	j := e.mustFinish(job, err, "completed")

	var p db.Product
	e.g.First(&p, 100)
	if p.Name != "Pro App" {
		t.Errorf("product = %+v", p)
	}
	if progressCounts(j)["overwritten"] != 1 || !e.logContains(j.ID, "warn", "the local row was overwritten") || !e.logContains(j.ID, "warn", "set to overwrite") {
		t.Errorf("an overwrite must be visible in the log:\n%s", e.logDump(j.ID))
	}
	if j.Options.V["on_conflict"] != ConflictOverwrite {
		t.Errorf("options = %v", j.Options.V)
	}
}

func TestIDConflictPolicyValidation(t *testing.T) {
	e := newEnv(t)
	f := newFakeLS(t, "lsk_secret_abcd")
	loadFixtures(f)
	o := apiOptions(f)
	o.OnConflict = "skip"
	_, err := e.r.StartLemonSqueezyAPI("admin", o)
	var se *service.Error
	if err == nil || !strings.Contains(err.Error(), "unknown conflict policy") {
		t.Fatalf("err = %v (%T)", err, se)
	}
	// default is abort and is recorded in the job options
	o.OnConflict = ""
	job, err := e.r.StartLemonSqueezyAPI("admin", o)
	j := e.mustFinish(job, err, "completed")
	if j.Options.V["on_conflict"] != ConflictAbort {
		t.Errorf("options = %v", j.Options.V)
	}
}

func TestIDConflictArchiveImport(t *testing.T) {
	src := newFakeLS(t, "unused")
	loadFixtures(src)
	e := newEnv(t)
	e.g.Create(&db.Order{ID: 500, StoreID: 1, Identifier: "local-stripe-order", OrderNumber: 77, Total: 123, Source: "stripe"})
	zp := writeZip(t, []zipEntry{{"orders.json", jsonBytes(t, src.data["orders"])}})
	job, err := e.r.StartLemonSqueezyArchive("admin", zp, ArchiveOptions{})
	j := e.mustFinish(job, err, "failed")
	if !strings.Contains(j.Error, "already belong to different rows") {
		t.Errorf("error = %q", j.Error)
	}
	var o db.Order
	e.g.First(&o, 500)
	if o.Identifier != "local-stripe-order" || e.count(&db.Order{}) != 1 {
		t.Errorf("order = %+v, orders = %d", o, e.count(&db.Order{}))
	}

	// the explicit overwrite replaces it (the local row is older than the archive's)
	e.g.Model(&db.Order{}).Where("id = ?", 500).Updates(map[string]any{"created_at": time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), "updated_at": time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)})
	zp = writeZip(t, []zipEntry{{"orders.json", jsonBytes(t, src.data["orders"])}})
	job, err = e.r.StartLemonSqueezyArchive("admin", zp, ArchiveOptions{OnConflict: ConflictOverwrite})
	e.mustFinish(job, err, "completed")
	e.g.First(&o, 500)
	if o.Identifier != orderIdent1 || o.Source != "lemonsqueezy" {
		t.Errorf("order = %+v", o)
	}
}

// orders.csv places its rows by e-mail and identifier, so a customer or order
// that already exists locally is merged, never reported as a conflict.
func TestIDConflictCSVRowsMatchedByNaturalKeyAreNotConflicts(t *testing.T) {
	e := newEnv(t)
	e.g.Create(&db.Store{ID: 5, Name: "S", Slug: "s", Currency: "USD"})
	e.g.Create(&db.Customer{ID: 3, StoreID: 5, Email: "john@example.com", Name: "Created by Stripe"})
	e.g.Create(&db.Order{ID: 9, StoreID: 5, Identifier: "104e18a2-d755-4d4b-80c4-a6c1dcbe1c10", OrderNumber: 1, Source: "stripe"})
	zp := writeZip(t, []zipEntry{{"orders.csv", []byte(ordersCSV)}})
	job, err := e.r.StartLemonSqueezyArchive("admin", zp, ArchiveOptions{})
	j := e.mustFinish(job, err, "completed")
	if progressCounts(j)["conflicts"] != 0 {
		t.Errorf("conflicts = %d\n%s", progressCounts(j)["conflicts"], e.logDump(j.ID))
	}
}
