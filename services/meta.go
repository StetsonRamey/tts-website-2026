package services

// Meta Conversions API (CAPI) client.
//
// Sends a server-side "Lead" event to the TTS Website dataset after Airtable
// confirms a contact-form lead. The browser Pixel fires the same event name
// with the same event_id so Meta deduplicates the pair.
//
// Delivery is best-effort: a bounded timeout, one retry on transport/5xx
// failures with the same event ID, and sanitized logging. Failures never
// affect the form response — by the time this runs, the lead is already saved.
//
// Payload reference: facebook_business/adobjects/serverside/{event,event_request,user_data}.py

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// DefaultMetaGraphAPIVersion is the explicit, currently supported Graph API
	// version used when META_GRAPH_API_VERSION is unset. Bump deliberately.
	DefaultMetaGraphAPIVersion = "v25.0"

	metaRequestTimeout = 5 * time.Second
	metaMaxAttempts    = 2
)

// MetaClient sends Conversions API events. A nil or unconfigured client is
// disabled and SendLead is a safe no-op.
type MetaClient struct {
	PixelID       string
	AccessToken   string
	GraphVersion  string
	TestEventCode string // applied to every event; dev only
	Endpoint      string // override for tests; default https://graph.facebook.com
	HTTPClient    *http.Client
	Logf          func(format string, args ...any)
}

// LoadMetaClient builds the CAPI client from environment variables:
//
//	META_PIXEL_ID            dataset / pixel ID (public — also embedded by Hugo)
//	META_CAPI_ACCESS_TOKEN   server-only system-user token
//	META_GRAPH_API_VERSION   optional, defaults to DefaultMetaGraphAPIVersion
//	META_TEST_EVENT_CODE     optional; ignored when env is prod
//
// Missing configuration returns a disabled client and leaves intake untouched.
func LoadMetaClient(env string) *MetaClient {
	c := &MetaClient{
		PixelID:       strings.TrimSpace(os.Getenv("META_PIXEL_ID")),
		AccessToken:   strings.TrimSpace(os.Getenv("META_CAPI_ACCESS_TOKEN")),
		GraphVersion:  strings.TrimSpace(os.Getenv("META_GRAPH_API_VERSION")),
		TestEventCode: strings.TrimSpace(os.Getenv("META_TEST_EVENT_CODE")),
	}
	if c.GraphVersion == "" {
		c.GraphVersion = DefaultMetaGraphAPIVersion
	}
	switch {
	case c.PixelID == "" && c.AccessToken == "":
		log.Println("[meta] META_PIXEL_ID / META_CAPI_ACCESS_TOKEN not set — Conversions API disabled")
	case c.PixelID == "" || c.AccessToken == "":
		log.Println("WARNING: [meta] both META_PIXEL_ID and META_CAPI_ACCESS_TOKEN are required — Conversions API disabled")
	default:
		log.Printf("[meta] Conversions API enabled for dataset %s (%s)", c.PixelID, c.GraphVersion)
	}
	if c.TestEventCode != "" {
		if env == "prod" {
			log.Println("WARNING: [meta] META_TEST_EVENT_CODE is set in prod — ignored so real visitors are never tagged as test events")
			c.TestEventCode = ""
		} else {
			log.Printf("[meta] test_event_code %s applied to all events (dev)", c.TestEventCode)
		}
	}
	return c
}

// Enabled reports whether the client has enough configuration to send events.
func (c *MetaClient) Enabled() bool {
	return c != nil && c.PixelID != "" && c.AccessToken != ""
}

// MetaLeadEvent is the input for one server-side Lead event. Only the fields
// listed here are ever sent; inquiry text, quote details, and addresses stay out.
type MetaLeadEvent struct {
	EventID   string    // shared with the browser Pixel for deduplication
	EventTime time.Time // when the lead was actually saved
	SourceURL string    // public page URL the form was submitted from
	Email     string    // raw; normalized + hashed here
	Phone     string    // raw; normalized + hashed here
	ClientIP  string    // not hashed
	UserAgent string    // not hashed
	FBC       string    // _fbc cookie value or fb.1.<ms>.<fbclid> built by the browser; not hashed
	FBP       string    // _fbp cookie value; not hashed

	// TestEventCode overrides the client-wide code for one event. Used by the
	// internal synthetic-event tool so a test never reaches production reporting.
	TestEventCode string
}

// NewMetaEventID returns a unique event identifier shared by both channels.
func NewMetaEventID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("tts-lead-%d", time.Now().UnixNano())
	}
	return "tts-lead-" + hex.EncodeToString(b[:])
}

// BuildFBC constructs the fbc parameter from a real Meta click ID in the
// documented format "fb.1.<creationTimeMs>.<fbclid>". Used only when the
// browser Pixel did not set the _fbc cookie (for example when it was blocked).
// The click ID must come from the visitor's own landing URL — never synthesized.
func BuildFBC(fbclid string, at time.Time) string {
	fbclid = strings.TrimSpace(fbclid)
	if fbclid == "" {
		return ""
	}
	return fmt.Sprintf("fb.1.%d.%s", at.UnixMilli(), fbclid)
}

// NormalizeEmail applies Meta's email normalization: trim and lowercase.
func NormalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// NormalizePhone applies Meta's phone normalization: digits only, no leading
// zeros, with country code. Ten-digit US numbers get a leading 1.
func NormalizePhone(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	digits := strings.TrimLeft(b.String(), "0")
	if len(digits) == 10 {
		digits = "1" + digits
	}
	return digits
}

// HashUserData returns the lowercase hex SHA-256 of a normalized value, or "".
func HashUserData(normalized string) string {
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// userData builds the CAPI user_data object. Email/phone are hashed;
// IP, user agent and cookie identifiers are sent as-is per Meta's spec.
func (e MetaLeadEvent) userData() map[string]any {
	ud := map[string]any{}
	if h := HashUserData(NormalizeEmail(e.Email)); h != "" {
		ud["em"] = []string{h}
	}
	if h := HashUserData(NormalizePhone(e.Phone)); h != "" {
		ud["ph"] = []string{h}
	}
	if e.ClientIP != "" {
		ud["client_ip_address"] = e.ClientIP
	}
	if e.UserAgent != "" {
		ud["client_user_agent"] = e.UserAgent
	}
	if e.FBC != "" {
		ud["fbc"] = e.FBC
	}
	if e.FBP != "" {
		ud["fbp"] = e.FBP
	}
	return ud
}

// Payload returns the form-encoded request body (matching Meta's SDKs, which
// post `data` as a JSON array). The access token travels in the body — never
// in the URL — so it cannot leak through request logging.
func (c *MetaClient) Payload(e MetaLeadEvent) (url.Values, error) {
	event := map[string]any{
		"event_name":       "Lead",
		"event_time":       e.EventTime.Unix(),
		"event_id":         e.EventID,
		"event_source_url": e.SourceURL,
		"action_source":    "website",
		"user_data":        e.userData(),
	}
	data, err := json.Marshal([]any{event})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	form := url.Values{}
	form.Set("data", string(data))
	form.Set("access_token", c.AccessToken)
	if code := firstNonEmpty(e.TestEventCode, c.TestEventCode); code != "" {
		form.Set("test_event_code", code)
	}
	return form, nil
}

// SendLead delivers one Lead event. The same event ID is reused across the
// retry so Meta never sees two distinct conversions for one lead.
func (c *MetaClient) SendLead(ctx context.Context, e MetaLeadEvent) error {
	if !c.Enabled() {
		return nil
	}
	if e.EventID == "" {
		return fmt.Errorf("meta: event ID is required")
	}
	if e.EventTime.IsZero() {
		e.EventTime = time.Now()
	}
	form, err := c.Payload(e)
	if err != nil {
		return err
	}
	endpoint := firstNonEmpty(c.Endpoint, "https://graph.facebook.com")
	reqURL := fmt.Sprintf("%s/%s/%s/events", strings.TrimRight(endpoint, "/"), c.GraphVersion, c.PixelID)
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: metaRequestTimeout}
	}

	var lastErr error
	for attempt := 1; attempt <= metaMaxAttempts; attempt++ {
		retry, err := c.post(ctx, client, reqURL, form.Encode())
		if err == nil {
			c.logf("[meta] Lead delivered event_id=%s attempt=%d", e.EventID, attempt)
			return nil
		}
		lastErr = err
		if !retry || attempt == metaMaxAttempts || ctx.Err() != nil {
			break
		}
		select {
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
		}
	}
	c.logf("[meta] Lead delivery failed event_id=%s: %v", e.EventID, lastErr)
	return lastErr
}

// post performs one request. The returned bool reports whether a retry is
// reasonable (transport error or 5xx); 4xx responses are not retried.
func (c *MetaClient) post(ctx context.Context, client *http.Client, reqURL, body string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewBufferString(body))
	if err != nil {
		return false, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return true, fmt.Errorf("request: %s", c.redact(err.Error()))
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var ok struct {
			EventsReceived int      `json:"events_received"`
			Messages       []string `json:"messages"`
		}
		if json.Unmarshal(b, &ok) == nil && ok.EventsReceived == 0 {
			return false, fmt.Errorf("http %d but events_received=0 messages=%v", resp.StatusCode, ok.Messages)
		}
		return false, nil
	}

	var fail struct {
		Error struct {
			Message   string `json:"message"`
			Type      string `json:"type"`
			Code      int    `json:"code"`
			Subcode   int    `json:"error_subcode"`
			UserMsg   string `json:"error_user_msg"`
			FBTraceID string `json:"fbtrace_id"`
		} `json:"error"`
	}
	_ = json.Unmarshal(b, &fail)
	msg := firstNonEmpty(fail.Error.UserMsg, fail.Error.Message)
	if len(msg) > 200 {
		msg = msg[:200]
	}
	err = fmt.Errorf("http %d code=%d subcode=%d type=%s fbtrace=%s: %s",
		resp.StatusCode, fail.Error.Code, fail.Error.Subcode, fail.Error.Type, fail.Error.FBTraceID, c.redact(msg))
	return resp.StatusCode >= 500, err
}

func (c *MetaClient) logf(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

// redact strips the access token from any message that might echo the
// request URL or body.
func (c *MetaClient) redact(s string) string {
	if c.AccessToken == "" {
		return s
	}
	return strings.ReplaceAll(s, c.AccessToken, "[redacted]")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// MetaTestHandler is an owner-only tool (internal listener) that sends one
// synthetic Lead to Meta's Test Events tool and renders a page which fires the
// matching browser Pixel Lead with the same event ID, so browser/server
// deduplication can be confirmed in Events Manager without creating an
// Airtable lead, sending customer email, or touching payments.
//
// The test_event_code is required: a synthetic event must never land in
// production reporting. Open Events Manager → Test Events in the same browser
// first so the browser-side event is labelled as a test too.
func MetaTestHandler(client *MetaClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		if !client.Enabled() {
			http.Error(w, "Meta Conversions API is not configured (META_PIXEL_ID / META_CAPI_ACCESS_TOKEN)", http.StatusServiceUnavailable)
			return
		}
		if code == "" {
			fmt.Fprint(w, `<!doctype html><meta charset=utf-8><title>Meta test event</title>
<style>body{font:15px/1.5 -apple-system,Segoe UI,Roboto,sans-serif;background:#0f1115;color:#e6e6e6;padding:40px;max-width:40rem}input,button{font:inherit;padding:6px 10px}a{color:#6fb6ff}</style>
<h1>Send a synthetic Meta Lead (test only)</h1>
<p>1. In Events Manager open the dataset → <b>Test events</b> and copy the <code>TEST…</code> code.<br>
2. Keep that tab open in this browser so the Pixel event is labelled as a test too.<br>
3. Enter the code below. One server Lead and one browser Lead are sent with the same event ID.</p>
<form method="GET"><label>Test event code <input name="code" placeholder="TEST1234" required pattern="TEST[0-9]+"></label> <button>Send</button></form>
<p><a href="/">← Internal tools</a></p>`)
			return
		}
		if !strings.HasPrefix(code, "TEST") {
			http.Error(w, "test event code must start with TEST", http.StatusBadRequest)
			return
		}

		eventID := "tts-test-" + strings.TrimPrefix(NewMetaEventID(), "tts-lead-")
		e := MetaLeadEvent{
			EventID:       eventID,
			EventTime:     time.Now(),
			SourceURL:     "https://tistheseasonkc.com/contact/",
			Email:         "meta-test@example.com",
			Phone:         "8165550100",
			ClientIP:      testClientIP(r),
			UserAgent:     r.UserAgent(),
			TestEventCode: code,
		}
		if c, err := r.Cookie("_fbp"); err == nil {
			e.FBP = c.Value
		}
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		result := "Server Lead accepted by Meta (test_event_code " + code + ")."
		if err := client.SendLead(ctx, e); err != nil {
			result = "Server Lead FAILED: " + err.Error()
		}

		fmt.Fprintf(w, `<!doctype html><meta charset=utf-8><title>Meta test event</title>
<style>body{font:15px/1.5 -apple-system,Segoe UI,Roboto,sans-serif;background:#0f1115;color:#e6e6e6;padding:40px;max-width:40rem}code{background:#1a1d24;padding:2px 6px;border-radius:4px}a{color:#6fb6ff}</style>
<script>
!function(f,b,e,v,n,t,s){if(f.fbq)return;n=f.fbq=function(){n.callMethod?
n.callMethod.apply(n,arguments):n.queue.push(arguments)};if(!f._fbq)f._fbq=n;
n.push=n;n.loaded=!0;n.version='2.0';n.queue=[];t=b.createElement(e);t.async=!0;
t.src=v;s=b.getElementsByTagName(e)[0];s.parentNode.insertBefore(t,s)}(window,
document,'script','https://connect.facebook.net/en_US/fbevents.js');
fbq('init', %q);
fbq('track', 'Lead', {}, {eventID: %q});
</script>
<h1>Synthetic Meta Lead sent</h1>
<p>%s</p>
<p>Browser Pixel Lead fired from this page with the same ID.</p>
<p>Event ID: <code>%s</code></p>
<p>In Events Manager → Test events, expect a <b>Server</b> Lead and a <b>Browser</b> Lead with this ID, one marked deduplicated. If the browser row is missing, the Test events tab was not active in this browser (or the Pixel is blocked).</p>
<p><a href="/internal/meta-test">Send another</a> · <a href="/">← Internal tools</a></p>`,
			client.PixelID, eventID, htmlEscape(result), eventID)
	}
}

// testClientIP returns the requester's address without a port for the
// synthetic test event (the owner's own address, via the exe.dev proxy).
func testClientIP(r *http.Request) string {
	ip := clientIPFromRequest(r)
	if host, _, err := net.SplitHostPort(ip); err == nil {
		return host
	}
	return ip
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
