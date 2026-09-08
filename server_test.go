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

	"tts-server/services"
)

func TestWhichForm(t *testing.T) {
	if got := whichForm("/free-estimate/ ads lander"); got != "/free-estimate/ ads lander" {
		t.Fatalf("whichForm() = %q, want paid landing-page value", got)
	}
	if got := whichForm("untrusted value"); got != "Main Contact" {
		t.Fatalf("whichForm() = %q, want default form value", got)
	}
}

// vanQRAttribution is the session snapshot a visitor gets from the exact
// printed van QR URL: https://tistheseasonkc.com/free-estimate/?utm_source=van&utm_medium=qr&utm_campaign=van_wrap
const vanQRAttribution = `{"landing_page":"/free-estimate/","utm_source":"van","utm_medium":"qr","utm_campaign":"van_wrap"}`

func TestIsVanQR(t *testing.T) {
	if !isVanQR(vanQRAttribution) {
		t.Fatalf("isVanQR() = false, want true for exact van QR combination")
	}
}

func TestIsVanQRRequiresExactCombination(t *testing.T) {
	tests := []struct {
		name        string
		attribution string
	}{
		{name: "missing tags", attribution: `{"utm_source":"van"}`},
		{name: "wrong source", attribution: `{"utm_source":"facebook","utm_medium":"qr","utm_campaign":"van_wrap"}`},
		{name: "wrong medium", attribution: `{"utm_source":"van","utm_medium":"print","utm_campaign":"van_wrap"}`},
		{name: "wrong campaign", attribution: `{"utm_source":"van","utm_medium":"qr","utm_campaign":"other"}`},
		{name: "empty", attribution: ""},
		{name: "invalid JSON", attribution: "not json"},
		{name: "extra campaign only is not enough", attribution: `{"utm_campaign":"van_wrap"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if isVanQR(tt.attribution) {
				t.Fatalf("isVanQR(%q) = true, want false", tt.attribution)
			}
		})
	}
}

func TestLeadSourceMapsExactVanQRToQRCode(t *testing.T) {
	// Regardless of which form the visitor submits from, the exact UTM
	// combination (carried by session attribution from the QR landing page)
	// maps to the literal "QR Code" lead source.
	if got := leadSource("/free-estimate/ ads lander", vanQRAttribution); got != "QR Code" {
		t.Fatalf("leadSource() from free-estimate = %q, want %q", got, "QR Code")
	}
	if got := leadSource("", vanQRAttribution); got != "QR Code" {
		t.Fatalf("leadSource() from contact = %q, want %q", got, "QR Code")
	}
}

func TestLeadSourcePreservesNonQRBehavior(t *testing.T) {
	tests := []struct {
		name        string
		formSource  string
		attribution string
		want        string
	}{
		{name: "ordinary contact lead", formSource: "", attribution: "", want: "Main Contact"},
		{name: "free-estimate with no UTMs", formSource: "/free-estimate/ ads lander", attribution: `{"landing_page":"/free-estimate/"}`, want: "/free-estimate/ ads lander"},
		{name: "Google Ads lead", formSource: "", attribution: `{"gclid":"abc123","utm_source":"google","utm_medium":"cpc","utm_campaign":"holiday_lighting_2026"}`, want: "Main Contact"},
		{name: "Meta lead", formSource: "", attribution: `{"fbclid":"IwARxyz","utm_source":"facebook"}`, want: "Main Contact"},
		{name: "van but wrong source", formSource: "", attribution: `{"utm_source":"google","utm_medium":"qr","utm_campaign":"van_wrap"}`, want: "Main Contact"},
		{name: "van but wrong campaign", formSource: "/free-estimate/ ads lander", attribution: `{"utm_source":"van","utm_medium":"qr","utm_campaign":"spring_promo"}`, want: "/free-estimate/ ads lander"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := leadSource(tt.formSource, tt.attribution); got != tt.want {
				t.Fatalf("leadSource() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAirtablePayloadUsesQRCodeAndRetainsUTMDetails(t *testing.T) {
	fd := validFormData()
	fd.FormSource = "/free-estimate/ ads lander"
	fd.Attribution = vanQRAttribution
	fd.TrackingMetrics = formatAttribution(fd.Attribution, "")

	payload := airtableLeadPayload(fd)
	records, ok := payload["records"].([]map[string]any)
	if !ok || len(records) != 1 {
		t.Fatalf("payload records = %#v, want one lead record", payload["records"])
	}
	fields, ok := records[0]["fields"].(map[string]any)
	if !ok {
		t.Fatalf("lead fields = %#v, want map", records[0]["fields"])
	}

	if got := fields["fldZWX56UF9UNbZOW"]; got != "QR Code" {
		t.Fatalf("Which Form = %q, want %q", got, "QR Code")
	}

	// The original UTM details ride along in the Tracking Metrics field so van
	// leads stay distinguishable from future QR campaigns, and the comments
	// field carries only the customer's own notes.
	metrics, _ := fields["fldqlM7utGqqVnVa7"].(string)
	for _, want := range []string{"UTM source: van", "UTM medium: qr", "UTM campaign: van_wrap"} {
		if !strings.Contains(metrics, want) {
			t.Fatalf("tracking metrics %q do not retain %q", metrics, want)
		}
	}
	if comments, _ := fields["fldGhAjDinMRV827I"].(string); comments != fd.Message {
		t.Fatalf("comments %q must contain only the customer message %q", comments, fd.Message)
	}
}

func TestAirtablePayloadKeepsWhichFormValue(t *testing.T) {
	// A regular free-estimate lead (no van UTMs) keeps its existing label.
	fd := validFormData()
	fd.FormSource = "/free-estimate/ ads lander"
	fd.Attribution = `{"gclid":"abc123"}`

	payload := airtableLeadPayload(fd)
	records := payload["records"].([]map[string]any)
	fields := records[0]["fields"].(map[string]any)
	if got := fields["fldZWX56UF9UNbZOW"]; got != "/free-estimate/ ads lander" {
		t.Fatalf("Which Form = %q, want %q", got, "/free-estimate/ ads lander")
	}
}

func TestFormatAttribution(t *testing.T) {
	got := formatAttribution(`{"landing_page":"/free-estimate/","utm_source":"google","utm_campaign":"holiday_lighting_2026","gclid":"abc123","ignored":"nope"}`, "")
	want := "Landing page: /free-estimate/\nUTM source: google\nUTM campaign: holiday_lighting_2026\nGoogle click ID: abc123"
	if got != want {
		t.Fatalf("formatAttribution() = %q, want %q", got, want)
	}
}

func TestFormatAttributionRetainsMetaIdentifiers(t *testing.T) {
	got := formatAttribution(`{"fbclid":"IwAR0abc","fbc":"fb.1.1700000000000.IwAR0abc","fbp":"fb.1.1700000000000.123456","page":"/contact/","message":"ignored"}`, "tts-lead-deadbeef")
	want := "Meta click ID: IwAR0abc\nMeta fbc: fb.1.1700000000000.IwAR0abc\nMeta browser ID: fb.1.1700000000000.123456\nMeta event ID: tts-lead-deadbeef"
	if got != want {
		t.Fatalf("formatAttribution() = %q, want %q", got, want)
	}
}

func TestFormatAttributionIgnoresInvalidData(t *testing.T) {
	if got := formatAttribution("not json", ""); got != "" {
		t.Fatalf("formatAttribution() = %q, want empty", got)
	}
	// An event ID alone is still recorded so the Airtable lead can be reconciled.
	if got := formatAttribution("not json", "tts-lead-1"); got != "Meta event ID: tts-lead-1" {
		t.Fatalf("formatAttribution() = %q", got)
	}
}

func TestMetaLeadEventPrefersPixelCookiesAndExcludesInquiryData(t *testing.T) {
	fd := validFormData()
	fd.Attribution = `{"fbclid":"IwARfromurl","fbc":"fb.1.1.stored","fbp":"fb.1.1.storedfbp","page":"/free-estimate/"}`
	req := httptest.NewRequest(http.MethodPost, "/contact", nil)
	req.Header.Set("Referer", "https://tistheseasonkc.com/free-estimate/?fbclid=IwARfromurl")
	req.Header.Set("User-Agent", "TestBrowser/1.0")
	req.RemoteAddr = "192.0.2.1:1234"
	req.AddCookie(&http.Cookie{Name: "_fbc", Value: "fb.1.2.cookie"})
	req.AddCookie(&http.Cookie{Name: "_fbp", Value: "fb.1.2.cookiefbp"})

	at := time.Unix(1_700_000_000, 0)
	e := metaLeadEvent(req, fd, "tts-lead-1", at)
	if e.FBC != "fb.1.2.cookie" || e.FBP != "fb.1.2.cookiefbp" {
		t.Fatalf("cookie identifiers not preferred: fbc=%q fbp=%q", e.FBC, e.FBP)
	}
	if e.SourceURL != "https://tistheseasonkc.com/free-estimate/" {
		t.Fatalf("SourceURL = %q; must be the public page without query string", e.SourceURL)
	}
	if e.ClientIP != "192.0.2.1" || e.UserAgent != "TestBrowser/1.0" || !e.EventTime.Equal(at) {
		t.Fatalf("unexpected event basics: %+v", e)
	}

	// Without cookies, fall back to the stored attribution, then a real fbclid.
	req2 := httptest.NewRequest(http.MethodPost, "/contact", nil)
	e2 := metaLeadEvent(req2, fd, "tts-lead-2", at)
	if e2.FBC != "fb.1.1.stored" || e2.FBP != "fb.1.1.storedfbp" {
		t.Fatalf("stored identifiers not used: fbc=%q fbp=%q", e2.FBC, e2.FBP)
	}
	if e2.SourceURL != "https://tistheseasonkc.com/free-estimate/" {
		t.Fatalf("SourceURL fallback = %q", e2.SourceURL)
	}
	fd.Attribution = `{"fbclid":"IwARfromurl","fbclid_ts":"1699999000000"}`
	e3 := metaLeadEvent(req2, fd, "tts-lead-3", at)
	if e3.FBC != "fb.1.1699999000000.IwARfromurl" {
		t.Fatalf("fbc from real fbclid = %q", e3.FBC)
	}
	// No identifiers anywhere: nothing is invented.
	fd.Attribution = ""
	e4 := metaLeadEvent(req2, fd, "tts-lead-4", at)
	if e4.FBC != "" || e4.FBP != "" || e4.SourceURL != "https://tistheseasonkc.com/contact/" {
		t.Fatalf("unexpected synthesized data: %+v", e4)
	}
}

func TestHandleContactSetsLeadSavedOnlyAfterAirtableSuccess(t *testing.T) {
	leadCreated := false
	var savedForm FormData
	stubContactDependencies(t, func(fd FormData) error {
		leadCreated = true
		savedForm = fd
		return nil
	}, func(string) (string, error) { return "US", nil })
	metaEvents := captureMetaLeads(t)

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

	// Browser and server Lead events must share one ID, and the saved lead keeps
	// it in Tracking Metrics — never in the customer-facing comments.
	eventID := res.Header().Get("X-Meta-Event-Id")
	if !strings.HasPrefix(eventID, "tts-lead-") {
		t.Fatalf("X-Meta-Event-Id = %q, want generated event ID", eventID)
	}
	select {
	case e := <-metaEvents:
		if e.EventID != eventID {
			t.Fatalf("server Meta event ID %q != browser event ID %q", e.EventID, eventID)
		}
		if e.Email != "taylor@example.com" || e.Phone != "8165551234" || e.EventTime.IsZero() {
			t.Fatalf("server Meta event missing match data: %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("server-side Meta Lead was not sent after Airtable success")
	}
	if !strings.Contains(savedForm.TrackingMetrics, "Meta event ID: "+eventID) {
		t.Fatalf("Airtable Tracking Metrics did not retain Meta event ID: %q", savedForm.TrackingMetrics)
	}
	if savedForm.Message != "Please send an estimate." {
		t.Fatalf("comments must carry only the customer message, got %q", savedForm.Message)
	}
}

func TestHandleContactSucceedsWhenMetaDeliveryFails(t *testing.T) {
	stubContactDependencies(t, func(FormData) error { return nil }, func(string) (string, error) { return "US", nil })
	// Simulate Meta rejecting/timing out: the real deliverMetaLead only logs, and
	// a slow sender must not delay the response either.
	oldSend := sendMetaLead
	sendMetaLead = func(services.MetaLeadEvent) { time.Sleep(200 * time.Millisecond) }
	t.Cleanup(func() { sendMetaLead = oldSend })

	start := time.Now()
	res := submitContactJSON(t, validFormData())
	if res.Code != http.StatusOK || res.Header().Get("X-Lead-Saved") != "true" {
		t.Fatalf("saved lead must still succeed: status=%d saved=%q", res.Code, res.Header().Get("X-Lead-Saved"))
	}
	if time.Since(start) > 150*time.Millisecond {
		t.Fatalf("response waited on Meta delivery (%v)", time.Since(start))
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

	metaEvents := captureMetaLeads(t)
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
	case e := <-metaEvents:
		t.Fatalf("Meta Lead %s sent before Airtable confirmed the save", e.EventID)
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
	select {
	case <-metaEvents:
	case <-time.After(time.Second):
		t.Fatal("Meta Lead not sent after Airtable confirmed the save")
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

			metaEvents := captureMetaLeads(t)
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
			assertNoMetaLead(t, res, metaEvents)
		})
	}
}

func TestHandleContactValidationFailureDoesNotSetLeadSaved(t *testing.T) {
	leadCreated := false
	stubContactDependencies(t, func(FormData) error {
		leadCreated = true
		return nil
	}, func(string) (string, error) { return "US", nil })

	metaEvents := captureMetaLeads(t)
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
	assertNoMetaLead(t, res, metaEvents)
}

func TestHandleContactAirtableFailureKeepsFormUsable(t *testing.T) {
	stubContactDependencies(t, func(FormData) error {
		return assertError("Airtable unavailable")
	}, func(string) (string, error) { return "US", nil })

	metaEvents := captureMetaLeads(t)
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
	assertNoMetaLead(t, res, metaEvents)
}

func TestHandleContactNativeSubmissionRedirectsOnlyAfterAirtableSuccess(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		stubContactDependencies(t, func(FormData) error { return nil }, func(string) (string, error) { return "US", nil })
		metaEvents := captureMetaLeads(t)

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
		// Server-side tracking does not depend on JavaScript.
		select {
		case <-metaEvents:
		case <-time.After(time.Second):
			t.Fatal("native form submission did not send server-side Meta Lead")
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
	oldCreate, oldCountry, oldRecord, oldLimiter, oldMeta := createLead, countryLookup, recordSubmission, limiter, sendMetaLead
	createLead = create
	countryLookup = country
	recordSubmission = func(FormData, string, string, string) {}
	sendMetaLead = func(services.MetaLeadEvent) {}
	limiter = &rateLimiter{windows: make(map[string][]time.Time)}
	t.Cleanup(func() {
		createLead = oldCreate
		countryLookup = oldCountry
		recordSubmission = oldRecord
		sendMetaLead = oldMeta
		limiter = oldLimiter
	})
}

// captureMetaLeads replaces the Meta sender with one that records events.
// Call after stubContactDependencies so its cleanup restores the original.
func captureMetaLeads(t *testing.T) chan services.MetaLeadEvent {
	t.Helper()
	events := make(chan services.MetaLeadEvent, 4)
	old := sendMetaLead
	sendMetaLead = func(e services.MetaLeadEvent) { events <- e }
	t.Cleanup(func() { sendMetaLead = old })
	return events
}

func assertNoMetaLead(t *testing.T, res *httptest.ResponseRecorder, events chan services.MetaLeadEvent) {
	t.Helper()
	if got := res.Header().Get("X-Meta-Event-Id"); got != "" {
		t.Fatalf("X-Meta-Event-Id = %q, want no browser Lead eligibility", got)
	}
	select {
	case e := <-events:
		t.Fatalf("unexpected server-side Meta Lead %s", e.EventID)
	case <-time.After(50 * time.Millisecond):
	}
}
