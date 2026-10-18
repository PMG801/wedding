package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/PMG801/wedding/internal/auth"
	"github.com/PMG801/wedding/internal/upload"
)

// maxAdminBodyBytes bounds the JSON bodies of the administrator endpoints.
const maxAdminBodyBytes = 4 << 10

// uploadSwitch is the manual upload switch; *settings.Store implements it.
type uploadSwitch interface {
	UploadsEnabled() bool
	SetUploadsEnabled(context.Context, bool) error
}

// diskUsageReporter measures the data volume; *upload.Store implements it.
type diskUsageReporter interface {
	DiskUsage() (upload.DiskUsage, error)
}

type adminOptions struct {
	passwordHash string
	switches     uploadSwitch
	disk         diskUsageReporter
}

// WithAdmin enables the administrator routes with the bcrypt or argon2id password hash,
// the manual upload switch and the disk usage source.
func WithAdmin(passwordHash string, switches uploadSwitch, disk diskUsageReporter) Option {
	return func(o *handlerOptions) {
		o.admin = &adminOptions{passwordHash: passwordHash, switches: switches, disk: disk}
	}
}

// WithLoginThrottle replaces the default administrator login throttle, letting tests inject a clock.
func WithLoginThrottle(throttle *auth.LoginThrottle) Option {
	return func(o *handlerOptions) { o.throttle = throttle }
}

// registerAdminRoutes mounts /api/admin/*. Every response carries Cache-Control: no-store.
func registerAdminRoutes(mux *http.ServeMux, sessionKey []byte, options handlerOptions) {
	throttle := options.throttle
	if throttle == nil {
		throttle = auth.NewLoginThrottle(auth.ThrottleOptions{})
	}
	var passwordHash string
	var switches uploadSwitch
	var disk diskUsageReporter
	if options.admin != nil {
		passwordHash, switches, disk = options.admin.passwordHash, options.admin.switches, options.admin.disk
	}

	mux.Handle("POST /api/admin/login", noStore(adminLoginHandler(sessionKey, passwordHash, throttle)))
	registerModerationRoutes(mux, sessionKey, options.moderator)
	registerHiddenMediaRoutes(mux, sessionKey, options.hiddenMedia)
	mux.Handle("POST /api/admin/logout", noStore(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, auth.ExpiredAdminCookie())
		w.WriteHeader(http.StatusNoContent)
	})))

	if switches == nil || disk == nil {
		unavailable := noStore(auth.RequireAdminSession(sessionKey, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeUploadError(w, http.StatusServiceUnavailable, "admin_unavailable")
		})))
		mux.Handle("GET /api/admin/status", unavailable)
		mux.Handle("POST /api/admin/settings", unavailable)
		return
	}
	mux.Handle("GET /api/admin/status", noStore(auth.RequireAdminSession(sessionKey, adminStatusHandler(switches, disk))))
	mux.Handle("POST /api/admin/settings", noStore(auth.RequireAdminSession(sessionKey, adminSettingsHandler(switches, disk))))
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func adminLoginHandler(sessionKey []byte, passwordHash string, throttle *auth.LoginThrottle) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Password string `json:"password"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxAdminBodyBytes)
		if err := decodeStrictJSON(r, &body); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeUploadError(w, http.StatusRequestEntityTooLarge, "request_too_large")
				return
			}
			writeUploadError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		ok, err := throttle.Attempt(r.Context(), func() bool { return auth.VerifyAdminPassword(passwordHash, body.Password) })
		if err != nil {
			return
		}
		if !ok {
			writeUploadError(w, http.StatusUnauthorized, "invalid_credentials")
			return
		}
		http.SetCookie(w, auth.NewAdminCookie(sessionKey, time.Now()))
		w.WriteHeader(http.StatusNoContent)
	})
}

type adminStatus struct {
	UploadsEnabled bool   `json:"uploadsEnabled"`
	DiskPaused     bool   `json:"diskPaused"`
	FreeBytes      uint64 `json:"freeBytes"`
	ReservedBytes  uint64 `json:"reservedBytes"`
	MinFreeBytes   uint64 `json:"minFreeBytes"`
}

func currentAdminStatus(switches uploadSwitch, disk diskUsageReporter) (adminStatus, error) {
	usage, err := disk.DiskUsage()
	if err != nil {
		return adminStatus{}, err
	}
	// Reservations can exceed free space while uploads are in flight; that also counts as paused.
	paused := usage.FreeBytes < usage.ReservedBytes || usage.FreeBytes-usage.ReservedBytes < usage.MinFreeBytes
	return adminStatus{
		UploadsEnabled: switches.UploadsEnabled(),
		DiskPaused:     paused,
		FreeBytes:      usage.FreeBytes,
		ReservedBytes:  usage.ReservedBytes,
		MinFreeBytes:   usage.MinFreeBytes,
	}, nil
}

func adminStatusHandler(switches uploadSwitch, disk diskUsageReporter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		status, err := currentAdminStatus(switches, disk)
		if err != nil {
			writeUploadError(w, http.StatusServiceUnavailable, "status_unavailable")
			return
		}
		writeJSON(w, http.StatusOK, status)
	})
}

func adminSettingsHandler(switches uploadSwitch, disk diskUsageReporter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			UploadsEnabled *bool `json:"uploadsEnabled"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxAdminBodyBytes)
		if err := decodeStrictJSON(r, &body); err != nil || body.UploadsEnabled == nil {
			writeUploadError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		if err := switches.SetUploadsEnabled(r.Context(), *body.UploadsEnabled); err != nil {
			writeUploadError(w, http.StatusInternalServerError, "settings_failed")
			return
		}
		status, err := currentAdminStatus(switches, disk)
		if err != nil {
			writeUploadError(w, http.StatusServiceUnavailable, "status_unavailable")
			return
		}
		writeJSON(w, http.StatusOK, status)
	})
}

// uploadsSwitchGate refuses uploads while the manual switch is off. It runs before
// any upload slot is taken or request body is read. Reads are never gated.
func uploadsSwitchGate(switches uploadSwitch, next http.Handler) http.Handler {
	if switches == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !switches.UploadsEnabled() {
			writeUploadError(w, http.StatusServiceUnavailable, "uploads_paused")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func decodeStrictJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
