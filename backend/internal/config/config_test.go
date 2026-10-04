package config

import (
	"strings"
	"testing"
	"time"
)

func validEnvironment() map[string]string {
	return map[string]string{
		"BODA_DATA_DIR":            "/srv/evento",
		"BODA_EVENT_TOKEN":         "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"BODA_ADMIN_PASSWORD_HASH": "$2b$12$" + strings.Repeat("a", 53),
		"BODA_SESSION_KEY":         "0123456789abcdef0123456789abcdef",
	}
}

func loadFrom(values map[string]string) (Config, error) {
	return LoadFrom(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
}

func TestLoadFromRequiresConfigurationWithoutLeakingSecrets(t *testing.T) {
	for _, key := range []string{"BODA_DATA_DIR", "BODA_EVENT_TOKEN", "BODA_ADMIN_PASSWORD_HASH", "BODA_SESSION_KEY"} {
		t.Run(key, func(t *testing.T) {
			values := validEnvironment()
			delete(values, key)
			_, err := loadFrom(values)
			if err == nil {
				t.Fatalf("LoadFrom() error = nil; want missing %s", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error %q does not identify %s", err, key)
			}
		})
	}
}

func TestLoadFromAcceptsArgon2IDHash(t *testing.T) {
	values := validEnvironment()
	values["BODA_ADMIN_PASSWORD_HASH"] = "$argon2id$v=19$m=65536,t=3,p=1$c2FsdA$aGFzaA"
	if _, err := loadFrom(values); err != nil {
		t.Fatalf("LoadFrom() rejected valid argon2id format: %v", err)
	}
}

func TestLoadFromParsesDefaultsAndOverrides(t *testing.T) {
	values := validEnvironment()
	values["BODA_MAX_PHOTO_BYTES"] = "1024"
	values["BODA_UPLOAD_IDLE_TIMEOUT"] = "45s"
	values["BODA_THUMB_FALLBACK"] = "on"
	got, err := loadFrom(values)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if got.DataDir != "/srv/evento" || got.MaxPhotoBytes != 1024 || got.MaxVideoBytes != 1610612736 {
		t.Errorf("unexpected configuration values: %+v", got)
	}
	if got.UploadIdleTimeout != 45*time.Second || !got.ThumbFallback {
		t.Errorf("overrides were not parsed: %+v", got)
	}
	if got.MaxConcurrentUploads != 30 || got.CleanupInterval != 10*time.Minute || got.DiskMinFreeBytes != 16106127360 {
		t.Errorf("unexpected defaults: %+v", got)
	}
}

func TestLoadFromRejectsInvalidValuesWithoutLeakingSecret(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "event token", key: "BODA_EVENT_TOKEN", value: "not-a-token-secret"},
		{name: "session key", key: "BODA_SESSION_KEY", value: "too-short-secret"},
		{name: "password hash", key: "BODA_ADMIN_PASSWORD_HASH", value: "invalid-secret-hash"},
		{name: "photo limit", key: "BODA_MAX_PHOTO_BYTES", value: "zero"},
		{name: "timeout", key: "BODA_UPLOAD_IDLE_TIMEOUT", value: "soon"},
		{name: "fallback", key: "BODA_THUMB_FALLBACK", value: "maybe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := validEnvironment()
			values[tt.key] = tt.value
			_, err := loadFrom(values)
			if err == nil {
				t.Fatal("LoadFrom() error = nil; want invalid value")
			}
			if strings.Contains(err.Error(), tt.value) {
				t.Errorf("error leaked configured value: %q", err)
			}
		})
	}
}
