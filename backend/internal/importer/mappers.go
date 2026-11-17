package importer

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"fulfillment/internal/db"
)

// Mappers turn a Lemon Squeezy object into a model. They never touch the
// database for writing; lookups of related rows go through the id caches.
// ok=false means the object was rejected after logging why.

func orStr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// stamps returns created_at / updated_at with sensible fallbacks.
func stamps(a attrs) (time.Time, time.Time) {
	created := a.when("created_at", db.Now())
	updated := a.when("updated_at", created)
	if a.present("createdAt") && !a.present("created_at") {
		created = a.when("createdAt", created)
		updated = a.when("updatedAt", created)
	}
	return created, updated
}

// ---------------------------------------------------------------- stores

func (in *ingest) loadSlugs() {
	if in.slugsDone {
		return
	}
	in.slugsDone = true
	in.slugs = map[string]int64{}
	var rows []db.Store
	if in.g.Select("id", "slug").Find(&rows).Error == nil {
		for _, s := range rows {
			in.slugs[s.Slug] = s.ID
		}
	}
}

func (in *ingest) mapStore(o lsObject) (db.Store, bool) {
	a := o.A
	created, updated := stamps(a)
	in.loadSlugs()
	slug := orStr(a.trim("slug"), fmt.Sprintf("store-%d", o.ID))
	if owner, taken := in.slugs[slug]; taken && owner != o.ID {
		renamed := fmt.Sprintf("%s-%d", slug, o.ID)
		in.j.warn("store #%d: slug %q is already used by store #%d, imported as %q", o.ID, slug, owner, renamed)
		slug = renamed
	}
	in.slugs[slug] = o.ID
	s := db.Store{
		ID: o.ID, Name: orStr(a.str("name"), fmt.Sprintf("Store %d", o.ID)), Slug: slug, Domain: a.str("domain"), URL: a.str("url"),
		AvatarURL: a.str("avatar_url"), Plan: orStr(a.str("plan"), "self-hosted"), Country: strings.ToUpper(orStr(a.trim("country"), "US")),
		Currency: strings.ToUpper(orStr(a.trim("currency"), "USD")), CreatedAt: created, UpdatedAt: updated,
	}
	in.storeCur.put(s.ID, s.Currency)
	return s, true
}

// ---------------------------------------------------------------- catalogue

func (in *ingest) mapProduct(o lsObject) (db.Product, bool) {
	a := o.A
	created, updated := stamps(a)
	p := db.Product{
		ID: o.ID, StoreID: a.i64("store_id"), Name: orStr(a.str("name"), fmt.Sprintf("Product %d", o.ID)), Slug: a.str("slug"),
		Description: a.str("description"), Status: orStr(strings.ToLower(a.trim("status")), "draft"),
		ThumbURL: a.str("thumb_url"), LargeThumbURL: a.str("large_thumb_url"), TestMode: in.testMode(a), CreatedAt: created, UpdatedAt: updated,
	}
	in.prodStore.put(p.ID, p.StoreID)
	return p, true
}

func (in *ingest) mapVariant(o lsObject) (db.Variant, bool) {
	a := o.A
	created, updated := stamps(a)
	links := []db.VariantLink{}
	for _, l := range a.list("links") {
		if m, ok := l.(map[string]any); ok {
			links = append(links, db.VariantLink{Title: attrs(m).str("title"), URL: attrs(m).str("url")})
		}
	}
	v := db.Variant{
		ID: o.ID, ProductID: a.i64("product_id"), Name: orStr(a.str("name"), "Default"), Slug: orStr(a.trim("slug"), fmt.Sprintf("variant-%d", o.ID)),
		Description: a.str("description"), Links: db.J(links), Sort: a.intv("sort"), Status: orStr(strings.ToLower(a.trim("status")), "published"),
		HasLicenseKeys: a.boolean("has_license_keys"), LicenseActivationLimit: a.intv("license_activation_limit"),
		IsLicenseLimitUnlimited: a.boolean("is_license_limit_unlimited"), LicenseLengthValue: a.intv("license_length_value"),
		LicenseLengthUnit: orStr(strings.ToLower(a.trim("license_length_unit")), "years"), IsLicenseLengthUnlimited: a.boolean("is_license_length_unlimited"),
		TestMode: in.testMode(a), CreatedAt: created, UpdatedAt: updated,
	}
	in.varProd.put(v.ID, v.ProductID)
	return v, true
}

// currencyOf is the currency of the store a variant belongs to.
func (in *ingest) currencyOf(variantID int64) string {
	if pid, ok := in.varProd.get(variantID); ok {
		if sid, ok := in.prodStore.get(pid); ok {
			if c, ok := in.storeCur.get(sid); ok && c != "" {
				return c
			}
		}
	}
	return "USD"
}

func (in *ingest) mapPrice(o lsObject) (db.Price, bool) {
	a := o.A
	created, updated := stamps(a)
	tiers := []db.PriceTier{}
	for _, t := range a.list("tiers") {
		if m, ok := t.(map[string]any); ok {
			ta := attrs(m)
			tiers = append(tiers, db.PriceTier{LastUnit: m["last_unit"], UnitPrice: ta.i64p("unit_price"), UnitPriceDecimal: ta.dec("unit_price_decimal"), FixedFee: ta.i64p("fixed_fee")})
		}
	}
	pkg := a.intv("package_size")
	if pkg <= 0 {
		pkg = 1
	}
	variantID := a.i64("variant_id")
	p := db.Price{
		ID: o.ID, VariantID: variantID, Category: orStr(strings.ToLower(a.trim("category")), "one_time"), Scheme: orStr(strings.ToLower(a.trim("scheme")), "standard"),
		UsageAggregation: a.strp("usage_aggregation"), UnitPrice: a.i64("unit_price"), UnitPriceDecimal: a.dec("unit_price_decimal"),
		SetupFeeEnabled: a.boolean("setup_fee_enabled"), SetupFee: a.i64p("setup_fee"), PackageSize: pkg, Tiers: db.J(tiers),
		RenewalIntervalUnit: a.strp("renewal_interval_unit"), RenewalIntervalQuantity: a.intp("renewal_interval_quantity"),
		TrialIntervalUnit: a.strp("trial_interval_unit"), TrialIntervalQuantity: a.intp("trial_interval_quantity"),
		MinPrice: a.i64p("min_price"), SuggestedPrice: a.i64p("suggested_price"), TaxCode: orStr(a.trim("tax_code"), "eservice"),
		Currency: in.currencyOf(variantID), CreatedAt: created, UpdatedAt: updated,
	}
	return p, true
}

// ---------------------------------------------------------------- orders

// customerRef maps a Lemon Squeezy customer id onto the id it was stored
// under (duplicates are merged). Unknown customers are looked up by e-mail.
func (in *ingest) customerRef(what string, id, storeID int64, test bool, email string) int64 {
	if id == 0 {
		return 0
	}
	if to, ok := in.custRemap[id]; ok {
		return to
	}
	if in.custIDs[id] {
		return id
	}
	if ok, _ := in.custExist.get(id); ok {
		return id
	}
	if email != "" {
		if to := in.customerByEmail(storeID, test, email); to != 0 {
			in.warnOnce(fmt.Sprintf("custmail-%d", id), "%s: customer #%d is unknown, linked to customer #%d by e-mail %s", what, id, to, email)
			return to
		}
	}
	in.warnOnce(fmt.Sprintf("custmiss-%d", id), "%s refers to customer #%d which is not part of the import", what, id)
	return id
}

func custKey(storeID int64, test bool, email string) string {
	return fmt.Sprintf("%d|%t|%s", storeID, test, strings.ToLower(strings.TrimSpace(email)))
}

func (in *ingest) customerByEmail(storeID int64, test bool, email string) int64 {
	k := custKey(storeID, test, email)
	if id, ok := in.custMail[k]; ok {
		return id
	}
	var rows []db.Customer
	err := in.g.Select("id").Where("store_id = ? AND test_mode = ? AND LOWER(email) = ?", storeID, test, strings.ToLower(strings.TrimSpace(email))).Order("id").Limit(1).Find(&rows).Error
	id := int64(0)
	if err == nil && len(rows) > 0 {
		id = rows[0].ID
	}
	in.custMail[k] = id
	return id
}

func (in *ingest) mapOrder(o lsObject) (db.Order, bool) {
	a := o.A
	created, updated := stamps(a)
	status := orStr(strings.ToLower(a.trim("status")), "paid")
	total, totalUSD := a.i64("total"), a.i64("total_usd")
	refundedAmt, refundedUSD := a.i64("refunded_amount"), a.i64("refunded_amount_usd")
	refunded := a.boolean("refunded") || status == "refunded"
	if status == "refunded" && refundedAmt == 0 {
		refundedAmt, refundedUSD = total, totalUSD
	}
	refundedAt := a.tm("refunded_at")
	if refunded && refundedAt == nil {
		refundedAt = &updated
	}
	identifier := a.trim("identifier")
	if identifier == "" {
		identifier = uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("lemonsqueezy-order-%d", o.ID))).String()
		in.j.warn("order #%d has no identifier; generated %s", o.ID, identifier)
	}
	storeID, test := a.i64("store_id"), in.testMode(a)
	email := strings.TrimSpace(a.str("user_email"))
	ord := db.Order{
		ID: o.ID, StoreID: storeID, CustomerID: in.customerRef(fmt.Sprintf("order #%d", o.ID), a.i64("customer_id"), storeID, test, email),
		Identifier: identifier, OrderNumber: a.i64("order_number"), UserName: a.str("user_name"), UserEmail: email,
		Currency: strings.ToUpper(orStr(a.trim("currency"), "USD")), CurrencyRate: orStr(a.trim("currency_rate"), "1.0000"),
		Subtotal: a.i64("subtotal"), SetupFee: a.i64("setup_fee"), DiscountTotal: a.i64("discount_total"), Tax: a.i64("tax"), Total: total,
		RefundedAmount: refundedAmt, SubtotalUSD: a.i64("subtotal_usd"), SetupFeeUSD: a.i64("setup_fee_usd"),
		DiscountTotalUSD: a.i64("discount_total_usd"), TaxUSD: a.i64("tax_usd"), TotalUSD: totalUSD, RefundedAmountUSD: refundedUSD,
		TaxName: a.strp("tax_name"), TaxRate: orStr(a.trim("tax_rate"), "0.00"), TaxInclusive: a.boolean("tax_inclusive"),
		Status: status, Refunded: refunded, RefundedAt: refundedAt, AffiliateID: a.i64p("affiliate_id"), ReferralAmount: a.i64p("referral_amount"),
		City: a.str("city"), Country: a.str("country"), PostalCode: a.str("postal_code"), TaxNumber: a.str("tax_number"),
		CardBrand: strings.ToLower(a.str("card_brand")), CardLastFour: a.str("card_last_four"),
		Source: "lemonsqueezy", TestMode: test, CreatedAt: created, UpdatedAt: updated,
	}
	if !refunded {
		ord.RefundedAt = nil
	}
	in.orderTest.put(ord.ID, ord.TestMode)
	return ord, true
}

func (in *ingest) mapOrderItem(o lsObject) (db.OrderItem, bool) {
	a := o.A
	created, updated := stamps(a)
	orderID := a.i64("order_id")
	test := in.cfg.testMode
	if a.present("test_mode") {
		test = a.boolean("test_mode")
	} else if t, ok := in.orderTest.get(orderID); ok {
		test = t
	}
	qty := a.intv("quantity")
	if qty <= 0 {
		qty = 1
	}
	it := db.OrderItem{
		ID: o.ID, OrderID: orderID, ProductID: a.i64("product_id"), VariantID: a.i64("variant_id"), PriceID: a.i64p("price_id"),
		ProductName: a.str("product_name"), VariantName: a.str("variant_name"), Price: a.i64("price"), Quantity: qty,
		TestMode: test, CreatedAt: created, UpdatedAt: updated,
	}
	in.itemRefs.put(it.ID, itemRef{product: it.ProductID, variant: it.VariantID})
	return it, true
}

// ---------------------------------------------------------------- subscriptions

type subRow struct {
	sub  db.Subscription
	item *db.SubscriptionItem
}

func (in *ingest) mapSubscription(o lsObject) (subRow, bool) {
	a := o.A
	created, updated := stamps(a)
	status := orStr(strings.ToLower(a.trim("status")), "active")
	processor := orStr(strings.ToLower(a.trim("payment_processor")), "stripe")
	if processor == "paypal" {
		in.paypal++
	}
	var pause *db.SubscriptionPause
	if p := a.obj("pause"); p != nil {
		pause = &db.SubscriptionPause{Mode: p.str("mode"), ResumesAt: p.tm("resumes_at")}
	}
	qty := a.intv("quantity")
	var priceID *int64
	var item *db.SubscriptionItem
	if first := a.obj("first_subscription_item"); first != nil {
		priceID = first.i64p("price_id")
		if q := first.intv("quantity"); q > 0 {
			qty = q
		}
		if id := first.i64("id"); id > 0 {
			ic, iu := stamps(first)
			item = &db.SubscriptionItem{ID: id, SubscriptionID: o.ID, PriceID: first.i64("price_id"), Quantity: max(first.intv("quantity"), 1), IsUsageBased: first.boolean("is_usage_based"), CreatedAt: ic, UpdatedAt: iu}
		}
	}
	if qty <= 0 {
		qty = 1
	}
	storeID, test := a.i64("store_id"), in.testMode(a)
	email := strings.TrimSpace(a.str("user_email"))
	s := db.Subscription{
		ID: o.ID, StoreID: storeID, CustomerID: in.customerRef(fmt.Sprintf("subscription #%d", o.ID), a.i64("customer_id"), storeID, test, email),
		OrderID: a.i64("order_id"), OrderItemID: a.i64("order_item_id"), ProductID: a.i64("product_id"), VariantID: a.i64("variant_id"), PriceID: priceID,
		ProductName: a.str("product_name"), VariantName: a.str("variant_name"), UserName: a.str("user_name"), UserEmail: email, Status: status,
		CardBrand: strings.ToLower(a.str("card_brand")), CardLastFour: a.str("card_last_four"), PaymentProcessor: processor, Pause: db.J(pause),
		Cancelled: a.boolean("cancelled") || status == "cancelled", TrialEndsAt: a.tm("trial_ends_at"), BillingAnchor: a.intv("billing_anchor"),
		Quantity: qty, RenewsAt: a.tm("renews_at"), EndsAt: a.tm("ends_at"), TestMode: test, CreatedAt: created, UpdatedAt: updated,
	}
	if s.OrderItemID != 0 {
		if cur, ok := in.subByItem[s.OrderItemID]; !ok || cur == 0 || s.ID < cur {
			in.subByItem[s.OrderItemID] = s.ID
		}
	}
	return subRow{sub: s, item: item}, true
}

func (in *ingest) mapSubscriptionItem(o lsObject) (db.SubscriptionItem, bool) {
	a := o.A
	created, updated := stamps(a)
	return db.SubscriptionItem{
		ID: o.ID, SubscriptionID: a.i64("subscription_id"), PriceID: a.i64("price_id"), Quantity: max(a.intv("quantity"), 1),
		IsUsageBased: a.boolean("is_usage_based"), CreatedAt: created, UpdatedAt: updated,
	}, true
}

func (in *ingest) mapInvoice(o lsObject) (db.SubscriptionInvoice, bool) {
	a := o.A
	created, updated := stamps(a)
	status := orStr(strings.ToLower(a.trim("status")), "paid")
	refunded := a.boolean("refunded") || status == "refunded"
	storeID, test := a.i64("store_id"), in.testMode(a)
	email := strings.TrimSpace(a.str("user_email"))
	refundedAt := a.tm("refunded_at")
	if refunded && refundedAt == nil {
		refundedAt = &updated
	}
	refundedAmt, refundedUSD := a.i64("refunded_amount"), a.i64("refunded_amount_usd")
	if status == "refunded" && refundedAmt == 0 {
		refundedAmt, refundedUSD = a.i64("total"), a.i64("total_usd")
	}
	inv := db.SubscriptionInvoice{
		ID: o.ID, StoreID: storeID, SubscriptionID: a.i64("subscription_id"),
		CustomerID: in.customerRef(fmt.Sprintf("subscription invoice #%d", o.ID), a.i64("customer_id"), storeID, test, email),
		UserName:   a.str("user_name"), UserEmail: email, BillingReason: orStr(strings.ToLower(a.trim("billing_reason")), "renewal"),
		CardBrand: strings.ToLower(a.str("card_brand")), CardLastFour: a.str("card_last_four"), Currency: strings.ToUpper(orStr(a.trim("currency"), "USD")),
		CurrencyRate: orStr(a.trim("currency_rate"), "1.0000"), Status: status, Refunded: refunded, RefundedAt: refundedAt,
		Subtotal: a.i64("subtotal"), DiscountTotal: a.i64("discount_total"), Tax: a.i64("tax"), TaxInclusive: a.boolean("tax_inclusive"),
		Total: a.i64("total"), RefundedAmount: refundedAmt, SubtotalUSD: a.i64("subtotal_usd"), DiscountTotalUSD: a.i64("discount_total_usd"),
		TaxUSD: a.i64("tax_usd"), TotalUSD: a.i64("total_usd"), RefundedAmountUSD: refundedUSD,
		TestMode: test, CreatedAt: created, UpdatedAt: updated,
	}
	if u := a.obj("urls"); u != nil {
		inv.InvoiceURL = u.str("invoice_url")
	}
	if !refunded {
		inv.RefundedAt = nil
	}
	return inv, true
}

// ---------------------------------------------------------------- licensing

// subscriptionForItem finds the subscription created by an order item.
func (in *ingest) subscriptionForItem(itemID int64) int64 {
	if itemID == 0 {
		return 0
	}
	if id, ok := in.subByItem[itemID]; ok {
		return id
	}
	var rows []db.Subscription
	id := int64(0)
	if in.g.Select("id").Where("order_item_id = ?", itemID).Order("id").Limit(1).Find(&rows).Error == nil && len(rows) > 0 {
		id = rows[0].ID
	}
	in.subByItem[itemID] = id
	return id
}

func (in *ingest) mapLicenseKey(o lsObject) (db.LicenseKey, bool) {
	a := o.A
	key := strings.TrimSpace(a.str("key"))
	if key == "" {
		in.invalid("license_keys", fmt.Sprintf("#%d has no key", o.ID))
		return db.LicenseKey{}, false
	}
	created, updated := stamps(a)
	storeID, test := a.i64("store_id"), in.testMode(a)
	email := strings.TrimSpace(a.str("user_email"))
	itemID, productID, variantID := a.i64("order_item_id"), a.i64("product_id"), a.i64("variant_id")
	if ref, ok := in.itemRefs.get(itemID); ok {
		if ref.variant != 0 {
			variantID = ref.variant
		}
		if productID == 0 {
			productID = ref.product
		}
	} else if itemID != 0 && variantID == 0 {
		in.warnOnce(fmt.Sprintf("item-%d", itemID), "license key #%d: order item #%d is unknown, so the variant could not be determined", o.ID, itemID)
	}
	var subID *int64
	if sid := in.subscriptionForItem(itemID); sid != 0 {
		subID = &sid
	}
	disabled := a.boolean("disabled") || strings.EqualFold(a.trim("status"), "disabled")
	k := db.LicenseKey{
		ID: o.ID, StoreID: storeID, CustomerID: in.customerRef(fmt.Sprintf("license key #%d", o.ID), a.i64("customer_id"), storeID, test, email),
		OrderID: a.i64("order_id"), OrderItemID: itemID, ProductID: productID, VariantID: variantID, SubscriptionID: subID,
		UserName: a.str("user_name"), UserEmail: email, Key: key, ActivationLimit: a.intp("activation_limit"),
		InstancesCount: a.intv("instances_count"), Disabled: disabled, ExpiresAt: a.tm("expires_at"), TestMode: test, CreatedAt: created, UpdatedAt: updated,
	}
	return k, true
}

func (in *ingest) mapInstance(o lsObject) (db.LicenseKeyInstance, bool) {
	a := o.A
	ident := a.trim("identifier")
	if ident == "" {
		in.invalid("license_key_instances", fmt.Sprintf("#%d has no identifier", o.ID))
		return db.LicenseKeyInstance{}, false
	}
	created, updated := stamps(a)
	return db.LicenseKeyInstance{ID: o.ID, LicenseKeyID: a.i64("license_key_id"), Identifier: ident, Name: a.str("name"), CreatedAt: created, UpdatedAt: updated}, true
}
