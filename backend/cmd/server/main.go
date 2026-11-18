// Command server runs the Keypost API, webhooks and storefront.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"fulfillment/internal/config"
	"fulfillment/internal/server"
	"fulfillment/internal/version"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Println("keypost-server", version.String())
		return
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	log.Info("starting keypost", "version", version.String())
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
