package importer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"fulfillment/internal/db"
)

const (
	flushEvery   = time.Second
	flushRows    = 25
	maxWarnLogs  = 500
	maxErrorLogs = 200
)

// jobCtl tracks the progress of one running import and persists it (and the
// log lines) to the database.
type jobCtl struct {
	r    *Runner
	g    *gorm.DB
	ctx  context.Context
	id   int64
	kind string

	mu         sync.Mutex
	stage      string
	done       int
	total      int
	counts     map[string]int
	warnings   int
	errs       int
	warnLogs   int
	errLogs    int
	extra      map[string]any
	lastFlush  time.Time
	sinceFlush int
}

func newJobCtl(r *Runner, ctx context.Context, id int64, kind string) *jobCtl {
	return &jobCtl{r: r, g: r.svc.DB, ctx: ctx, id: id, kind: kind, counts: map[string]int{}, extra: map[string]any{}, lastFlush: time.Now()}
}

// snapshot builds the progress document under the lock.
func (j *jobCtl) snapshot() map[string]any {
	counts := make(map[string]any, len(j.counts))
	for k, v := range j.counts {
		counts[k] = v
	}
	p := map[string]any{"stage": j.stage, "done": j.done, "total": j.total, "counts": counts, "warnings": j.warnings}
	if j.errs > 0 {
		p["errors"] = j.errs
	}
	for k, v := range j.extra {
		p[k] = v
	}
	return p
}

func (j *jobCtl) flush() {
	j.mu.Lock()
	p := j.snapshot()
	j.lastFlush, j.sinceFlush = time.Now(), 0
	j.mu.Unlock()
	err := j.g.Model(&db.ImportJob{}).Where("id = ?", j.id).Updates(map[string]any{"progress": db.J(p), "updated_at": db.Now()}).Error
	if err != nil {
		j.r.logf("import job %d: save progress: %v", j.id, err)
	}
}

func (j *jobCtl) maybeFlush() {
	j.mu.Lock()
	due := j.sinceFlush >= flushRows || time.Since(j.lastFlush) >= flushEvery
	j.mu.Unlock()
	if due {
		j.flush()
	}
}

// setStage starts a new stage; the previous one is persisted first.
func (j *jobCtl) setStage(name string) {
	j.flush()
	j.mu.Lock()
	j.stage, j.done, j.total = name, 0, 0
	j.mu.Unlock()
	j.flush()
}

func (j *jobCtl) setTotal(n int) {
	j.mu.Lock()
	j.total = n
	j.mu.Unlock()
	j.maybeFlush()
}

// advance records n processed rows of the current stage.
func (j *jobCtl) advance(n int) {
	j.mu.Lock()
	j.done += n
	j.sinceFlush += n
	j.mu.Unlock()
	j.maybeFlush()
}

// count adds to a named counter ("orders", "invalid", "failed", ...).
func (j *jobCtl) count(key string, n int) {
	j.mu.Lock()
	j.counts[key] += n
	j.mu.Unlock()
}

func (j *jobCtl) set(key string, v any) {
	j.mu.Lock()
	j.extra[key] = v
	j.mu.Unlock()
}

func (j *jobCtl) writeLog(level, msg string) {
	if err := j.g.Create(&db.ImportLog{JobID: j.id, Level: level, Message: msg, CreatedAt: db.Now()}).Error; err != nil {
		j.r.logf("import job %d: write log: %v", j.id, err)
	}
}

func (j *jobCtl) info(format string, args ...any) {
	j.writeLog("info", fmt.Sprintf(format, args...))
	j.maybeFlush()
}

// warn records a warning; the individual log rows are capped so a broken
// export cannot flood the table, but the counter keeps counting.
func (j *jobCtl) warn(format string, args ...any) {
	j.mu.Lock()
	j.warnings++
	j.warnLogs++
	n := j.warnLogs
	j.mu.Unlock()
	switch {
	case n <= maxWarnLogs:
		j.writeLog("warn", fmt.Sprintf(format, args...))
	case n == maxWarnLogs+1:
		j.writeLog("info", fmt.Sprintf("more than %d warnings: further warnings are only counted", maxWarnLogs))
	}
	j.maybeFlush()
}

func (j *jobCtl) errorf(format string, args ...any) {
	j.mu.Lock()
	j.errs++
	j.errLogs++
	n := j.errLogs
	j.mu.Unlock()
	switch {
	case n <= maxErrorLogs:
		j.writeLog("error", fmt.Sprintf(format, args...))
	case n == maxErrorLogs+1:
		j.writeLog("info", fmt.Sprintf("more than %d errors: further errors are only counted", maxErrorLogs))
	}
	j.maybeFlush()
}
