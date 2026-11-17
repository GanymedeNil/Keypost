// Package server wires every component into a running application.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"gorm.io/gorm"

	"fulfillment/internal/admin"
	"fulfillment/internal/auth"
	"fulfillment/internal/config"
	"fulfillment/internal/crypt"
	"fulfillment/internal/db"
	"fulfillment/internal/httpx"
	"fulfillment/internal/importer"
	"fulfillment/internal/lsapi"
	"fulfillment/internal/mail"
	"fulfillment/internal/resource"
	"fulfillment/internal/service"
	"fulfillment/internal/settings"
	"fulfillment/internal/storage"
	"fulfillment/internal/storefront"
	"fulfillment/internal/urls"
	"fulfillment/internal/webhooks"
)

type App struct {
	Cfg        *config.Config
	DB         *gorm.DB
	Svc        *service.Svc
	Dispatcher *webhooks.Dispatcher
	Importer   *importer.Runner
	Handler    http.Handler
	Log        *slog.Logger
}

// New builds the application. An optional store overrides the file storage (tests).
func New(cfg *config.Config, log *slog.Logger, store storage.Storage) (*App, error) {
	crypt.SetSecret(cfg.AppSecret)
	g, err := db.Open(cfg.DatabaseURL, false)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	if err := db.Migrate(g); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if _, err := db.EnsureDefaultStore(g, cfg.PublicURL); err != nil {
		return nil, err
	}
	st := settings.New(g, cfg)
	if store == nil {
		store, err = storage.New(cfg)
		if err != nil {
			return nil, fmt.Errorf("storage: %w", err)
		}
	}
	u := urls.Builder{Base: cfg.PublicURL, DownloadTTL: cfg.DownloadURLTTL}
	res := &resource.Builder{DB: g, URLs: u}
	disp := webhooks.New(g, res, log, cfg.WebhookMaxAttempts)
	svc := &service.Svc{Cfg: cfg, DB: g, Settings: st, URLs: u, Res: res, Events: disp, Mail: mail.New(g, st), Storage: store, Log: log}
	imp := importer.NewRunner(svc)

	if err := bootstrapAdmin(g, cfg, st, log); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if sqlDB, err := g.DB(); err != nil || sqlDB.PingContext(r.Context()) != nil {
			httpx.JSON(w, http.StatusServiceUnavailable, map[string]any{"status": "unhealthy"})
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	lsapi.New(svc).Routes(mux)
	storefront.New(svc).Routes(mux)
	admin.New(svc, disp, imp).Routes(mux)

	var h http.Handler = mux
	h = httpx.CORS(cfg.CORSAllowedOrigins, h)
	h = httpx.Recover(log, h)
	return &App{Cfg: cfg, DB: g, Svc: svc, Dispatcher: disp, Importer: imp, Handler: h, Log: log}, nil
}

// bootstrapAdmin creates the first owner from the environment or prints a
// one-time setup token for the /admin first-run form.
func bootstrapAdmin(g *gorm.DB, cfg *config.Config, st *settings.Service, log *slog.Logger) error {
	var n int64
	g.Model(&db.User{}).Count(&n)
	if n > 0 {
		return nil
	}
	if cfg.AdminEmail != "" && cfg.AdminPassword != "" {
		hash, err := auth.HashPassword(cfg.AdminPassword)
		if err != nil {
			return err
		}
		now := db.Now()
		u := db.User{Email: cfg.AdminEmail, Name: cfg.AdminName, PasswordHash: hash, Role: auth.RoleOwner, CreatedAt: now, UpdatedAt: now}
		if err := g.Create(&u).Error; err != nil {
			return err
		}
		log.Info("created owner account from ADMIN_EMAIL/ADMIN_PASSWORD", "email", u.Email)
		return nil
	}
	token := st.Get("setup.token")
	if token == "" {
		token = crypt.RandomToken(18)
		if err := st.Set("setup.token", token); err != nil {
			return err
		}
	}
	log.Warn("no admin user yet — open /admin and finish setup with this one-time token", "setup_token", token)
	return nil
}

// Run starts background workers and the HTTP server until ctx is cancelled.
func (a *App) Run(ctx context.Context) error {
	go a.Dispatcher.Run(ctx)
	go a.janitor(ctx)
	srv := &http.Server{
		Addr: a.Cfg.ListenAddr, Handler: a.Handler,
		ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		a.Log.Info("listening", "addr", a.Cfg.ListenAddr, "public_url", a.Cfg.PublicURL)
		errc <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		err := srv.Shutdown(sctx)
		service.WaitBackground()
		return err
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (a *App) janitor(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	prune := time.NewTicker(6 * time.Hour)
	defer prune.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.Svc.RetryFailedStripeEvents(ctx)
		case <-prune.C:
			a.Svc.PruneOldRecords(ctx)
		}
	}
}
