package httpapi

import "net/http"

// NewHandler builds the HTTP handler for public and guest-session routes.
func NewHandler(eventToken string, sessionKey []byte) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /e/{token}", guestEntryHandler(eventToken, sessionKey))
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	// ServeMux treats HEAD as matching GET, so reject it explicitly.
	mux.HandleFunc("HEAD /api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	return mux
}
