package importer

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"

	"fulfillment/internal/db"
)

// maxBufferedCustomers bounds the memory used to find duplicate customers.
const maxBufferedCustomers = 2_000_000

func (in *ingest) initHandlers() {
	in.handlers = map[string]*stageHandler{
		"stores": newBatch(in, "stores", in.mapStore, func(s db.Store) int64 { return s.ID }, nil,
			upsertWriter[db.Store](in, in.storeSpec())),
		"customers": in.customerHandler(),
		"products": newBatch(in, "products", in.mapProduct, func(p db.Product) int64 { return p.ID }, nil,
			upsertWriter[db.Product](in, in.newSpec(&db.Product{}, []string{"stripe_product_id"}, nil))),
		"variants": newBatch(in, "variants", in.mapVariant, func(v db.Variant) int64 { return v.ID }, nil,
			upsertWriter[db.Variant](in, in.newSpec(&db.Variant{}, []string{"stripe_product_id"}, nil))),
		"prices": newBatch(in, "prices", in.mapPrice, func(p db.Price) int64 { return p.ID }, nil,
			upsertWriter[db.Price](in, in.newSpec(&db.Price{}, []string{"stripe_price_id"}, nil))),
		"files": in.fileHandler(),
		"orders": newBatch(in, "orders", in.mapOrder, func(o db.Order) int64 { return o.ID }, in.vetOrders,
			upsertWriter[db.Order](in, in.newSpec(&db.Order{},
				[]string{"custom_data", "stripe_checkout_session", "stripe_payment_intent", "stripe_invoice_id", "stripe_charge_id", "stripe_receipt_url", "stripe_customer_id"},
				[]string{"card_brand", "card_last_four", "city", "country", "postal_code", "tax_number"}))),
		"order_items": newBatch(in, "order_items", in.mapOrderItem, func(i db.OrderItem) int64 { return i.ID }, nil,
			upsertWriter[db.OrderItem](in, in.newSpec(&db.OrderItem{}, nil, nil))),
		"subscriptions": newBatch(in, "subscriptions", in.mapSubscription, func(r subRow) int64 { return r.sub.ID }, nil,
			in.subscriptionWriter()),
		"subscription_items": newBatch(in, "subscription_items", in.mapSubscriptionItem, func(i db.SubscriptionItem) int64 { return i.ID }, nil,
			upsertWriter[db.SubscriptionItem](in, in.newSpec(&db.SubscriptionItem{}, []string{"stripe_subscription_item"}, nil))),
		"subscription_invoices": newBatch(in, "subscription_invoices", in.mapInvoice, func(i db.SubscriptionInvoice) int64 { return i.ID }, nil,
			upsertWriter[db.SubscriptionInvoice](in, in.newSpec(&db.SubscriptionInvoice{}, []string{"stripe_invoice_id", "stripe_payment_intent", "stripe_charge_id"}, nil))),
		"license_keys": newBatch(in, "license_keys", in.mapLicenseKey, func(k db.LicenseKey) int64 { return k.ID }, nil,
			upsertWriter[db.LicenseKey](in, in.newSpec(&db.LicenseKey{}, nil, nil))),
		"license_key_instances": newBatch(in, "license_key_instances", in.mapInstance, func(i db.LicenseKeyInstance) int64 { return i.ID }, nil,
			upsertWriter[db.LicenseKeyInstance](in, in.newSpec(&db.LicenseKeyInstance{}, nil, nil))),
	}
}

// storeSpec overwrites even a freshly created placeholder store (the
// default store of an empty installation is newer than any Lemon Squeezy
// timestamp) but keeps the settings that only exist locally.
func (in *ingest) storeSpec() upsertSpec {
	spec := in.newSpec(&db.Store{}, []string{"stripe_account_id", "support_email", "email_from_name", "receipt_subject", "receipt_intro"}, nil)
	spec.always = true
	return spec
}

// vetOrders refuses to overwrite a local order (one that was not imported
// from Lemon Squeezy) that happens to use the same id as an imported one.
func (in *ingest) vetOrders(rows []db.Order) ([]db.Order, error) {
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	var existing []db.Order
	if err := in.g.Select("id", "identifier", "source", "order_number").Where("id IN ?", ids).Find(&existing).Error; err != nil {
		return rows, nil // the write itself will report a broken database
	}
	local := map[int64]db.Order{}
	for _, o := range existing {
		if o.Source != "lemonsqueezy" {
			local[o.ID] = o
		}
	}
	if len(local) == 0 {
		return rows, nil
	}
	out := make([]db.Order, 0, len(rows))
	for _, r := range rows {
		if ex, clash := local[r.ID]; clash && ex.Identifier != r.Identifier {
			if err := in.rowFailed("orders", r.ID, fmt.Errorf("the id is used by local order #%d (%s, source %s); it was not overwritten", ex.OrderNumber, ex.Identifier, ex.Source)); err != nil {
				return nil, err
			}
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// subscriptionWriter upserts subscriptions with their first item and copies
// what the subscription knows about the payer onto its (blank) order.
func (in *ingest) subscriptionWriter() func(tx *gorm.DB, rows []subRow) error {
	subSpec := in.newSpec(&db.Subscription{}, []string{"stripe_subscription_id", "stripe_customer_id", "custom_data"}, nil)
	itemSpec := in.newSpec(&db.SubscriptionItem{}, []string{"stripe_subscription_item"}, nil)
	return func(tx *gorm.DB, rows []subRow) error {
		subs := make([]db.Subscription, 0, len(rows))
		var items []db.SubscriptionItem
		for _, r := range rows {
			subs = append(subs, r.sub)
			if r.item != nil {
				items = append(items, *r.item)
			}
		}
		if err := upsert(tx, subSpec, subs, in.r.batch()); err != nil {
			return err
		}
		if err := upsert(tx, itemSpec, items, in.r.batch()); err != nil {
			return err
		}
		for _, r := range rows {
			if err := fillOrderFromSubscription(tx, r.sub); err != nil {
				return err
			}
		}
		return nil
	}
}

func fillOrderFromSubscription(tx *gorm.DB, s db.Subscription) error {
	if s.OrderID == 0 {
		return nil
	}
	fill := func(column, value string) error {
		if value == "" {
			return nil
		}
		return tx.Model(&db.Order{}).Where("id = ? AND ("+column+" IS NULL OR "+column+" = '')", s.OrderID).UpdateColumn(column, value).Error
	}
	for _, f := range [][2]string{{"card_brand", s.CardBrand}, {"card_last_four", s.CardLastFour}, {"user_name", s.UserName}, {"user_email", s.UserEmail}} {
		if err := fill(f[0], f[1]); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- customers

func (in *ingest) mapCustomer(o lsObject) (db.Customer, bool) {
	a := o.A
	email := strings.TrimSpace(a.str("email"))
	if email == "" {
		in.invalid("customers", fmt.Sprintf("#%d has no e-mail address", o.ID))
		return db.Customer{}, false
	}
	created, updated := stamps(a)
	return db.Customer{
		ID: o.ID, StoreID: a.i64("store_id"), TestMode: in.testMode(a), Email: email, Name: a.str("name"),
		Status: orStr(strings.ToLower(a.trim("status")), "subscribed"), City: a.str("city"), Region: a.str("region"),
		Country: strings.ToUpper(a.trim("country")), CreatedAt: created, UpdatedAt: updated,
	}, true
}

// customerHandler buffers a whole customer list: duplicates (same store, mode
// and e-mail) can only be detected once every customer has been seen.
func (in *ingest) customerHandler() *stageHandler {
	spec := in.newSpec(&db.Customer{}, []string{"stripe_customer_id"}, nil)
	var buf []db.Customer
	h := &stageHandler{}
	h.add = func(_ context.Context, o lsObject) error {
		c, ok := in.mapCustomer(o)
		if !ok {
			return nil
		}
		if len(buf) >= maxBufferedCustomers {
			return fmt.Errorf("more than %d customers cannot be merged in one run", maxBufferedCustomers)
		}
		in.j.count("customers", 1)
		in.j.advance(1)
		buf = append(buf, c)
		return nil
	}
	h.flush = func(context.Context) error {
		rows := in.mergeCustomers(buf)
		buf = nil
		write := upsertWriter[db.Customer](in, spec)
		for start := 0; start < len(rows); start += in.r.batch() {
			end := min(start+in.r.batch(), len(rows))
			if err := writeBatch(in, "customers", rows[start:end], func(c db.Customer) int64 { return c.ID }, write); err != nil {
				return err
			}
		}
		return nil
	}
	return h
}

// existingCustomers returns the lowest local id per (store, mode, e-mail).
func (in *ingest) existingCustomers(cs []db.Customer) map[string]int64 {
	seen := map[string]bool{}
	var emails []string
	for _, c := range cs {
		e := strings.ToLower(c.Email)
		if !seen[e] {
			seen[e] = true
			emails = append(emails, e)
		}
	}
	out := map[string]int64{}
	for start := 0; start < len(emails); start += 400 {
		chunk := emails[start:min(start+400, len(emails))]
		var rows []db.Customer
		if err := in.g.Select("id", "store_id", "test_mode", "email").Where("LOWER(email) IN ?", chunk).Find(&rows).Error; err != nil {
			continue
		}
		for _, r := range rows {
			k := custKey(r.StoreID, r.TestMode, r.Email)
			if cur, ok := out[k]; !ok || r.ID < cur {
				out[k] = r.ID
			}
		}
	}
	return out
}

// mergeCustomers collapses duplicate customers onto the lowest id (or onto
// the row that already exists locally) and records the remapping used for
// orders, subscriptions and license keys.
func (in *ingest) mergeCustomers(buf []db.Customer) []db.Customer {
	if len(buf) == 0 {
		return nil
	}
	groups := map[string][]*db.Customer{}
	var order []string
	for i := range buf {
		k := custKey(buf[i].StoreID, buf[i].TestMode, buf[i].Email)
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], &buf[i])
	}
	existing := in.existingCustomers(buf)
	out := make([]db.Customer, 0, len(order))
	for _, k := range order {
		members := groups[k]
		sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
		canonical, write := members[0].ID, members[0]
		if ex, ok := existing[k]; ok && ex != canonical {
			canonical, write = ex, nil
			for _, m := range members {
				if m.ID == ex {
					write = m
				}
			}
		}
		for _, m := range members {
			if m.ID == canonical {
				continue
			}
			in.custRemap[m.ID] = canonical
			in.j.warn("customer #%d (%s) duplicates customer #%d: merged into #%d (orders, subscriptions and license keys are re-pointed)", m.ID, m.Email, canonical, canonical)
		}
		if write != nil {
			out = append(out, *write)
		}
		in.custIDs[canonical] = true
		in.custMail[k] = canonical
	}
	return out
}
