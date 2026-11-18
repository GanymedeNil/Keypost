package importer

import (
	"fmt"
	"strings"
)

const (
	tsA = "2024-01-02T03:04:05.000000Z"
	tsB = "2024-02-03T04:05:06.000000Z"

	fileIdent1 = "6dce5ba7-76f2-481f-ad1e-9c2bec6eb0e2"
	fileIdent2 = "0b6a7a52-1c1e-4b4b-9a52-3f1d5c2b9e11"
	instIdent  = "f70a79fa-6054-433e-9c1b-6075344292e4"

	keyActive   = "80e15db5-c796-436b-850c-8f9c98a48abe"
	keyUnlim    = "38b1460a-5104-4067-a91d-77b872934d51"
	keyDisabled = "5f2b0a6e-3e6c-4a0e-8f3b-0c1a7a1c2d3e"
	keyExpired  = "9a7d1c44-0f1e-4c53-b1a2-6d7e8f9a0b1c"

	orderIdent1 = "104e18a2-d755-4d4b-80c4-a6c1dcbe1c10"
)

var (
	fileBytes1 = []byte("PK-fake-zip-content-" + strings.Repeat("x", 4096))
	fileBytes2 = []byte("%PDF-1.4 fake pdf")
)

// loadFixtures fills the fake API with a small but complete store history.
func loadFixtures(f *fakeLS) {
	base := f.URL()
	f.setFile("/download/"+fileIdent1, fileBytes1)
	f.setFile("/download/"+fileIdent2, fileBytes2)

	store := func(id int64, name, slug, cur string) map[string]any {
		return jobj("stores", id, map[string]any{
			"name": name, "slug": slug, "domain": slug + ".lemonsqueezy.com", "url": "https://" + slug + ".lemonsqueezy.com", "avatar_url": "",
			"plan": "fresh", "country": "US", "country_nicename": "United States", "currency": cur, "total_sales": 3, "total_revenue": 6000,
			"thirty_day_sales": 0, "thirty_day_revenue": 0, "created_at": tsA, "updated_at": tsB,
		})
	}
	f.set("stores", store(1, "My Store", "my-store", "USD"), store(2, "Second Store", "second-store", "EUR"))

	customer := func(id, storeID int64, name, email string) map[string]any {
		return jobj("customers", id, map[string]any{
			"store_id": storeID, "name": name, "email": email, "status": "subscribed", "city": "Berlin", "region": "BE", "country": "DE",
			"created_at": tsA, "updated_at": tsB, "test_mode": false,
		})
	}
	f.set("customers", customer(11, 1, "John Doe", "john@example.com"), customer(12, 1, "Jane Roe", "jane@example.com"), customer(21, 2, "Eva Lind", "eva@example.com"))

	product := func(id, storeID int64, name, slug string) map[string]any {
		return jobj("products", id, map[string]any{
			"store_id": storeID, "name": name, "slug": slug, "description": "<p>" + name + "</p>", "status": "published",
			"thumb_url": "https://cdn.example/t.png", "large_thumb_url": "https://cdn.example/l.png",
			"created_at": tsA, "updated_at": tsB, "test_mode": false,
		})
	}
	f.set("products", product(100, 1, "Pro App", "pro-app"), product(101, 1, "Course", "course"), product(110, 2, "Plugin", "plugin"))

	variant := func(id, productID int64, name, slug string, license bool, limit int, unlimited bool) map[string]any {
		return jobj("variants", id, map[string]any{
			"product_id": productID, "name": name, "slug": slug, "description": "<p>" + name + "</p>",
			"links":            []any{map[string]any{"title": "Docs", "url": "https://docs.example"}},
			"has_license_keys": license, "license_activation_limit": limit, "is_license_limit_unlimited": unlimited,
			"license_length_value": 1, "license_length_unit": "years", "is_license_length_unlimited": true,
			"sort": int(id % 10), "status": "published", "created_at": tsA, "updated_at": tsB, "test_mode": false,
		})
	}
	f.set("variants",
		variant(200, 100, "Default", "46beb127-a8a9-33e6-89b5-078505657239", true, 3, false),
		variant(201, 100, "Monthly", "1e5f8c1a-2b3c-4d5e-8f60-71829304a5b6", true, 0, true),
		variant(202, 101, "Default", "c3d4e5f6-0718-4293-a4b5-c6d7e8f90a1b", false, 0, true),
		variant(210, 110, "Default", "d4e5f607-1829-43a4-b5c6-d7e8f90a1b2c", false, 0, true),
	)

	price := func(id, variantID int64, category string, unit int64, renewal any) map[string]any {
		a := map[string]any{
			"variant_id": variantID, "category": category, "scheme": "standard", "usage_aggregation": nil, "unit_price": unit,
			"unit_price_decimal": nil, "setup_fee_enabled": nil, "setup_fee": nil, "package_size": 1, "tiers": nil, // one-time prices have both as null in the real API
			"renewal_interval_unit": nil, "renewal_interval_quantity": nil, "trial_interval_unit": nil, "trial_interval_quantity": nil,
			"min_price": nil, "suggested_price": nil, "tax_code": "eservice", "created_at": tsA, "updated_at": tsB,
		}
		if renewal != nil {
			a["setup_fee_enabled"] = false // subscription prices carry false
			a["renewal_interval_unit"], a["renewal_interval_quantity"] = renewal, 1
			a["trial_interval_unit"], a["trial_interval_quantity"] = "day", 14
		}
		return jobj("prices", id, a)
	}
	f.set("prices", price(300, 200, "one_time", 4900, nil), price(301, 201, "subscription", 999, "month"), price(302, 202, "one_time", 1500, nil), price(310, 210, "one_time", 2500, nil))

	file := func(id, variantID int64, ident, name, ext string, body []byte) map[string]any {
		return jobj("files", id, map[string]any{
			"variant_id": variantID, "identifier": ident, "name": name, "extension": ext,
			"download_url": fmt.Sprintf("%s/download/%s?expires=1&signature=abc", base, ident),
			"size":         len(body), "size_formatted": "x", "version": "1.0.0", "sort": 1, "status": "published",
			"createdAt": tsA, "updatedAt": tsB, "test_mode": false,
		})
	}
	f.set("files", file(400, 200, fileIdent1, "app.zip", "zip", fileBytes1), file(401, 202, fileIdent2, "guide.pdf", "pdf", fileBytes2))

	order := func(id, customerID int64, ident string, number int64, name, email string, total int64, status string, refunded bool, itemID int64) map[string]any {
		fmtMoney := func(c int64) string { return fmt.Sprintf("$%d.%02d", c/100, c%100) }
		refundedAmount := int64(0)
		var refundedAt any
		if status == "refunded" {
			refundedAmount, refundedAt = total, tsB
		}
		a := map[string]any{
			"store_id": 1, "customer_id": customerID, "affiliate_id": nil, "referral_amount": nil, "identifier": ident, "order_number": number,
			"user_name": name, "user_email": email, "currency": "USD", "currency_rate": "1.0000",
			"subtotal": total, "setup_fee": 0, "discount_total": 0, "tax": 0, "total": total, "refunded_amount": refundedAmount,
			"subtotal_usd": total, "setup_fee_usd": 0, "discount_total_usd": 0, "tax_usd": 0, "total_usd": total, "refunded_amount_usd": refundedAmount,
			"tax_name": nil, "tax_rate": "0.00", "tax_inclusive": false, "status": status, "status_formatted": strings.ToUpper(status[:1]) + status[1:],
			"refunded": refunded, "refunded_at": refundedAt,
			"subtotal_formatted": fmtMoney(total), "setup_fee_formatted": "$0.00", "discount_total_formatted": "$0.00", "tax_formatted": "$0.00",
			"total_formatted": fmtMoney(total), "refunded_amount_formatted": fmtMoney(refundedAmount),
			"urls":       map[string]any{"receipt": "https://app.lemonsqueezy.com/my-orders/" + ident},
			"created_at": tsA, "updated_at": tsB, "test_mode": false,
		}
		_ = itemID
		return jobj("orders", id, a)
	}
	o500 := order(500, 11, orderIdent1, 1, "John Doe", "john@example.com", 5880, "paid", false, 600)
	attrOf(o500)["subtotal"], attrOf(o500)["subtotal_usd"] = 4900, 4900
	attrOf(o500)["tax"], attrOf(o500)["tax_usd"] = 980, 980
	attrOf(o500)["tax_name"], attrOf(o500)["tax_rate"] = "VAT", "20.00"
	attrOf(o500)["subtotal_formatted"], attrOf(o500)["tax_formatted"] = "$49.00", "$9.80"
	o502 := order(502, 11, "7c1c2f5e-8f3d-4d0e-a5c9-1b2a3c4d5e6f", 3, "John Doe", "john@example.com", 1500, "refunded", true, 602)
	attrOf(o502)["affiliate_id"], attrOf(o502)["referral_amount"] = 7, 150
	f.set("orders",
		o500,
		order(501, 12, "2d6e8f1a-4b7c-4e9d-8a1b-3c5d7e9f1a2b", 2, "Jane Roe", "jane@example.com", 999, "paid", false, 601),
		o502,
		order(503, 11, "e8f9a0b1-c2d3-4e5f-8a7b-9c0d1e2f3a4b", 4, "John Doe", "john@example.com", 999, "paid", false, 603),
		jobj("orders", 520, map[string]any{ // belongs to the second store
			"store_id": 2, "customer_id": 21, "identifier": "aaaa1111-2222-4333-8444-555566667777", "order_number": 1, "user_name": "Eva Lind",
			"user_email": "eva@example.com", "currency": "EUR", "currency_rate": "0.9200", "subtotal": 2300, "total": 2300, "subtotal_usd": 2500, "total_usd": 2500,
			"status": "paid", "tax_rate": "0.00", "created_at": tsA, "updated_at": tsB, "test_mode": false,
		}),
	)
	// first_order_item is derived from the order items, check it against the LS shape
	attrOf(o500)["first_order_item"] = map[string]any{
		"id": 600, "order_id": 500, "product_id": 100, "variant_id": 200, "product_name": "Pro App", "variant_name": "Default",
		"price": 4900, "created_at": tsA, "updated_at": tsB, "test_mode": false,
	}

	item := func(id, orderID, productID, variantID int64, pn, vn string, price int64) map[string]any {
		return jobj("order-items", id, map[string]any{
			"order_id": orderID, "product_id": productID, "variant_id": variantID, "product_name": pn, "variant_name": vn,
			"price": price, "quantity": 1, "created_at": tsA, "updated_at": tsB, "test_mode": false,
		})
	}
	f.set("order-items",
		item(600, 500, 100, 200, "Pro App", "Default", 4900),
		item(601, 501, 100, 201, "Pro App", "Monthly", 999),
		item(602, 502, 101, 202, "Course", "Default", 1500),
		item(603, 503, 100, 201, "Pro App", "Monthly", 999),
		item(620, 520, 110, 210, "Plugin", "Default", 2300),
	)

	sub := func(id, customerID, orderID, itemID int64, name, email, status, processor string, itemSub, price, qty int64) map[string]any {
		a := map[string]any{
			"store_id": 1, "customer_id": customerID, "order_id": orderID, "order_item_id": itemID, "product_id": 100, "variant_id": 201,
			"product_name": "Pro App", "variant_name": "Monthly", "user_name": name, "user_email": email, "status": status,
			"status_formatted": strings.ToUpper(status[:1]) + strings.ReplaceAll(status[1:], "_", " "),
			"card_brand":       "visa", "card_last_four": "4242", "payment_processor": processor, "pause": nil, "cancelled": status == "cancelled",
			"trial_ends_at": nil, "billing_anchor": 12,
			"first_subscription_item": map[string]any{"id": itemSub, "subscription_id": id, "price_id": price, "quantity": qty, "created_at": tsA, "updated_at": tsB},
			"renews_at":               "2030-01-12T00:00:00.000000Z", "ends_at": nil, "created_at": tsA, "updated_at": tsB, "test_mode": false,
		}
		return jobj("subscriptions", id, a)
	}
	s700 := sub(700, 12, 501, 601, "Jane Roe", "jane@example.com", "active", "stripe", 800, 301, 5)
	s701 := sub(701, 11, 503, 603, "John Doe", "john@example.com", "cancelled", "paypal", 802, 301, 1)
	attrOf(s701)["ends_at"] = "2025-03-01T00:00:00.000000Z"
	attrOf(s701)["card_brand"], attrOf(s701)["card_last_four"] = "", ""
	f.set("subscriptions", s700, s701)

	f.set("subscription-items",
		jobj("subscription-items", 800, map[string]any{"subscription_id": 700, "price_id": 301, "quantity": 5, "is_usage_based": false, "created_at": tsA, "updated_at": tsB}),
		jobj("subscription-items", 801, map[string]any{"subscription_id": 700, "price_id": 301, "quantity": 2, "is_usage_based": false, "created_at": tsA, "updated_at": tsB}),
		jobj("subscription-items", 802, map[string]any{"subscription_id": 701, "price_id": 301, "quantity": 1, "is_usage_based": false, "created_at": tsA, "updated_at": tsB}),
	)

	invoice := func(id int64, reason string) map[string]any {
		return jobj("subscription-invoices", id, map[string]any{
			"store_id": 1, "subscription_id": 700, "customer_id": 12, "user_name": "Jane Roe", "user_email": "jane@example.com",
			"billing_reason": reason, "card_brand": "visa", "card_last_four": "4242", "currency": "USD", "currency_rate": "1.0000",
			"status": "paid", "status_formatted": "Paid", "refunded": false, "refunded_at": nil, "subtotal": 999, "discount_total": 0, "tax": 0,
			"tax_inclusive": false, "total": 999, "refunded_amount": 0, "subtotal_usd": 999, "discount_total_usd": 0, "tax_usd": 0, "total_usd": 999,
			"refunded_amount_usd": 0, "subtotal_formatted": "$9.99", "discount_total_formatted": "$0.00", "tax_formatted": "$0.00",
			"total_formatted": "$9.99", "refunded_amount_formatted": "$0.00",
			"urls":       map[string]any{"invoice_url": "https://app.lemonsqueezy.com/my-orders/x/subscription-invoice/" + reason},
			"created_at": tsA, "updated_at": tsB, "test_mode": false,
		})
	}
	f.set("subscription-invoices", invoice(900, "initial"), invoice(901, "renewal"))

	key := func(id, orderID, itemID, customerID int64, name, email, k string, limit any, instances int, disabled int, status string, expires any) map[string]any {
		return jobj("license-keys", id, map[string]any{
			"store_id": 1, "customer_id": customerID, "order_id": orderID, "order_item_id": itemID, "product_id": 100,
			"user_name": name, "user_email": email, "key": k, "key_short": "XXXX-" + k[len(k)-12:], "activation_limit": limit,
			"instances_count": instances, "disabled": disabled, "status": status, "status_formatted": strings.ToUpper(status[:1]) + status[1:],
			"expires_at": expires, "created_at": tsA, "updated_at": tsB,
		})
	}
	f.set("license-keys",
		key(1000, 500, 600, 11, "John Doe", "john@example.com", keyActive, 3, 1, 0, "active", nil),
		key(1001, 501, 601, 12, "Jane Roe", "jane@example.com", keyUnlim, nil, 0, 0, "inactive", nil),
		key(1002, 502, 602, 11, "John Doe", "john@example.com", keyDisabled, 1, 0, 1, "disabled", nil),
		key(1003, 503, 603, 11, "John Doe", "john@example.com", keyExpired, 5, 0, 0, "expired", "2025-03-01T00:00:00.000000Z"),
	)
	f.set("license-key-instances",
		jobj("license-key-instances", 1100, map[string]any{"license_key_id": 1000, "identifier": instIdent, "name": "laptop", "created_at": tsA, "updated_at": tsB}),
	)
}

func apiOptions(f *fakeLS) LSOptions {
	return LSOptions{APIKey: "lsk_secret_abcd", BaseURL: f.URL(), DownloadFiles: true}
}
