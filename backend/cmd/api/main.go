// Command api runs the Yozigo REST API.
//
//	api          migrate, optionally seed (SEED_ON_START=true), then serve
//	api migrate  apply migrations and exit
//	api seed     apply migrations, insert demo data and exit
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"yozigo/internal/config"
	"yozigo/internal/database"
	"yozigo/internal/server"
	"yozigo/migrations"
	"yozigo/seeds"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := database.Migrate(ctx, pool, migrations.FS); err != nil {
		return err
	}

	switch cmd {
	case "migrate":
		return nil
	case "seed":
		return seeds.Run(ctx, pool, cfg.DemoPassword, cfg.CinemaTimezone)
	case "serve":
	default:
		return errors.New("unknown command " + cmd + " (want serve, migrate or seed)")
	}

	if cfg.SeedOnStart {
		if err := seeds.Run(ctx, pool, cfg.DemoPassword, cfg.CinemaTimezone); err != nil {
			return err
		}
	}
	if cfg.DemoLoginEnabled {
		slog.Warn("DEMO LOGIN IS ENABLED: anyone can log in as the demo accounts. Use only in an isolated demo environment.")
	}

	stores := server.NewStores(pool, cfg)
	go cleanupLoop(ctx, stores)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.New(cfg, pool, stores),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", srv.Addr, "env", cfg.AppEnv, "timezone", cfg.CinemaTimezone.String())
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// cleanupLoop periodically deletes stale hold rows and expired sessions. This is
// housekeeping only: expired holds already stop blocking seats because every query
// compares expires_at with now().
func cleanupLoop(ctx context.Context, st *server.Stores) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := st.Seats.PurgeExpiredHoldSeats(ctx); err != nil {
				slog.Error("hold cleanup failed", "err", err)
			} else if n > 0 {
				slog.Info("purged expired hold seats", "rows", n)
			}
			if _, err := st.Auth.DeleteExpiredSessions(ctx); err != nil {
				slog.Error("session cleanup failed", "err", err)
			}
		}
	}
}
