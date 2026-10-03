package services

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestWriteOOSSuccessResponse(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeOOSSuccessResponse(recorder, "lead@example.com")

	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Recipient string `json:"recipient"`
			Subject   string `json:"subject"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success {
		t.Fatal("response success = false, want true")
	}
	if response.Data.Recipient != "lead@example.com" {
		t.Errorf("recipient = %q, want lead@example.com", response.Data.Recipient)
	}
	if response.Data.Subject != oosEmailSubject {
		t.Errorf("subject = %q, want %q", response.Data.Subject, oosEmailSubject)
	}
}
