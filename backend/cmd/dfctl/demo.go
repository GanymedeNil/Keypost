package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"fulfillment/internal/config"
	"fulfillment/internal/db"
	"fulfillment/internal/storage"
)

// demoData fills an empty installation with believable sample data so the
// admin UI can be explored without a Stripe account. Never run it on production.
func demoData(g *gorm.DB, cfg *config.Config, args []string) {
	fs := flag.NewFlagSet("demo-data", flag.ExitOnError)
	test := fs.Bool("test-mode", false, "create the data in test mode")
	force := fs.Bool("force", false, "run even if orders already exist")
	_ = fs.Parse(args)

	var existing int64
	g.Model(&db.Order{}).Count(&existing)
	if existing > 0 && !*force {
		fatal(fmt.Errorf("the database already has %d orders; pass --force to add demo data anyway", existing))
	}
	var store db.Store
	if err := g.Order("id").First(&store).Error; err != nil {
		fatal(err)
	}
	st, err := storage.New(cfg)
	if err != nil {
		fatal(err)
	}
	rng := rand.New(rand.NewSource(42))
	now := db.Now()
	ago := func(days int, hour int) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day(), hour, rng.Intn(60), 0, 0, time.UTC).AddDate(0, 0, -days)
	}

	type vdef struct {
		name        string
		price       int64
		sub         bool
		licenses    bool
		limit       int
		files       []string
		stripeProd  string
		stripePrice string
	}
	type pdef struct {
		name, desc string
		variants   []vdef
	}
	catalog := []pdef{
		{"Pixel Studio", "<p>A fast desktop image editor for macOS and Windows.</p>", []vdef{
			{"Personal", 2900, false, true, 2, []string{"pixel-studio-1.4.2.dmg", "pixel-studio-1.4.2.exe"}, "prod_demo_ps1", "price_demo_ps1"},
			{"Team (5 seats)", 9900, false, true, 5, []string{"pixel-studio-1.4.2.dmg"}, "prod_demo_ps2", "price_demo_ps2"},
		}},
		{"Notebook Cloud", "<p>Sync notes everywhere. Subscription with license-gated clients.</p>", []vdef{
			{"Monthly", 800, true, true, 0, []string{"notebook-client.zip"}, "prod_demo_nb1", "price_demo_nb1"},
			{"Yearly", 7900, true, true, 0, []string{"notebook-client.zip"}, "prod_demo_nb2", "price_demo_nb2"},
		}},
		{"Design Icons Pack", "<p>1,200 hand-drawn SVG icons.</p>", []vdef{
			{"Standard", 1900, false, false, 0, []string{"icons-pack.zip", "icons-pack-sketch.zip"}, "prod_demo_ic1", "price_demo_ic1"},
		}},
	}

	var variants []db.Variant
	prices := map[int64]db.Price{}
	for pi, pd := range catalog {
		p := db.Product{StoreID: store.ID, Name: pd.name, Slug: strings.ToLower(strings.ReplaceAll(pd.name, " ", "-")), Description: pd.desc, Status: "published", TestMode: *test, CreatedAt: ago(70, 9), UpdatedAt: now}
		g.Create(&p)
		for vi, vd := range pd.variants {
			v := db.Variant{ProductID: p.ID, Name: vd.name, Slug: uuid.NewString(), Sort: vi, Status: "published", HasLicenseKeys: vd.licenses, LicenseActivationLimit: vd.limit,
				IsLicenseLimitUnlimited: vd.limit == 0, IsLicenseLengthUnlimited: true, LicenseLengthUnit: "years", LicenseLengthValue: 1,
				StripeProductID: vd.stripeProd, TestMode: *test, Links: db.J([]db.VariantLink{{Title: "Getting started", URL: "https://docs.example.com/start?key=[license_key]"}}), CreatedAt: p.CreatedAt, UpdatedAt: now}
			g.Create(&v)
			pr := db.Price{VariantID: v.ID, Category: "one_time", Scheme: "standard", UnitPrice: vd.price, PackageSize: 1, TaxCode: "eservice", Currency: "USD", StripePriceID: vd.stripePrice, Tiers: db.J([]db.PriceTier{}), CreatedAt: p.CreatedAt, UpdatedAt: now}
			if vd.sub {
				unit, qty := "month", 1
				if strings.Contains(vd.name, "Year") {
					unit = "year"
				}
				pr.Category, pr.RenewalIntervalUnit, pr.RenewalIntervalQuantity, pr.TaxCode = "subscription", &unit, &qty, "saas"
			}
			g.Create(&pr)
			prices[v.ID] = pr
			variants = append(variants, v)
			for fi, fname := range vd.files {
				body := fmt.Sprintf("demo file %s for %s %s\n", fname, pd.name, vd.name)
				key := storage.NewKey(fname)
				size, sha, _ := st.Put(context.Background(), key, strings.NewReader(body), "application/octet-stream")
				ext := ""
				if i := strings.LastIndex(fname, "."); i >= 0 {
					ext = fname[i+1:]
				}
				g.Create(&db.File{VariantID: v.ID, Identifier: uuid.NewString(), Name: fname, Extension: ext, Size: size, SHA256: sha, Version: "1.4.2", Sort: fi + 1, Status: "published",
					StorageDriver: st.Driver(), StorageKey: key, DownloadsCount: int64(rng.Intn(40)), TestMode: *test, CreatedAt: p.CreatedAt, UpdatedAt: now})
			}
		}
		_ = pi
	}

	names := []string{"Ava Johnson", "Liam Chen", "Noah Müller", "Emma Dubois", "Olivia Rossi", "Mia Tanaka", "Lucas Silva", "Sofia García", "Ethan Kim", "Chloé Martin", "Aiden Clark", "Zoe Walker"}
	countries := []string{"US", "CN", "DE", "FR", "IT", "JP", "BR", "ES", "KR", "FR", "GB", "CA"}
	var customers []db.Customer
	for i, n := range names {
		c := db.Customer{StoreID: store.ID, TestMode: *test, Email: fmt.Sprintf("%s@example.com", strings.ToLower(strings.ReplaceAll(strings.Split(n, " ")[0], "é", "e"))), Name: n, Status: "subscribed", Country: countries[i], StripeCustomerID: fmt.Sprintf("cus_demo%02d", i+1), CreatedAt: ago(65-i*5, 10), UpdatedAt: now}
		g.Create(&c)
		customers = append(customers, c)
	}

	orderNo := int64(1000)
	var subs int
	for day := 60; day >= 0; day-- {
		n := rng.Intn(3)
		if day%7 == 0 {
			n++
		}
		for k := 0; k < n; k++ {
			c := customers[rng.Intn(len(customers))]
			v := variants[rng.Intn(len(variants))]
			pr := prices[v.ID]
			var prod db.Product
			g.First(&prod, v.ProductID)
			orderNo++
			tax := pr.UnitPrice * 8 / 100
			created := ago(day, 8+rng.Intn(12))
			o := db.Order{StoreID: store.ID, CustomerID: c.ID, Identifier: uuid.NewString(), OrderNumber: orderNo, UserName: c.Name, UserEmail: c.Email, Currency: "USD", CurrencyRate: "1.0000",
				Subtotal: pr.UnitPrice, Tax: tax, Total: pr.UnitPrice + tax, SubtotalUSD: pr.UnitPrice, TaxUSD: tax, TotalUSD: pr.UnitPrice + tax, TaxRate: "8.00", Status: "paid",
				Country: c.Country, CardBrand: []string{"visa", "mastercard", "amex"}[rng.Intn(3)], CardLastFour: fmt.Sprintf("%04d", rng.Intn(10000)), CustomData: db.J(map[string]any{}),
				StripeCheckoutSession: db.Ptr(fmt.Sprintf("cs_demo_%d", orderNo)), StripePaymentIntent: fmt.Sprintf("pi_demo_%d", orderNo), StripeChargeID: fmt.Sprintf("ch_demo_%d", orderNo),
				StripeCustomerID: c.StripeCustomerID, Source: "stripe", TestMode: *test, CreatedAt: created, UpdatedAt: created}
			if rng.Intn(14) == 0 {
				o.Status, o.Refunded, o.RefundedAmount, o.RefundedAmountUSD, o.RefundedAt = "refunded", true, o.Total, o.Total, &created
			}
			g.Create(&o)
			it := db.OrderItem{OrderID: o.ID, ProductID: prod.ID, VariantID: v.ID, PriceID: &pr.ID, ProductName: prod.Name, VariantName: v.Name, Price: pr.UnitPrice, Quantity: 1, TestMode: *test, CreatedAt: created, UpdatedAt: created}
			g.Create(&it)
			var subID *int64
			if pr.Category == "subscription" {
				subs++
				renews := created.AddDate(0, 1, 0)
				for renews.Before(now) {
					renews = renews.AddDate(0, 1, 0)
				}
				status := "active"
				if subs%4 == 0 {
					status = "cancelled"
				}
				sub := db.Subscription{StoreID: store.ID, CustomerID: c.ID, OrderID: o.ID, OrderItemID: it.ID, ProductID: prod.ID, VariantID: v.ID, PriceID: &pr.ID, ProductName: prod.Name, VariantName: v.Name,
					UserName: c.Name, UserEmail: c.Email, Status: status, Cancelled: status == "cancelled", CardBrand: o.CardBrand, CardLastFour: o.CardLastFour, PaymentProcessor: "stripe", Quantity: 1,
					BillingAnchor: created.Day(), RenewsAt: &renews, StripeSubscriptionID: db.Ptr(fmt.Sprintf("sub_demo_%d", subs)), StripeCustomerID: c.StripeCustomerID, TestMode: *test, CreatedAt: created, UpdatedAt: now}
				if status == "cancelled" {
					sub.EndsAt = &renews
				}
				g.Create(&sub)
				g.Create(&db.SubscriptionItem{SubscriptionID: sub.ID, PriceID: pr.ID, Quantity: 1, CreatedAt: created, UpdatedAt: created})
				for m, t := 0, created; t.Before(now) && m < 6; m, t = m+1, t.AddDate(0, 1, 0) {
					reason := "renewal"
					if m == 0 {
						reason = "initial"
					}
					g.Create(&db.SubscriptionInvoice{StoreID: store.ID, SubscriptionID: sub.ID, CustomerID: c.ID, UserName: c.Name, UserEmail: c.Email, BillingReason: reason, CardBrand: o.CardBrand,
						CardLastFour: o.CardLastFour, Currency: "USD", CurrencyRate: "1.0000", Status: "paid", Subtotal: pr.UnitPrice, Total: pr.UnitPrice, SubtotalUSD: pr.UnitPrice, TotalUSD: pr.UnitPrice,
						StripeInvoiceID: db.Ptr(fmt.Sprintf("in_demo_%d_%d", sub.ID, m)), TestMode: *test, CreatedAt: t, UpdatedAt: t})
				}
				subID = &sub.ID
			}
			if v.HasLicenseKeys {
				var limit *int
				if !v.IsLicenseLimitUnlimited {
					l := v.LicenseActivationLimit
					limit = &l
				}
				k := db.LicenseKey{StoreID: store.ID, CustomerID: c.ID, OrderID: o.ID, OrderItemID: it.ID, ProductID: prod.ID, VariantID: v.ID, SubscriptionID: subID, UserName: c.Name, UserEmail: c.Email,
					Key: uuid.NewString(), ActivationLimit: limit, Disabled: o.Refunded, TestMode: *test, CreatedAt: created, UpdatedAt: created}
				g.Create(&k)
				if !o.Refunded {
					for n := rng.Intn(3); n > 0 && (limit == nil || k.InstancesCount < *limit); n-- {
						g.Create(&db.LicenseKeyInstance{LicenseKeyID: k.ID, Identifier: uuid.NewString(), Name: []string{"MacBook Pro", "Windows desktop", "build-server-01", "example.com"}[rng.Intn(4)], CreatedAt: created, UpdatedAt: created})
						k.InstancesCount++
					}
					g.Model(&k).Update("instances_count", k.InstancesCount)
				}
			}
		}
	}

	hook := db.Webhook{StoreID: store.ID, URL: "https://example.com/hooks/fulfillment", Events: db.J([]string{"order_created", "order_refunded", "license_key_created", "subscription_created", "subscription_updated"}),
		Secret: "demo-secret", TestMode: *test, CreatedAt: ago(30, 9), UpdatedAt: now}
	g.Create(&hook)
	for i := 0; i < 8; i++ {
		status, code, errMsg := "success", 200, ""
		if i == 3 {
			status, code, errMsg = "failed", 500, "HTTP 500"
		}
		t := now.Add(-time.Duration(i) * 3 * time.Hour)
		g.Create(&db.WebhookDelivery{WebhookID: hook.ID, StoreID: store.ID, EventName: []string{"order_created", "license_key_created", "subscription_updated"}[i%3], ResourceType: "orders", ResourceID: fmt.Sprint(1001 + i),
			Payload: `{"meta":{"event_name":"order_created"},"data":{"type":"orders","id":"1"}}`, Status: status, Attempts: 1, LastStatusCode: code, LastError: errMsg, DurationMS: 120 + i*7, CreatedAt: t, NextAttemptAt: t, DeliveredAt: &t})
	}
	for i, typ := range []string{"checkout.session.completed", "invoice.paid", "charge.refunded", "customer.subscription.updated", "checkout.session.completed"} {
		t := now.Add(-time.Duration(i) * 5 * time.Hour)
		g.Create(&db.StripeEvent{ID: fmt.Sprintf("evt_demo_%02d", i+1), Type: typ, Status: "processed", Attempts: 1, Payload: fmt.Sprintf(`{"id":"evt_demo_%02d","type":%q,"data":{"object":{"id":"obj_%d"}}}`, i+1, typ, i), ReceivedAt: t, ProcessedAt: &t})
	}
	fmt.Printf("demo data created: %d products' worth of variants, %d customers, orders up to #%d, %d subscriptions\n", len(variants), len(customers), orderNo, subs)
}
