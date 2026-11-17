package importer

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"fulfillment/internal/db"
	"fulfillment/internal/service"
)

const maxFailedRows = 1000

// ingestCfg is the part of the import options shared by every source.
type ingestCfg struct {
	testMode      bool
	dryRun        bool
	downloadFiles bool
	storeIDs      []int64
	resources     map[string]bool
}

// stageHandler maps and stores the objects of one resource.
type stageHandler struct {
	add   func(ctx context.Context, o lsObject) error
	flush func(ctx context.Context) error
}

// idCache memoises id lookups; misses fall back to the database so that
// importing a subset of the resources still resolves references to rows that
// were imported earlier.
type idCache[V any] struct {
	m    map[int64]V
	miss map[int64]bool
	load func(id int64) (V, bool)
}

func newIDCache[V any](load func(id int64) (V, bool)) *idCache[V] {
	return &idCache[V]{m: map[int64]V{}, miss: map[int64]bool{}, load: load}
}

func (c *idCache[V]) put(id int64, v V) { c.m[id] = v }

func (c *idCache[V]) get(id int64) (V, bool) {
	if v, ok := c.m[id]; ok {
		return v, true
	}
	var zero V
	if c.miss[id] || c.load == nil {
		return zero, false
	}
	v, ok := c.load(id)
	if !ok {
		c.miss[id] = true
		return zero, false
	}
	c.m[id] = v
	return v, true
}

type itemRef struct{ product, variant int64 }

// ingest turns the objects of a source into rows. It is the shared core of
// the API importer and the archive importer.
type ingest struct {
	r   *Runner
	svc *service.Svc
	g   *gorm.DB
	j   *jobCtl
	cfg ingestCfg
	src source

	handlers map[string]*stageHandler
	failed   int

	storeCur  *idCache[string]
	prodStore *idCache[int64]
	varProd   *idCache[int64]
	itemRefs  *idCache[itemRef]
	orderTest *idCache[bool]
	subByItem map[int64]int64

	custRemap map[int64]int64
	custIDs   map[int64]bool
	custExist *idCache[bool]
	custMail  map[string]int64 // store|test|email -> id (csv resolution, duplicates)

	slugs     map[string]int64
	slugsDone bool
	scope     scopeSets
	nextIDs   map[string]int64
	orderIDs  map[string]int64 // order identifier -> id (orders.csv)
	csvStore  int64
	names     map[string][2]int64 // lower(product)\x00lower(variant) -> product, variant
	namesDone bool
	paypal    int
	warned    map[string]bool
}

func newIngest(r *Runner, j *jobCtl, cfg ingestCfg, src source) *ingest {
	in := &ingest{
		// row failures are reported through the job log; keep gorm from
		// printing every failed statement (with all its values) as well
		r: r, svc: r.svc, g: r.svc.DB.Session(&gorm.Session{Logger: r.svc.DB.Logger.LogMode(logger.Silent)}), j: j, cfg: cfg, src: src,
		subByItem: map[int64]int64{}, custRemap: map[int64]int64{}, custIDs: map[int64]bool{}, custMail: map[string]int64{},
		nextIDs: map[string]int64{}, orderIDs: map[string]int64{}, warned: map[string]bool{},
	}
	in.initCaches()
	in.initScope()
	in.initHandlers()
	return in
}

func (in *ingest) initCaches() {
	g := in.g
	in.storeCur = newIDCache(func(id int64) (string, bool) {
		var rows []db.Store
		if g.Select("id", "currency").Where("id = ?", id).Limit(1).Find(&rows).Error != nil || len(rows) == 0 {
			return "", false
		}
		return rows[0].Currency, true
	})
	in.prodStore = newIDCache(func(id int64) (int64, bool) {
		var rows []db.Product
		if g.Select("id", "store_id").Where("id = ?", id).Limit(1).Find(&rows).Error != nil || len(rows) == 0 {
			return 0, false
		}
		return rows[0].StoreID, true
	})
	in.varProd = newIDCache(func(id int64) (int64, bool) {
		var rows []db.Variant
		if g.Select("id", "product_id").Where("id = ?", id).Limit(1).Find(&rows).Error != nil || len(rows) == 0 {
			return 0, false
		}
		return rows[0].ProductID, true
	})
	in.itemRefs = newIDCache(func(id int64) (itemRef, bool) {
		var rows []db.OrderItem
		if g.Select("id", "product_id", "variant_id").Where("id = ?", id).Limit(1).Find(&rows).Error != nil || len(rows) == 0 {
			return itemRef{}, false
		}
		return itemRef{product: rows[0].ProductID, variant: rows[0].VariantID}, true
	})
	in.orderTest = newIDCache(func(id int64) (bool, bool) {
		var rows []db.Order
		if g.Select("id", "test_mode").Where("id = ?", id).Limit(1).Find(&rows).Error != nil || len(rows) == 0 {
			return false, false
		}
		return rows[0].TestMode, true
	})
	in.custExist = newIDCache(func(id int64) (bool, bool) {
		var n int64
		if g.Model(&db.Customer{}).Where("id = ?", id).Count(&n).Error != nil || n == 0 {
			return false, false
		}
		return true, true
	})
}

// ---------------------------------------------------------------- run

// run executes the selected stages in dependency order.
func (in *ingest) run(ctx context.Context) (err error) {
	defer func() { in.finish(err) }()
	in.preflight()
	var absent []string
	for _, res := range stageOrder {
		if !in.cfg.resources[res] {
			continue
		}
		if !in.src.has(res) {
			absent = append(absent, res)
			continue
		}
		if err = in.runStage(ctx, res); err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
	}
	if len(absent) > 0 {
		in.j.info("not present in the source, nothing to import: %s", strings.Join(absent, ", "))
	}
	return nil
}

// preflight warns about data that Lemon Squeezy ids could collide with.
func (in *ingest) preflight() {
	var n int64
	if in.g.Model(&db.Order{}).Where("source <> ?", "lemonsqueezy").Count(&n).Error == nil && n > 0 {
		in.j.warn("this database already holds %d order(s) created by Stripe checkouts or by hand: Lemon Squeezy ids are kept, so imported rows can collide with local rows that use the same ids (orders are protected, other tables are not). A clean migration imports into an empty database", n)
	}
}

func (in *ingest) runStage(ctx context.Context, res string) error {
	in.j.setStage(res)
	h := &streamHandler{
		SetTotal: in.j.setTotal,
		Object:   func(o lsObject) error { return in.add(ctx, res, o) },
		Invalid: func(err error) {
			in.j.count("invalid", 1)
			in.j.warn("%s: skipped an unreadable object: %v", res, err)
		},
	}
	serr := in.src.stream(ctx, res, h)
	// Rows that were already read are written even when the stream failed
	// or the job was cancelled, so a re-run only has to continue.
	ferr := in.handlers[res].flush(ctx)
	if serr == nil && ferr == nil {
		in.afterStage(res)
		return nil
	}
	var le *lsError
	if errors.As(serr, &le) && le.Status == 404 {
		in.j.warn("%s: endpoint not available (HTTP 404), skipped", res)
		return ferr
	}
	if serr != nil {
		return fmt.Errorf("%s: %w", res, serr)
	}
	return fmt.Errorf("%s: %w", res, ferr)
}

func (in *ingest) afterStage(res string) {
	if res == "subscriptions" && in.paypal > 0 {
		in.j.warn("%d PayPal subscription(s) were imported for history only: PayPal billing cannot be migrated to Stripe", in.paypal)
	}
}

func (in *ingest) finish(runErr error) {
	if !in.cfg.dryRun {
		if err := db.ResetSequences(in.g); err != nil {
			in.j.warn("could not reset id sequences: %v", err)
		}
	}
	if runErr == nil {
		mode := ""
		if in.cfg.dryRun {
			mode = " (dry run: nothing was written)"
		}
		in.j.info("import finished%s: %s", mode, in.countSummary())
	}
}

func (in *ingest) countSummary() string {
	in.j.mu.Lock()
	defer in.j.mu.Unlock()
	keys := make([]string, 0, len(in.j.counts))
	for k := range in.j.counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, in.j.counts[k]))
	}
	if len(parts) == 0 {
		return "no rows"
	}
	return strings.Join(parts, ", ")
}

// add routes one object to the handler of its resource.
func (in *ingest) add(ctx context.Context, res string, o lsObject) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if o.csv {
		if !in.resolveCSV(res, &o) {
			in.invalid(res, "row of orders.csv could not be placed")
			return nil
		}
	} else if o.ID <= 0 {
		in.invalid(res, "object without a valid id")
		return nil
	}
	if !in.inScope(res, o) {
		return nil
	}
	return in.handlers[res].add(ctx, o)
}

func (in *ingest) invalid(res, why string) {
	in.j.count("invalid", 1)
	in.j.warn("%s: skipped an object: %s", res, why)
}

// warnOnce logs a warning once per distinct key (repeated problems with the
// same reference would otherwise flood the log).
func (in *ingest) warnOnce(key, format string, args ...any) {
	if in.warned[key] {
		return
	}
	in.warned[key] = true
	in.j.warn(format, args...)
}

// ---------------------------------------------------------------- writing

// writeBatch stores rows in one transaction. When that fails the rows are
// retried one by one so a single bad row does not lose its neighbours.
func writeBatch[T any](in *ingest, res string, rows []T, id func(T) int64, write func(tx *gorm.DB, rows []T) error) error {
	if len(rows) == 0 || in.cfg.dryRun {
		return nil
	}
	err := in.g.Transaction(func(tx *gorm.DB) error { return write(tx, rows) })
	if err == nil {
		return nil
	}
	if len(rows) == 1 {
		return in.rowFailed(res, id(rows[0]), err)
	}
	for _, row := range rows {
		one := []T{row}
		if err := in.g.Transaction(func(tx *gorm.DB) error { return write(tx, one) }); err != nil {
			if ferr := in.rowFailed(res, id(row), err); ferr != nil {
				return ferr
			}
		}
	}
	return nil
}

func (in *ingest) rowFailed(res string, id int64, err error) error {
	in.failed++
	in.j.count("failed", 1)
	in.j.errorf("%s #%d could not be stored: %v", res, id, err)
	if in.failed > maxFailedRows {
		return fmt.Errorf("too many rows could not be stored (%d); last error: %w", in.failed, err)
	}
	return nil
}

// newBatch builds a handler that maps objects, buffers them and writes them
// in batches. vet (optional) may drop or reject rows just before a write.
func newBatch[T any](in *ingest, res string, mapFn func(lsObject) (T, bool), id func(T) int64, vet func([]T) ([]T, error), write func(tx *gorm.DB, rows []T) error) *stageHandler {
	var rows []T
	h := &stageHandler{}
	h.flush = func(context.Context) error {
		batch := rows
		rows = nil
		if vet != nil && len(batch) > 0 {
			var err error
			if batch, err = vet(batch); err != nil {
				return err
			}
		}
		return writeBatch(in, res, batch, id, write)
	}
	h.add = func(ctx context.Context, o lsObject) error {
		row, ok := mapFn(o)
		if !ok {
			return nil
		}
		in.j.count(res, 1)
		in.j.advance(1)
		rows = append(rows, row)
		if len(rows) >= in.r.batch() {
			return h.flush(ctx)
		}
		return nil
	}
	return h
}

// testMode resolves the test_mode attribute with the option as fallback.
func (in *ingest) testMode(a attrs) bool {
	if a.present("test_mode") {
		return a.boolean("test_mode")
	}
	return in.cfg.testMode
}
