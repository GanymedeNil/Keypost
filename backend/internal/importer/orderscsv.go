package importer

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"fulfillment/internal/db"
)

// The Lemon Squeezy order export (Orders → Export) is a CSV without numeric
// ids, store, status or customer ids. It is a best-effort fallback for
// archives that have no orders.json: the rows become customers, orders and
// one order item each; ids are allocated (or re-used when the order or
// customer already exists) by ingest.resolveCSV.

type csvRow struct {
	idx map[string]int
	rec []string
}

func (r csvRow) get(name string) string {
	i, ok := r.idx[name]
	if !ok || i >= len(r.rec) {
		return ""
	}
	return strings.TrimSpace(r.rec[i])
}

func (s *archiveSource) streamCSV(ctx context.Context, res string, h *streamHandler) error {
	seenCustomers := map[string]bool{}
	for _, f := range s.ordersCSV {
		rc, err := f.Open()
		if err != nil {
			s.j.errorf("%s: %v", f.Name, err)
			continue
		}
		rows, rerr := readCSV(&limitedReader{r: rc, n: maxEntryBytes})
		rc.Close()
		if rerr != nil {
			s.j.errorf("%s: %v (rows read so far are still used)", f.Name, rerr)
		}
		if len(rows) < 2 {
			continue
		}
		idx := map[string]int{}
		for i, c := range rows[0] {
			idx[normHeader(c)] = i
		}
		if _, ok := idx["identifier"]; !ok {
			s.j.errorf("%s: no \"identifier\" column; this does not look like a Lemon Squeezy order export", f.Name)
			continue
		}
		for _, rec := range rows[1:] {
			if err := ctx.Err(); err != nil {
				return err
			}
			row := csvRow{idx: idx, rec: rec}
			if row.get("identifier") == "" {
				h.invalid(fmt.Errorf("row without an order identifier"))
				continue
			}
			var obj lsObject
			switch res {
			case "customers":
				email := strings.ToLower(row.get("user_email"))
				if email == "" || seenCustomers[email] {
					continue
				}
				seenCustomers[email] = true
				obj = csvCustomer(row)
			case "orders":
				obj = csvOrder(row)
			case "order_items":
				obj = csvOrderItem(row)
			}
			if err := h.Object(obj); err != nil {
				return err
			}
		}
	}
	return nil
}

func csvCustomer(r csvRow) lsObject {
	return lsObject{Type: "customers", csv: true, A: attrs{
		"email": r.get("user_email"), "name": r.get("user_name"), "city": r.get("city"), "country": r.get("country"),
		"created_at": r.get("date_utc"), "status": "subscribed",
	}}
}

// csvOrder converts the export's USD cent amounts: the order currency
// columns are derived with the exported rate.
func csvOrder(r csvRow) lsObject {
	cur := strings.ToUpper(orStr(r.get("currency"), "USD"))
	rate := 1.0
	if v, err := strconv.ParseFloat(strings.ReplaceAll(r.get("currency_rate"), ",", "."), 64); err == nil && v > 0 && cur != "USD" {
		rate = v
	}
	usd := func(col string) int64 { n, _ := toInt(r.get(col)); return n }
	local := func(col string) int64 { return int64(math.Round(float64(usd(col)) * rate)) }
	rateStr := "1.0000"
	if rate != 1 {
		rateStr = strconv.FormatFloat(rate, 'f', -1, 64)
	}
	return lsObject{Type: "orders", csv: true, A: attrs{
		"identifier": r.get("identifier"), "order_number": r.get("order_number"), "user_name": r.get("user_name"), "user_email": r.get("user_email"),
		"currency": cur, "currency_rate": rateStr,
		"subtotal": local("subtotal"), "discount_total": local("discount_total"), "tax": local("tax"), "total": local("total"),
		"subtotal_usd": usd("subtotal"), "discount_total_usd": usd("discount_total"), "tax_usd": usd("tax"), "total_usd": usd("total"),
		"tax_name": r.get("tax_name"), "tax_rate": r.get("tax_rate"), "tax_number": r.get("tax_number"),
		"city": r.get("city"), "country": r.get("country"), "postal_code": r.get("postal_code"),
		"card_brand": r.get("card_brand"), "card_last_four": r.get("card_last_four"),
		"status": "paid", "created_at": r.get("date_utc"),
	}}
}

func csvOrderItem(r csvRow) lsObject {
	cur := strings.ToUpper(orStr(r.get("currency"), "USD"))
	rate := 1.0
	if v, err := strconv.ParseFloat(strings.ReplaceAll(r.get("currency_rate"), ",", "."), 64); err == nil && v > 0 && cur != "USD" {
		rate = v
	}
	sub, _ := toInt(r.get("subtotal"))
	return lsObject{Type: "order-items", csv: true, A: attrs{
		"_order_identifier": r.get("identifier"), "product_name": r.get("product_name"), "variant_name": r.get("variant_name"),
		"price": int64(math.Round(float64(sub) * rate)), "quantity": 1, "created_at": r.get("date_utc"),
	}}
}

// ---------------------------------------------------------------- id allocation

// allocID hands out ids above everything stored in the table.
func (in *ingest) allocID(table string) int64 {
	n, ok := in.nextIDs[table]
	if !ok {
		in.g.Table(table).Select("COALESCE(MAX(id), 0)").Scan(&n)
	}
	n++
	in.nextIDs[table] = n
	return n
}

// csvStoreID picks the store the CSV rows belong to: the export does not say.
func (in *ingest) csvStoreID() int64 {
	if in.csvStore != 0 {
		return in.csvStore
	}
	var stores []db.Store
	in.g.Select("id").Order("id").Limit(2).Find(&stores)
	switch {
	case len(stores) > 0:
		in.csvStore = stores[0].ID
		if len(stores) > 1 {
			in.j.warn("orders.csv does not say which store it belongs to: attached to the first store (#%d)", in.csvStore)
		}
	case !in.cfg.dryRun:
		url := ""
		if in.svc.Cfg != nil {
			url = in.svc.Cfg.PublicURL
		}
		if s, err := db.EnsureDefaultStore(in.g, url); err == nil {
			in.csvStore = s.ID
			in.j.warn("no store exists yet: created a default store (#%d) for orders.csv", s.ID)
		}
	}
	return in.csvStore
}

func (in *ingest) loadNames() {
	if in.namesDone {
		return
	}
	in.namesDone = true
	in.names = map[string][2]int64{}
	var rows []struct {
		PID, VID     int64
		PName, VName string
	}
	in.g.Raw("SELECT p.id AS p_id, v.id AS v_id, p.name AS p_name, v.name AS v_name FROM variants v JOIN products p ON p.id = v.product_id ORDER BY v.id DESC").Scan(&rows)
	for _, r := range rows {
		in.names[nameKey(r.PName, r.VName)] = [2]int64{r.PID, r.VID}
	}
}

func nameKey(product, variant string) string {
	return strings.ToLower(strings.TrimSpace(product)) + "\x00" + strings.ToLower(strings.TrimSpace(variant))
}

// resolveCSV gives a row derived from orders.csv its id and links it to the
// rows it refers to. It returns false for rows that cannot be placed.
func (in *ingest) resolveCSV(res string, o *lsObject) bool {
	a := o.A
	store := in.csvStoreID()
	a["store_id"] = store
	test := in.cfg.testMode
	switch res {
	case "customers":
		email := a.trim("email")
		if id := in.customerByEmail(store, test, email); id != 0 {
			o.ID = id
		} else {
			o.ID = in.allocID("customers")
			in.custMail[custKey(store, test, email)] = o.ID
		}
	case "orders":
		ident := a.trim("identifier")
		var rows []db.Order
		in.g.Select("id").Where("identifier = ?", ident).Limit(1).Find(&rows)
		if len(rows) > 0 {
			o.ID = rows[0].ID
		} else {
			o.ID = in.allocID("orders")
		}
		in.orderIDs[ident] = o.ID
		if cid := in.customerByEmail(store, test, a.trim("user_email")); cid != 0 {
			a["customer_id"] = cid
		}
	case "order_items":
		ident := a.trim("_order_identifier")
		orderID, ok := in.orderIDs[ident]
		if !ok {
			var rows []db.Order
			in.g.Select("id").Where("identifier = ?", ident).Limit(1).Find(&rows)
			if len(rows) == 0 {
				return false
			}
			orderID = rows[0].ID
		}
		a["order_id"] = orderID
		var items []db.OrderItem
		in.g.Select("id").Where("order_id = ?", orderID).Order("id").Limit(1).Find(&items)
		if len(items) > 0 {
			o.ID = items[0].ID
		} else {
			o.ID = in.allocID("order_items")
		}
		in.loadNames()
		if m, ok := in.names[nameKey(a.str("product_name"), a.str("variant_name"))]; ok {
			a["product_id"], a["variant_id"] = m[0], m[1]
		} else {
			in.warnOnce("name-"+nameKey(a.str("product_name"), a.str("variant_name")),
				"orders.csv: no variant matches %q / %q; its order items are imported without a variant", a.str("product_name"), a.str("variant_name"))
		}
	}
	if o.ID > 0 {
		in.matched[matchKey(res, o.ID)] = true // placed by e-mail / identifier: the same row by definition
	}
	return o.ID > 0
}
