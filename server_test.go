package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWhichForm(t *testing.T) {
	if got := whichForm("/free-estimate/ ads lander"); got != "/free-estimate/ ads lander" {
		t.Fatalf("whichForm() = %q, want paid landing-page value", got)
	}
	if got := whichForm("untrusted value"); got != "Main Contact" {
		t.Fatalf("whichForm() = %q, want default form value", got)
	}
}

func TestAppendAttribution(t *testing.T) {
	got := appendAttribution("Needs roofline lighting", `{"landing_page":"/free-estimate/","utm_source":"google","utm_campaign":"holiday_lighting_2026","gclid":"abc123","ignored":"nope"}`)
	want := "Needs roofline lighting\n\nLead attribution:\nLanding page: /free-estimate/\nUTM source: google\nUTM campaign: holiday_lighting_2026\nGoogle click ID: abc123"
	if got != want {
		t.Fatalf("appendAttribution() = %q, want %q", got, want)
	}
}

func TestAppendAttributionIgnoresInvalidData(t *testing.T) {
	if got := appendAttribution("Customer note", "not json"); got != "Customer note" {
		t.Fatalf("appendAttribution() = %q, want original message", got)
	}
}

func TestHandleContactSetsLeadSavedOnlyAfterAirtableSuccess(t *testing.T) {
	leadCreated := false
	stubContactDependencies(t, func(FormData) error {
		leadCreated = true
		return nil
	}, func(string) (string, error) { return "US", nil })

	res := submitContactJSON(t, validFormData())
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	if !leadCreated {
		t.Fatal("lead was not created before success response")
	}
	if got := res.Header().Get("X-Lead-Saved"); got != "true" {
		t.Fatalf("X-Lead-Saved = %q, want true", got)
	}
	if !strings.Contains(res.Body.String(), "Thank you for your interest") {
		t.Fatalf("success response did not contain thank-you HTML: %q", res.Body.String())
	}
}

func TestHandleContactWaitsForAirtableBeforeAcknowledgingLead(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	stubContactDependencies(t, func(FormData) error {
		close(started)
		<-release
		return nil
	}, func(string) (string, error) { return "US", nil })

	result := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		result <- submitContactJSON(t, validFormData())
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Airtable creation did not start")
	}
	select {
	case res := <-result:
		t.Fatalf("received premature response with status %d", res.Code)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case res := <-result:
		if got := res.Header().Get("X-Lead-Saved"); got != "true" {
			t.Fatalf("X-Lead-Saved = %q, want true", got)
		}
	case <-time.After(time.Second):
		t.Fatal("contact handler did not respond after Airtable creation")
	}
}

func TestHandleContactSpamResponsesDoNotSetLeadSaved(t *testing.T) {
	tests := []struct {
		name    string
		data    FormData
		country string
		limited bool
	}{
		{name: "timing", data: FormData{FillTime: 0}, country: "US"},
		{name: "non-US", data: validFormData(), country: "CA"},
		{name: "rate limit", data: validFormData(), country: "US", limited: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			leadCreated := false
			stubContactDependencies(t, func(FormData) error {
				leadCreated = true
				return nil
			}, func(string) (string, error) { return tt.country, nil })
			if tt.limited {
				limiter.windows["192.0.2.1"] = []time.Time{time.Now(), time.Now(), time.Now()}
			}

			res := submitContactJSON(t, tt.data)
			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, want neutral %d response", res.Code, http.StatusOK)
			}
			if leadCreated {
				t.Fatal("spam rejection attempted Airtable lead creation")
			}
			if got := res.Header().Get("X-Lead-Saved"); got != "" {
				t.Fatalf("X-Lead-Saved = %q, want no lead-saved signal", got)
			}
		})
	}
}

func TestHandleContactValidationFailureDoesNotSetLeadSaved(t *testing.T) {
	leadCreated := false
	stubContactDependencies(t, func(FormData) error {
		leadCreated = true
		return nil
	}, func(string) (string, error) { return "US", nil })

	data := validFormData()
	data.Email = "not-an-email"
	res := submitContactJSON(t, data)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusBadRequest)
	}
	if leadCreated {
		t.Fatal("invalid submission attempted Airtable lead creation")
	}
	if got := res.Header().Get("X-Lead-Saved"); got != "" {
		t.Fatalf("X-Lead-Saved = %q, want no lead-saved signal", got)
	}
}

func TestHandleContactAirtableFailureKeepsFormUsable(t *testing.T) {
	stubContactDependencies(t, func(FormData) error {
		return assertError("Airtable unavailable")
	}, func(string) (string, error) { return "US", nil })

	res := submitContactJSON(t, validFormData())
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusServiceUnavailable)
	}
	if got := res.Header().Get("X-Lead-Saved"); got != "" {
		t.Fatalf("X-Lead-Saved = %q, want no lead-saved signal", got)
	}
	if !strings.Contains(res.Body.String(), "couldn't save your request") {
		t.Fatalf("failure response did not include retry guidance: %q", res.Body.String())
	}
}

func TestHandleContactNativeSubmissionRedirectsOnlyAfterAirtableSuccess(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		stubContactDependencies(t, func(FormData) error { return nil }, func(string) (string, error) { return "US", nil })

		res := submitContactForm(t, validFormData())
		if res.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want %d", res.Code, http.StatusSeeOther)
		}
		if got := res.Header().Get("Location"); got != "/thank-you/" {
			t.Fatalf("redirect = %q, want /thank-you/", got)
		}
		if got := res.Header().Get("X-Lead-Saved"); got != "true" {
			t.Fatalf("X-Lead-Saved = %q, want true", got)
		}
	})

	t.Run("Airtable failure", func(t *testing.T) {
		stubContactDependencies(t, func(FormData) error { return assertError("Airtable unavailable") }, func(string) (string, error) { return "US", nil })

		res := submitContactForm(t, validFormData())
		if res.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d", res.Code, http.StatusServiceUnavailable)
		}
		if got := res.Header().Get("Location"); got != "" {
			t.Fatalf("unexpected redirect to %q", got)
		}
		if !strings.Contains(res.Body.String(), "Please try again in a moment") {
			t.Fatalf("failure page did not include retry guidance: %q", res.Body.String())
		}
	})
}

type assertError string

func (e assertError) Error() string { return string(e) }

func validFormData() FormData {
	return FormData{
		FirstName:     "Taylor",
		LastName:      "Example",
		Email:         "taylor@example.com",
		Phone:         "8165551234",
		StreetAddress: "123 Main Street",
		City:          "Lee's Summit",
		State:         "MO",
		Zip:           "64063",
		Message:       "Please send an estimate.",
		FormSource:    "/free-estimate/ ads lander",
		Attribution:   `{"gclid":"test-click"}`,
		FillTime:      minFillTimeSeconds + 1,
	}
}

func submitContactJSON(t *testing.T, data FormData) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal form data: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/contact", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/html")
	req.RemoteAddr = "192.0.2.1:1234"
	res := httptest.NewRecorder()
	handleContact(res, req)
	return res
}

func submitContactForm(t *testing.T, data FormData) *httptest.ResponseRecorder {
	t.Helper()
	form := make(url.Values)
	form.Set("firstName", data.FirstName)
	form.Set("lastName", data.LastName)
	form.Set("email", data.Email)
	form.Set("phone", data.Phone)
	form.Set("streetAddress", data.StreetAddress)
	form.Set("city", data.City)
	form.Set("state", data.State)
	form.Set("zip", data.Zip)
	form.Set("message", data.Message)
	form.Set("formSource", data.FormSource)
	form.Set("_attribution", data.Attribution)
	form.Set("_loaded", strconv.FormatInt(time.Now().Add(-time.Duration(data.FillTime*float64(time.Second))).UnixMilli(), 10))
	req := httptest.NewRequest(http.MethodPost, "/contact", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "192.0.2.1:1234"
	res := httptest.NewRecorder()
	handleContact(res, req)
	return res
}

func stubContactDependencies(t *testing.T, create func(FormData) error, country func(string) (string, error)) {
	t.Helper()
	oldCreate, oldCountry, oldRecord, oldLimiter := createLead, countryLookup, recordSubmission, limiter
	createLead = create
	countryLookup = country
	recordSubmission = func(FormData, string, string, string) {}
	limiter = &rateLimiter{windows: make(map[string][]time.Time)}
	t.Cleanup(func() {
		createLead = oldCreate
		countryLookup = oldCountry
		recordSubmission = oldRecord
		limiter = oldLimiter
	})
}
