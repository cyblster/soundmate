package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/cyblster/soundmate/internal/bot"
	"github.com/cyblster/soundmate/internal/config"
	"github.com/cyblster/soundmate/internal/logger"
	"github.com/cyblster/soundmate/internal/storage"
)

func main() {
	envFile := flag.String("env", ".env", "path to an optional env file")
	flag.Parse()

	if err := run(*envFile); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run(envFile string) error {
	cfg, err := config.Load(envFile)
	if err != nil {
		return err
	}
	log := logger.New(os.Stdout, cfg.Debug)
	log.Info("starting soundmate", "go", runtime.Version(), "debug", cfg.Debug)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("connecting to postgres", "host", cfg.Postgres.Host, "port", cfg.Postgres.Port, "database", cfg.Postgres.Database)
	db, err := storage.Connect(ctx, cfg.Postgres.DSN(), log.With("component", "storage"))
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	b, err := bot.New(cfg, log, db)
	if err != nil {
		return err
	}
	return b.Run(ctx)
}
