package importer

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"fulfillment/internal/db"
	"fulfillment/internal/service"
)

// The mapping CSV comes from Lemon Squeezy's migration tooling and its exact
// header names are not documented, so columns are recognised by header
// keywords and by the shape of their values (cus_…, prod_…, price_…, sub_…
// for Stripe; plain digits for Lemon Squeezy ids).

const maxMappingProblems = 50

var (
	digitsRe   = regexp.MustCompile(`^#?\d+$`)
	stripeIDRe = regexp.MustCompile(`^[a-z]+_[A-Za-z0-9]+$`)
)

type mappingCols struct {
	header  bool
	ls      int    // Lemon Squeezy id column
	lsKind  string // products: "variant" | "product" | "" (unknown, try both)
	stripe  int    // Stripe id column
	price   int    // products: Stripe price column
	email   int    // customers: e-mail column (fallback match)
	headers []string
}

var stripePrefix = map[string]string{"customers": "cus_", "products": "prod_", "subscriptions": "sub_"}

type colStat struct {
	n, digits, stripe, price, email int
	hint                            string
}

func (c colStat) ratio(k int) float64 {
	if c.n == 0 {
		return 0
	}
	return float64(k) / float64(c.n)
}

func hasAny(s string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// lsHint recognises "lemon", "squeezy" and the abbreviation "ls" in a header.
func lsHint(h string) bool {
	if hasAny(h, "lemon", "squeezy") {
		return true
	}
	for _, part := range strings.FieldsFunc(h, func(r rune) bool { return r == '_' }) {
		if part == "ls" || part == "lsq" {
			return true
		}
	}
	return false
}

// looksLikeData tells a value (digits, cus_Abc123 ...) from a header word
// (customer_id, price_id ...): Stripe ids carry digits or capitals.
func looksLikeData(c string) bool {
	if digitsRe.MatchString(c) {
		return true
	}
	for _, p := range []string{"cus_", "prod_", "price_", "sub_"} {
		if rest, ok := strings.CutPrefix(c, p); ok && len(rest) >= 3 && stripeIDRe.MatchString(c) && strings.IndexFunc(rest, func(r rune) bool { return r < 'a' || r > 'z' }) >= 0 {
			return true
		}
	}
	return false
}

// detectMappingColumns finds the id columns of a mapping CSV.
func detectMappingColumns(kind string, rows [][]string) (mappingCols, error) {
	cols := mappingCols{ls: -1, stripe: -1, price: -1, email: -1}
	if len(rows) == 0 {
		return cols, fmt.Errorf("the CSV file is empty")
	}
	first := rows[0]
	dataLike := false
	for _, c := range first {
		if looksLikeData(strings.TrimSpace(c)) {
			dataLike = true
		}
	}
	start := 0
	if !dataLike {
		cols.header = true
		start = 1
		for _, c := range first {
			cols.headers = append(cols.headers, normHeader(c))
		}
	}
	width := 0
	for _, r := range rows {
		width = max(width, len(r))
	}
	stats := make([]colStat, width)
	for i := range stats {
		if cols.header && i < len(cols.headers) {
			stats[i].hint = cols.headers[i]
		}
	}
	sample := rows[start:]
	if len(sample) > 500 {
		sample = sample[:500]
	}
	for _, r := range sample {
		for i := 0; i < width && i < len(r); i++ {
			v := strings.TrimSpace(r[i])
			if v == "" {
				continue
			}
			s := &stats[i]
			s.n++
			switch {
			case digitsRe.MatchString(v):
				s.digits++
			case strings.HasPrefix(v, stripePrefix[kind]):
				s.stripe++
			case strings.HasPrefix(v, "price_"):
				s.price++
			case strings.Contains(v, "@"):
				s.email++
			}
		}
	}

	pick := func(score func(i int, s colStat) float64, exclude ...int) int {
		best, bestScore := -1, 0.0
		for i, s := range stats {
			skip := false
			for _, e := range exclude {
				if i == e {
					skip = true
				}
			}
			if skip {
				continue
			}
			if sc := score(i, s); sc > bestScore {
				best, bestScore = i, sc
			}
		}
		return best
	}
	cols.stripe = pick(func(_ int, s colStat) float64 {
		if len(sample) > 0 && s.n > 0 && s.stripe == 0 {
			return 0 // the header says "stripe" but no value has the expected prefix
		}
		sc := s.ratio(s.stripe) * 10
		if strings.Contains(s.hint, "stripe") {
			sc += 3
			if sc < 4 { // a header hint alone is enough when there is no data
				sc = 4
			}
		}
		if sc < 3 {
			return 0
		}
		return sc
	})
	if cols.stripe < 0 {
		return cols, fmt.Errorf("no column with Stripe ids (%s…) was found", stripePrefix[kind])
	}
	if kind == "products" {
		cols.price = pick(func(_ int, s colStat) float64 {
			sc := s.ratio(s.price) * 10
			if sc < 5 {
				return 0
			}
			return sc
		}, cols.stripe)
	}
	cols.ls = pick(func(_ int, s colStat) float64 {
		sc := s.ratio(s.digits) * 10
		if sc < 5 && !lsHint(s.hint) {
			return 0
		}
		if lsHint(s.hint) {
			sc += 3
		}
		if hasAny(s.hint, "customer", "variant", "product", "subscription") {
			sc += 2
		}
		if strings.Contains(s.hint, "id") {
			sc++
		}
		if strings.Contains(s.hint, "variant") {
			sc += 0.5 // a variant id beats a product id (each variant became a Stripe product)
		}
		return sc + 0.01
	}, cols.stripe, cols.price)
	if kind == "customers" {
		cols.email = pick(func(_ int, s colStat) float64 {
			sc := s.ratio(s.email) * 10
			if sc < 5 && !strings.Contains(s.hint, "email") {
				return 0
			}
			return sc + 0.01
		}, cols.stripe, cols.ls)
	}
	if cols.ls < 0 && cols.email < 0 {
		return cols, fmt.Errorf("no column with Lemon Squeezy ids (digits) was found")
	}
	if kind == "products" && cols.ls >= 0 {
		h := stats[cols.ls].hint
		switch {
		case strings.Contains(h, "variant"):
			cols.lsKind = "variant"
		case strings.Contains(h, "product"):
			cols.lsKind = "product"
		}
	}
	return cols, nil
}

type mapOutcome int

const (
	mapMatched mapOutcome = iota
	mapUnchanged
	mapUnknown
	mapInvalid
	mapConflict
)

// mappingRun applies the rows of one mapping CSV.
type mappingRun struct {
	r      *Runner
	j      *jobCtl
	kind   string
	cols   mappingCols
	counts [5]int
	shown  int
	extra  int
}

func (m *mappingRun) problem(row int, format string, args ...any) {
	if m.shown < maxMappingProblems {
		m.shown++
		m.j.warn("row %d: %s", row, fmt.Sprintf(format, args...))
		return
	}
	m.extra++
	m.j.mu.Lock()
	m.j.warnings++
	m.j.mu.Unlock()
}

func (m *mappingRun) cell(rec []string, col int) string {
	if col < 0 || col >= len(rec) {
		return ""
	}
	return strings.TrimSpace(rec[col])
}

func parseLSID(s string) (int64, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil && n > 0
}

// StartMappingCSV applies an "LS id → Stripe id" CSV. kind: customers | products | subscriptions.
// The runner owns csvPath and removes it when done.
func (r *Runner) StartMappingCSV(createdBy, kind, csvPath string) (*db.ImportJob, error) {
	cleanup := removeFile(csvPath)
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		kind = "customers"
	}
	if _, ok := stripePrefix[kind]; !ok {
		if cleanup != nil {
			cleanup()
		}
		return nil, service.Invalid("/kind", "The mapping kind must be customers, products or subscriptions.")
	}
	if strings.TrimSpace(csvPath) == "" {
		return nil, service.Invalid("/file", "A CSV file is required.")
	}
	return r.start(kindMapping, createdBy, map[string]any{"kind": kind}, cleanup, func(ctx context.Context, j *jobCtl) error {
		return r.runMapping(ctx, j, kind, csvPath)
	})
}

func (r *Runner) runMapping(ctx context.Context, j *jobCtl, kind, csvPath string) error {
	f, err := os.Open(csvPath)
	if err != nil {
		return fmt.Errorf("cannot read the CSV file: %w", err)
	}
	rows, err := readCSV(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("cannot parse the CSV file: %w", err)
	}
	cols, err := detectMappingColumns(kind, rows)
	if err != nil {
		return err
	}
	m := &mappingRun{r: r, j: j, kind: kind, cols: cols}
	data := rows
	first := 1
	if cols.header {
		data = rows[1:]
		first = 2
	}
	j.setStage(kind)
	j.setTotal(len(data))
	j.info("mapping %s: %d row(s); Lemon Squeezy id column %s, Stripe id column %s%s", kind, len(data), m.colName(cols.ls), m.colName(cols.stripe), m.extraCols())

	const chunk = 200
	for start := 0; start < len(data); start += chunk {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := min(start+chunk, len(data))
		var outcomes []mapOutcome
		var msgs []string
		err := r.svc.DB.Transaction(func(tx *gorm.DB) error {
			outcomes, msgs = outcomes[:0], msgs[:0]
			for i := start; i < end; i++ {
				o, msg, err := m.apply(tx, data[i])
				if err != nil {
					return err
				}
				outcomes, msgs = append(outcomes, o), append(msgs, msg)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("rows %d-%d: %w", start+first, end+first-1, err)
		}
		for i, o := range outcomes {
			m.counts[o]++
			if o == mapUnknown || o == mapInvalid || o == mapConflict {
				m.problem(start+i+first, "%s", msgs[i])
			}
		}
		j.advance(end - start)
	}
	m.finish()
	return nil
}

func (m *mappingRun) colName(i int) string {
	if i < 0 {
		return "(none)"
	}
	if m.cols.header && i < len(m.cols.headers) {
		return fmt.Sprintf("%d %q", i+1, m.cols.headers[i])
	}
	return strconv.Itoa(i + 1)
}

func (m *mappingRun) extraCols() string {
	s := ""
	if m.cols.price >= 0 {
		s += ", Stripe price column " + m.colName(m.cols.price)
	}
	if m.cols.email >= 0 {
		s += ", e-mail column " + m.colName(m.cols.email)
	}
	return s
}

func (m *mappingRun) finish() {
	c := m.counts
	m.j.count("matched", c[mapMatched])
	m.j.count("unchanged", c[mapUnchanged])
	m.j.count("unknown_ls_id", c[mapUnknown])
	m.j.count("invalid_rows", c[mapInvalid])
	m.j.count("conflicts", c[mapConflict])
	if m.extra > 0 {
		m.j.info("%d more problem row(s) were not listed", m.extra)
	}
	m.j.info("mapping finished: %d matched, %d already linked, %d unknown Lemon Squeezy id, %d invalid row(s), %d conflict(s)",
		c[mapMatched], c[mapUnchanged], c[mapUnknown], c[mapInvalid], c[mapConflict])
}

// apply links one row. The returned message explains problems.
func (m *mappingRun) apply(tx *gorm.DB, rec []string) (mapOutcome, string, error) {
	stripeID := m.cell(rec, m.cols.stripe)
	if !strings.HasPrefix(stripeID, stripePrefix[m.kind]) || !stripeIDRe.MatchString(stripeID) {
		return mapInvalid, fmt.Sprintf("%q is not a Stripe id (expected %s…)", stripeID, stripePrefix[m.kind]), nil
	}
	lsRaw := m.cell(rec, m.cols.ls)
	lsID, hasID := parseLSID(lsRaw)
	email := m.cell(rec, m.cols.email)
	if !hasID && (m.kind != "customers" || email == "") {
		return mapInvalid, fmt.Sprintf("%q is not a Lemon Squeezy id", lsRaw), nil
	}
	switch m.kind {
	case "customers":
		return m.applyCustomer(tx, lsID, hasID, email, stripeID)
	case "products":
		return m.applyProduct(tx, lsID, stripeID, m.cell(rec, m.cols.price))
	default:
		return m.applySubscription(tx, lsID, stripeID)
	}
}

// merge combines the outcomes of several rows touched by one CSV row.
func merge(a, b mapOutcome) mapOutcome {
	rank := func(o mapOutcome) int {
		switch o {
		case mapConflict:
			return 4
		case mapMatched:
			return 3
		case mapUnchanged:
			return 2
		}
		return 1
	}
	if rank(b) > rank(a) {
		return b
	}
	return a
}

func (m *mappingRun) applyCustomer(tx *gorm.DB, lsID int64, hasID bool, email, stripeID string) (mapOutcome, string, error) {
	var cs []db.Customer
	if hasID {
		if err := tx.Where("id = ?", lsID).Find(&cs).Error; err != nil {
			return 0, "", err
		}
	}
	via := fmt.Sprintf("Lemon Squeezy customer %d", lsID)
	if len(cs) == 0 && email != "" {
		if err := tx.Where("LOWER(email) = ?", strings.ToLower(email)).Find(&cs).Error; err != nil {
			return 0, "", err
		}
		via = "customer " + email
	}
	if len(cs) == 0 {
		return mapUnknown, fmt.Sprintf("%s is not in the database (import customers first)", via), nil
	}
	out := mapUnchanged
	msg := ""
	for _, c := range cs {
		switch {
		case c.StripeCustomerID == stripeID:
		case c.StripeCustomerID != "":
			out, msg = merge(out, mapConflict), fmt.Sprintf("customer %d (%s) is already linked to %s, not %s", c.ID, c.Email, c.StripeCustomerID, stripeID)
		default:
			if err := tx.Model(&db.Customer{}).Where("id = ?", c.ID).Updates(map[string]any{"stripe_customer_id": stripeID, "updated_at": db.Now()}).Error; err != nil {
				return 0, "", err
			}
			if err := tx.Model(&db.Subscription{}).Where("customer_id = ? AND (stripe_customer_id IS NULL OR stripe_customer_id = '')", c.ID).UpdateColumn("stripe_customer_id", stripeID).Error; err != nil {
				return 0, "", err
			}
			out = merge(out, mapMatched)
		}
	}
	return out, msg, nil
}

// setIfFree writes a Stripe id into column of table row id unless a different
// id is already there.
func setIfFree(tx *gorm.DB, model any, id int64, column, current, value string) (mapOutcome, error) {
	switch current {
	case value:
		return mapUnchanged, nil
	case "":
		err := tx.Model(model).Where("id = ?", id).Updates(map[string]any{column: value, "updated_at": db.Now()}).Error
		return mapMatched, err
	}
	return mapConflict, nil
}

func (m *mappingRun) applyProduct(tx *gorm.DB, lsID int64, prodID, priceID string) (mapOutcome, string, error) {
	var variant db.Variant
	var product db.Product
	var vs []db.Variant
	if m.cols.lsKind != "product" {
		if err := tx.Where("id = ?", lsID).Limit(1).Find(&vs).Error; err != nil {
			return 0, "", err
		}
	}
	var viaProduct bool
	if len(vs) == 0 && m.cols.lsKind != "variant" {
		var ps []db.Product
		if err := tx.Where("id = ?", lsID).Limit(1).Find(&ps).Error; err != nil {
			return 0, "", err
		}
		if len(ps) == 0 {
			return mapUnknown, fmt.Sprintf("Lemon Squeezy %s id %d is not a known variant or product", orStr(m.cols.lsKind, "variant/product"), lsID), nil
		}
		product, viaProduct = ps[0], true
		if err := tx.Where("product_id = ?", product.ID).Order("id").Find(&vs).Error; err != nil {
			return 0, "", err
		}
	}
	if len(vs) == 0 {
		return mapUnknown, fmt.Sprintf("Lemon Squeezy variant %d is not in the database (import variants first)", lsID), nil
	}
	out, msg := mapUnchanged, ""
	if viaProduct {
		o, err := setIfFree(tx, &db.Product{}, product.ID, "stripe_product_id", product.StripeProductID, prodID)
		if err != nil {
			return 0, "", err
		}
		out = merge(out, o)
		if o == mapConflict {
			msg = fmt.Sprintf("product %d is already linked to %s, not %s", product.ID, product.StripeProductID, prodID)
		}
		if len(vs) > 1 {
			// several variants share one Stripe product id: only the product link is safe to set
			return out, msg, nil
		}
	}
	variant = vs[0]
	o, err := setIfFree(tx, &db.Variant{}, variant.ID, "stripe_product_id", variant.StripeProductID, prodID)
	if err != nil {
		return 0, "", err
	}
	out = merge(out, o)
	if o == mapConflict {
		msg = fmt.Sprintf("variant %d is already linked to %s, not %s", variant.ID, variant.StripeProductID, prodID)
	}
	if priceID != "" && strings.HasPrefix(priceID, "price_") {
		var prices []db.Price
		if err := tx.Where("variant_id = ?", variant.ID).Order("id").Limit(1).Find(&prices).Error; err != nil {
			return 0, "", err
		}
		if len(prices) == 0 {
			msg = orStr(msg, fmt.Sprintf("variant %d has no price to link %s to", variant.ID, priceID))
			out = merge(out, mapUnknown)
		} else {
			o, err := setIfFree(tx, &db.Price{}, prices[0].ID, "stripe_price_id", prices[0].StripePriceID, priceID)
			if err != nil {
				return 0, "", err
			}
			out = merge(out, o)
			if o == mapConflict {
				msg = fmt.Sprintf("price %d is already linked to %s, not %s", prices[0].ID, prices[0].StripePriceID, priceID)
			}
		}
	}
	return out, msg, nil
}

func (m *mappingRun) applySubscription(tx *gorm.DB, lsID int64, stripeID string) (mapOutcome, string, error) {
	var subs []db.Subscription
	if err := tx.Where("id = ?", lsID).Limit(1).Find(&subs).Error; err != nil {
		return 0, "", err
	}
	if len(subs) == 0 {
		return mapUnknown, fmt.Sprintf("Lemon Squeezy subscription %d is not in the database (import subscriptions first)", lsID), nil
	}
	s := subs[0]
	cur := ""
	if s.StripeSubscriptionID != nil {
		cur = *s.StripeSubscriptionID
	}
	switch cur {
	case stripeID:
		return mapUnchanged, "", nil
	case "":
	default:
		return mapConflict, fmt.Sprintf("subscription %d is already linked to %s, not %s", s.ID, cur, stripeID), nil
	}
	var clash int64
	if err := tx.Model(&db.Subscription{}).Where("stripe_subscription_id = ? AND id <> ?", stripeID, s.ID).Count(&clash).Error; err != nil {
		return 0, "", err
	}
	if clash > 0 {
		return mapConflict, fmt.Sprintf("%s is already linked to another subscription", stripeID), nil
	}
	err := tx.Model(&db.Subscription{}).Where("id = ?", s.ID).Updates(map[string]any{"stripe_subscription_id": stripeID, "updated_at": db.Now()}).Error
	return mapMatched, "", err
}
