package services

// Google booking feedback is separate from sold sync, invoices and messages.
// Airtable enqueues an immutable booking. A durable outbox submits it once and
// checks Google's asynchronous diagnostics before marking it Processed.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	bookingCustomer      = "8159232231"
	bookingAction        = "7883540386"
	bookingDateField     = "fldI4XfUgt8DxXzOl"
	bookingTrackingField = "fldqlM7utGqqVnVa7"
	bookingStateField    = "fldy9hhY3H0e3Ti9c"
	bookingDetailsField  = "fldDLczv541bIHAiH"
)

var bookingRecordID = regexp.MustCompile(`^rec[A-Za-z0-9]{14}$`)

type bookingEvent struct {
	AdIdentifiers struct {
		GCLID string `json:"gclid"`
	} `json:"adIdentifiers"`
	EventTimestamp        string   `json:"eventTimestamp"`
	TransactionID         string   `json:"transactionId"`
	EventSource           string   `json:"eventSource"`
	DestinationReferences []string `json:"destinationReferences"`
}

type bookingJob struct {
	RecordID  string       `json:"recordId"`
	Event     bookingEvent `json:"event"`
	State     string       `json:"state"`
	RequestID string       `json:"requestId,omitempty"`
	Attempts  int          `json:"attempts"`
	NextCheck time.Time    `json:"nextCheck"`
	UpdatedAt time.Time    `json:"updatedAt"`
	Reason    string       `json:"reason,omitempty"`
	Warnings  []string     `json:"warnings,omitempty"`
	NeedsSync bool         `json:"needsSync"`
}

type bookingCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
}

// GoogleBookingUploader owns one process's durable outbox. It never scans or
// backfills CRM records; only explicit authenticated callbacks create jobs.
type GoogleBookingUploader struct {
	mu                         sync.Mutex
	runMu                      sync.Mutex
	jobs                       map[string]bookingJob
	stateFile, credentialsFile string
	enabled                    bool
	loadErr                    error
	client                     *http.Client
	apiBase, tokenURL          string
	token                      string
	tokenExpiry                time.Time
	fetch                      func(string) (atRecord, error)
	patch                      func(string, map[string]interface{}) error
	now                        func() time.Time
}

func NewGoogleBookingUploader() *GoogleBookingUploader {
	u := &GoogleBookingUploader{
		jobs:            make(map[string]bookingJob),
		stateFile:       getEnvOr("GOOGLE_ADS_BOOKING_STATE_FILE", "/var/lib/tts/google-bookings.json"),
		credentialsFile: os.Getenv("GOOGLE_ADS_BOOKING_CREDENTIALS_FILE"),
		enabled:         os.Getenv("GOOGLE_ADS_BOOKING_ENABLED") == "true",
		client:          &http.Client{Timeout: 45 * time.Second},
		apiBase:         "https://datamanager.googleapis.com/v1",
		tokenURL:        "https://oauth2.googleapis.com/token",
		patch:           PatchLeadFields, now: time.Now,
	}
	u.fetch = func(id string) (atRecord, error) {
		params := atParams()
		for _, f := range []string{fieldLeadStatus, bookingDateField, bookingTrackingField} {
			params.Add("fields[]", f)
		}
		// The single-record endpoint ignores field filters, so use a one-row list
		// query to avoid fetching customer contact/payment fields.
		params.Set("maxRecords", "1")
		params.Set("filterByFormula", `RECORD_ID()="`+id+`"`)
		req, _ := http.NewRequest(http.MethodGet, leadsURL()+"?"+params.Encode(), nil)
		response, err := u.client.Do(req)
		if err != nil {
			return atRecord{}, errors.New("airtable_read_failed")
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return atRecord{}, fmt.Errorf("airtable_read_http_%d", response.StatusCode)
		}
		var list atListResponse
		if err := json.NewDecoder(response.Body).Decode(&list); err != nil {
			return atRecord{}, errors.New("airtable_read_invalid_response")
		}
		if len(list.Records) != 1 {
			return atRecord{}, errors.New("airtable_record_not_found")
		}
		return list.Records[0], nil
	}
	data, err := os.ReadFile(u.stateFile)
	if err == nil {
		if err = json.Unmarshal(data, &u.jobs); err != nil || u.jobs == nil {
			u.loadErr = errors.New("outbox_invalid")
		}
	} else if !os.IsNotExist(err) {
		u.loadErr = errors.New("outbox_unreadable")
	}
	if u.loadErr != nil {
		log.Printf("[google-booking] disabled: %s", u.loadErr)
	}
	return u
}

func eventForBooking(record atRecord, now time.Time) (bookingEvent, string, error) {
	var event bookingEvent
	if str(record.Fields[fieldLeadStatus]) != "Sold" {
		return event, "not_sold", nil
	}
	metrics := str(record.Fields[bookingTrackingField])
	for _, line := range strings.Split(metrics, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(key), "Google click ID") {
			event.AdIdentifiers.GCLID = strings.TrimSpace(value)
			break
		}
	}
	if event.AdIdentifiers.GCLID == "" || event.AdIdentifiers.GCLID == "-" {
		return event, "no_google_click_id", nil
	}
	booked, err := time.Parse(time.RFC3339Nano, str(record.Fields[bookingDateField]))
	if err != nil {
		return event, "", errors.New("booked_at_missing_or_invalid")
	}
	if booked.After(now.Add(time.Minute)) || booked.Before(now.AddDate(0, 0, -90)) {
		return event, "", errors.New("booked_at_outside_upload_window")
	}
	event.EventTimestamp = booked.UTC().Format(time.RFC3339Nano)
	event.TransactionID = "tts-booked-2026-" + record.ID
	event.EventSource = "WEB"
	event.DestinationReferences = []string{"tts-booked"}
	return event, "", nil
}

func (u *GoogleBookingUploader) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(u.stateFile), 0700); err != nil {
		return errors.New("outbox_directory_failed")
	}
	data, err := json.MarshalIndent(u.jobs, "", "  ")
	if err != nil {
		return errors.New("outbox_encode_failed")
	}
	file, err := os.CreateTemp(filepath.Dir(u.stateFile), ".google-bookings-*")
	if err != nil {
		return errors.New("outbox_write_failed")
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return errors.New("outbox_write_failed")
	}
	if err = os.Rename(file.Name(), u.stateFile); err != nil {
		return errors.New("outbox_commit_failed")
	}
	return nil
}

func (u *GoogleBookingUploader) view(job bookingJob) map[string]interface{} {
	return map[string]interface{}{"recordId": job.RecordID, "state": job.State, "requestId": job.RequestID,
		"bookingDate": job.Event.EventTimestamp, "transactionId": job.Event.TransactionID,
		"reason": job.Reason, "warnings": job.Warnings, "sendingEnabled": u.enabled}
}

func bookingJSON(w http.ResponseWriter, code int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}

func (u *GoogleBookingUploader) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !RequireBearerAuth(w, r) {
			return
		}
		u.mu.Lock()
		unavailable := u.loadErr != nil
		u.mu.Unlock()
		if unavailable {
			bookingJSON(w, 503, map[string]string{"error": "outbox_unavailable"})
			return
		}
		if r.Method == http.MethodGet {
			u.mu.Lock()
			job, found := u.jobs[r.URL.Query().Get("recordId")]
			u.mu.Unlock()
			if !found {
				bookingJSON(w, 404, map[string]string{"error": "booking_not_queued"})
				return
			}
			bookingJSON(w, 200, u.view(job))
			return
		}
		if r.Method != http.MethodPost {
			bookingJSON(w, 405, map[string]string{"error": "method_not_allowed"})
			return
		}
		var request struct {
			RecordID  string `json:"recordId"`
			CheckOnly bool   `json:"checkOnly"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil || !bookingRecordID.MatchString(request.RecordID) {
			bookingJSON(w, 400, map[string]string{"error": "valid_recordId_required"})
			return
		}
		record, err := u.fetch(request.RecordID)
		if err != nil || record.ID != request.RecordID {
			bookingJSON(w, 502, map[string]string{"error": "airtable_read_failed"})
			return
		}
		event, skip, err := eventForBooking(record, u.now())
		if err != nil {
			bookingJSON(w, 422, map[string]string{"error": err.Error()})
			return
		}
		if skip != "" {
			bookingJSON(w, 200, map[string]string{"state": "Skipped", "reason": skip})
			return
		}
		if request.CheckOnly {
			bookingJSON(w, 200, map[string]interface{}{"state": "Ready", "bookingDate": event.EventTimestamp,
				"transactionId": event.TransactionID, "sendingEnabled": u.enabled, "checkOnly": true})
			return
		}
		u.mu.Lock()
		job, exists := u.jobs[request.RecordID]
		if !exists {
			job = bookingJob{RecordID: request.RecordID, Event: event, State: "Queued", NextCheck: u.now(), UpdatedAt: u.now(), NeedsSync: true}
			u.jobs[request.RecordID] = job
			if err := u.saveLocked(); err != nil {
				delete(u.jobs, request.RecordID)
				u.mu.Unlock()
				bookingJSON(w, 503, map[string]string{"error": "outbox_write_failed"})
				return
			}
		}
		u.mu.Unlock()
		// The worker writes CRM delivery status; the callback never waits on Google.
		bookingJSON(w, 202, u.view(job))
	}
}

func (u *GoogleBookingUploader) Start(ctx context.Context) {
	if u.loadErr != nil {
		return
	}
	log.Printf("[google-booking] callback ready; Google sending enabled=%t", u.enabled)
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			u.runDue()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (u *GoogleBookingUploader) runDue() {
	u.runMu.Lock()
	defer u.runMu.Unlock()
	u.mu.Lock()
	if u.loadErr != nil {
		u.mu.Unlock()
		return
	}
	jobs := make([]bookingJob, 0, len(u.jobs))
	for _, job := range u.jobs {
		jobs = append(jobs, job)
	}
	u.mu.Unlock()
	for _, job := range jobs {
		if u.enabled && !job.NextCheck.After(u.now()) && (job.State == "Queued" || job.State == "Processing") {
			u.process(&job)
			job.UpdatedAt = u.now()
			job.NeedsSync = true
			u.mu.Lock()
			u.jobs[job.RecordID] = job
			err := u.saveLocked()
			u.mu.Unlock()
			if err != nil {
				u.mu.Lock()
				u.loadErr = err
				u.mu.Unlock()
				log.Printf("[google-booking] stopped: outbox persistence failed")
				return
			}
		}
		if job.NeedsSync {
			details := "Updated: " + job.UpdatedAt.UTC().Format(time.RFC3339)
			if !u.enabled {
				details += "\nGoogle sending is paused."
			}
			if job.RequestID != "" {
				details += "\nGoogle request: " + job.RequestID
			}
			if job.Reason != "" {
				details += "\nReason: " + job.Reason
			}
			if len(job.Warnings) > 0 {
				details += "\nWarnings: " + strings.Join(job.Warnings, ", ")
			}
			if err := u.patch(job.RecordID, map[string]interface{}{bookingStateField: job.State, bookingDetailsField: details}); err != nil {
				log.Printf("[google-booking] %s: delivery status write pending", job.RecordID)
				continue
			}
			job.NeedsSync = false
			u.mu.Lock()
			u.jobs[job.RecordID] = job
			err := u.saveLocked()
			u.mu.Unlock()
			if err != nil {
				u.mu.Lock()
				u.loadErr = err
				u.mu.Unlock()
				log.Printf("[google-booking] stopped: outbox persistence failed")
				return
			}
		}
	}
}

func (u *GoogleBookingUploader) googleRequest(method, path string, payload interface{}) (int, []byte, error) {
	if u.token == "" || !u.tokenExpiry.After(u.now().Add(time.Minute)) {
		data, err := os.ReadFile(u.credentialsFile)
		if err != nil {
			return 0, nil, errors.New("google_credentials_unavailable")
		}
		var credentials bookingCredentials
		if json.Unmarshal(data, &credentials) != nil || credentials.RefreshToken == "" || credentials.ClientID == "" || credentials.ClientSecret == "" {
			return 0, nil, errors.New("google_credentials_invalid")
		}
		form := url.Values{"grant_type": {"refresh_token"}, "client_id": {credentials.ClientID}, "client_secret": {credentials.ClientSecret}, "refresh_token": {credentials.RefreshToken}}
		response, err := u.client.PostForm(u.tokenURL, form)
		if err != nil {
			return 0, nil, errors.New("google_token_network_error")
		}
		var token struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
		}
		decodeErr := json.NewDecoder(response.Body).Decode(&token)
		response.Body.Close()
		if response.StatusCode != 200 || decodeErr != nil || token.AccessToken == "" {
			return 0, nil, errors.New("google_token_refresh_failed")
		}
		u.token = token.AccessToken
		u.tokenExpiry = u.now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}
	var body io.Reader
	if payload != nil {
		data, _ := json.Marshal(payload)
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequest(method, u.apiBase+path, body)
	if err != nil {
		return 0, nil, errors.New("google_request_invalid")
	}
	request.Header.Set("Authorization", "Bearer "+u.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := u.client.Do(request)
	if err != nil {
		return 0, nil, errors.New("google_network_error")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024))
	if err != nil {
		return 0, nil, errors.New("google_response_read_error")
	}
	return response.StatusCode, data, nil
}

func (u *GoogleBookingUploader) retry(job *bookingJob, reason string) {
	job.Reason = reason
	job.NextCheck = u.now().Add(15 * time.Minute)
	// The original event and transaction ID never change across retries.
}

func (u *GoogleBookingUploader) process(job *bookingJob) {
	if job.State == "Processing" {
		u.checkGoogle(job)
		return
	}
	record, err := u.fetch(job.RecordID)
	if err != nil {
		u.retry(job, "airtable_read_failed")
		return
	}
	if str(record.Fields[fieldLeadStatus]) != "Sold" {
		job.State = "Cancelled"
		job.Reason = "no_longer_sold_before_upload"
		return
	}
	booked, err := time.Parse(time.RFC3339Nano, job.Event.EventTimestamp)
	if err != nil || booked.Before(u.now().AddDate(0, 0, -90)) {
		job.State = "Failed"
		job.Reason = "booking_outside_upload_window"
		return
	}
	job.Attempts++
	destination := map[string]interface{}{
		"operatingAccount":     map[string]string{"accountType": "GOOGLE_ADS", "accountId": bookingCustomer},
		"loginAccount":         map[string]string{"accountType": "GOOGLE_ADS", "accountId": bookingCustomer},
		"productDestinationId": bookingAction, "reference": "tts-booked",
	}
	payload := map[string]interface{}{"destinations": []interface{}{destination}, "events": []bookingEvent{job.Event}, "validateOnly": false}
	status, body, err := u.googleRequest(http.MethodPost, "/events:ingest", payload)
	if err != nil {
		u.retry(job, err.Error())
		return
	}
	if status != 200 {
		if status == 401 {
			u.token = ""
		}
		if status == 401 || status == 429 || status >= 500 {
			u.retry(job, fmt.Sprintf("google_http_%d", status))
			return
		}
		job.State = "Failed"
		job.Reason = fmt.Sprintf("google_http_%d", status)
		return
	}
	var receipt struct {
		RequestID string `json:"requestId"`
	}
	if json.Unmarshal(body, &receipt) != nil || receipt.RequestID == "" {
		u.retry(job, "google_receipt_missing")
		return
	}
	job.RequestID = receipt.RequestID
	job.State = "Processing"
	job.Reason = ""
	job.NextCheck = u.now().Add(30 * time.Minute)
}

func (u *GoogleBookingUploader) checkGoogle(job *bookingJob) {
	status, body, err := u.googleRequest(http.MethodGet, "/requestStatus:retrieve?requestId="+url.QueryEscape(job.RequestID), nil)
	if err != nil {
		u.retry(job, err.Error())
		return
	}
	if status != 200 {
		if status == 401 {
			u.token = ""
		}
		u.retry(job, fmt.Sprintf("google_diagnostics_http_%d", status))
		return
	}
	var result struct {
		Destinations []struct {
			Destination struct {
				ProductDestinationID string `json:"productDestinationId"`
			} `json:"destination"`
			Status string `json:"requestStatus"`
			Errors struct {
				Counts []struct {
					Reason string `json:"reason"`
				} `json:"errorCounts"`
			} `json:"errorInfo"`
			Warnings struct {
				Counts []struct {
					Reason string `json:"reason"`
				} `json:"warningCounts"`
			} `json:"warningInfo"`
			Events struct {
				Count string `json:"recordCount"`
			} `json:"eventsIngestionStatus"`
		} `json:"requestStatusPerDestination"`
	}
	if json.Unmarshal(body, &result) != nil || len(result.Destinations) != 1 || result.Destinations[0].Destination.ProductDestinationID != bookingAction {
		u.retry(job, "google_diagnostics_invalid")
		return
	}
	destination := result.Destinations[0]
	job.Warnings = nil
	for _, warning := range destination.Warnings.Counts {
		job.Warnings = append(job.Warnings, warning.Reason)
	}
	switch destination.Status {
	case "SUCCESS":
		if destination.Events.Count != "1" || len(destination.Errors.Counts) > 0 {
			u.retry(job, "google_diagnostics_count_mismatch")
			return
		}
		job.State = "Processed"
		job.Reason = ""
	case "FAILED", "PARTIAL_SUCCESS":
		job.State = "Failed"
		job.Reason = "google_processing_failed"
		if len(destination.Errors.Counts) > 0 {
			job.Reason = destination.Errors.Counts[0].Reason
		}
		if destination.Status == "FAILED" && len(destination.Errors.Counts) == 1 && job.Reason == "PROCESSING_ERROR_REASON_DESTINATION_TOO_RECENTLY_CREATED" {
			job.State = "Queued"
			job.NextCheck = u.now().Add(6 * time.Hour)
		}
	default:
		job.NextCheck = u.now().Add(30 * time.Minute)
	}
}
