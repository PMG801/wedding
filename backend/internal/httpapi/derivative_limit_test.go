package httpapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/upload"
)

func TestDerivativeUploadUsesItsOwnLimitInsteadOfThePhotoLimit(t *testing.T) {
	store := newHTTPPhotoStore(t, upload.Options{MaxBytes: 5, MaxDerivativeBytes: 100, MaxConcurrent: 1})
	handler := NewHandler(testEventToken, []byte(testSessionKey), store, 1, time.Second, 5, nil, WithMaxDerivativeBytes(100))
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	body := append([]byte{0xff, 0xd8, 0xff}, bytes.Repeat([]byte{1}, 47)...)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, derivativeRequest(testMediaID, "t", body, cookie))
	if response.Code != http.StatusCreated {
		t.Fatalf("derivative of 50 bytes above the 5-byte photo limit = %d %q; want 201", response.Code, response.Body.String())
	}
}

func TestDerivativeUploadRejectsDeclaredSizeAboveDerivativeLimitBeforeReadingBody(t *testing.T) {
	store := newHTTPPhotoStore(t, upload.Options{MaxBytes: 1024, MaxDerivativeBytes: 100, MaxConcurrent: 1})
	handler := NewHandler(testEventToken, []byte(testSessionKey), store, 1, time.Second, 1024, nil, WithMaxDerivativeBytes(100))
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	body := append([]byte{0xff, 0xd8, 0xff}, bytes.Repeat([]byte{1}, 97)...)

	for _, test := range []struct {
		name   string
		id     string
		length int
		want   int
	}{
		{"exactly at the limit", testMediaID, 100, http.StatusCreated},
		{"one byte above the limit", "223e4567-e89b-42d3-a456-426614174001", 101, http.StatusRequestEntityTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := body
			if test.length == 101 {
				payload = append(append([]byte{}, body...), 1)
			}
			request := derivativeRequest(test.id, "d", payload, cookie)
			counter := &countingBody{reader: bytes.NewReader(payload)}
			request.Body = counter
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("declared %d bytes = %d %q; want %d", test.length, response.Code, response.Body.String(), test.want)
			}
			if test.want == http.StatusRequestEntityTooLarge {
				if response.Body.String() != "{\"error\":\"derivative_too_large\"}\n" {
					t.Errorf("error body = %q; want derivative_too_large", response.Body.String())
				}
				if counter.reads != 0 {
					t.Errorf("oversized derivative read the body %d times; want 0", counter.reads)
				}
			}
		})
	}
}

func TestDerivativeUploadMapsStoreDerivativeTooLargeToTheSameResponse(t *testing.T) {
	handler := NewHandler(testEventToken, []byte(testSessionKey), derivativeTooLargeUploader{}, 1, time.Second, 1024, nil, WithMaxDerivativeBytes(100))
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, derivativeRequest(testMediaID, "t", []byte{0xff, 0xd8, 0xff, 1}, cookie))
	if response.Code != http.StatusRequestEntityTooLarge || response.Body.String() != "{\"error\":\"derivative_too_large\"}\n" {
		t.Fatalf("store ErrDerivativeTooLarge = %d %q; want 413 derivative_too_large", response.Code, response.Body.String())
	}
}

type derivativeTooLargeUploader struct{}

func (derivativeTooLargeUploader) SavePhoto(context.Context, string, int64, io.Reader) (upload.Media, error) {
	return upload.Media{}, nil
}

func (derivativeTooLargeUploader) SaveDerivative(context.Context, string, string, int64, io.Reader) (upload.Derivative, error) {
	return upload.Derivative{}, upload.ErrDerivativeTooLarge
}

func TestOversizedDerivativeIsRefusedBeforeCapacityIsChecked(t *testing.T) {
	photos := &blockingPhotoDerivativeUploader{started: make(chan struct{}), release: make(chan struct{})}
	handler := NewHandler(testEventToken, []byte(testSessionKey), photos, 1, time.Second, 1024, nil, WithMaxDerivativeBytes(100))
	cookie := auth.NewGuestCookie([]byte(testSessionKey), time.Now())
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		handler.ServeHTTP(httptest.NewRecorder(), photoRequest(testMediaID, []byte("raw"), cookie))
	}()
	select {
	case <-photos.started:
	case <-time.After(time.Second):
		close(photos.release)
		t.Fatal("original upload did not occupy the only capacity slot")
	}

	body := append([]byte{0xff, 0xd8, 0xff}, bytes.Repeat([]byte{1}, 98)...)
	request := derivativeRequest("223e4567-e89b-42d3-a456-426614174002", "t", body, cookie)
	counter := &countingBody{reader: bytes.NewReader(body)}
	request.Body = counter
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	close(photos.release)
	<-firstDone

	if response.Code != http.StatusRequestEntityTooLarge || response.Body.String() != "{\"error\":\"derivative_too_large\"}\n" {
		t.Fatalf("oversized derivative with saturated capacity = %d %q; want 413 before the capacity check", response.Code, response.Body.String())
	}
	if counter.reads != 0 {
		t.Errorf("oversized derivative read the body %d times; want 0", counter.reads)
	}
}
