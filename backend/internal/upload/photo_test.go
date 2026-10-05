package upload

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PMG801/wedding/internal/db"
	"github.com/PMG801/wedding/internal/storage"
)

const testID = "123e4567-e89b-42d3-a456-426614174000"

func TestDetectPhotoTypeUsesSignature(t *testing.T) {
	for _, test := range []struct {
		name string
		sig  []byte
		mime string
		ext  string
	}{
		{"jpeg", []byte{0xff, 0xd8, 0xff, 0}, "image/jpeg", ".jpg"},
		{"png", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, "image/png", ".png"},
		{"heic", []byte{0, 0, 0, 0, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c'}, "image/heic", ".heic"},
		{"heif", []byte{0, 0, 0, 0, 'f', 't', 'y', 'p', 'm', 'i', 'f', '1'}, "image/heif", ".heif"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := DetectPhotoType(test.sig)
			if err != nil || got.MIME != test.mime || got.Extension != test.ext {
				t.Fatalf("DetectPhotoType() = %+v, %v; want %s %s", got, err, test.mime, test.ext)
			}
		})
	}
	if _, err := DetectPhotoType([]byte("not an image")); err == nil {
		t.Fatal("DetectPhotoType() accepted an unknown signature")
	}
}

func TestValidIDRequiresCanonicalUUIDv4(t *testing.T) {
	if !validID(testID) || validID(strings.ToUpper(testID)) || validID("123e4567-e89b-12d3-a456-426614174000") || validID("not-a-uuid") {
		t.Fatal("validID did not enforce canonical lowercase UUIDv4")
	}
}

func TestSavePhotoConfirmsAndRetryDoesNotReadOrRewrite(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 2})
	body := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2, 3}
	got, err := store.SavePhoto(context.Background(), testID, int64(len(body)), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("SavePhoto() error = %v", err)
	}
	if got.MIME != "image/png" || got.SizeBytes != int64(len(body)) || got.AlreadyConfirmed {
		t.Fatalf("unexpected first confirmation: %+v", got)
	}
	filePath := filepath.Join(paths.Originals, got.StorageName)
	before, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := store.SavePhoto(context.Background(), testID, 0, errorReader{})
	if err != nil || !retry.AlreadyConfirmed || retry.StorageName != got.StorageName {
		t.Fatalf("idempotent retry = %+v, %v", retry, err)
	}
	after, err := os.ReadFile(filePath)
	if err != nil || string(after) != string(before) {
		t.Fatalf("confirmed file changed on retry: %v", err)
	}
	var count int
	if err := store.db.QueryRow("SELECT count(*) FROM media WHERE id = ?", testID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("confirmed row count = %d, %v; want one", count, err)
	}
}

func TestSavePhotoRejectsSizeSignatureAndIncompleteBodyWithoutPublishing(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 9, MaxConcurrent: 1})
	for _, test := range []struct {
		name string
		size int64
		body []byte
	}{
		{"oversize", 10, []byte("ignored")},
		{"unknown signature", 3, []byte("bad")},
		{"short body", 9, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := store.SavePhoto(context.Background(), testID, test.size, bytes.NewReader(test.body)); err == nil {
				t.Fatal("SavePhoto() succeeded; want rejection")
			}
			entries, err := os.ReadDir(paths.Tmp)
			if err != nil || len(entries) != 0 {
				t.Fatalf("partial files remain: %v, %v", entries, err)
			}
			entries, err = os.ReadDir(paths.Originals)
			if err != nil || len(entries) != 0 {
				t.Fatalf("unconfirmed files published: %v, %v", entries, err)
			}
		})
	}
}

func TestSavePhotoHonorsMinimumFreeDisk(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 1, MinFreeBytes: ^uint64(0)})
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if _, err := store.SavePhoto(context.Background(), testID, int64(len(png)), bytes.NewReader(png)); err == nil {
		t.Fatal("SavePhoto() succeeded below the configured free-disk threshold")
	}
	entries, err := os.ReadDir(paths.Tmp)
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial files remain: %v, %v", entries, err)
	}
}

func newTestStore(t *testing.T, options Options) (*Store, storage.Paths) {
	t.Helper()
	root := t.TempDir()
	paths, err := storage.Initialize(root)
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(context.Background(), filepath.Join(paths.Data, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	store, err := NewStore(database, paths, options)
	if err != nil {
		t.Fatal(err)
	}
	return store, paths
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("retry body must not be read") }
