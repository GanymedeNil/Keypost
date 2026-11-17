package importer

import "fulfillment/internal/db"

// scopeSets restricts an API import to the requested stores. Only a few
// resources can be filtered by store on the server; the rest are filtered
// through the ids of their parents (variants by product, order items by
// order, ...), which are collected while importing or read from the database
// when the parent resource is not part of this run.
type scopeSets struct {
	on     bool
	stores map[int64]bool
	sets   map[string]map[int64]bool
}

func (in *ingest) initScope() {
	in.scope = scopeSets{sets: map[string]map[int64]bool{}}
	if len(in.cfg.storeIDs) == 0 {
		return
	}
	in.scope.on = true
	in.scope.stores = map[int64]bool{}
	for _, id := range in.cfg.storeIDs {
		in.scope.stores[id] = true
	}
}

// scopeSet returns the ids of one resource inside the selected stores.
func (in *ingest) scopeSet(name string) map[int64]bool {
	if s, ok := in.scope.sets[name]; ok {
		return s
	}
	s := map[int64]bool{}
	in.scope.sets[name] = s
	var ids []int64
	var q = in.g
	switch name {
	case "products":
		q = q.Model(&db.Product{}).Where("store_id IN ?", in.cfg.storeIDs)
	case "orders":
		q = q.Model(&db.Order{}).Where("store_id IN ?", in.cfg.storeIDs)
	case "subscriptions":
		q = q.Model(&db.Subscription{}).Where("store_id IN ?", in.cfg.storeIDs)
	case "license_keys":
		q = q.Model(&db.LicenseKey{}).Where("store_id IN ?", in.cfg.storeIDs)
	case "variants":
		q = q.Model(&db.Variant{}).Where("product_id IN (?)", in.g.Model(&db.Product{}).Select("id").Where("store_id IN ?", in.cfg.storeIDs))
	default:
		return s
	}
	if err := q.Pluck("id", &ids).Error; err == nil {
		for _, id := range ids {
			s[id] = true
		}
	}
	return s
}

// inScope decides whether an object belongs to the selected stores and
// remembers its id so its children can be matched later.
func (in *ingest) inScope(res string, o lsObject) bool {
	if !in.scope.on {
		return true
	}
	a := o.A
	parent := func(set, attr string) bool {
		id := a.i64(attr)
		return id != 0 && in.scopeSet(set)[id]
	}
	remember := func(set string) {
		in.scopeSet(set)[o.ID] = true
	}
	switch res {
	case "stores":
		return in.scope.stores[o.ID]
	case "customers", "subscription_invoices":
		return !a.present("store_id") || in.scope.stores[a.i64("store_id")]
	case "products", "orders", "subscriptions", "license_keys":
		if a.present("store_id") && !in.scope.stores[a.i64("store_id")] {
			return false
		}
		remember(res)
		return true
	case "variants":
		if !parent("products", "product_id") {
			return false
		}
		remember("variants")
		return true
	case "prices", "files":
		return parent("variants", "variant_id")
	case "order_items":
		return parent("orders", "order_id")
	case "subscription_items":
		return parent("subscriptions", "subscription_id")
	case "license_key_instances":
		return parent("license_keys", "license_key_id")
	}
	return true
}
