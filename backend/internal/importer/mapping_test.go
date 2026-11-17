package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fulfillment/internal/db"
)

func writeCSV(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "mapping.csv")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func seedMappingData(e *env) {
	e.g.Create(&db.Store{ID: 1, Name: "S", Slug: "s", Currency: "USD"})
	e.g.Create(&db.Customer{ID: 11, StoreID: 1, Email: "john@example.com", Name: "John"})
	e.g.Create(&db.Customer{ID: 12, StoreID: 1, Email: "jane@example.com", Name: "Jane", StripeCustomerID: "cus_OldJane1"})
	e.g.Create(&db.Customer{ID: 13, StoreID: 1, Email: "bob@example.com", Name: "Bob"})
	e.g.Create(&db.Product{ID: 100, StoreID: 1, Name: "Pro App"})
	e.g.Create(&db.Product{ID: 101, StoreID: 1, Name: "Course"})
	e.g.Create(&db.Product{ID: 102, StoreID: 1, Name: "Bundle"})
	e.g.Create(&db.Variant{ID: 200, ProductID: 100, Name: "Default", Slug: "v200"})
	e.g.Create(&db.Variant{ID: 201, ProductID: 100, Name: "Monthly", Slug: "v201"})
	e.g.Create(&db.Variant{ID: 202, ProductID: 101, Name: "Default", Slug: "v202", StripeProductID: "prod_OldCourse1"})
	e.g.Create(&db.Variant{ID: 203, ProductID: 102, Name: "Single", Slug: "v203"})
	e.g.Create(&db.Price{ID: 300, VariantID: 200, UnitPrice: 4900})
	e.g.Create(&db.Price{ID: 301, VariantID: 200, UnitPrice: 5900}) // a later price of the same variant
	e.g.Create(&db.Price{ID: 302, VariantID: 202, UnitPrice: 1500, StripePriceID: "price_OldCourse1"})
	e.g.Create(&db.Price{ID: 303, VariantID: 203, UnitPrice: 900})
	e.g.Create(&db.Subscription{ID: 700, StoreID: 1, CustomerID: 11, Status: "active"})
	e.g.Create(&db.Subscription{ID: 701, StoreID: 1, CustomerID: 11, Status: "active"})
	e.g.Create(&db.Subscription{ID: 702, StoreID: 1, CustomerID: 12, Status: "active", StripeSubscriptionID: ptr("sub_OldJane1")})
}

func (e *env) customerStripe(id int64) string {
	var c db.Customer
	e.g.First(&c, id)
	return c.StripeCustomerID
}

func TestMappingCustomers(t *testing.T) {
	cases := []struct {
		name  string
		csv   string
		want  map[string]int // matched / unchanged / unknown_ls_id / invalid_rows / conflicts
		check func(t *testing.T, e *env)
	}{
		{
			name: "comma with header, bad rows reported",
			csv:  "lemon_squeezy_customer_id,stripe_customer_id\n11,cus_John1111\n999,cus_Ghost999\nabc,cus_Bad1234\n13,not_a_stripe_id\n12,cus_NewJane22\n",
			want: map[string]int{"matched": 1, "unknown_ls_id": 1, "invalid_rows": 2, "conflicts": 1},
			check: func(t *testing.T, e *env) {
				if e.customerStripe(11) != "cus_John1111" || e.customerStripe(12) != "cus_OldJane1" || e.customerStripe(13) != "" {
					t.Errorf("links: %q %q %q", e.customerStripe(11), e.customerStripe(12), e.customerStripe(13))
				}
			},
		},
		{
			name: "semicolon, BOM, CRLF, extra columns",
			csv:  "\ufeffLS Customer ID;Name;Stripe Customer ID\r\n11;John;cus_John1111\r\n13;Bob;cus_Bob22222\r\n",
			want: map[string]int{"matched": 2},
			check: func(t *testing.T, e *env) {
				if e.customerStripe(11) != "cus_John1111" || e.customerStripe(13) != "cus_Bob22222" {
					t.Errorf("links: %q %q", e.customerStripe(11), e.customerStripe(13))
				}
			},
		},
		{
			name: "tab separated, stripe column first",
			csv:  "Stripe ID\tLemon Squeezy ID\ncus_John1111\t11\n",
			want: map[string]int{"matched": 1},
			check: func(t *testing.T, e *env) {
				if e.customerStripe(11) != "cus_John1111" {
					t.Errorf("link: %q", e.customerStripe(11))
				}
			},
		},
		{
			name: "no header at all",
			csv:  "11,cus_John1111\n13,cus_Bob22222\n",
			want: map[string]int{"matched": 2},
			check: func(t *testing.T, e *env) {
				if e.customerStripe(13) != "cus_Bob22222" {
					t.Errorf("link: %q", e.customerStripe(13))
				}
			},
		},
		{
			name: "match by e-mail when there is no id column",
			csv:  "Email,Stripe Customer\njohn@example.com,cus_John1111\nJANE@example.com,cus_OldJane1\nnobody@example.com,cus_Nobody11\n",
			want: map[string]int{"matched": 1, "unchanged": 1, "unknown_ls_id": 1},
			check: func(t *testing.T, e *env) {
				if e.customerStripe(11) != "cus_John1111" {
					t.Errorf("link: %q", e.customerStripe(11))
				}
			},
		},
		{
			name: "e-mail is the fallback for an unknown id",
			csv:  "ls_customer_id,email,stripe_customer_id\n5000,bob@example.com,cus_Bob22222\n",
			want: map[string]int{"matched": 1},
			check: func(t *testing.T, e *env) {
				if e.customerStripe(13) != "cus_Bob22222" {
					t.Errorf("link: %q", e.customerStripe(13))
				}
			},
		},
		{
			name: "re-applying the same file changes nothing",
			csv:  "id,stripe_id\n12,cus_OldJane1\n",
			want: map[string]int{"unchanged": 1},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			seedMappingData(e)
			p := writeCSV(t, c.csv)
			job, err := e.r.StartMappingCSV("admin", "customers", p)
			j := e.mustFinish(job, err, "completed")
			got := progressCounts(j)
			for _, k := range []string{"matched", "unchanged", "unknown_ls_id", "invalid_rows", "conflicts"} {
				if got[k] != c.want[k] {
					t.Errorf("%s = %d, want %d\n%s", k, got[k], c.want[k], e.logDump(j.ID))
				}
			}
			if c.check != nil {
				c.check(t, e)
			}
			if _, err := os.Stat(p); !os.IsNotExist(err) {
				t.Errorf("the runner must delete the CSV")
			}
			if j.Kind != "stripe_mapping" {
				t.Errorf("kind = %s", j.Kind)
			}
		})
	}
}

func TestMappingCustomerLinkReachesSubscriptions(t *testing.T) {
	e := newEnv(t)
	seedMappingData(e)
	job, err := e.r.StartMappingCSV("admin", "customers", writeCSV(t, "ls id,stripe id\n11,cus_John1111\n"))
	e.mustFinish(job, err, "completed")
	var s db.Subscription
	e.g.First(&s, 700)
	if s.StripeCustomerID != "cus_John1111" {
		t.Errorf("subscription stripe_customer_id = %q", s.StripeCustomerID)
	}
}

func TestMappingProducts(t *testing.T) {
	e := newEnv(t)
	seedMappingData(e)
	csv := strings.Join([]string{
		"Lemon Squeezy Variant ID,Stripe Product ID,Stripe Price ID",
		"200,prod_Pro11111,price_Pro11111", // variant, first price gets the Stripe price
		"202,prod_Other222,price_Other222", // both already linked to something else: conflicts
		"202,prod_OldCourse1,price_OldCourse1",
		"9999,prod_Ghost111,price_Ghost111",
		"201,nope,price_x",
	}, "\n")
	job, err := e.r.StartMappingCSV("admin", "products", writeCSV(t, csv))
	j := e.mustFinish(job, err, "completed")
	got := progressCounts(j)
	if got["matched"] != 1 || got["conflicts"] != 1 || got["unchanged"] != 1 || got["unknown_ls_id"] != 1 || got["invalid_rows"] != 1 {
		t.Errorf("counts = %v\n%s", got, e.logDump(j.ID))
	}
	var v200, v202 db.Variant
	var p300, p301 db.Price
	e.g.First(&v200, 200)
	e.g.First(&v202, 202)
	e.g.First(&p300, 300)
	e.g.First(&p301, 301)
	if v200.StripeProductID != "prod_Pro11111" || p300.StripePriceID != "price_Pro11111" || p301.StripePriceID != "" || v202.StripeProductID != "prod_OldCourse1" {
		t.Errorf("v200=%q p300=%q p301=%q v202=%q", v200.StripeProductID, p300.StripePriceID, p301.StripePriceID, v202.StripeProductID)
	}
}

func TestMappingProductsByProductID(t *testing.T) {
	e := newEnv(t)
	seedMappingData(e)
	// a product id (header says so): a single-variant product links its variant too, a multi-variant one only the product
	csv := "LS Product ID;Stripe Product\n102;prod_Bundle111\n100;prod_ProApp111\n"
	job, err := e.r.StartMappingCSV("admin", "products", writeCSV(t, csv))
	e.mustFinish(job, err, "completed")
	var p102, p100 db.Product
	var v203, v200 db.Variant
	e.g.First(&p102, 102)
	e.g.First(&p100, 100)
	e.g.First(&v203, 203)
	e.g.First(&v200, 200)
	if p102.StripeProductID != "prod_Bundle111" || v203.StripeProductID != "prod_Bundle111" {
		t.Errorf("single variant product: product=%q variant=%q", p102.StripeProductID, v203.StripeProductID)
	}
	if p100.StripeProductID != "prod_ProApp111" || v200.StripeProductID != "" {
		t.Errorf("multi variant product: product=%q variant=%q (variants must stay unlinked)", p100.StripeProductID, v200.StripeProductID)
	}
}

func TestMappingSubscriptions(t *testing.T) {
	e := newEnv(t)
	seedMappingData(e)
	csv := strings.Join([]string{
		"subscription_id,stripe_subscription_id",
		"700,sub_Alpha1111",
		"701,sub_Alpha1111", // the unique index: one Stripe subscription, one local row
		"702,sub_Other2222", // already linked to a different subscription
		"702,sub_OldJane1",  // already linked to this one
		"555,sub_Ghost1111",
		"700,sub_Alpha1111",
	}, "\n")
	job, err := e.r.StartMappingCSV("admin", "subscriptions", writeCSV(t, csv))
	j := e.mustFinish(job, err, "completed")
	got := progressCounts(j)
	if got["matched"] != 1 || got["conflicts"] != 2 || got["unchanged"] != 2 || got["unknown_ls_id"] != 1 {
		t.Errorf("counts = %v\n%s", got, e.logDump(j.ID))
	}
	var s700, s701 db.Subscription
	e.g.First(&s700, 700)
	e.g.First(&s701, 701)
	if s700.StripeSubscriptionID == nil || *s700.StripeSubscriptionID != "sub_Alpha1111" || s701.StripeSubscriptionID != nil {
		t.Errorf("s700=%v s701=%v", s700.StripeSubscriptionID, s701.StripeSubscriptionID)
	}
}

func TestMappingProblemLogsAreCapped(t *testing.T) {
	e := newEnv(t)
	seedMappingData(e)
	var b strings.Builder
	b.WriteString("ls_id,stripe_id\n")
	for i := 0; i < 120; i++ {
		fmt.Fprintf(&b, "%d,cus_Ghost%04d\n", 1000+i, i)
	}
	job, err := e.r.StartMappingCSV("admin", "customers", writeCSV(t, b.String()))
	j := e.mustFinish(job, err, "completed")
	warns := 0
	for _, l := range e.logs(j.ID) {
		if l.Level == "warn" {
			warns++
		}
	}
	if warns != maxMappingProblems || progressWarnings(j) != 120 || progressCounts(j)["unknown_ls_id"] != 120 {
		t.Errorf("warn rows=%d warnings=%d counts=%v", warns, progressWarnings(j), progressCounts(j))
	}
	if !e.logContains(j.ID, "info", "70 more problem") {
		t.Errorf("a summary of the unlisted problems is expected:\n%s", e.logDump(j.ID))
	}
}

func TestMappingRejectsUnusableFiles(t *testing.T) {
	cases := []struct{ name, kind, csv, wantErr string }{
		{"empty", "customers", "", "empty"},
		{"no stripe ids", "customers", "a,b\n1,2\n3,4\n", "Stripe ids"},
		{"no ls ids", "customers", "stripe_customer_id\ncus_Abc12345\ncus_Def12345\n", "Lemon Squeezy"},
		{"wrong stripe kind", "subscriptions", "ls_id,stripe_id\n1,cus_Abc12345\n", "Stripe ids"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			job, err := e.r.StartMappingCSV("admin", c.kind, writeCSV(t, c.csv))
			j := e.mustFinish(job, err, "failed")
			if !strings.Contains(j.Error, c.wantErr) {
				t.Errorf("error = %q, want it to mention %q", j.Error, c.wantErr)
			}
		})
	}
	e := newEnv(t)
	p := writeCSV(t, "a,b\n")
	if _, err := e.r.StartMappingCSV("admin", "orders", p); err == nil {
		t.Errorf("an unknown kind must be rejected")
	}
	if _, serr := os.Stat(p); !os.IsNotExist(serr) {
		t.Errorf("the CSV must be removed even when the request is rejected")
	}
	job, err := e.r.StartMappingCSV("admin", "", writeCSV(t, "ls_id,stripe_id\n1,cus_Abc12345\n"))
	j := e.mustFinish(job, err, "completed")
	if j.Options.V["kind"] != "customers" {
		t.Errorf("the default kind is customers: %v", j.Options.V)
	}
}

func TestDetectMappingColumns(t *testing.T) {
	cases := []struct {
		name     string
		kind     string
		csv      string
		header   bool
		ls, st   int
		price    int
		email    int
		lsKind   string
		wantFail bool
	}{
		{"customers, plain", "customers", "lemon_squeezy_customer_id,stripe_customer_id\n11,cus_Abc12345\n", true, 0, 1, -1, -1, "", false},
		{"customers, swapped with noise", "customers", "name,stripe_customer_id,ls_id,email\nJohn,cus_Abc12345,11,j@x.com\n", true, 2, 1, -1, 3, "", false},
		{"customers, headerless", "customers", "11,cus_Abc12345\n12,cus_Def12345\n", false, 0, 1, -1, -1, "", false},
		{"customers, headerless swapped", "customers", "cus_Abc12345,11\ncus_Def12345,12\n", false, 1, 0, -1, -1, "", false},
		{"customers, header names unhelpful", "customers", "a,b\n11,cus_Abc12345\n12,cus_Def12345\n", true, 0, 1, -1, -1, "", false},
		{"products, variant", "products", "variant_id,product_id,stripe_product_id,stripe_price_id\n200,100,prod_Abc12345,price_Abc12345\n", true, 0, 2, 3, -1, "variant", false},
		{"products, product only", "products", "LS Product ID,Stripe Product ID\n100,prod_Abc12345\n", true, 0, 1, -1, -1, "product", false},
		{"products, unlabeled", "products", "x,y,z\n200,prod_Abc12345,price_Abc12345\n", true, 0, 1, 2, -1, "", false},
		{"subscriptions", "subscriptions", "Lemon Squeezy Subscription,Stripe Subscription\n700,sub_Abc12345\n", true, 0, 1, -1, -1, "", false},
		{"nothing useful", "customers", "x,y\nfoo,bar\n", true, -1, -1, -1, -1, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows, err := readCSV(strings.NewReader(c.csv))
			if err != nil {
				t.Fatal(err)
			}
			cols, err := detectMappingColumns(c.kind, rows)
			if (err != nil) != c.wantFail {
				t.Fatalf("err = %v, wantFail %v", err, c.wantFail)
			}
			if c.wantFail {
				return
			}
			if cols.header != c.header || cols.ls != c.ls || cols.stripe != c.st || cols.price != c.price || cols.email != c.email || cols.lsKind != c.lsKind {
				t.Errorf("cols = %+v, want header=%v ls=%d stripe=%d price=%d email=%d kind=%q", cols, c.header, c.ls, c.st, c.price, c.email, c.lsKind)
			}
		})
	}
}

func TestDetectDelimiter(t *testing.T) {
	cases := map[string]rune{
		"a,b,c\n1,2,3":              ',',
		"a;b;c\n1;2;3":              ';',
		"a\tb\tc\n1\t2\t3":          '\t',
		`"a,b";c;d` + "\n1;2;3":     ';',
		"single\nvalue":             ',',
		"a;b,c;d\n":                 ';',
		`"x;y","z;w",q` + "\n1,2,3": ',',
	}
	for in, want := range cases {
		if got := detectDelimiter([]byte(in)); got != want {
			t.Errorf("detectDelimiter(%q) = %q, want %q", in, got, want)
		}
	}
}
