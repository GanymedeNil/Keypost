package importer

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// upsertSpec says which columns an upsert may overwrite. Columns that hold
// local state (Stripe ids, settings, download counters) are left out so that
// re-running an import never undoes a mapping.
type upsertSpec struct {
	table       string
	cols        []string // overwritten with the incoming value
	keepIfEmpty []string // overwritten only by a non-empty incoming value
	always      bool     // overwrite even rows that were changed locally later
}

// newSpec derives the column lists from the model schema: every column
// except the primary key, the protected ones (skip) and keepIfEmpty ones.
func (in *ingest) newSpec(model any, skip, keepIfEmpty []string) upsertSpec {
	stmt := &gorm.Statement{DB: in.g}
	if err := stmt.Parse(model); err != nil {
		panic(fmt.Sprintf("importer: parse model %T: %v", model, err))
	}
	set := func(l []string) map[string]bool {
		m := make(map[string]bool, len(l))
		for _, s := range l {
			m[s] = true
		}
		return m
	}
	skipSet, keepSet := set(skip), set(keepIfEmpty)
	spec := upsertSpec{table: stmt.Schema.Table, keepIfEmpty: keepIfEmpty}
	for _, f := range stmt.Schema.Fields {
		if f.DBName == "" || f.PrimaryKey || skipSet[f.DBName] || keepSet[f.DBName] {
			continue
		}
		spec.cols = append(spec.cols, f.DBName)
	}
	return spec
}

func (s upsertSpec) clause() clause.OnConflict {
	sets := make(clause.Set, 0, len(s.cols)+len(s.keepIfEmpty))
	for _, c := range s.cols {
		sets = append(sets, clause.Assignment{Column: clause.Column{Name: c}, Value: clause.Column{Table: "excluded", Name: c}})
	}
	for _, c := range s.keepIfEmpty {
		sets = append(sets, clause.Assignment{
			Column: clause.Column{Name: c},
			Value:  clause.Expr{SQL: fmt.Sprintf("COALESCE(NULLIF(excluded.%s, ''), %s.%s)", c, s.table, c)},
		})
	}
	// A row changed locally after it was imported (a license activation, a
	// Stripe synchronisation, an admin edit) carries a newer updated_at than
	// the Lemon Squeezy data: such a row is left alone.
	oc := clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: sets}
	if !s.always {
		oc.Where = clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: fmt.Sprintf("%s.updated_at <= excluded.updated_at", s.table)}}}
	}
	return oc
}

// upsert inserts rows keyed by their primary key, updating the spec columns
// of rows that already exist.
func upsert[T any](tx *gorm.DB, spec upsertSpec, rows []T, batch int) error {
	if len(rows) == 0 {
		return nil
	}
	return tx.Clauses(spec.clause()).CreateInBatches(rows, batch).Error
}

// upsertWriter adapts upsert to writeBatch.
func upsertWriter[T any](in *ingest, spec upsertSpec) func(tx *gorm.DB, rows []T) error {
	return func(tx *gorm.DB, rows []T) error { return upsert(tx, spec, rows, in.r.batch()) }
}
