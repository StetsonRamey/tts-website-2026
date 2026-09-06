package services

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNormalizeAndHash(t *testing.T) {
	if got := NormalizeEmail("  Taylor@Example.COM "); got != "taylor@example.com" {
		t.Fatalf("NormalizeEmail = %q", got)
	}
	for in, want := range map[string]string{
		"(816) 555-1234":  "18165551234",
		"816-555-1234":    "18165551234",
		"+1 816 555 1234": "18165551234",
		"0018165551234":   "18165551234",
		"":                "",
	} {
		if got := NormalizePhone(in); got != want {
			t.Fatalf("NormalizePhone(%q) = %q, want %q", in, got, want)
		}
	}
	// Known SHA-256 of "taylor@example.com".
	if got := HashUserData("taylor@example.com"); len(got) != 64 || got != strings.ToLower(got) {
		t.Fatalf("HashUserData = %q, want 64-char lowercase hex", got)
	}
	if HashUserData("") != "" {
		t.Fatal("empty values must not be hashed")
	}
}

func TestBuildFBC(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000)
	if got := BuildFBC(" IwAR0abc ", at); got != "fb.1.1700000000000.IwAR0abc" {
		t.Fatalf("BuildFBC = %q", got)
	}
	if BuildFBC("", at) != "" {
		t.Fatal("BuildFBC must not invent a click ID")
	}
}

func TestPayloadShape(t *testing.T) {
	c := &MetaClient{PixelID: "123", AccessToken: "secret-token", GraphVersion: "v25.0"}
	at := time.Unix(1_700_000_000, 0)
	form, err := c.Payload(MetaLeadEvent{
		EventID:   "tts-lead-1",
		EventTime: at,
		SourceURL: "https://tistheseasonkc.com/contact/",
		Email:     "Taylor@Example.com",
		Phone:     "(816) 555-1234",
		ClientIP:  "192.0.2.1",
		UserAgent: "UA/1",
		FBC:       "fb.1.1.abc",
		FBP:       "fb.1.1.def",
	})
	if err != nil {
		t.Fatal(err)
	}
	if form.Get("access_token") != "secret-token" || form.Get("test_event_code") != "" {
		t.Fatalf("unexpected form: %v", form)
	}
	var events []map[string]any
	if err := json.Unmarshal([]byte(form.Get("data")), &events); err != nil || len(events) != 1 {
		t.Fatalf("data = %q: %v", form.Get("data"), err)
	}
	ev := events[0]
	if ev["event_name"] != "Lead" || ev["event_id"] != "tts-lead-1" || ev["action_source"] != "website" ||
		ev["event_source_url"] != "https://tistheseasonkc.com/contact/" || ev["event_time"] != float64(1_700_000_000) {
		t.Fatalf("event = %v", ev)
	}
	if _, ok := ev["custom_data"]; ok {
		t.Fatal("custom_data must not be sent")
	}
	ud := ev["user_data"].(map[string]any)
	em := ud["em"].([]any)[0].(string)
	if em != HashUserData("taylor@example.com") {
		t.Fatalf("em = %q, want normalized hash", em)
	}
	ph := ud["ph"].([]any)[0].(string)
	if ph != HashUserData("18165551234") {
		t.Fatalf("ph = %q, want normalized hash", ph)
	}
	if ud["client_ip_address"] != "192.0.2.1" || ud["client_user_agent"] != "UA/1" || ud["fbc"] != "fb.1.1.abc" || ud["fbp"] != "fb.1.1.def" {
		t.Fatalf("unhashed identifiers wrong: %v", ud)
	}
	if strings.Contains(form.Get("data"), "Taylor") || strings.Contains(form.Get("data"), "555") {
		t.Fatal("raw PII leaked into payload")
	}

	// Per-event test code overrides the client default; client default applies otherwise.
	c.TestEventCode = "TEST1"
	f2, _ := c.Payload(MetaLeadEvent{EventID: "x", EventTime: at})
	if f2.Get("test_event_code") != "TEST1" {
		t.Fatalf("client test code not applied: %v", f2)
	}
	f3, _ := c.Payload(MetaLeadEvent{EventID: "x", EventTime: at, TestEventCode: "TEST2"})
	if f3.Get("test_event_code") != "TEST2" {
		t.Fatalf("event test code not applied: %v", f3)
	}
}

func TestSendLeadRetriesWithSameEventIDAndRedactsToken(t *testing.T) {
	var calls int32
	var seenIDs []string
	var seenURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		seenURL = r.URL.String()
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		var events []map[string]any
		_ = json.Unmarshal([]byte(form.Get("data")), &events)
		seenIDs = append(seenIDs, events[0]["event_id"].(string))
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":{"message":"transient secret-token","code":2}}`))
			return
		}
		w.Write([]byte(`{"events_received":1,"messages":[],"fbtrace_id":"abc"}`))
	}))
	defer srv.Close()

	var logs []string
	c := &MetaClient{PixelID: "123", AccessToken: "secret-token", GraphVersion: "v25.0", Endpoint: srv.URL,
		Logf: func(f string, a ...any) {
			logs = append(logs, strings.TrimSpace(strings.ReplaceAll(f, "%v", "%s"))+" "+strings.Join(toStrings(a), " "))
		}}
	if err := c.SendLead(context.Background(), MetaLeadEvent{EventID: "tts-lead-1", EventTime: time.Now()}); err != nil {
		t.Fatalf("SendLead: %v", err)
	}
	if calls != 2 || seenIDs[0] != "tts-lead-1" || seenIDs[1] != "tts-lead-1" {
		t.Fatalf("calls=%d ids=%v, want one retry with identical event ID", calls, seenIDs)
	}
	if seenURL != "/v25.0/123/events" {
		t.Fatalf("URL = %q; token must never be in the URL", seenURL)
	}
	for _, l := range logs {
		if strings.Contains(l, "secret-token") {
			t.Fatalf("token leaked into log: %q", l)
		}
	}
}

func TestSendLeadDoesNotRetryClientErrors(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"Invalid parameter","type":"OAuthException","code":100,"error_subcode":2804003,"fbtrace_id":"x"}}`))
	}))
	defer srv.Close()
	c := &MetaClient{PixelID: "123", AccessToken: "tok", GraphVersion: "v25.0", Endpoint: srv.URL, Logf: func(string, ...any) {}}
	err := c.SendLead(context.Background(), MetaLeadEvent{EventID: "tts-lead-1", EventTime: time.Now()})
	if err == nil || !strings.Contains(err.Error(), "code=100") || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestSendLeadTimeoutIsBounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()
	c := &MetaClient{PixelID: "123", AccessToken: "tok", GraphVersion: "v25.0", Endpoint: srv.URL,
		HTTPClient: &http.Client{Timeout: 100 * time.Millisecond}, Logf: func(string, ...any) {}}
	start := time.Now()
	err := c.SendLead(context.Background(), MetaLeadEvent{EventID: "tts-lead-1", EventTime: time.Now()})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if time.Since(start) > 1500*time.Millisecond {
		t.Fatalf("SendLead took %v; should be bounded", time.Since(start))
	}
}

func TestDisabledClientIsNoop(t *testing.T) {
	var c *MetaClient
	if err := c.SendLead(context.Background(), MetaLeadEvent{EventID: "x"}); err != nil {
		t.Fatalf("nil client: %v", err)
	}
	if (&MetaClient{PixelID: "123"}).Enabled() {
		t.Fatal("pixel ID alone must not enable CAPI")
	}
}

func TestLoadMetaClientDropsTestCodeInProd(t *testing.T) {
	t.Setenv("META_PIXEL_ID", "123")
	t.Setenv("META_CAPI_ACCESS_TOKEN", "tok")
	t.Setenv("META_TEST_EVENT_CODE", "TEST1")
	t.Setenv("META_GRAPH_API_VERSION", "")
	if c := LoadMetaClient("prod"); c.TestEventCode != "" || c.GraphVersion != DefaultMetaGraphAPIVersion || !c.Enabled() {
		t.Fatalf("prod client = %+v", c)
	}
	if c := LoadMetaClient("dev"); c.TestEventCode != "TEST1" {
		t.Fatalf("dev client = %+v", c)
	}
	t.Setenv("META_CAPI_ACCESS_TOKEN", "")
	if LoadMetaClient("dev").Enabled() {
		t.Fatal("missing token must disable CAPI")
	}
}

func toStrings(a []any) []string {
	out := make([]string, len(a))
	for i, v := range a {
		switch x := v.(type) {
		case string:
			out[i] = x
		case error:
			out[i] = x.Error()
		default:
			b, _ := json.Marshal(x)
			out[i] = string(b)
		}
	}
	return out
}
