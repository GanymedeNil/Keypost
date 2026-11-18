package importer

import (
	"fmt"
	"strings"
	"time"

	"fulfillment/internal/db"
)

// ID conflict guard.
//
// Lemon Squeezy ids are kept on import, so an incoming row can land on an id
// that a *different* local row already owns (an order from a Stripe checkout,
// a product created by hand, ...). Overwriting it would corrupt that row, and
// skipping it would silently re-parent the rows that refer to it, so by default
// the import stops. A row that exists under the same id is "the same row" (and
// is updated as before) when
//
//   - its created_at equals the one in the source (a re-run of an import, a
//     local edit never changes created_at), or
//   - it has the same natural key (e-mail, order identifier, license key, ...).
//
// Everything else is a conflict. The check runs for dry runs too, which makes
// a dry run the way to find conflicts before anything is written.

// Conflict policies for LSOptions.OnConflict / ArchiveOptions.OnConflict.
const (
	// ConflictAbort (default) stops at the first batch with a conflict; rows of
	// earlier batches are valid Lemon Squeezy data and stay.
	ConflictAbort = "abort"
	// ConflictOverwrite replaces the conflicting local rows (use only when they
	// are disposable, e.g. leftovers from testing).
	ConflictOverwrite = "overwrite"
)

// normalizeConflict validates the policy; empty means ConflictAbort.
func normalizeConflict(s string) (string, error) {
	switch p := strings.ToLower(strings.TrimSpace(s)); p {
	case "", ConflictAbort:
		return ConflictAbort, nil
	case ConflictOverwrite:
		return ConflictOverwrite, nil
	default:
		return "", fmt.Errorf("unknown conflict policy %q (valid: %s, %s)", s, ConflictAbort, ConflictOverwrite)
	}
}

// conflictError ends an import because incoming ids belong to other rows.
type conflictError struct{ msg string }

func (e *conflictError) Error() string { return e.msg }

const (
	conflictHint = "Lemon Squeezy ids are kept, so rows that already use them were not overwritten. Import into an empty database, or, if those local rows are disposable, run the import again with id conflicts set to overwrite"
	// maxConflictLogs bounds the log lines written per batch.
	maxConflictLogs = 20
)

// rowGuard says how rows of one table are recognised and described.
type rowGuard[T any] struct {
	id      func(T) int64
	created func(T) time.Time
	// same (optional) reports that local and incoming are the same entity
	// although created_at differs: it compares the natural key.
	same func(local, incoming T) bool
	// label describes a row in log lines.
	label func(T) string
}

func (g rowGuard[T]) describe(r T) string {
	return fmt.Sprintf("%s, created %s", g.label(r), g.created(r).UTC().Format("2006-01-02"))
}

// guardOf builds the vet function of a batch handler for the table of T.
func guardOf[T any](in *ingest, res string, g rowGuard[T]) func([]T) ([]T, error) {
	return func(rows []T) ([]T, error) {
		if len(rows) == 0 {
			return rows, nil
		}
		ids := make([]int64, len(rows))
		for i, r := range rows {
			ids[i] = g.id(r)
		}
		var local []T
		if err := in.g.Where("id IN ?", ids).Find(&local).Error; err != nil || len(local) == 0 {
			return rows, nil // the write itself will report a broken database
		}
		byID := make(map[int64]T, len(local))
		for _, l := range local {
			byID[g.id(l)] = l
		}
		keep := make([]T, 0, len(rows))
		var clashIDs []int64
		var lines []string
		for _, r := range rows {
			l, taken := byID[g.id(r)]
			if !taken || in.matched[matchKey(res, g.id(r))] || g.created(l).Unix() == g.created(r).Unix() || (g.same != nil && g.same(l, r)) {
				keep = append(keep, r)
				continue
			}
			clashIDs = append(clashIDs, g.id(r))
			lines = append(lines, fmt.Sprintf("%s #%d: the id already belongs to a different local row (%s); the source has %s", res, g.id(r), g.describe(l), g.describe(r)))
		}
		overwrite, err := in.conflictVerdict(res, clashIDs, lines)
		switch {
		case err != nil:
			return nil, err
		case overwrite:
			return rows, nil
		default:
			return keep, nil
		}
	}
}

// conflictVerdict applies the conflict policy to the clashes found in one
// batch: overwrite=true lets the batch through unchanged, an error stops the
// import, and (dry run) neither means "carry on, the clashing rows are out".
func (in *ingest) conflictVerdict(res string, ids []int64, lines []string) (overwrite bool, err error) {
	n := len(ids)
	if n == 0 {
		return false, nil
	}
	in.conflicts += n
	in.j.count("conflicts", n)
	logLines := func(log func(string, ...any), suffix string) {
		for i, l := range lines {
			if i >= maxConflictLogs {
				log("%s: %d more conflict(s) in this batch are not listed", res, n-i)
				break
			}
			log("%s%s", l, suffix)
		}
	}
	switch {
	case in.cfg.onConflict == ConflictOverwrite:
		in.j.count("overwritten", n)
		logLines(in.j.warn, "; the local row was overwritten")
		return true, nil
	case in.cfg.dryRun:
		logLines(in.j.errorf, "")
		return false, nil
	default:
		logLines(in.j.errorf, "; nothing was overwritten")
		return false, &conflictError{fmt.Sprintf("%d Lemon Squeezy id(s) already belong to different rows in this database (%s). %s", n, idList(ids), conflictHint)}
	}
}

// finishConflicts turns the conflicts a dry run collected into its result: a
// real run would have stopped, so the dry run fails too.
func (in *ingest) finishConflicts() error {
	if !in.cfg.dryRun || in.cfg.onConflict == ConflictOverwrite || in.conflicts == 0 {
		return nil
	}
	return &conflictError{fmt.Sprintf("dry run found %d id conflict(s) (listed in the log); a real import would stop at the first one. %s", in.conflicts, conflictHint)}
}

func idList(ids []int64) string {
	const show = 10
	parts := make([]string, 0, show+1)
	for i, id := range ids {
		if i == show {
			parts = append(parts, fmt.Sprintf("and %d more", len(ids)-show))
			break
		}
		parts = append(parts, fmt.Sprintf("#%d", id))
	}
	return strings.Join(parts, ", ")
}

func matchKey(res string, id int64) string { return fmt.Sprintf("%s#%d", res, id) }

// ---------------------------------------------------------------- per-table rules

// The guards below say which columns identify a row of each table. Tables
// without a natural key rely on created_at alone.

func (in *ingest) storeGuard() func([]db.Store) ([]db.Store, error) {
	return guardOf(in, "stores", rowGuard[db.Store]{
		id: func(r db.Store) int64 { return r.ID }, created: func(r db.Store) time.Time { return r.CreatedAt },
		same:  func(l, r db.Store) bool { return l.Slug == r.Slug }, // the default store of a fresh install is replaced by "its" Lemon Squeezy store
		label: func(r db.Store) string { return fmt.Sprintf("store %q", r.Slug) },
	})
}

func (in *ingest) customerGuard() func([]db.Customer) ([]db.Customer, error) {
	return guardOf(in, "customers", rowGuard[db.Customer]{
		id: func(r db.Customer) int64 { return r.ID }, created: func(r db.Customer) time.Time { return r.CreatedAt },
		same: func(l, r db.Customer) bool {
			return custKey(l.StoreID, l.TestMode, l.Email) == custKey(r.StoreID, r.TestMode, r.Email)
		},
		label: func(r db.Customer) string { return r.Email },
	})
}

func (in *ingest) productGuard() func([]db.Product) ([]db.Product, error) {
	return guardOf(in, "products", rowGuard[db.Product]{
		id: func(r db.Product) int64 { return r.ID }, created: func(r db.Product) time.Time { return r.CreatedAt },
		label: func(r db.Product) string { return fmt.Sprintf("product %q", r.Name) },
	})
}

func (in *ingest) variantGuard() func([]db.Variant) ([]db.Variant, error) {
	return guardOf(in, "variants", rowGuard[db.Variant]{
		id: func(r db.Variant) int64 { return r.ID }, created: func(r db.Variant) time.Time { return r.CreatedAt },
		label: func(r db.Variant) string { return fmt.Sprintf("variant %q of product #%d", r.Name, r.ProductID) },
	})
}

func (in *ingest) priceGuard() func([]db.Price) ([]db.Price, error) {
	return guardOf(in, "prices", rowGuard[db.Price]{
		id: func(r db.Price) int64 { return r.ID }, created: func(r db.Price) time.Time { return r.CreatedAt },
		label: func(r db.Price) string { return fmt.Sprintf("price of variant #%d", r.VariantID) },
	})
}

func (in *ingest) fileGuard() func([]db.File) ([]db.File, error) {
	return guardOf(in, "files", rowGuard[db.File]{
		id: func(r db.File) int64 { return r.ID }, created: func(r db.File) time.Time { return r.CreatedAt },
		same:  func(l, r db.File) bool { return l.Identifier == r.Identifier },
		label: func(r db.File) string { return fmt.Sprintf("file %q", r.Name) },
	})
}

func (in *ingest) orderGuard() func([]db.Order) ([]db.Order, error) {
	return guardOf(in, "orders", rowGuard[db.Order]{
		id: func(r db.Order) int64 { return r.ID }, created: func(r db.Order) time.Time { return r.CreatedAt },
		same:  func(l, r db.Order) bool { return l.Identifier == r.Identifier },
		label: func(r db.Order) string { return fmt.Sprintf("order %d (%s)", r.OrderNumber, r.Source) },
	})
}

func (in *ingest) orderItemGuard() func([]db.OrderItem) ([]db.OrderItem, error) {
	return guardOf(in, "order_items", rowGuard[db.OrderItem]{
		id: func(r db.OrderItem) int64 { return r.ID }, created: func(r db.OrderItem) time.Time { return r.CreatedAt },
		label: func(r db.OrderItem) string { return fmt.Sprintf("item of order #%d", r.OrderID) },
	})
}

func (in *ingest) subscriptionGuard() func([]db.Subscription) ([]db.Subscription, error) {
	return guardOf(in, "subscriptions", rowGuard[db.Subscription]{
		id: func(r db.Subscription) int64 { return r.ID }, created: func(r db.Subscription) time.Time { return r.CreatedAt },
		label: func(r db.Subscription) string { return fmt.Sprintf("subscription of %s", r.UserEmail) },
	})
}

func (in *ingest) subscriptionItemGuard() func([]db.SubscriptionItem) ([]db.SubscriptionItem, error) {
	return guardOf(in, "subscription_items", rowGuard[db.SubscriptionItem]{
		id: func(r db.SubscriptionItem) int64 { return r.ID }, created: func(r db.SubscriptionItem) time.Time { return r.CreatedAt },
		label: func(r db.SubscriptionItem) string { return fmt.Sprintf("item of subscription #%d", r.SubscriptionID) },
	})
}

func (in *ingest) invoiceGuard() func([]db.SubscriptionInvoice) ([]db.SubscriptionInvoice, error) {
	return guardOf(in, "subscription_invoices", rowGuard[db.SubscriptionInvoice]{
		id: func(r db.SubscriptionInvoice) int64 { return r.ID }, created: func(r db.SubscriptionInvoice) time.Time { return r.CreatedAt },
		label: func(r db.SubscriptionInvoice) string {
			return fmt.Sprintf("invoice of subscription #%d", r.SubscriptionID)
		},
	})
}

func (in *ingest) licenseKeyGuard() func([]db.LicenseKey) ([]db.LicenseKey, error) {
	return guardOf(in, "license_keys", rowGuard[db.LicenseKey]{
		id: func(r db.LicenseKey) int64 { return r.ID }, created: func(r db.LicenseKey) time.Time { return r.CreatedAt },
		same:  func(l, r db.LicenseKey) bool { return l.Key == r.Key },
		label: func(r db.LicenseKey) string { return fmt.Sprintf("license key %s", keyHint(r.Key)) },
	})
}

func (in *ingest) instanceGuard() func([]db.LicenseKeyInstance) ([]db.LicenseKeyInstance, error) {
	return guardOf(in, "license_key_instances", rowGuard[db.LicenseKeyInstance]{
		id: func(r db.LicenseKeyInstance) int64 { return r.ID }, created: func(r db.LicenseKeyInstance) time.Time { return r.CreatedAt },
		same: func(l, r db.LicenseKeyInstance) bool { return l.Identifier == r.Identifier },
		label: func(r db.LicenseKeyInstance) string {
			return fmt.Sprintf("instance %q of key #%d", r.Name, r.LicenseKeyID)
		},
	})
}

// keyHint shortens a license key for log lines (keys are credentials).
func keyHint(k string) string {
	if len(k) <= 8 {
		return "…"
	}
	return "…" + k[len(k)-8:]
}
