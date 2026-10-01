package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFullAreaEmailBody(t *testing.T) {
	got := fullAreaEmailBody("Stetson")
	want := "Hi Stetson,\r\n\r\nWe are not taking new clients in your neighborhood. Sorry that we cannot help, but thanks for reaching out to us!\r\n\r\nThank You🌲"
	if got != want {
		t.Fatalf("fullAreaEmailBody() = %q, want %q", got, want)
	}
}

func TestFullAreaHandlerMethodAuthAndValidation(t *testing.T) {
	t.Setenv("WEBHOOK_AUTH_KEY", "testkey123")
	h := FullAreaHandler(&Config{Env: "dev"})

	tests := []struct {
		name       string
		method     string
		auth       string
		body       string
		wantStatus int
	}{
		{name: "GET rejected", method: http.MethodGet, auth: "Bearer testkey123", wantStatus: http.StatusMethodNotAllowed},
		{name: "missing auth rejected", method: http.MethodPost, body: `{"firstName":"Stetson","email":"s@example.com"}`, wantStatus: http.StatusUnauthorized},
		{name: "wrong auth rejected", method: http.MethodPost, auth: "Bearer wrong", body: `{"firstName":"Stetson","email":"s@example.com"}`, wantStatus: http.StatusUnauthorized},
		{name: "invalid JSON rejected", method: http.MethodPost, auth: "Bearer testkey123", body: `{`, wantStatus: http.StatusBadRequest},
		{name: "missing input rejected", method: http.MethodPost, auth: "Bearer testkey123", body: `{}`, wantStatus: http.StatusBadRequest},
		{name: "invalid email rejected", method: http.MethodPost, auth: "Bearer testkey123", body: `{"firstName":"Stetson","email":"not-an-email"}`, wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/full", strings.NewReader(tt.body))
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body=%q)", rr.Code, tt.wantStatus, rr.Body.String())
			}
		})
	}
}
