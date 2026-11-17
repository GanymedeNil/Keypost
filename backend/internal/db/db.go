package db

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open connects to PostgreSQL (postgres:// or postgresql://) or SQLite
// (sqlite:path, or sqlite::memory: for tests).
func Open(url string, debug bool) (*gorm.DB, error) {
	lvl := logger.Warn
	if debug {
		lvl = logger.Info
	}
	cfg := &gorm.Config{
		Logger: logger.New(log.New(os.Stderr, "", log.LstdFlags), logger.Config{
			SlowThreshold: 500 * time.Millisecond, LogLevel: lvl, IgnoreRecordNotFoundError: true,
		}),
		NowFunc:                                  func() time.Time { return time.Now().UTC() },
		DisableForeignKeyConstraintWhenMigrating: true,
		TranslateError:                           true,
	}

	switch {
	case strings.HasPrefix(url, "postgres://"), strings.HasPrefix(url, "postgresql://"):
		g, err := gorm.Open(postgres.Open(url), cfg)
		if err != nil {
			return nil, err
		}
		sqlDB, err := g.DB()
		if err != nil {
			return nil, err
		}
		sqlDB.SetMaxOpenConns(20)
		sqlDB.SetMaxIdleConns(5)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
		return g, nil

	case strings.HasPrefix(url, "sqlite:"):
		path := strings.TrimPrefix(strings.TrimPrefix(url, "sqlite:"), "//")
		var dsn string
		mem := path == ":memory:" || path == "" || strings.HasPrefix(path, "memory")
		if mem {
			dsn = fmt.Sprintf("file:mem%d?mode=memory&cache=shared&_pragma=busy_timeout(10000)", time.Now().UnixNano())
		} else {
			if dir := filepath.Dir(path); dir != "." {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return nil, err
				}
			}
			dsn = fmt.Sprintf("file:%s?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_txlock=immediate", path)
		}
		g, err := gorm.Open(sqlite.Open(dsn), cfg)
		if err != nil {
			return nil, err
		}
		sqlDB, err := g.DB()
		if err != nil {
			return nil, err
		}
		if mem {
			// A single connection keeps the shared in-memory database alive and
			// surfaces accidental nested-connection use immediately in tests.
			sqlDB.SetMaxOpenConns(1)
		} else {
			sqlDB.SetMaxOpenConns(4)
		}
		return g, nil
	}
	return nil, fmt.Errorf("unsupported DATABASE_URL %q (use postgres://… or sqlite:path)", redact(url))
}

func redact(u string) string {
	if i := strings.Index(u, "@"); i > 0 {
		return "…" + u[i:]
	}
	return u
}

func Migrate(g *gorm.DB) error {
	return g.AutoMigrate(AllModels()...)
}

// IsPostgres reports whether the connection is PostgreSQL.
func IsPostgres(g *gorm.DB) bool { return g.Dialector.Name() == "postgres" }

// ResetSequences advances PostgreSQL serial sequences after rows were
// inserted with explicit ids (Lemon Squeezy import). It is a no-op on SQLite
// where AUTOINCREMENT already follows the largest rowid.
func ResetSequences(g *gorm.DB) error {
	if !IsPostgres(g) {
		return nil
	}
	tables := []string{
		"users", "api_keys", "audit_logs", "stores", "products", "variants", "prices", "files",
		"customers", "orders", "order_items", "subscriptions", "subscription_items",
		"subscription_invoices", "license_keys", "license_key_instances", "webhooks",
	}
	for _, t := range tables {
		q := fmt.Sprintf(`SELECT setval(pg_get_serial_sequence('%s','id'), GREATEST(COALESCE((SELECT MAX(id) FROM %s),0),1), (SELECT COUNT(*) FROM %s) > 0)`, t, t, t)
		if err := g.Exec(q).Error; err != nil {
			return fmt.Errorf("reset sequence %s: %w", t, err)
		}
	}
	return nil
}

// EnsureDefaultStore creates the first store when the database is empty.
func EnsureDefaultStore(g *gorm.DB, publicURL string) (*Store, error) {
	var s Store
	err := g.Order("id").First(&s).Error
	if err == nil {
		return &s, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}
	host := strings.TrimPrefix(strings.TrimPrefix(publicURL, "https://"), "http://")
	s = Store{Name: "My Store", Slug: "my-store", Domain: host, URL: publicURL, Plan: "self-hosted", Country: "US", Currency: "USD"}
	if err := g.Create(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}
