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
	if !ValidID(testID) || ValidID(strings.ToUpper(testID)) || ValidID("123e4567-e89b-12d3-a456-426614174000") || ValidID("not-a-uuid") {
		t.Fatal("validID did not enforce canonical lowercase UUIDv4")
	}
}

func TestSaveDerivativeStaysPrivateUntilPhotoConfirmationAndRetriesWithoutRewrite(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 2})
	jpeg := []byte{0xff, 0xd8, 0xff, 1, 2, 3}
	staged, err := store.SaveDerivative(context.Background(), testID, "t", int64(len(jpeg)), bytes.NewReader(jpeg))
	if err != nil || staged.AlreadyConfirmed {
		t.Fatalf("first staged derivative = %+v, %v", staged, err)
	}
	stagedPath := filepath.Join(paths.Tmp, testID+"_t.staged")
	if got, err := os.ReadFile(stagedPath); err != nil || !bytes.Equal(got, jpeg) {
		t.Fatalf("private staged derivative = %v, %v; want original JPEG bytes", got, err)
	}
	publishedPath := filepath.Join(paths.Derived, testID+"_t.jpg")
	if _, err := os.Stat(publishedPath); !os.IsNotExist(err) {
		t.Fatalf("derivative before original confirmation is published: %v", err)
	}

	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2, 3}
	if _, err := store.SavePhoto(context.Background(), testID, int64(len(png)), bytes.NewReader(png)); err != nil {
		t.Fatalf("SavePhoto() after staged derivative: %v", err)
	}
	before, err := os.ReadFile(publishedPath)
	if err != nil || !bytes.Equal(before, jpeg) {
		t.Fatalf("published derivative = %v, %v; want original JPEG bytes", before, err)
	}

	retry, err := store.SaveDerivative(context.Background(), testID, "t", 0, errorReader{})
	if err != nil || !retry.AlreadyConfirmed {
		t.Fatalf("derivative retry = %+v, %v; want already confirmed", retry, err)
	}
	after, err := os.ReadFile(publishedPath)
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("published derivative changed on retry: %v", err)
	}
}

func TestSaveDerivativeCannotPublishForHiddenOriginal(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 1})
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2, 3}
	if _, err := store.SavePhoto(context.Background(), testID, int64(len(png)), bytes.NewReader(png)); err != nil {
		t.Fatalf("SavePhoto() = %v", err)
	}
	if _, err := store.db.Exec("UPDATE media SET visibility = 'hidden' WHERE id = ?", testID); err != nil {
		t.Fatal(err)
	}

	jpeg := []byte{0xff, 0xd8, 0xff, 1, 2, 3}
	if _, err := store.SaveDerivative(context.Background(), testID, "t", int64(len(jpeg)), bytes.NewReader(jpeg)); !errors.Is(err, ErrMediaHidden) {
		t.Fatalf("SaveDerivative() error = %v; want ErrMediaHidden", err)
	}
	for _, directory := range []string{paths.Tmp, paths.Derived} {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.Contains(entry.Name(), "_t") {
				t.Fatalf("hidden media derivative was persisted in %s: %s", directory, entry.Name())
			}
		}
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

func ftypHeader(brand string) []byte {
	return append([]byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p'}, []byte(brand)...)
}

func TestDetectMediaTypeRecognizesVideosAndKeepsPhotos(t *testing.T) {
	for _, test := range []struct {
		brand, mime, ext string
	}{
		{"isom", "video/mp4", ".mp4"},
		{"mp42", "video/mp4", ".mp4"},
		{"M4V ", "video/mp4", ".mp4"},
		{"qt  ", "video/quicktime", ".mov"},
		{"heic", "image/heic", ".heic"},
		{"mif1", "image/heif", ".heif"},
	} {
		got, err := DetectMediaType(ftypHeader(test.brand))
		if err != nil || got.MIME != test.mime || got.Extension != test.ext {
			t.Fatalf("DetectMediaType(%q) = %+v, %v; want %s %s", test.brand, got, err, test.mime, test.ext)
		}
	}
	if _, err := DetectMediaType(ftypHeader("zzzz")); !errors.Is(err, ErrUnsupportedPhoto) {
		t.Fatalf("unknown brand error = %v", err)
	}
	if _, err := DetectPhotoType(ftypHeader("isom")); err == nil {
		t.Fatal("DetectPhotoType() accepted a video")
	}
}

func TestSavePhotoStoresVideoUsingVideoLimitAndKeepsPhotoLimit(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 20, MaxVideoBytes: 200, MaxConcurrent: 1})
	video := append(ftypHeader("mp42"), bytes.Repeat([]byte{7}, 100)...)
	media, err := store.SavePhoto(context.Background(), testID, int64(len(video)), bytes.NewReader(video))
	if err != nil || media.MIME != "video/mp4" || !strings.HasSuffix(media.StorageName, ".mp4") {
		t.Fatalf("SavePhoto(video) = %+v, %v", media, err)
	}
	if got, err := os.ReadFile(filepath.Join(paths.Originals, media.StorageName)); err != nil || !bytes.Equal(got, video) {
		t.Fatalf("stored video differs: %v", err)
	}

	otherID := "223e4567-e89b-42d3-a456-426614174000"
	photo := append([]byte{0xff, 0xd8, 0xff}, bytes.Repeat([]byte{1}, 30)...)
	if _, err := store.SavePhoto(context.Background(), otherID, int64(len(photo)), bytes.NewReader(photo)); !errors.Is(err, ErrPhotoTooLarge) {
		t.Fatalf("oversized photo error = %v, want ErrPhotoTooLarge", err)
	}
	tooBig := append(ftypHeader("mp42"), bytes.Repeat([]byte{7}, 300)...)
	if _, err := store.SavePhoto(context.Background(), otherID, int64(len(tooBig)), bytes.NewReader(tooBig)); !errors.Is(err, ErrMediaTooLarge) {
		t.Fatalf("oversized media error = %v, want ErrMediaTooLarge", err)
	}
	small, _ := newTestStore(t, Options{MaxBytes: 300, MaxVideoBytes: 100, MaxConcurrent: 1})
	if _, err := small.SavePhoto(context.Background(), otherID, 150, bytes.NewReader(append(ftypHeader("mp42"), bytes.Repeat([]byte{7}, 130)...))); !errors.Is(err, ErrVideoTooLarge) {
		t.Fatalf("video above a smaller video limit error = %v, want ErrVideoTooLarge", err)
	}
	entries, _ := os.ReadDir(paths.Originals)
	if len(entries) != 1 {
		t.Fatalf("rejected uploads left %d originals, want only the first video", len(entries))
	}
	if left, _ := os.ReadDir(paths.Tmp); len(left) != 0 {
		t.Fatalf("rejected uploads left temp files: %v", left)
	}
}
