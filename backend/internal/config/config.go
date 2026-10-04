// Package config loads and validates the backend's environment-based settings.
package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains runtime settings that are fixed for the lifetime of the process.
type Config struct {
	DataDir              string
	EventToken           string
	AdminPasswordHash    string
	SessionKey           string
	MaxPhotoBytes        int64
	MaxVideoBytes        int64
	MaxConcurrentUploads int
	UploadIdleTimeout    time.Duration
	DiskMinFreeBytes     int64
	CleanupInterval      time.Duration
	ThumbFallback        bool
	LogLevel             string
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	return LoadFrom(os.LookupEnv)
}

// LoadFrom reads and validates configuration using lookup. Errors identify the
// setting to fix but never include configured values, which may be secret.
func LoadFrom(lookup func(string) (string, bool)) (Config, error) {
	if lookup == nil {
		return Config{}, fmt.Errorf("configuration lookup is required")
	}
	get := func(key string) (string, error) {
		value, ok := lookup(key)
		if !ok || strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("%s is required", key)
		}
		return value, nil
	}

	var cfg Config
	var err error
	if cfg.DataDir, err = get("BODA_DATA_DIR"); err != nil {
		return Config{}, err
	}
	if cfg.EventToken, err = get("BODA_EVENT_TOKEN"); err != nil {
		return Config{}, err
	}
	if !validEventToken(cfg.EventToken) {
		return Config{}, fmt.Errorf("BODA_EVENT_TOKEN must be a base64url value encoding exactly 32 bytes")
	}
	if cfg.AdminPasswordHash, err = get("BODA_ADMIN_PASSWORD_HASH"); err != nil {
		return Config{}, err
	}
	if !validPasswordHash(cfg.AdminPasswordHash) {
		return Config{}, fmt.Errorf("BODA_ADMIN_PASSWORD_HASH must be a bcrypt or argon2id hash")
	}
	if cfg.SessionKey, err = get("BODA_SESSION_KEY"); err != nil {
		return Config{}, err
	}
	if len(cfg.SessionKey) != 32 {
		return Config{}, fmt.Errorf("BODA_SESSION_KEY must contain exactly 32 bytes")
	}

	if cfg.MaxPhotoBytes, err = positiveInt64(lookup, "BODA_MAX_PHOTO_BYTES", 52428800); err != nil {
		return Config{}, err
	}
	if cfg.MaxVideoBytes, err = positiveInt64(lookup, "BODA_MAX_VIDEO_BYTES", 1610612736); err != nil {
		return Config{}, err
	}
	if cfg.MaxConcurrentUploads, err = positiveInt(lookup, "BODA_MAX_CONCURRENT_UPLOADS", 30); err != nil {
		return Config{}, err
	}
	if cfg.UploadIdleTimeout, err = positiveDuration(lookup, "BODA_UPLOAD_IDLE_TIMEOUT", 90*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.DiskMinFreeBytes, err = positiveInt64(lookup, "BODA_DISK_MIN_FREE_BYTES", 16106127360); err != nil {
		return Config{}, err
	}
	if cfg.CleanupInterval, err = positiveDuration(lookup, "BODA_CLEANUP_INTERVAL", 10*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.ThumbFallback, err = boolSetting(lookup, "BODA_THUMB_FALLBACK", false); err != nil {
		return Config{}, err
	}
	cfg.LogLevel, err = stringSetting(lookup, "BODA_LOG_LEVEL", "info")
	if err != nil {
		return Config{}, err
	}
	cfg.LogLevel = strings.ToLower(cfg.LogLevel)
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("BODA_LOG_LEVEL must be debug, info, warn, or error")
	}
	return cfg, nil
}

func positiveInt64(lookup func(string) (string, bool), key string, fallback int64) (int64, error) {
	value, ok := lookup(key)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return parsed, nil
}

func positiveInt(lookup func(string) (string, bool), key string, fallback int) (int, error) {
	value, ok := lookup(key)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return parsed, nil
}

func positiveDuration(lookup func(string) (string, bool), key string, fallback time.Duration) (time.Duration, error) {
	value, ok := lookup(key)
	if !ok || value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return parsed, nil
}

func boolSetting(lookup func(string) (string, bool), key string, fallback bool) (bool, error) {
	value, ok := lookup(key)
	if !ok || value == "" {
		return fallback, nil
	}
	switch strings.ToLower(value) {
	case "on", "true":
		return true, nil
	case "off", "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be on or off", key)
	}
}

func stringSetting(lookup func(string) (string, bool), key, fallback string) (string, error) {
	value, ok := lookup(key)
	if !ok || value == "" {
		return fallback, nil
	}
	return value, nil
}

func validEventToken(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(value)
	}
	return err == nil && len(decoded) == 32
}

func validPasswordHash(value string) bool {
	if len(value) == 60 && (strings.HasPrefix(value, "$2a$") || strings.HasPrefix(value, "$2b$") || strings.HasPrefix(value, "$2y$")) {
		cost, err := strconv.Atoi(value[4:6])
		if err != nil || cost < 4 || cost > 31 || value[6] != '$' {
			return false
		}
		for _, char := range value[7:] {
			if !(char == '.' || char == '/' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9') {
				return false
			}
		}
		return true
	}
	parts := strings.Split(value, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return false
	}
	for i, name := range []string{"m", "t", "p"} {
		keyValue := strings.SplitN(params[i], "=", 2)
		if len(keyValue) != 2 || keyValue[0] != name {
			return false
		}
		parsed, err := strconv.Atoi(keyValue[1])
		if err != nil || parsed <= 0 {
			return false
		}
	}
	return validEncodedHashPart(parts[4]) && validEncodedHashPart(parts[5])
}

func validEncodedHashPart(value string) bool {
	if value == "" {
		return false
	}
	decoded, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(value)
	}
	return err == nil && len(decoded) > 0
}
