package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/PMG801/wedding/internal/upload"
)

type photoUploader interface {
	SavePhoto(context.Context, string, int64, io.Reader) (upload.Media, error)
}

func newPhotoUploadHandler(photos photoUploader, maxConcurrent int, maxBytes int64, idleTimeout time.Duration) http.Handler {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	capacity := make(chan struct{}, maxConcurrent)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if photos == nil || maxBytes <= 0 {
			writeUploadError(w, http.StatusServiceUnavailable, "uploads_unavailable")
			return
		}
		id := r.PathValue("id")
		if !upload.ValidID(id) {
			writeUploadError(w, http.StatusBadRequest, "invalid_media_id")
			return
		}
		if r.ContentLength < 0 {
			writeUploadError(w, http.StatusLengthRequired, "content_length_required")
			return
		}
		if r.ContentLength == 0 {
			writeUploadError(w, http.StatusBadRequest, "empty_photo")
			return
		}
		if r.ContentLength > maxBytes {
			writeUploadError(w, http.StatusRequestEntityTooLarge, "photo_too_large")
			return
		}
		select {
		case capacity <- struct{}{}:
			defer func() { <-capacity }()
		default:
			w.Header().Set("Retry-After", "1")
			writeUploadError(w, http.StatusServiceUnavailable, "upload_capacity_reached")
			return
		}

		var body io.Reader = r.Body
		controller := http.NewResponseController(w)
		if idleTimeout > 0 {
			body = &idleTimeoutBody{body: r.Body, controller: controller, timeout: idleTimeout}
			defer func() { _ = controller.SetReadDeadline(time.Time{}) }()
		}
		media, err := photos.SavePhoto(r.Context(), id, r.ContentLength, body)
		if err != nil {
			switch {
			case errors.Is(err, upload.ErrInsufficientStorage):
				writeUploadError(w, http.StatusInsufficientStorage, "insufficient_storage")
			case errors.Is(err, upload.ErrUnsupportedPhoto):
				writeUploadError(w, http.StatusUnsupportedMediaType, "unsupported_photo_format")
			case errors.Is(err, upload.ErrPhotoSizeMismatch), errors.Is(err, io.ErrUnexpectedEOF):
				writeUploadError(w, http.StatusBadRequest, "photo_body_size_mismatch")
			default:
				writeUploadError(w, http.StatusInternalServerError, "upload_failed")
			}
			return
		}
		if media.AlreadyConfirmed {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
}

func writeUploadError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "{\"error\":%s}\n", strconv.Quote(code))
}

type idleTimeoutBody struct {
	body       io.ReadCloser
	controller *http.ResponseController
	timeout    time.Duration
}

func (b *idleTimeoutBody) Close() error { return b.body.Close() }

func (b *idleTimeoutBody) Read(p []byte) (int, error) {
	if err := b.controller.SetReadDeadline(time.Now().Add(b.timeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return 0, err
	}
	return b.body.Read(p)
}
