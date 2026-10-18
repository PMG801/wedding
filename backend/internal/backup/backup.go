// Package backup writes periodic consistent copies of the SQLite database.
package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	prefix = "boda-"
	suffix = ".db"
)

// Run writes a consistent copy of database to dir with VACUUM INTO, named by
// the UTC time now, and then keeps only the newest keep copies. The copy is
// written under a temporary name and renamed, so a crash never leaves a
// truncated file that looks like a backup. Files that are not backups are
// never touched.
func Run(ctx context.Context, database *sql.DB, dir string, keep int, now time.Time) (string, error) {
	if keep <= 0 {
		return "", errors.New("backup keep count must be positive")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}
	name := prefix + now.UTC().Format("20060102T150405Z") + suffix
	final := filepath.Join(dir, name)
	tmp := final + ".tmp"
	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove stale temporary backup: %w", err)
	}
	if _, err := database.ExecContext(ctx, "VACUUM INTO ?", tmp); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("write backup: %w", err)
	}
	if err := syncFile(tmp); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("publish backup: %w", err)
	}
	if err := syncFile(dir); err != nil {
		return "", err
	}
	return final, prune(dir, keep)
}

func prune(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("list backups: %w", err)
	}
	var backups []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type().IsRegular() && strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) {
			backups = append(backups, name)
		}
	}
	// Timestamped names sort chronologically.
	sort.Strings(backups)
	for len(backups) > keep {
		if err := os.Remove(filepath.Join(dir, backups[0])); err != nil {
			return fmt.Errorf("remove old backup: %w", err)
		}
		backups = backups[1:]
	}
	return nil
}

func syncFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s for sync: %w", path, err)
	}
	defer file.Close()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", path, err)
	}
	return nil
}
