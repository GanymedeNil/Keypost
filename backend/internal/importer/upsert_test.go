package importer

import (
	"database/sql"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"fulfillment/internal/db"
)

// The importer is developed against SQLite but runs on PostgreSQL in
// production: check the generated upsert statement for that dialect without a
// server (DryRun builds the SQL and never connects).
func TestUpsertSQLForPostgres(t *testing.T) {
	sqlDB, err := sql.Open("pgx", "postgres://nobody:nothing@127.0.0.1:1/none")
	if err != nil {
		t.Fatal(err)
	}
	pg, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	e := newEnv(t)
	in := newIngest(e.r, newJobCtl(e.r, t.Context(), 1, "x"), ingestCfg{}, nil)
	in.g = pg

	spec := in.newSpec(&db.Order{}, []string{"stripe_payment_intent", "custom_data"}, []string{"card_brand"})
	tx := pg.Session(&gorm.Session{DryRun: true})
	res := tx.Clauses(spec.clause()).Create(&[]db.Order{{ID: 5, Identifier: "x"}})
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	sqlText := res.Statement.SQL.String()
	for _, want := range []string{
		`INSERT INTO "orders"`, `ON CONFLICT ("id") DO UPDATE SET`, `"store_id"="excluded"."store_id"`,
		`"card_brand"=COALESCE(NULLIF(excluded.card_brand, ''), orders.card_brand)`, `RETURNING "id"`,
		`WHERE orders.updated_at <= excluded.updated_at`,
	} {
		if !strings.Contains(sqlText, want) {
			t.Errorf("SQL lacks %q:\n%s", want, sqlText)
		}
	}
	for _, protected := range []string{`"stripe_payment_intent"="excluded"`, `"custom_data"="excluded"`, `"id"="excluded"`} {
		if strings.Contains(sqlText, protected) {
			t.Errorf("SQL must not overwrite %s:\n%s", protected, sqlText)
		}
	}
}
