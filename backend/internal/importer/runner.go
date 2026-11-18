// Package importer migrates data from Lemon Squeezy into this service and
// links it to Stripe objects.
//
// Every entry point creates an ImportJob that runs in the background, records
// progress and log lines in the database and can be cancelled. Lemon Squeezy
// data (API or offline archive) flows through one pipeline: a source streams
// resource objects, the mappers turn them into models and batched upserts
// write them, keeping the original ids.
package importer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"fulfillment/internal/db"
	"fulfillment/internal/service"
)

const (
	kindLSAPI     = "lemonsqueezy_api"
	kindLSFiles   = "lemonsqueezy_files"
	kindMapping   = "stripe_mapping"
	kindAutoMatch = "stripe_auto_match"
)

type Runner struct {
	svc *service.Svc

	// Unexported knobs, replaced by tests.
	sleep      func(ctx context.Context, d time.Duration) error
	rateLimit  int           // Lemon Squeezy requests per rateWindow
	rateWindow time.Duration //
	batchSize  int
	httpc      *http.Client // Lemon Squeezy API and file downloads
	matchDelay time.Duration
}

func NewRunner(s *service.Svc) *Runner {
	return &Runner{svc: s, sleep: sleepCtx, rateLimit: 300, rateWindow: time.Minute, batchSize: 100, matchDelay: 100 * time.Millisecond}
}

// LSOptions configures an import straight from the Lemon Squeezy API.
type LSOptions struct {
	APIKey        string
	BaseURL       string // default https://api.lemonsqueezy.com (tests override)
	TestMode      bool
	StoreIDs      []int64  // empty = all stores visible to the key
	Resources     []string // empty = all
	DownloadFiles bool
	DryRun        bool
	OnConflict    string // what to do when an id belongs to a different local row: "abort" (default) | "overwrite"
}

// ArchiveOptions configures an offline import from a ZIP archive.
type ArchiveOptions struct {
	TestMode      bool
	DownloadFiles bool
	DryRun        bool
	OnConflict    string // as LSOptions.OnConflict
}

// MatchOptions selects what the Stripe auto-matcher links.
type MatchOptions struct{ Customers, Products, Subscriptions bool }

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (r *Runner) logf(format string, args ...any) {
	if r.svc.Log != nil {
		r.svc.Log.Warn(fmt.Sprintf(format, args...))
	}
}

func (r *Runner) batch() int {
	if r.batchSize > 0 {
		return r.batchSize
	}
	return 100
}

func (r *Runner) client() *http.Client {
	if r.httpc != nil {
		return r.httpc
	}
	return defaultHTTP
}

var defaultHTTP = func() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 60 * time.Second
	return &http.Client{Transport: t}
}()

// ---------------------------------------------------------------- registry

// activeJob is the in-process handle of a running job. The registry is shared
// by every Runner of a database so any of them can cancel a job and the
// "one running import per kind" rule holds however many Runners exist.
type activeJob struct {
	cancel    context.CancelFunc
	cancelled atomic.Bool
}

type (
	jobKey struct {
		g  *gorm.DB
		id int64
	}
	kindKey struct {
		g    *gorm.DB
		kind string
	}
)

var registry = struct {
	sync.Mutex
	jobs  map[jobKey]*activeJob
	kinds map[kindKey]int64
}{jobs: map[jobKey]*activeJob{}, kinds: map[kindKey]int64{}}

func (r *Runner) release(kind string, id int64) {
	registry.Lock()
	delete(registry.jobs, jobKey{r.svc.DB, id})
	delete(registry.kinds, kindKey{r.svc.DB, kind})
	registry.Unlock()
}

// Cancel requests cancellation of a running job.
func (r *Runner) Cancel(jobID int64) bool {
	registry.Lock()
	a := registry.jobs[jobKey{r.svc.DB, jobID}]
	registry.Unlock()
	if a == nil {
		return false
	}
	a.cancelled.Store(true)
	a.cancel()
	return true
}

// runFunc is the body of a job.
type runFunc func(ctx context.Context, j *jobCtl) error

// start registers and launches a job. cleanup (may be nil) always runs once
// the job is over, or immediately when the job cannot be started.
func (r *Runner) start(kind, createdBy string, opts map[string]any, cleanup func(), run runFunc) (*db.ImportJob, error) {
	fail := func(err error) (*db.ImportJob, error) {
		if cleanup != nil {
			cleanup()
		}
		return nil, err
	}
	kk := kindKey{r.svc.DB, kind}
	registry.Lock()
	if _, busy := registry.kinds[kk]; busy {
		registry.Unlock()
		return fail(service.Conflict("An import of this kind is already running."))
	}
	registry.kinds[kk] = 0
	registry.Unlock()

	now := db.Now()
	job := db.ImportJob{
		Kind: kind, Status: "pending", Options: db.J(opts), CreatedBy: createdBy, CreatedAt: now, UpdatedAt: now,
		Progress: db.J(map[string]any{"stage": "", "done": 0, "total": 0, "counts": map[string]any{}, "warnings": 0}),
	}
	if err := r.svc.DB.Create(&job).Error; err != nil {
		registry.Lock()
		delete(registry.kinds, kk)
		registry.Unlock()
		return fail(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	a := &activeJob{cancel: cancel}
	registry.Lock()
	registry.kinds[kk] = job.ID
	registry.jobs[jobKey{r.svc.DB, job.ID}] = a
	registry.Unlock()

	id := job.ID
	r.svc.Go(func() { r.execute(ctx, a, id, kind, cleanup, run) })
	ret := job
	return &ret, nil
}

func (r *Runner) execute(ctx context.Context, a *activeJob, id int64, kind string, cleanup func(), run runFunc) {
	defer a.cancel()
	if cleanup != nil {
		defer cleanup()
	}
	g := r.svc.DB
	j := newJobCtl(r, ctx, id, kind)
	started := db.Now()
	g.Model(&db.ImportJob{}).Where("id = ?", id).Updates(map[string]any{"status": "running", "started_at": started, "updated_at": started})

	var err error
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				err = fmt.Errorf("panic: %v", rec)
				r.logf("import job %d panicked: %v\n%s", id, rec, debug.Stack())
			}
		}()
		err = run(ctx, j)
	}()

	status, msg := "completed", ""
	switch {
	case err != nil && (a.cancelled.Load() || errors.Is(err, context.Canceled)):
		status = "cancelled"
		j.info("cancelled by request")
	case err != nil:
		status, msg = "failed", err.Error()
		j.writeLog("error", msg)
	}
	j.flush()
	r.release(kind, id)
	finished := db.Now()
	g.Model(&db.ImportJob{}).Where("id = ?", id).Updates(map[string]any{
		"status": status, "error": msg, "finished_at": finished, "updated_at": finished,
	})
}

// RecoverInterrupted marks jobs left "running" by a previous process as failed.
func (r *Runner) RecoverInterrupted() {
	var jobs []db.ImportJob
	if err := r.svc.DB.Where("status IN ?", []string{"running", "pending"}).Find(&jobs).Error; err != nil {
		r.logf("recover interrupted imports: %v", err)
		return
	}
	for _, jb := range jobs {
		registry.Lock()
		_, live := registry.jobs[jobKey{r.svc.DB, jb.ID}]
		registry.Unlock()
		if live {
			continue
		}
		now := db.Now()
		r.svc.DB.Model(&db.ImportJob{}).Where("id = ?", jb.ID).Updates(map[string]any{
			"status": "failed", "error": "interrupted by restart", "finished_at": now, "updated_at": now,
		})
		r.svc.DB.Create(&db.ImportLog{JobID: jb.ID, Level: "error", Message: "interrupted by restart", CreatedAt: now})
	}
}

// ---------------------------------------------------------------- helpers

func redactKey(k string) string {
	k = strings.TrimSpace(k)
	if len(k) <= 4 {
		return "****"
	}
	return "****" + k[len(k)-4:]
}

func removeFile(path string) func() {
	if path == "" {
		return nil
	}
	return func() { _ = os.Remove(path) }
}
