package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/PMG801/wedding/internal/config"
	"github.com/PMG801/wedding/internal/db"
	"github.com/PMG801/wedding/internal/httpapi"
	"github.com/PMG801/wedding/internal/storage"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: boda <command>")
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr, "  check    Run health check")
		fmt.Fprintln(os.Stderr, "  serve    Start the HTTP server")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "check":
		if err := runCheck(); err != nil {
			fmt.Fprintln(os.Stderr, "check failed:", err)
			os.Exit(1)
		}
		fmt.Println("ok")
	case "serve":
		if err := runServe(); err != nil {
			fmt.Fprintln(os.Stderr, "server failed:", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

// initialize opens the shared, validated runtime configuration and persistence.
func initialize(ctx context.Context) (*sql.DB, config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, config.Config{}, fmt.Errorf("load configuration: %w", err)
	}
	paths, err := storage.Initialize(cfg.DataDir)
	if err != nil {
		return nil, config.Config{}, fmt.Errorf("initialize data storage: %w", err)
	}
	database, err := db.Open(ctx, filepath.Join(paths.Data, "boda.db"))
	if err != nil {
		return nil, config.Config{}, fmt.Errorf("initialize database: %w", err)
	}
	return database, cfg, nil
}

func runCheck() (resultErr error) {
	database, _, err := initialize(context.Background())
	if err != nil {
		return err
	}
	defer func() {
		if err := database.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close database: %w", err)
		}
	}()
	return nil
}

func runServe() (resultErr error) {
	database, cfg, err := initialize(context.Background())
	if err != nil {
		return err
	}
	defer func() {
		if err := database.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close database: %w", err)
		}
	}()
	addr := os.Getenv("BODA_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewHandler(cfg.EventToken, []byte(cfg.SessionKey)),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		if err := <-serverErr; err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
}
