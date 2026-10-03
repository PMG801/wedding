package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	tests := []struct {
		name               string
		method             string
		wantStatus         int
		wantContentType    string
		wantBody           string
	}{
		{
			name:            "GET returns health JSON",
			method:          http.MethodGet,
			wantStatus:      http.StatusOK,
			wantContentType: "application/json",
			wantBody:        `{"status":"ok"}`,
		},
		{
			name:       "HEAD is not allowed",
			method:     http.MethodHead,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "POST is not allowed",
			method:     http.MethodPost,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "PUT is not allowed",
			method:     http.MethodPut,
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/health", nil)
			res := httptest.NewRecorder()

			NewHandler().ServeHTTP(res, req)

			if res.Code != tt.wantStatus {
				t.Fatalf("status = %d; want %d", res.Code, tt.wantStatus)
			}
			if tt.wantContentType != "" {
				if got := res.Header().Get("Content-Type"); got != tt.wantContentType {
					t.Errorf("Content-Type = %q; want %q", got, tt.wantContentType)
				}
			}
			if tt.wantBody != "" {
				if got := res.Body.String(); got != tt.wantBody {
					t.Errorf("body = %q; want %q", got, tt.wantBody)
				}
			}
		})
	}
}
