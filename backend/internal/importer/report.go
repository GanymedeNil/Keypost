package importer

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"gorm.io/gorm"

	"fulfillment/internal/db"
)

const reportBatch = 500

// eachBatch walks the rows matching where in id order, reportBatch at a time.
func eachBatch[T any](g *gorm.DB, where string, id func(T) int64, fn func([]T) error) error {
	var last int64
	for {
		var rows []T
		q := g.Where("id > ?", last)
		if where != "" {
			q = q.Where("(" + where + ")")
		}
		if err := q.Order("id").Limit(reportBatch).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		if err := fn(rows); err != nil {
			return err
		}
		last = id(rows[len(rows)-1])
	}
}

const (
	noStripeCustomer = "stripe_customer_id IS NULL OR stripe_customer_id = ''"
	noStripeProduct  = "stripe_product_id IS NULL OR stripe_product_id = ''"
	noStripePrice    = "stripe_price_id IS NULL OR stripe_price_id = ''"
	noStripeSub      = "status <> 'expired' AND (stripe_subscription_id IS NULL OR stripe_subscription_id = '')"
)

func detail(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "; ")
}

func flag(on bool, label string) string {
	if on {
		return label
	}
	return ""
}

// productsByID loads product rows for a set of ids.
func productsByID(g *gorm.DB, ids []int64) map[int64]db.Product {
	out := map[int64]db.Product{}
	if len(ids) == 0 {
		return out
	}
	var ps []db.Product
	if g.Where("id IN ?", ids).Find(&ps).Error == nil {
		for _, p := range ps {
			out[p.ID] = p
		}
	}
	return out
}

func variantsByID(g *gorm.DB, ids []int64) map[int64]db.Variant {
	out := map[int64]db.Variant{}
	if len(ids) == 0 {
		return out
	}
	var vs []db.Variant
	if g.Where("id IN ?", ids).Find(&vs).Error == nil {
		for _, v := range vs {
			out[v.ID] = v
		}
	}
	return out
}

// MappingReport writes a CSV of customers/variants/prices/subscriptions that have no Stripe id yet.
func (r *Runner) MappingReport(w io.Writer) error {
	g := r.svc.DB
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"type", "id", "name_or_email", "store_id", "detail"}); err != nil {
		return err
	}
	row := func(typ string, id int64, name string, store int64, d string) error {
		return cw.Write([]string{typ, fmt.Sprint(id), name, fmt.Sprint(store), d})
	}

	err := eachBatch(g, noStripeCustomer, func(c db.Customer) int64 { return c.ID }, func(cs []db.Customer) error {
		for _, c := range cs {
			if err := row("customer", c.ID, c.Email, c.StoreID, detail(flag(c.Name != "", "name="+c.Name), flag(c.TestMode, "test_mode"))); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	err = eachBatch(g, noStripeProduct, func(v db.Variant) int64 { return v.ID }, func(vs []db.Variant) error {
		pids := make([]int64, len(vs))
		for i, v := range vs {
			pids[i] = v.ProductID
		}
		products := productsByID(g, pids)
		for _, v := range vs {
			p := products[v.ProductID]
			name := v.Name
			if p.Name != "" {
				name = p.Name + " - " + v.Name
			}
			d := detail(fmt.Sprintf("product_id=%d", v.ProductID), "status="+v.Status, flag(v.TestMode, "test_mode"))
			if err := row("variant", v.ID, name, p.StoreID, d); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	err = eachBatch(g, noStripePrice, func(p db.Price) int64 { return p.ID }, func(ps []db.Price) error {
		vids := make([]int64, len(ps))
		for i, p := range ps {
			vids[i] = p.VariantID
		}
		variants := variantsByID(g, vids)
		pids := make([]int64, 0, len(variants))
		for _, v := range variants {
			pids = append(pids, v.ProductID)
		}
		products := productsByID(g, pids)
		for _, p := range ps {
			v := variants[p.VariantID]
			prod := products[v.ProductID]
			name := v.Name
			if prod.Name != "" {
				name = prod.Name + " - " + v.Name
			}
			interval := ""
			if p.RenewalIntervalUnit != nil {
				q := 1
				if p.RenewalIntervalQuantity != nil {
					q = *p.RenewalIntervalQuantity
				}
				interval = fmt.Sprintf("every %d %s", q, *p.RenewalIntervalUnit)
			}
			d := detail(fmt.Sprintf("variant_id=%d", p.VariantID), p.Category, fmt.Sprintf("%d %s", p.UnitPrice, p.Currency), interval)
			if err := row("price", p.ID, name, prod.StoreID, d); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	err = eachBatch(g, noStripeSub, func(s db.Subscription) int64 { return s.ID }, func(ss []db.Subscription) error {
		for _, s := range ss {
			item := s.ProductName
			if s.VariantName != "" {
				item = detail(s.ProductName, s.VariantName)
				item = strings.Replace(item, "; ", " - ", 1)
			}
			d := detail("status="+s.Status, item, flag(s.PaymentProcessor == "paypal", "paypal (cannot migrate to Stripe)"), flag(s.TestMode, "test_mode"))
			if err := row("subscription", s.ID, s.UserEmail, s.StoreID, d); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}
