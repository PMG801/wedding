package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/PMG801/wedding/internal/backup"
	"github.com/PMG801/wedding/internal/config"
	"github.com/PMG801/wedding/internal/db"
	"github.com/PMG801/wedding/internal/httpapi"
	"github.com/PMG801/wedding/internal/jobs"
	"github.com/PMG801/wedding/internal/media"
	"github.com/PMG801/wedding/internal/settings"
	"github.com/PMG801/wedding/internal/storage"
	"github.com/PMG801/wedding/internal/upload"
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
func initialize(ctx context.Context) (*sql.DB, config.Config, storage.Paths, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, config.Config{}, storage.Paths{}, fmt.Errorf("load configuration: %w", err)
	}
	paths, err := storage.Initialize(cfg.DataDir)
	if err != nil {
		return nil, config.Config{}, storage.Paths{}, fmt.Errorf("initialize data storage: %w", err)
	}
	database, err := db.Open(ctx, filepath.Join(paths.Data, "boda.db"))
	if err != nil {
		return nil, config.Config{}, storage.Paths{}, fmt.Errorf("initialize database: %w", err)
	}
	return database, cfg, paths, nil
}

func runCheck() (resultErr error) {
	database, _, _, err := initialize(context.Background())
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

// newAppHandler composes the HTTP handler: guest and public routes plus the
// administrator routes, sharing one cached upload switch loaded from SQLite.
func newAppHandler(ctx context.Context, database *sql.DB, cfg config.Config, photos *upload.Store, gallery *media.Gallery) (http.Handler, error) {
	uploadSwitch, err := settings.Open(ctx, database)
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}
	return httpapi.NewHandler(cfg.EventToken, []byte(cfg.SessionKey), photos, cfg.MaxConcurrentUploads, cfg.UploadIdleTimeout, cfg.MaxPhotoBytes, gallery,
		httpapi.WithMaxVideoBytes(cfg.MaxVideoBytes),
		httpapi.WithMaxDerivativeBytes(cfg.MaxDerivativeBytes),
		httpapi.WithAdmin(cfg.AdminPasswordHash, uploadSwitch, photos),
		httpapi.WithModeration(photos)), nil
}

// newPhotoStore builds the upload store with the configured limits, disk
// reservation and staged-derivative retention.
func newPhotoStore(database *sql.DB, cfg config.Config, paths storage.Paths) (*upload.Store, error) {
	return upload.NewStore(database, paths, upload.Options{
		MaxBytes:            cfg.MaxPhotoBytes,
		MaxVideoBytes:       cfg.MaxVideoBytes,
		MaxDerivativeBytes:  cfg.MaxDerivativeBytes,
		StagedDerivativeTTL: cfg.StagedDerivativeTTL,
		MaxConcurrent:       cfg.MaxConcurrentUploads,
		MinFreeBytes:        uint64(cfg.DiskMinFreeBytes),
	})
}

// newServer applies the connection limits: the per-read idle timeout protects
// slow uploads, so ReadTimeout and WriteTimeout stay unset on purpose.
func newServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
}

// backupOnce writes one SQLite backup and logs its path or the failure. A
// failed backup never stops the server.
func backupOnce(ctx context.Context, database *sql.DB, dir string, keep int, now time.Time) {
	path, err := backup.Run(ctx, database, dir, keep, now)
	if err != nil {
		slog.Error("backup sqlite database", "path", path, "error", err)
		return
	}
	slog.Info("backed up sqlite database", "path", path)
}

func runServe() (resultErr error) {
	database, cfg, paths, err := initialize(context.Background())
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

	photos, err := newPhotoStore(database, cfg, paths)
	if err != nil {
		return fmt.Errorf("initialize photo uploads: %w", err)
	}
	gallery, err := media.NewGallery(database, paths)
	if err != nil {
		return fmt.Errorf("initialize gallery: %w", err)
	}
	handler, err := newAppHandler(context.Background(), database, cfg, photos, gallery)
	if err != nil {
		return err
	}
	server := newServer(addr, handler)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go jobs.Every(ctx, cfg.CleanupInterval, func(ctx context.Context) {
		report, err := photos.Reconcile(ctx)
		if err != nil {
			slog.Error("reconcile upload storage", "error", err)
		}
		slog.Info("reconciled upload storage",
			"removed_partials", report.RemovedPartials, "quarantined", report.Quarantined,
			"promoted_derivatives", report.PromotedDerivatives, "removed_staged", report.RemovedStaged, "skipped_active", report.SkippedActive, "visibility_repairs", report.VisibilityRepairs)
	})

	// The backup runs in the background so a slow copy never delays startup.
	backupDir := filepath.Join(paths.Root, "backup")
	go jobs.Every(ctx, cfg.BackupInterval, func(ctx context.Context) {
		backupOnce(ctx, database, backupDir, cfg.BackupKeep, time.Now())
	})

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
