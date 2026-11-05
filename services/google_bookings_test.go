package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const bookingTestRecord = "recTestBooking001"

type bookingFixture struct {
	u           *GoogleBookingUploader
	now         time.Time
	status      string
	patches     []map[string]interface{}
	posts       []map[string]interface{}
	googleState string
	googleError string
	nextHTTP    int
	patchError  bool
}

func newBookingFixture(t *testing.T) *bookingFixture {
	t.Helper()
	f := &bookingFixture{now: time.Date(2026, 11, 4, 19, 0, 0, 0, time.UTC), status: "Sold", googleState: "SUCCESS"}
	dir := t.TempDir()
	t.Setenv("WEBHOOK_AUTH_KEY", "test-auth")
	t.Setenv("GOOGLE_ADS_BOOKING_ENABLED", "false")
	t.Setenv("GOOGLE_ADS_BOOKING_STATE_FILE", filepath.Join(dir, "outbox.json"))
	t.Setenv("GOOGLE_ADS_BOOKING_CREDENTIALS_FILE", filepath.Join(dir, "credentials.json"))
	if err := os.WriteFile(filepath.Join(dir, "credentials.json"), []byte(`{"client_id":"test-client","client_secret":"test-secret","refresh_token":"test-refresh"}`), 0600); err != nil {
		t.Fatal(err)
	}
	f.u = NewGoogleBookingUploader()
	f.u.now = func() time.Time { return f.now }
	f.u.fetch = func(id string) (atRecord, error) {
		return atRecord{ID: id, Fields: map[string]interface{}{
			fieldLeadStatus: f.status, bookingDateField: "2026-11-03T20:00:00.000Z",
			bookingTrackingField: "UTM source: google\nGoogle click ID: retained-opaque-click\nGoogle iOS click ID: \n",
		}}, nil
	}
	f.u.patch = func(id string, fields map[string]interface{}) error {
		if f.patchError {
			return fmt.Errorf("mock status write failure")
		}
		if id != bookingTestRecord {
			t.Fatalf("unexpected record %s", id)
		}
		if len(fields) != 2 || fields[bookingStateField] == nil || fields[bookingDetailsField] == nil {
			t.Fatal("unrelated CRM fields modified")
		}
		f.patches = append(f.patches, fields)
		return nil
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_ = r.ParseForm()
			if r.Form.Get("refresh_token") != "test-refresh" {
				t.Error("wrong refresh token")
			}
			fmt.Fprint(w, `{"access_token":"test-access","expires_in":3600}`)
		case "/events:ingest":
			if r.Header.Get("Authorization") != "Bearer test-access" {
				t.Error("missing Google auth")
			}
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			f.posts = append(f.posts, payload)
			if f.nextHTTP != 0 {
				w.WriteHeader(f.nextHTTP)
				f.nextHTTP = 0
				fmt.Fprint(w, `{"error":{}}`)
				return
			}
			fmt.Fprintf(w, `{"requestId":"test-request-%d"}`, len(f.posts))
		case "/requestStatus:retrieve":
			if r.Method != http.MethodGet || r.URL.Query().Get("requestId") == "" {
				t.Error("wrong diagnostics lookup")
			}
			errorJSON := ""
			if f.googleError != "" {
				errorJSON = fmt.Sprintf(`,"errorInfo":{"errorCounts":[{"recordCount":"1","reason":%q}]}`, f.googleError)
			}
			fmt.Fprintf(w, `{"requestStatusPerDestination":[{"destination":{"productDestinationId":%q},"requestStatus":%q,"eventsIngestionStatus":{"recordCount":"1"}%s}]}`, bookingAction, f.googleState, errorJSON)
		default:
			t.Errorf("unexpected outbound path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	f.u.apiBase = server.URL
	f.u.tokenURL = server.URL + "/token"
	f.u.client = server.Client()
	return f
}

func (f *bookingFixture) callback(t *testing.T, body string, expected int) {
	t.Helper()
	req := httptest.NewRequest("POST", "/ads/google/booked", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-auth")
	w := httptest.NewRecorder()
	f.u.Handler()(w, req)
	if w.Code != expected {
		t.Fatalf("callback %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "retained-opaque-click") {
		t.Fatal("callback leaked click identifier")
	}
}

func TestGoogleBookingAuthAndReadOnlyCheck(t *testing.T) {
	f := newBookingFixture(t)
	w := httptest.NewRecorder()
	f.u.Handler()(w, httptest.NewRequest("POST", "/ads/google/booked", strings.NewReader(`{}`)))
	if w.Code != 401 {
		t.Fatal("unauthenticated callback accepted")
	}
	f.callback(t, `{"recordId":"recTestBooking001","checkOnly":true}`, 200)
	if len(f.u.jobs) != 0 || len(f.posts) != 0 || len(f.patches) != 0 {
		t.Fatal("read-only check created side effects")
	}
}

func TestGoogleBookingQueuePausedDurableAndDuplicateSafe(t *testing.T) {
	f := newBookingFixture(t)
	f.callback(t, `{"recordId":"recTestBooking001"}`, 202)
	f.callback(t, `{"recordId":"recTestBooking001"}`, 202)
	f.u.runDue()
	if len(f.u.jobs) != 1 || len(f.posts) != 0 || len(f.patches) != 1 {
		t.Fatal("paused queue changed Google or duplicated work")
	}
	reloaded := NewGoogleBookingUploader()
	if reloaded.loadErr != nil || len(reloaded.jobs) != 1 {
		t.Fatal("outbox did not survive reload")
	}
	job := reloaded.jobs[bookingTestRecord]
	if job.Event.TransactionID != "tts-booked-2026-"+bookingTestRecord || job.Event.EventTimestamp != "2026-11-03T20:00:00Z" {
		t.Fatal("incorrect booking identity or date")
	}
	info, err := os.Stat(f.u.stateFile)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("outbox not private")
	}
}

func TestGoogleBookingWaitsForFinalDiagnosticsAndDoesNotResend(t *testing.T) {
	f := newBookingFixture(t)
	f.u.enabled = true
	f.callback(t, `{"recordId":"recTestBooking001"}`, 202)
	f.u.runDue()
	if f.u.jobs[bookingTestRecord].State != "Processing" {
		t.Fatal("HTTP acceptance falsely marked final")
	}
	if len(f.posts) != 1 {
		t.Fatal("booking not submitted")
	}
	payload := f.posts[0]
	if payload["validateOnly"] != false || len(payload["events"].([]interface{})) != 1 {
		t.Fatal("wrong ingestion request")
	}
	event := payload["events"].([]interface{})[0].(map[string]interface{})
	if len(event) != 5 || event["conversionValue"] != nil || event["userData"] != nil {
		t.Fatal("unexpected customer data or invented value")
	}
	f.now = f.now.Add(10 * time.Minute)
	f.u.runDue()
	if len(f.posts) != 1 {
		t.Fatal("resent before diagnostics")
	}
	f.now = f.now.Add(21 * time.Minute)
	f.u.runDue()
	if f.u.jobs[bookingTestRecord].State != "Processed" {
		t.Fatal("final Google result not recorded")
	}
	f.callback(t, `{"recordId":"recTestBooking001"}`, 202)
	f.now = f.now.Add(time.Hour)
	f.u.runDue()
	if len(f.posts) != 1 {
		t.Fatal("processed booking resent")
	}
}

func TestGoogleBookingRetryKeepsIdenticalEvent(t *testing.T) {
	f := newBookingFixture(t)
	f.u.enabled = true
	f.nextHTTP = 503
	f.callback(t, `{"recordId":"recTestBooking001"}`, 202)
	f.u.runDue()
	if f.u.jobs[bookingTestRecord].State != "Queued" {
		t.Fatal("transient error treated as final")
	}
	f.now = f.now.Add(16 * time.Minute)
	f.u.runDue()
	a, _ := json.Marshal(f.posts[0])
	b, _ := json.Marshal(f.posts[1])
	if string(a) != string(b) {
		t.Fatal("retry changed the booking event")
	}
}

func TestGoogleBookingGoogleFailureIsVisibleAndNotProcessed(t *testing.T) {
	f := newBookingFixture(t)
	f.u.enabled = true
	f.googleState = "FAILED"
	f.googleError = "PROCESSING_ERROR_REASON_INVALID_GCLID"
	f.callback(t, `{"recordId":"recTestBooking001"}`, 202)
	f.u.runDue()
	f.now = f.now.Add(31 * time.Minute)
	f.u.runDue()
	job := f.u.jobs[bookingTestRecord]
	if job.State != "Failed" || job.Reason != f.googleError {
		t.Fatal("Google rejection not recorded")
	}
	f.now = f.now.Add(time.Hour)
	f.u.runDue()
	if len(f.posts) != 1 {
		t.Fatal("permanent rejection automatically resent")
	}
}

func TestGoogleBookingTooRecentActionWaitsBeforeRetry(t *testing.T) {
	f := newBookingFixture(t)
	f.u.enabled = true
	f.googleState = "FAILED"
	f.googleError = "PROCESSING_ERROR_REASON_DESTINATION_TOO_RECENTLY_CREATED"
	f.callback(t, `{"recordId":"recTestBooking001"}`, 202)
	f.u.runDue()
	f.now = f.now.Add(31 * time.Minute)
	f.u.runDue()
	job := f.u.jobs[bookingTestRecord]
	if job.State != "Queued" || !job.NextCheck.Equal(f.now.Add(6*time.Hour)) {
		t.Fatal("new-action failure not delayed")
	}
	f.now = f.now.Add(time.Hour)
	f.u.runDue()
	if len(f.posts) != 1 {
		t.Fatal("new-action retry sent too early")
	}
}

func TestGoogleBookingCancellationAndMissingIdentifiers(t *testing.T) {
	f := newBookingFixture(t)
	f.u.enabled = true
	f.callback(t, `{"recordId":"recTestBooking001"}`, 202)
	f.status = "Dead"
	f.u.runDue()
	if f.u.jobs[bookingTestRecord].State != "Cancelled" || len(f.posts) != 0 {
		t.Fatal("non-sold booking sent")
	}
	r, _ := f.u.fetch(bookingTestRecord)
	r.Fields[fieldLeadStatus] = "Sold"
	r.Fields[bookingTrackingField] = "Google iOS click ID: opaque-braid"
	if _, skip, err := eventForBooking(r, f.now); err != nil || skip != "no_google_click_id" {
		t.Fatal("iOS identifier incorrectly used as GCLID")
	}
	r.Fields[bookingTrackingField] = "Google click ID: opaque"
	delete(r.Fields, bookingDateField)
	if _, _, err := eventForBooking(r, f.now); err == nil {
		t.Fatal("missing booking date invented")
	}
}

func TestGoogleBookingStatusWriteRetryDoesNotResend(t *testing.T) {
	f := newBookingFixture(t)
	f.u.enabled = true
	f.patchError = true
	f.callback(t, `{"recordId":"recTestBooking001"}`, 202)
	f.u.runDue()
	f.patchError = false
	f.now = f.now.Add(time.Minute)
	f.u.runDue()
	if len(f.posts) != 1 || len(f.patches) != 1 || f.u.jobs[bookingTestRecord].NeedsSync {
		t.Fatal("CRM write retry resent Google or did not recover")
	}
}

func TestGoogleBookingWorkerCanStop(t *testing.T) {
	f := newBookingFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.u.Start(ctx)
}

func TestBookingFetchUsesFilteredListQuery(t *testing.T) {
	var got *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		fmt.Fprint(w, `{"records":[{"id":"`+bookingTestRecord+`","fields":{}}]}`)
	}))
	defer server.Close()
	oldBase := airtableBase
	airtableBase = server.URL
	defer func() { airtableBase = oldBase }()
	f := newBookingFixture(t)
	u := NewGoogleBookingUploader() // restore the real fetch function
	_ = f
	record, err := u.fetch(bookingTestRecord)
	if err != nil || record.ID != bookingTestRecord {
		t.Fatalf("fetch failed: %v", err)
	}
	if strings.HasSuffix(got.URL.Path, bookingTestRecord) || got.URL.Query().Get("filterByFormula") != `RECORD_ID()="`+bookingTestRecord+`"` {
		t.Fatalf("expected filtered list query, got %s", got.URL.String())
	}
	if fields := got.URL.Query()["fields[]"]; len(fields) != 3 {
		t.Fatalf("expected only 3 fields, got %v", fields)
	}
}
