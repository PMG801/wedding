package upload

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/PMG801/wedding/internal/storage"
)

func TestSaveDerivativeRejectsInvalidSignatureAndBodySizesWithoutArtifacts(t *testing.T) {
	jpeg := []byte{0xff, 0xd8, 0xff, 1, 2, 3}
	for _, test := range []struct {
		name string
		size int64
		body []byte
		want error
	}{
		{"invalid signature", 3, []byte("bad"), ErrUnsupportedPhoto},
		{"short body", int64(len(jpeg)), jpeg[:len(jpeg)-1], ErrPhotoSizeMismatch},
		{"long body", int64(len(jpeg) - 1), jpeg, ErrPhotoSizeMismatch},
		{"oversize", int64(len(jpeg) + 1), nil, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, paths := newTestStore(t, Options{MaxBytes: int64(len(jpeg)), MaxConcurrent: 1})
			var body io.Reader = bytes.NewReader(test.body)
			if test.name == "oversize" {
				body = errorReader{}
			}
			_, err := store.SaveDerivative(context.Background(), testID, "t", test.size, body)
			if err == nil {
				t.Fatal("SaveDerivative() succeeded; want rejection")
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("SaveDerivative() error = %v; want %v", err, test.want)
			}
			assertNoDerivativeArtifacts(t, paths)
		})
	}
}

func TestSaveDerivativeHonorsMinimumFreeDisk(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 1, MinFreeBytes: ^uint64(0)})
	jpeg := []byte{0xff, 0xd8, 0xff, 1, 2, 3}
	if _, err := store.SaveDerivative(context.Background(), testID, "t", int64(len(jpeg)), bytes.NewReader(jpeg)); !errors.Is(err, ErrInsufficientStorage) {
		t.Fatalf("SaveDerivative() error = %v; want ErrInsufficientStorage", err)
	}
	assertNoDerivativeArtifacts(t, paths)
}

func TestSaveDerivativeDoesNotRewritePrivateStagedVariant(t *testing.T) {
	store, paths := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 1})
	original := []byte{0xff, 0xd8, 0xff, 1, 2, 3}
	if got, err := store.SaveDerivative(context.Background(), testID, "t", int64(len(original)), bytes.NewReader(original)); err != nil || got.AlreadyConfirmed {
		t.Fatalf("first SaveDerivative() = %+v, %v; want private first write", got, err)
	}

	retry, err := store.SaveDerivative(context.Background(), testID, "t", int64(len(original)), errorReader{})
	if err != nil || !retry.AlreadyConfirmed {
		t.Fatalf("staged retry = %+v, %v; want already confirmed without reading body", retry, err)
	}
	staged, err := os.ReadFile(filepath.Join(paths.Tmp, testID+"_t.staged"))
	if err != nil || !bytes.Equal(staged, original) {
		t.Fatalf("staged derivative changed on retry: %v, %v", staged, err)
	}
}

func TestSaveDerivativeHonorsConcurrentLimit(t *testing.T) {
	store, _ := newTestStore(t, Options{MaxBytes: 1024, MaxConcurrent: 1})
	jpeg := []byte{0xff, 0xd8, 0xff, 1, 2, 3}
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })

	firstBody := &gatedReader{started: make(chan struct{}), release: release, reader: bytes.NewReader(jpeg)}
	firstDone := make(chan error, 1)
	go func() {
		_, err := store.SaveDerivative(context.Background(), testID, "t", int64(len(jpeg)), firstBody)
		firstDone <- err
	}()
	select {
	case <-firstBody.started:
	case <-time.After(time.Second):
		t.Fatal("first derivative did not reach its body reader")
	}

	secondBody := &observedReader{started: make(chan struct{}), reader: bytes.NewReader(jpeg)}
	secondCallStarted := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		close(secondCallStarted)
		_, err := store.SaveDerivative(context.Background(), "223e4567-e89b-42d3-a456-426614174001", "t", int64(len(jpeg)), secondBody)
		secondDone <- err
	}()
	<-secondCallStarted
	select {
	case <-secondBody.started:
		t.Error("second derivative began reading while the first upload held the concurrency slot")
	case <-time.After(100 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(release) })
	if err := <-firstDone; err != nil {
		t.Errorf("first SaveDerivative() = %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Errorf("second SaveDerivative() = %v", err)
	}
}

func assertNoDerivativeArtifacts(t *testing.T, paths storage.Paths) {
	t.Helper()
	for _, directory := range []string{paths.Tmp, paths.Derived} {
		entries, err := os.ReadDir(directory)
		if err != nil || len(entries) != 0 {
			t.Errorf("derivative artifacts in %s = %v, %v; want none", directory, entries, err)
		}
	}
}

type gatedReader struct {
	started chan struct{}
	release <-chan struct{}
	reader  *bytes.Reader
	once    sync.Once
}

func (r *gatedReader) Read(p []byte) (int, error) {
	r.once.Do(func() {
		close(r.started)
		<-r.release
	})
	return r.reader.Read(p)
}

type observedReader struct {
	started chan struct{}
	reader  *bytes.Reader
	once    sync.Once
}

func (r *observedReader) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	return r.reader.Read(p)
}
