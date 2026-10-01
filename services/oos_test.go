package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOOSEmailBody(t *testing.T) {
	got := oosEmailBody("Stetson")
	want := "Hi Stetson,\r\n\r\nWe are not taking new clients in your neighborhood. Sorry that we cannot help, but thanks for reaching out to us!\r\n\r\nThank You🌲"
	if got != want {
		t.Fatalf("oosEmailBody() = %q, want %q", got, want)
	}
}

func TestOOSHandlerMethodAuthAndValidation(t *testing.T) {
	t.Setenv("WEBHOOK_AUTH_KEY", "testkey123")
	h := OOSHandler(&Config{Env: "dev"})

	tests := []struct {
		name       string
		method     string
		auth       string
		body       string
		wantStatus int
	}{
		{name: "GET rejected", method: http.MethodGet, auth: "Bearer testkey123", wantStatus: http.StatusMethodNotAllowed},
		{name: "missing auth rejected", method: http.MethodPost, body: `{"recordId":"rec123"}`, wantStatus: http.StatusUnauthorized},
		{name: "wrong auth rejected", method: http.MethodPost, auth: "Bearer wrong", body: `{"recordId":"rec123"}`, wantStatus: http.StatusUnauthorized},
		{name: "invalid JSON rejected", method: http.MethodPost, auth: "Bearer testkey123", body: `{`, wantStatus: http.StatusBadRequest},
		{name: "missing record ID rejected", method: http.MethodPost, auth: "Bearer testkey123", body: `{}`, wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/oos/send", strings.NewReader(tt.body))
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
