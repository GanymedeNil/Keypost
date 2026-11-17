// Package service holds the business logic shared by the LS-compatible API,
// the admin API, the storefront and the Stripe event processor.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"fulfillment/internal/config"
	"fulfillment/internal/db"
	"fulfillment/internal/mail"
	"fulfillment/internal/resource"
	"fulfillment/internal/settings"
	"fulfillment/internal/storage"
	"fulfillment/internal/urls"
)

// Event is a domain event that may fan out to merchant webhooks.
type Event struct {
	Name         string // order_created, license_key_created, …
	StoreID      int64
	TestMode     bool
	ResourceType string // orders | subscriptions | subscription-invoices | license-keys | customers
	ResourceID   int64
	CustomData   map[string]any
}

// Emitter delivers events to merchant webhook endpoints.
type Emitter interface {
	Emit(ctx context.Context, ev Event)
}

type NopEmitter struct{}

func (NopEmitter) Emit(context.Context, Event) {}

// Svc bundles every dependency of the business logic.
type Svc struct {
	Cfg      *config.Config
	DB       *gorm.DB
	Settings *settings.Service
	URLs     urls.Builder
	Res      *resource.Builder
	Events   Emitter
	Mail     *mail.Mailer
	Storage  storage.Storage
	Log      *slog.Logger
}

// Error is a typed application error that API layers translate into their
// own wire format.
type Error struct {
	Status  int
	Code    string
	Title   string
	Detail  string
	Pointer string // JSON pointer for validation errors
	Fields  map[string]string
}

func (e *Error) Error() string { return e.Detail }

func NotFound(what string) *Error {
	return &Error{Status: http.StatusNotFound, Code: "not_found", Title: "Not Found", Detail: what + " not found."}
}

func Invalid(pointer, detail string) *Error {
	f := map[string]string{}
	if pointer != "" {
		f[lastSegment(pointer)] = detail
	}
	return &Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Title: "Unprocessable Entity", Detail: detail, Pointer: pointer, Fields: f}
}

func Conflict(detail string) *Error {
	return &Error{Status: http.StatusConflict, Code: "conflict", Title: "Conflict", Detail: detail}
}

func Bad(detail string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: "bad_request", Title: "Bad Request", Detail: detail}
}

func Upstream(detail string) *Error {
	return &Error{Status: http.StatusBadGateway, Code: "stripe_error", Title: "Bad Gateway", Detail: detail}
}

func lastSegment(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

func AsError(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// lockForUpdate adds SELECT … FOR UPDATE on PostgreSQL (SQLite serialises
// writers through its immediate transactions).
func lockForUpdate(tx *gorm.DB) *gorm.DB {
	if db.IsPostgres(tx) {
		return tx.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return tx
}

// events buffers events during a transaction and flushes them after commit.
type eventBuf struct{ evs []Event }

func (b *eventBuf) add(e Event) { b.evs = append(b.evs, e) }

func (s *Svc) flush(ctx context.Context, b *eventBuf) {
	for _, e := range b.evs {
		s.Events.Emit(ctx, e)
	}
	b.evs = nil
}

func (s *Svc) logf(format string, args ...any) {
	if s.Log != nil {
		s.Log.Info(fmt.Sprintf(format, args...))
	}
}
