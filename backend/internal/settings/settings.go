// Package settings stores the small operational switches kept in the SQLite settings table.
package settings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// UploadsEnabledKey is the manual upload switch ("true" or "false"). A missing row means enabled.
const UploadsEnabledKey = "uploads_enabled"

var (
	// ErrUnknownKey is returned for keys outside the allow-list.
	ErrUnknownKey = errors.New("unknown setting key")
	// ErrInvalidValue is returned when a value does not fit its key.
	ErrInvalidValue = errors.New("invalid setting value")
)

// validators is the allow-list: only these keys may be read or written.
var validators = map[string]func(string) error{
	UploadsEnabledKey: func(value string) error {
		if value != "true" && value != "false" {
			return fmt.Errorf("%s must be \"true\" or \"false\": %w", UploadsEnabledKey, ErrInvalidValue)
		}
		return nil
	},
}

// Store serves settings from memory and writes every change through to SQLite.
// Reads of the upload switch never touch the database, so the upload hot path
// stays free of queries. Safe for concurrent use.
type Store struct {
	db             *sql.DB
	writeMu        sync.Mutex
	uploadsEnabled atomic.Bool
}

// Open loads the upload switch from the database. A missing row means uploads are enabled.
func Open(ctx context.Context, database *sql.DB) (*Store, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	store := &Store{db: database}
	value, found, err := store.read(ctx, UploadsEnabledKey)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", UploadsEnabledKey, err)
	}
	enabled := true
	if found {
		if err := validators[UploadsEnabledKey](value); err != nil {
			return nil, err
		}
		enabled = value == "true"
	}
	store.uploadsEnabled.Store(enabled)
	return store, nil
}

// UploadsEnabled reports the cached manual switch without querying SQLite.
func (s *Store) UploadsEnabled() bool {
	return s.uploadsEnabled.Load()
}

// SetUploadsEnabled persists the manual switch and updates the cache once the write succeeds.
func (s *Store) SetUploadsEnabled(ctx context.Context, enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	return s.Set(ctx, UploadsEnabledKey, value)
}

// Get returns the stored value of an allow-listed key; found is false when no row exists.
func (s *Store) Get(ctx context.Context, key string) (string, bool, error) {
	if _, ok := validators[key]; !ok {
		return "", false, ErrUnknownKey
	}
	return s.read(ctx, key)
}

// Set validates key and value against the allow-list, then writes them to SQLite
// and refreshes the cache. The cache is untouched if the write fails.
func (s *Store) Set(ctx context.Context, key, value string) error {
	validate, ok := validators[key]
	if !ok {
		return ErrUnknownKey
	}
	if err := validate(value); err != nil {
		return err
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("write %s: %w", key, err)
	}
	if key == UploadsEnabledKey {
		s.uploadsEnabled.Store(value == "true")
	}
	return nil
}

func (s *Store) read(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}
