package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBookedSeasonHandlerMethodAuthAndValidation(t *testing.T) {
	t.Setenv("WEBHOOK_AUTH_KEY", "testkey123")
	h := BookedSeasonHandler(&Config{Env: "dev"})

	tests := []struct {
		name       string
		method     string
		auth       string
		body       string
		wantStatus int
	}{
		{"GET rejected", http.MethodGet, "Bearer testkey123", "", http.StatusMethodNotAllowed},
		{"missing auth rejected", http.MethodPost, "", `{"email":"s@example.com"}`, http.StatusUnauthorized},
		{"wrong auth rejected", http.MethodPost, "Bearer wrong", `{"email":"s@example.com"}`, http.StatusUnauthorized},
		{"invalid JSON rejected", http.MethodPost, "Bearer testkey123", `{`, http.StatusBadRequest},
		{"missing email rejected", http.MethodPost, "Bearer testkey123", `{}`, http.StatusBadRequest},
		{"invalid email rejected", http.MethodPost, "Bearer testkey123", `{"email":"nope"}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/booked-season", strings.NewReader(tt.body))
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
		})
	}
}
