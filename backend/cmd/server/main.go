// Command server runs the Keypost API, webhooks and storefront.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"fulfillment/internal/config"
	"fulfillment/internal/server"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	app, err := server.New(cfg, log, nil)
	if err != nil {
		log.Error("startup failed", "err", err)
		os.Exit(1)
	}
	app.Importer.RecoverInterrupted()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
