package lsapi

import (
	"net/http"
	"strconv"
	"time"

	"gorm.io/gorm"

	"fulfillment/internal/db"
	"fulfillment/internal/resource"
)

// ---------------------------------------------------------------- include helpers

func uniqIDs(ids []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id != 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func belongsTo[M, R any](fk func(M) int64, build func(*API, *Auth, []R) []resource.Resource) func(*API, *Auth, []M) []resource.Resource {
	return func(a *API, au *Auth, ms []M) []resource.Resource {
		ids := make([]int64, 0, len(ms))
		for _, m := range ms {
			ids = append(ids, fk(m))
		}
		ids = uniqIDs(ids)
		if len(ids) == 0 {
			return nil
		}
		var rows []R
		a.S.DB.Where("id IN ?", ids).Find(&rows)
		return build(a, au, rows)
	}
}

func hasMany[M, R any](idOf func(M) int64, col string, build func(*API, *Auth, []R) []resource.Resource) func(*API, *Auth, []M) []resource.Resource {
	return func(a *API, au *Auth, ms []M) []resource.Resource {
		ids := make([]int64, 0, len(ms))
		for _, m := range ms {
			ids = append(ids, idOf(m))
		}
		if len(ids) == 0 {
			return nil
		}
		var rows []R
		a.S.DB.Where(col+" IN ?", ids).Order("id").Find(&rows)
		return build(a, au, rows)
	}
}

func opt(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// ---------------------------------------------------------------- builders (signature adapters)

func buildStores(a *API, au *Auth, ms []db.Store) []resource.Resource {
	out := make([]resource.Resource, len(ms))
	for i, m := range ms {
		out[i] = a.S.Res.Store(m, au.TestMode)
	}
	return out
}
func buildCustomers(a *API, _ *Auth, ms []db.Customer) []resource.Resource {
	return a.S.Res.Customers(ms)
}
func buildProducts(a *API, _ *Auth, ms []db.Product) []resource.Resource { return a.S.Res.Products(ms) }
func buildVariants(a *API, _ *Auth, ms []db.Variant) []resource.Resource { return a.S.Res.Variants(ms) }
func buildPrices(a *API, _ *Auth, ms []db.Price) []resource.Resource     { return a.S.Res.Prices(ms) }
func buildFiles(a *API, _ *Auth, ms []db.File) []resource.Resource       { return a.S.Res.Files(ms) }
func buildOrders(a *API, _ *Auth, ms []db.Order) []resource.Resource     { return a.S.Res.Orders(ms) }
func buildOrderItems(a *API, _ *Auth, ms []db.OrderItem) []resource.Resource {
	return a.S.Res.OrderItems(ms)
}
func buildSubs(a *API, _ *Auth, ms []db.Subscription) []resource.Resource {
	return a.S.Res.Subscriptions(ms)
}
func buildSubItems(a *API, _ *Auth, ms []db.SubscriptionItem) []resource.Resource {
	return a.S.Res.SubscriptionItems(ms)
}
func buildSubInvoices(a *API, _ *Auth, ms []db.SubscriptionInvoice) []resource.Resource {
	return a.S.Res.SubscriptionInvoices(ms)
}
func buildKeys(a *API, _ *Auth, ms []db.LicenseKey) []resource.Resource {
	return a.S.Res.LicenseKeys(ms)
}
func buildInstances(a *API, _ *Auth, ms []db.LicenseKeyInstance) []resource.Resource {
	return a.S.Res.LicenseKeyInstances(ms)
}
func buildWebhooks(a *API, _ *Auth, ms []db.Webhook) []resource.Resource { return a.S.Res.Webhooks(ms) }

// ---------------------------------------------------------------- specs

var storeSpec = spec[db.Store]{
	typ: "stores", idOf: func(m db.Store) int64 { return m.ID }, build: buildStores,
}

var customerSpec = spec[db.Customer]{
	typ: "customers", scope: testModeScope, idOf: func(m db.Customer) int64 { return m.ID }, build: buildCustomers,
	filters: map[string]func(*gorm.DB, string) *gorm.DB{"store_id": eqFilter("store_id"), "email": eqFilter("email")},
	includes: map[string]func(*API, *Auth, []db.Customer) []resource.Resource{
		"store":         belongsTo(func(m db.Customer) int64 { return m.StoreID }, buildStores),
		"orders":        hasMany(func(m db.Customer) int64 { return m.ID }, "customer_id", buildOrders),
		"subscriptions": hasMany(func(m db.Customer) int64 { return m.ID }, "customer_id", buildSubs),
		"license-keys":  hasMany(func(m db.Customer) int64 { return m.ID }, "customer_id", buildKeys),
	},
}

var productSpec = spec[db.Product]{
	typ: "products", scope: testModeScope, idOf: func(m db.Product) int64 { return m.ID }, build: buildProducts,
	filters: map[string]func(*gorm.DB, string) *gorm.DB{"store_id": eqFilter("store_id"), "status": eqFilter("status")},
	includes: map[string]func(*API, *Auth, []db.Product) []resource.Resource{
		"store":    belongsTo(func(m db.Product) int64 { return m.StoreID }, buildStores),
		"variants": hasMany(func(m db.Product) int64 { return m.ID }, "product_id", buildVariants),
	},
}

var variantSpec = spec[db.Variant]{
	typ: "variants", scope: testModeScope, idOf: func(m db.Variant) int64 { return m.ID }, build: buildVariants,
	filters: map[string]func(*gorm.DB, string) *gorm.DB{"product_id": eqFilter("product_id"), "status": eqFilter("status")},
	includes: map[string]func(*API, *Auth, []db.Variant) []resource.Resource{
		"product":     belongsTo(func(m db.Variant) int64 { return m.ProductID }, buildProducts),
		"files":       hasMany(func(m db.Variant) int64 { return m.ID }, "variant_id", buildFiles),
		"price-model": hasMany(func(m db.Variant) int64 { return m.ID }, "variant_id", buildPrices),
		"prices":      hasMany(func(m db.Variant) int64 { return m.ID }, "variant_id", buildPrices),
	},
}

var priceSpec = spec[db.Price]{
	typ: "prices", idOf: func(m db.Price) int64 { return m.ID }, build: buildPrices,
	scope: func(q *gorm.DB, au *Auth) *gorm.DB {
		return q.Where("variant_id IN (?)", q.Session(&gorm.Session{NewDB: true}).Table("variants").Select("id").Where("test_mode = ?", au.TestMode))
	},
	filters: map[string]func(*gorm.DB, string) *gorm.DB{"variant_id": eqFilter("variant_id")},
	includes: map[string]func(*API, *Auth, []db.Price) []resource.Resource{
		"variant": belongsTo(func(m db.Price) int64 { return m.VariantID }, buildVariants),
	},
}

var fileSpec = spec[db.File]{
	typ: "files", scope: testModeScope, idOf: func(m db.File) int64 { return m.ID }, build: buildFiles,
	filters: map[string]func(*gorm.DB, string) *gorm.DB{"variant_id": eqFilter("variant_id"), "status": eqFilter("status")},
	includes: map[string]func(*API, *Auth, []db.File) []resource.Resource{
		"variant": belongsTo(func(m db.File) int64 { return m.VariantID }, buildVariants),
	},
}

var orderSpec = spec[db.Order]{
	typ: "orders", scope: testModeScope, idOf: func(m db.Order) int64 { return m.ID }, build: buildOrders,
	filters: map[string]func(*gorm.DB, string) *gorm.DB{
		"store_id": eqFilter("store_id"), "user_email": eqFilter("user_email"), "customer_id": eqFilter("customer_id"), "status": eqFilter("status"),
	},
	includes: map[string]func(*API, *Auth, []db.Order) []resource.Resource{
		"store":         belongsTo(func(m db.Order) int64 { return m.StoreID }, buildStores),
		"customer":      belongsTo(func(m db.Order) int64 { return m.CustomerID }, buildCustomers),
		"order-items":   hasMany(func(m db.Order) int64 { return m.ID }, "order_id", buildOrderItems),
		"subscriptions": hasMany(func(m db.Order) int64 { return m.ID }, "order_id", buildSubs),
		"license-keys":  hasMany(func(m db.Order) int64 { return m.ID }, "order_id", buildKeys),
	},
}

var orderItemSpec = spec[db.OrderItem]{
	typ: "order-items", scope: testModeScope, idOf: func(m db.OrderItem) int64 { return m.ID }, build: buildOrderItems,
	filters: map[string]func(*gorm.DB, string) *gorm.DB{
		"order_id": eqFilter("order_id"), "product_id": eqFilter("product_id"), "variant_id": eqFilter("variant_id"),
	},
	includes: map[string]func(*API, *Auth, []db.OrderItem) []resource.Resource{
		"order":   belongsTo(func(m db.OrderItem) int64 { return m.OrderID }, buildOrders),
		"product": belongsTo(func(m db.OrderItem) int64 { return m.ProductID }, buildProducts),
		"variant": belongsTo(func(m db.OrderItem) int64 { return m.VariantID }, buildVariants),
	},
}

var subscriptionSpec = spec[db.Subscription]{
	typ: "subscriptions", scope: testModeScope, idOf: func(m db.Subscription) int64 { return m.ID }, build: buildSubs,
	filters: map[string]func(*gorm.DB, string) *gorm.DB{
		"store_id": eqFilter("store_id"), "order_id": eqFilter("order_id"), "order_item_id": eqFilter("order_item_id"),
		"product_id": eqFilter("product_id"), "variant_id": eqFilter("variant_id"), "user_email": eqFilter("user_email"), "status": eqFilter("status"),
	},
	includes: map[string]func(*API, *Auth, []db.Subscription) []resource.Resource{
		"store":                 belongsTo(func(m db.Subscription) int64 { return m.StoreID }, buildStores),
		"customer":              belongsTo(func(m db.Subscription) int64 { return m.CustomerID }, buildCustomers),
		"order":                 belongsTo(func(m db.Subscription) int64 { return m.OrderID }, buildOrders),
		"order-item":            belongsTo(func(m db.Subscription) int64 { return m.OrderItemID }, buildOrderItems),
		"product":               belongsTo(func(m db.Subscription) int64 { return m.ProductID }, buildProducts),
		"variant":               belongsTo(func(m db.Subscription) int64 { return m.VariantID }, buildVariants),
		"subscription-items":    hasMany(func(m db.Subscription) int64 { return m.ID }, "subscription_id", buildSubItems),
		"subscription-invoices": hasMany(func(m db.Subscription) int64 { return m.ID }, "subscription_id", buildSubInvoices),
	},
}

var subItemSpec = spec[db.SubscriptionItem]{
	typ: "subscription-items", idOf: func(m db.SubscriptionItem) int64 { return m.ID }, build: buildSubItems,
	scope: func(q *gorm.DB, au *Auth) *gorm.DB {
		return q.Where("subscription_id IN (?)", q.Session(&gorm.Session{NewDB: true}).Table("subscriptions").Select("id").Where("test_mode = ?", au.TestMode))
	},
	filters: map[string]func(*gorm.DB, string) *gorm.DB{"subscription_id": eqFilter("subscription_id"), "price_id": eqFilter("price_id")},
	includes: map[string]func(*API, *Auth, []db.SubscriptionItem) []resource.Resource{
		"subscription": belongsTo(func(m db.SubscriptionItem) int64 { return m.SubscriptionID }, buildSubs),
		"price":        belongsTo(func(m db.SubscriptionItem) int64 { return m.PriceID }, buildPrices),
	},
}

var subInvoiceSpec = spec[db.SubscriptionInvoice]{
	typ: "subscription-invoices", scope: testModeScope, idOf: func(m db.SubscriptionInvoice) int64 { return m.ID }, build: buildSubInvoices,
	filters: map[string]func(*gorm.DB, string) *gorm.DB{
		"store_id": eqFilter("store_id"), "status": eqFilter("status"), "refunded": boolFilter("refunded"), "subscription_id": eqFilter("subscription_id"),
	},
	includes: map[string]func(*API, *Auth, []db.SubscriptionInvoice) []resource.Resource{
		"store":        belongsTo(func(m db.SubscriptionInvoice) int64 { return m.StoreID }, buildStores),
		"subscription": belongsTo(func(m db.SubscriptionInvoice) int64 { return m.SubscriptionID }, buildSubs),
	},
}

func licenseStatusFilter(q *gorm.DB, v string) *gorm.DB {
	now := time.Now().UTC()
	switch v {
	case "disabled":
		return q.Where("disabled = ?", true)
	case "expired":
		return q.Where("disabled = ? AND expires_at IS NOT NULL AND expires_at <= ?", false, now)
	case "active":
		return q.Where("disabled = ? AND instances_count > 0 AND (expires_at IS NULL OR expires_at > ?)", false, now)
	case "inactive":
		return q.Where("disabled = ? AND instances_count = 0 AND (expires_at IS NULL OR expires_at > ?)", false, now)
	}
	return q
}

var licenseKeySpec = spec[db.LicenseKey]{
	typ: "license-keys", scope: testModeScope, idOf: func(m db.LicenseKey) int64 { return m.ID }, build: buildKeys,
	filters: map[string]func(*gorm.DB, string) *gorm.DB{
		"store_id": eqFilter("store_id"), "order_id": eqFilter("order_id"), "order_item_id": eqFilter("order_item_id"),
		"product_id": eqFilter("product_id"), "status": licenseStatusFilter,
	},
	includes: map[string]func(*API, *Auth, []db.LicenseKey) []resource.Resource{
		"store":                 belongsTo(func(m db.LicenseKey) int64 { return m.StoreID }, buildStores),
		"customer":              belongsTo(func(m db.LicenseKey) int64 { return m.CustomerID }, buildCustomers),
		"order":                 belongsTo(func(m db.LicenseKey) int64 { return m.OrderID }, buildOrders),
		"order-item":            belongsTo(func(m db.LicenseKey) int64 { return m.OrderItemID }, buildOrderItems),
		"product":               belongsTo(func(m db.LicenseKey) int64 { return m.ProductID }, buildProducts),
		"license-key-instances": hasMany(func(m db.LicenseKey) int64 { return m.ID }, "license_key_id", buildInstances),
	},
}

var instanceSpec = spec[db.LicenseKeyInstance]{
	typ: "license-key-instances", idOf: func(m db.LicenseKeyInstance) int64 { return m.ID }, build: buildInstances,
	scope: func(q *gorm.DB, au *Auth) *gorm.DB {
		return q.Where("license_key_id IN (?)", q.Session(&gorm.Session{NewDB: true}).Table("license_keys").Select("id").Where("test_mode = ?", au.TestMode))
	},
	filters: map[string]func(*gorm.DB, string) *gorm.DB{"license_key_id": eqFilter("license_key_id")},
	includes: map[string]func(*API, *Auth, []db.LicenseKeyInstance) []resource.Resource{
		"license-key": belongsTo(func(m db.LicenseKeyInstance) int64 { return m.LicenseKeyID }, buildKeys),
	},
}

var webhookSpec = spec[db.Webhook]{
	typ: "webhooks", scope: testModeScope, idOf: func(m db.Webhook) int64 { return m.ID }, build: buildWebhooks,
	filters: map[string]func(*gorm.DB, string) *gorm.DB{"store_id": eqFilter("store_id")},
	includes: map[string]func(*API, *Auth, []db.Webhook) []resource.Resource{
		"store": belongsTo(func(m db.Webhook) int64 { return m.StoreID }, buildStores),
	},
}

// ---------------------------------------------------------------- routes

func scopeBy(col string) func(q *gorm.DB, r *http.Request) *gorm.DB {
	return func(q *gorm.DB, r *http.Request) *gorm.DB {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		return q.Where(col+" = ?", id)
	}
}

func emptyList(a *API) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, au *Auth) {
		base := a.S.URLs.Base + r.URL.Path
		write(w, http.StatusOK, listDoc{
			Meta:    map[string]any{"page": pageMeta{CurrentPage: 1, LastPage: 1, PerPage: 10}},
			JSONAPI: v10, Links: map[string]string{"first": base, "last": base}, Data: []resource.Resource{},
		})
	}
}

// Routes registers every /v1 endpoint on mux.
func (a *API) Routes(mux *http.ServeMux) {
	// License API (public)
	mux.HandleFunc("POST /v1/licenses/activate", a.licenseActivate)
	mux.HandleFunc("POST /v1/licenses/validate", a.licenseValidate)
	mux.HandleFunc("POST /v1/licenses/deactivate", a.licenseDeactivate)

	get := func(pattern string, h authedHandler) { mux.HandleFunc(pattern, a.auth(h)) }

	get("GET /v1/users/me", a.usersMe)

	get("GET /v1/stores", listOf(a, storeSpec, nil))
	get("GET /v1/stores/{id}", getOf(a, storeSpec))
	get("GET /v1/stores/{id}/products", listOf(a, productSpec, scopeBy("store_id")))
	get("GET /v1/stores/{id}/orders", listOf(a, orderSpec, scopeBy("store_id")))
	get("GET /v1/stores/{id}/subscriptions", listOf(a, subscriptionSpec, scopeBy("store_id")))
	get("GET /v1/stores/{id}/license-keys", listOf(a, licenseKeySpec, scopeBy("store_id")))
	get("GET /v1/stores/{id}/webhooks", listOf(a, webhookSpec, scopeBy("store_id")))
	get("GET /v1/stores/{id}/discounts", emptyList(a))

	get("GET /v1/customers", listOf(a, customerSpec, nil))
	get("POST /v1/customers", a.createCustomer)
	get("GET /v1/customers/{id}", getOf(a, customerSpec))
	get("PATCH /v1/customers/{id}", a.updateCustomer)
	get("GET /v1/customers/{id}/orders", listOf(a, orderSpec, scopeBy("customer_id")))
	get("GET /v1/customers/{id}/subscriptions", listOf(a, subscriptionSpec, scopeBy("customer_id")))
	get("GET /v1/customers/{id}/license-keys", listOf(a, licenseKeySpec, scopeBy("customer_id")))

	get("GET /v1/products", listOf(a, productSpec, nil))
	get("GET /v1/products/{id}", getOf(a, productSpec))
	get("GET /v1/products/{id}/variants", listOf(a, variantSpec, scopeBy("product_id")))

	get("GET /v1/variants", listOf(a, variantSpec, nil))
	get("GET /v1/variants/{id}", getOf(a, variantSpec))
	get("GET /v1/variants/{id}/files", listOf(a, fileSpec, scopeBy("variant_id")))
	get("GET /v1/variants/{id}/prices", listOf(a, priceSpec, scopeBy("variant_id")))
	get("GET /v1/variants/{id}/price-model", listOf(a, priceSpec, scopeBy("variant_id")))

	get("GET /v1/prices", listOf(a, priceSpec, nil))
	get("GET /v1/prices/{id}", getOf(a, priceSpec))

	get("GET /v1/files", listOf(a, fileSpec, nil))
	get("GET /v1/files/{id}", getOf(a, fileSpec))

	get("GET /v1/orders", listOf(a, orderSpec, nil))
	get("GET /v1/orders/{id}", getOf(a, orderSpec))
	get("POST /v1/orders/{id}/refund", a.refundOrder)
	get("POST /v1/orders/{id}/generate-invoice", a.generateInvoice)
	get("GET /v1/orders/{id}/order-items", listOf(a, orderItemSpec, scopeBy("order_id")))
	get("GET /v1/orders/{id}/subscriptions", listOf(a, subscriptionSpec, scopeBy("order_id")))
	get("GET /v1/orders/{id}/license-keys", listOf(a, licenseKeySpec, scopeBy("order_id")))
	get("GET /v1/orders/{id}/discount-redemptions", emptyList(a))

	get("GET /v1/order-items", listOf(a, orderItemSpec, nil))
	get("GET /v1/order-items/{id}", getOf(a, orderItemSpec))

	get("GET /v1/subscriptions", listOf(a, subscriptionSpec, nil))
	get("GET /v1/subscriptions/{id}", getOf(a, subscriptionSpec))
	get("PATCH /v1/subscriptions/{id}", a.updateSubscription)
	get("DELETE /v1/subscriptions/{id}", a.cancelSubscription)
	get("GET /v1/subscriptions/{id}/subscription-items", listOf(a, subItemSpec, scopeBy("subscription_id")))
	get("GET /v1/subscriptions/{id}/subscription-invoices", listOf(a, subInvoiceSpec, scopeBy("subscription_id")))

	get("GET /v1/subscription-invoices", listOf(a, subInvoiceSpec, nil))
	get("GET /v1/subscription-invoices/{id}", getOf(a, subInvoiceSpec))
	get("POST /v1/subscription-invoices/{id}/refund", a.refundSubscriptionInvoice)

	get("GET /v1/subscription-items", listOf(a, subItemSpec, nil))
	get("GET /v1/subscription-items/{id}", getOf(a, subItemSpec))

	get("GET /v1/license-keys", listOf(a, licenseKeySpec, nil))
	get("GET /v1/license-keys/{id}", getOf(a, licenseKeySpec))
	get("PATCH /v1/license-keys/{id}", a.updateLicenseKey)
	get("GET /v1/license-keys/{id}/license-key-instances", listOf(a, instanceSpec, scopeBy("license_key_id")))

	get("GET /v1/license-key-instances", listOf(a, instanceSpec, nil))
	get("GET /v1/license-key-instances/{id}", getOf(a, instanceSpec))

	get("GET /v1/webhooks", listOf(a, webhookSpec, nil))
	get("POST /v1/webhooks", a.createWebhook)
	get("GET /v1/webhooks/{id}", getOf(a, webhookSpec))
	get("PATCH /v1/webhooks/{id}", a.updateWebhook)
	get("DELETE /v1/webhooks/{id}", a.deleteWebhook)

	get("POST /v1/checkouts", a.createCheckout)
	get("GET /v1/checkouts", a.listCheckouts)
	get("GET /v1/checkouts/{id}", a.getCheckout)

	// to-one relationship endpoints, e.g. /v1/orders/1/customer
	get("GET /v1/{parent}/{id}/{rel}", a.relatedOne)

	// unsupported resources → explicit JSON:API 404 instead of an HTML page
	for _, p := range []string{"discounts", "discount-redemptions", "usage-records", "affiliates"} {
		pp := p
		get("GET /v1/"+pp, func(w http.ResponseWriter, r *http.Request, au *Auth) {
			fail(w, http.StatusNotFound, "Not Found", "The "+pp+" resource is not provided by this service (manage it in Stripe).", "")
		})
	}
}

// relatedOne resolves to-one relationships by loading the parent and
// delegating to the child's retrieve handler.
func (a *API) relatedOne(w http.ResponseWriter, r *http.Request, au *Auth) {
	id, ok := pathID(r)
	parent, rel := r.PathValue("parent"), r.PathValue("rel")
	notFound := func() {
		fail(w, http.StatusNotFound, "Not Found", "The requested resource could not be found.", "")
	}
	if !ok {
		notFound()
		return
	}
	var childID int64
	var child authedHandler
	db0 := a.S.DB.WithContext(r.Context())
	switch parent + "/" + rel {
	case "customers/store", "products/store", "orders/store", "subscriptions/store", "license-keys/store", "webhooks/store", "subscription-invoices/store":
		var storeID int64
		table := map[string]string{"customers": "customers", "products": "products", "orders": "orders", "subscriptions": "subscriptions", "license-keys": "license_keys", "webhooks": "webhooks", "subscription-invoices": "subscription_invoices"}[parent]
		if db0.Table(table).Select("store_id").Where("id = ?", id).Scan(&storeID).Error != nil || storeID == 0 {
			notFound()
			return
		}
		childID, child = storeID, getOf(a, storeSpec)
	case "orders/customer", "subscriptions/customer", "license-keys/customer":
		var cid int64
		table := map[string]string{"orders": "orders", "subscriptions": "subscriptions", "license-keys": "license_keys"}[parent]
		if db0.Table(table).Select("customer_id").Where("id = ?", id).Scan(&cid).Error != nil || cid == 0 {
			notFound()
			return
		}
		childID, child = cid, getOf(a, customerSpec)
	case "subscriptions/order", "license-keys/order", "order-items/order":
		var oid int64
		table := map[string]string{"subscriptions": "subscriptions", "license-keys": "license_keys", "order-items": "order_items"}[parent]
		if db0.Table(table).Select("order_id").Where("id = ?", id).Scan(&oid).Error != nil || oid == 0 {
			notFound()
			return
		}
		childID, child = oid, getOf(a, orderSpec)
	case "subscriptions/order-item", "license-keys/order-item":
		var oid int64
		table := map[string]string{"subscriptions": "subscriptions", "license-keys": "license_keys"}[parent]
		if db0.Table(table).Select("order_item_id").Where("id = ?", id).Scan(&oid).Error != nil || oid == 0 {
			notFound()
			return
		}
		childID, child = oid, getOf(a, orderItemSpec)
	case "subscriptions/product", "license-keys/product", "order-items/product":
		var pid int64
		table := map[string]string{"subscriptions": "subscriptions", "license-keys": "license_keys", "order-items": "order_items"}[parent]
		if db0.Table(table).Select("product_id").Where("id = ?", id).Scan(&pid).Error != nil || pid == 0 {
			notFound()
			return
		}
		childID, child = pid, getOf(a, productSpec)
	case "subscriptions/variant", "order-items/variant", "files/variant", "prices/variant":
		var vid int64
		table := map[string]string{"subscriptions": "subscriptions", "order-items": "order_items", "files": "files", "prices": "prices"}[parent]
		if db0.Table(table).Select("variant_id").Where("id = ?", id).Scan(&vid).Error != nil || vid == 0 {
			notFound()
			return
		}
		childID, child = vid, getOf(a, variantSpec)
	case "variants/product":
		var pid int64
		if db0.Table("variants").Select("product_id").Where("id = ?", id).Scan(&pid).Error != nil || pid == 0 {
			notFound()
			return
		}
		childID, child = pid, getOf(a, productSpec)
	case "license-key-instances/license-key":
		var kid int64
		if db0.Table("license_key_instances").Select("license_key_id").Where("id = ?", id).Scan(&kid).Error != nil || kid == 0 {
			notFound()
			return
		}
		childID, child = kid, getOf(a, licenseKeySpec)
	case "subscription-invoices/subscription", "subscription-items/subscription":
		var sid int64
		table := map[string]string{"subscription-invoices": "subscription_invoices", "subscription-items": "subscription_items"}[parent]
		if db0.Table(table).Select("subscription_id").Where("id = ?", id).Scan(&sid).Error != nil || sid == 0 {
			notFound()
			return
		}
		childID, child = sid, getOf(a, subscriptionSpec)
	case "subscription-items/price":
		var pid int64
		if db0.Table("subscription_items").Select("price_id").Where("id = ?", id).Scan(&pid).Error != nil || pid == 0 {
			notFound()
			return
		}
		childID, child = pid, getOf(a, priceSpec)
	default:
		notFound()
		return
	}
	r.SetPathValue("id", strconv.FormatInt(childID, 10))
	child(w, r, au)
}
