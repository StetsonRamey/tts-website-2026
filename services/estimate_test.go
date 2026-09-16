package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestEstimateHandlerAuthAndValidation covers the request-shaping guards on
// POST /estimate/send without touching Airtable or SMTP: wrong/missing auth
// must 401, malformed body must 400, empty recordId must 400.
func TestEstimateHandlerAuthAndValidation(t *testing.T) {
	t.Setenv("WEBHOOK_AUTH_KEY", "testkey123")
	// EstimateHandler parses services/email_templates/estimate.html relative to
	// the CWD, which is the repo root under systemd (WorkingDirectory).
	t.Chdir("..") // tests run from services/, so step up to the repo root
	h := EstimateHandler(&Config{Env: "dev"})

	tests := []struct {
		name       string
		auth       string
		body       string
		wantStatus int
	}{
		{name: "missing auth", auth: "", body: `{"recordId":"recAAA111"}`,
			wantStatus: http.StatusUnauthorized},
		{name: "wrong auth", auth: "Bearer nope", body: `{"recordId":"recAAA111"}`,
			wantStatus: http.StatusUnauthorized},
		{name: "malformed json", auth: "Bearer testkey123", body: `{`,
			wantStatus: http.StatusBadRequest},
		{name: "empty recordId", auth: "Bearer testkey123", body: `{"recordId":""}`,
			wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/estimate/send", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
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

// TestEstimateHandlerRejectsGET ensures the endpoint is POST-only.
func TestEstimateHandlerRejectsGET(t *testing.T) {
	t.Setenv("WEBHOOK_AUTH_KEY", "testkey123")
	t.Chdir("..") // tests run from services/, so step up to the repo root
	h := EstimateHandler(&Config{Env: "dev"})
	req := httptest.NewRequest(http.MethodGet, "/estimate/send", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

// TestPhotoFilenameSanity guards the filename builder used to cache photos.
func TestPhotoFilenameSanity(t *testing.T) {
	got := photoFilename([]byte("data"), "my photo.PNG", "Angela Smith")
	wantPrefix := "angela-smith-"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("photoFilename() = %q, want prefix %q", got, wantPrefix)
	}
	if !strings.HasSuffix(got, ".PNG") {
		t.Fatalf("photoFilename() = %q, want original extension preserved", got)
	}
}
