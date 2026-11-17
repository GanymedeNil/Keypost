package importer

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"fulfillment/internal/config"
	"fulfillment/internal/crypt"
	"fulfillment/internal/db"
	"fulfillment/internal/mail"
	"fulfillment/internal/resource"
	"fulfillment/internal/service"
	"fulfillment/internal/settings"
	"fulfillment/internal/storage"
	"fulfillment/internal/urls"
)

// env is a complete in-memory service for importer tests.
type env struct {
	t     *testing.T
	g     *gorm.DB
	svc   *service.Svc
	r     *Runner
	store *storage.Local
	dir   string

	mu     sync.Mutex
	sleeps []time.Duration
}

type envOpt func(*config.Config)

func newEnv(t *testing.T, opts ...envOpt) *env {
	t.Helper()
	crypt.SetSecret("test-secret-0123456789")
	url := "sqlite::memory:"
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" { // run the suite against PostgreSQL
		url = u
		raw, err := db.Open(u, false)
		if err != nil {
			t.Fatal(err)
		}
		raw.Exec("DROP SCHEMA public CASCADE")
		raw.Exec("CREATE SCHEMA public")
		if sqlDB, err := raw.DB(); err == nil {
			sqlDB.Close()
		}
	}
	g, err := db.Open(url, false)
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := g.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := db.Migrate(g); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		PublicURL: "http://localhost:3000", DownloadURLTTL: time.Hour,
		Stripe: config.StripeConfig{APIVersion: "2025-09-30.clover"},
	}
	for _, o := range opts {
		o(cfg)
	}
	st := settings.New(g, cfg)
	dir := t.TempDir()
	store, err := storage.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	ub := urls.Builder{Base: cfg.PublicURL, DownloadTTL: time.Hour}
	svc := &service.Svc{
		Cfg: cfg, DB: g, Settings: st, URLs: ub, Res: &resource.Builder{DB: g, URLs: ub}, Events: service.NopEmitter{},
		Mail: mail.New(g, st), Storage: store, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	e := &env{t: t, g: g, svc: svc, store: store, dir: dir}
	e.r = NewRunner(svc)
	e.r.sleep = func(ctx context.Context, d time.Duration) error {
		e.mu.Lock()
		e.sleeps = append(e.sleeps, d)
		e.mu.Unlock()
		return ctx.Err()
	}
	t.Cleanup(service.WaitBackground)
	return e
}

func (e *env) recordedSleeps() []time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]time.Duration{}, e.sleeps...)
}

// wait blocks until the job has left pending/running and returns it.
func (e *env) wait(job *db.ImportJob) db.ImportJob {
	e.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var j db.ImportJob
		if err := e.g.First(&j, job.ID).Error; err != nil {
			e.t.Fatal(err)
		}
		if j.Status != "pending" && j.Status != "running" {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	e.t.Fatalf("import job %d did not finish", job.ID)
	return db.ImportJob{}
}

func (e *env) mustFinish(job *db.ImportJob, err error, want string) db.ImportJob {
	e.t.Helper()
	if err != nil {
		e.t.Fatalf("start: %v", err)
	}
	j := e.wait(job)
	if j.Status != want {
		e.t.Fatalf("job status = %s (error %q), want %s\nlogs:\n%s", j.Status, j.Error, want, e.logDump(j.ID))
	}
	return j
}

func (e *env) logs(jobID int64) []db.ImportLog {
	var l []db.ImportLog
	e.g.Where("job_id = ?", jobID).Order("id").Find(&l)
	return l
}

func (e *env) logDump(jobID int64) string {
	var b strings.Builder
	for _, l := range e.logs(jobID) {
		b.WriteString(l.Level + ": " + l.Message + "\n")
	}
	return b.String()
}

func (e *env) logContains(jobID int64, level, sub string) bool {
	for _, l := range e.logs(jobID) {
		if (level == "" || l.Level == level) && strings.Contains(l.Message, sub) {
			return true
		}
	}
	return false
}

func (e *env) count(model any) int64 {
	var n int64
	if err := e.g.Model(model).Count(&n).Error; err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) counts() map[string]int64 {
	return map[string]int64{
		"stores": e.count(&db.Store{}), "customers": e.count(&db.Customer{}), "products": e.count(&db.Product{}),
		"variants": e.count(&db.Variant{}), "prices": e.count(&db.Price{}), "files": e.count(&db.File{}),
		"orders": e.count(&db.Order{}), "order_items": e.count(&db.OrderItem{}), "subscriptions": e.count(&db.Subscription{}),
		"subscription_items": e.count(&db.SubscriptionItem{}), "subscription_invoices": e.count(&db.SubscriptionInvoice{}),
		"license_keys": e.count(&db.LicenseKey{}), "license_key_instances": e.count(&db.LicenseKeyInstance{}),
	}
}

func progressCounts(j db.ImportJob) map[string]int {
	out := map[string]int{}
	if c, ok := j.Progress.V["counts"].(map[string]any); ok {
		for k, v := range c {
			if f, ok := v.(float64); ok {
				out[k] = int(f)
			}
		}
	}
	return out
}

func progressWarnings(j db.ImportJob) int {
	f, _ := j.Progress.V["warnings"].(float64)
	return int(f)
}

// ---------------------------------------------------------------- JSON helpers

func jobj(typ string, id int64, a map[string]any) map[string]any {
	ids := strconv.FormatInt(id, 10)
	return map[string]any{
		"type": typ, "id": ids, "attributes": a,
		"relationships": map[string]any{},
		"links":         map[string]any{"self": "https://api.lemonsqueezy.com/v1/" + typ + "/" + ids},
	}
}

func normalize(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// assertSubset checks that every key of want is present in got with an equal
// value (recursively for objects). skip lists dotted key paths to ignore.
func assertSubset(t *testing.T, label string, want, got any, skip ...string) {
	t.Helper()
	skipSet := map[string]bool{}
	for _, s := range skip {
		skipSet[s] = true
	}
	var walk func(path string, w, g any)
	walk = func(path string, w, g any) {
		if skipSet[path] {
			return
		}
		wm, wok := w.(map[string]any)
		gm, gok := g.(map[string]any)
		if wok && gok {
			for k, wv := range wm {
				p := k
				if path != "" {
					p = path + "." + k
				}
				gv, present := gm[k]
				if !present {
					if !skipSet[p] {
						t.Errorf("%s: key %q missing in output", label, p)
					}
					continue
				}
				walk(p, wv, gv)
			}
			return
		}
		if !reflect.DeepEqual(w, g) {
			t.Errorf("%s: %s = %v, want %v", label, path, g, w)
		}
	}
	walk("", normalize(t, want), normalize(t, got))
}

func attrsOf(t *testing.T, r resource.Resource) map[string]any {
	t.Helper()
	return normalize(t, r.Attributes).(map[string]any)
}

func ptr[T any](v T) *T { return &v }

// ---------------------------------------------------------------- fake Lemon Squeezy

type fakeLS struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	token    string
	data     map[string][]map[string]any
	files    map[string][]byte // download path -> bytes
	requests []string          // request URIs of API calls
	fileHits map[string]int
	authSeen []string // Authorization headers seen on file downloads
	failures map[string][]int
	hook     func(w http.ResponseWriter, r *http.Request, res string, page int) bool
}

func newFakeLS(t *testing.T, token string) *fakeLS {
	f := &fakeLS{t: t, token: token, data: map[string][]map[string]any{}, files: map[string][]byte{}, fileHits: map[string]int{}, failures: map[string][]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeLS) URL() string { return f.srv.URL }

func (f *fakeLS) set(res string, objs ...map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[res] = objs
}

func (f *fakeLS) setFile(path string, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if body == nil {
		delete(f.files, path)
		return
	}
	f.files[path] = body
}

func (f *fakeLS) setFailures(res string, codes ...int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures[res] = codes
}

func (f *fakeLS) setHook(h func(w http.ResponseWriter, r *http.Request, res string, page int) bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hook = h
}

func (f *fakeLS) object(res, id string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, o := range f.data[res] {
		if o["id"] == id {
			return o
		}
	}
	return nil
}

func (f *fakeLS) apiRequests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.requests...)
}

func (f *fakeLS) downloads(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fileHits[path]
}

func attrOf(o map[string]any) map[string]any {
	m, _ := o["attributes"].(map[string]any)
	return m
}

func (f *fakeLS) serve(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/download/") {
		f.mu.Lock()
		f.fileHits[r.URL.Path]++
		f.authSeen = append(f.authSeen, r.Header.Get("Authorization"))
		body, ok := f.files[r.URL.Path]
		f.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
		return
	}
	res := strings.TrimPrefix(r.URL.Path, "/v1/")
	f.mu.Lock()
	f.requests = append(f.requests, r.URL.RequestURI())
	if q := f.failures[res]; len(q) > 0 {
		code := q[0]
		f.failures[res] = q[1:]
		f.mu.Unlock()
		if code == 429 {
			w.Header().Set("Retry-After", "1")
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(code)
		w.Write([]byte(`{"errors":[{"detail":"try later","status":"` + strconv.Itoa(code) + `","title":"Error"}]}`))
		return
	}
	f.mu.Unlock()

	if r.Header.Get("Authorization") != "Bearer "+f.token {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"jsonapi":{"version":"1.0"},"errors":[{"detail":"Unauthenticated.","status":"401","title":"Unauthorized"}]}`))
		return
	}
	if r.Header.Get("Accept") != "application/vnd.api+json" || r.Header.Get("Content-Type") != "application/vnd.api+json" {
		f.t.Errorf("missing JSON:API headers on %s", r.URL)
	}
	q := r.URL.Query()
	size, _ := strconv.Atoi(q.Get("page[size]"))
	if size <= 0 {
		size = 10
	}
	page, _ := strconv.Atoi(q.Get("page[number]"))
	if page <= 0 {
		page = 1
	}
	f.mu.Lock()
	hook := f.hook
	all, known := f.data[res]
	f.mu.Unlock()
	if hook != nil && hook(w, r, res, page) {
		return
	}
	if !known {
		http.NotFound(w, r)
		return
	}
	var rows []map[string]any
	for _, o := range all {
		if sid := q.Get("filter[store_id]"); sid != "" {
			if v, ok := attrOf(o)["store_id"]; ok && normalizeID(v) != sid {
				continue
			}
		}
		rows = append(rows, o)
	}
	// newest first, like the real API
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	last := (len(rows) + size - 1) / size
	if last == 0 {
		last = 1
	}
	from := min((page-1)*size, len(rows))
	to := min(from+size, len(rows))
	link := func(n int) string {
		v := url2(q, n)
		return "http://" + r.Host + r.URL.Path + "?" + v
	}
	links := map[string]any{"first": link(1), "last": link(last)}
	if page < last {
		links["next"] = link(page + 1)
	}
	if page > 1 {
		links["prev"] = link(page - 1)
	}
	doc := map[string]any{
		"meta":    map[string]any{"page": map[string]any{"currentPage": page, "from": from + 1, "lastPage": last, "perPage": size, "to": to, "total": len(rows)}},
		"jsonapi": map[string]any{"version": "1.0"}, "links": links, "data": rows[from:to],
	}
	w.Header().Set("Content-Type", "application/vnd.api+json")
	json.NewEncoder(w).Encode(doc)
}

func normalizeID(v any) string {
	switch x := v.(type) {
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case string:
		return x
	}
	return ""
}

func url2(q map[string][]string, page int) string {
	parts := []string{"page%5Bnumber%5D=" + strconv.Itoa(page)}
	if s := q["page[size]"]; len(s) > 0 {
		parts = append(parts, "page%5Bsize%5D="+s[0])
	}
	if s := q["filter[store_id]"]; len(s) > 0 {
		parts = append(parts, "filter%5Bstore_id%5D="+s[0])
	}
	return strings.Join(parts, "&")
}

// storedObjects counts the files below the storage root.
func (e *env) storedObjects() int {
	n := 0
	filepath.WalkDir(e.dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}
