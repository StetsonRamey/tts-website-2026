package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"tts-server/services"
)

type FormData struct {
	FirstName     string  `json:"firstName"`
	LastName      string  `json:"lastName"`
	Email         string  `json:"email"`
	Phone         string  `json:"phone"`
	StreetAddress string  `json:"streetAddress"`
	City          string  `json:"city"`
	State         string  `json:"state"`
	Zip           string  `json:"zip"`
	Message       string  `json:"message"`
	FormSource    string  `json:"formSource"`
	Attribution   string  `json:"_attribution"`
	FillTime      float64 `json:"_fillTime"`
}

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
var digitsRe = regexp.MustCompile(`\D`)

// These variables keep the contact handler testable without making real Airtable
// or GeoIP requests. Production uses the functions assigned here.
var (
	countryLookup    = countryForIP
	createLead       = sendToAirtable
	recordSubmission = logSubmission
)

const (
	airtableProxy      = "https://airtable.int.exe.xyz"
	airtableBaseID     = "appg9012rLh2diVLq"
	leadsTableID       = "tblui0E6mBFkHGWvZ"
	logTableID         = "tbltosRhDRyIs2xZS"
	minFillTimeSeconds = 4.0
	maxSubmitsPerHour  = 3
)

// ── Rate Limiter ──

type rateLimiter struct {
	mu      sync.Mutex
	windows map[string][]time.Time
}

var limiter = &rateLimiter{windows: make(map[string][]time.Time)}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-1 * time.Hour)

	// Prune old entries
	var recent []time.Time
	for _, t := range rl.windows[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}

	if len(recent) >= maxSubmitsPerHour {
		rl.windows[ip] = recent
		return false
	}

	rl.windows[ip] = append(recent, now)
	return true
}

// ── Validation ──

func validate(d FormData) []string {
	var errs []string

	if len(strings.TrimSpace(d.FirstName)) < 2 {
		errs = append(errs, "First name must be at least 2 characters")
	}
	if len(strings.TrimSpace(d.LastName)) < 2 {
		errs = append(errs, "Last name must be at least 2 characters")
	}
	if !emailRe.MatchString(d.Email) {
		errs = append(errs, "Invalid email address")
	}

	phoneDigits := digitsRe.ReplaceAllString(d.Phone, "")
	if len(phoneDigits) != 10 {
		errs = append(errs, "Phone number must be exactly 10 digits")
	}

	if len(strings.TrimSpace(d.StreetAddress)) < 5 {
		errs = append(errs, "Street address is required and must be at least 5 characters")
	}
	if len(strings.TrimSpace(d.City)) < 2 {
		errs = append(errs, "City is required")
	}
	if d.State != "MO" && d.State != "KS" {
		errs = append(errs, "State is required (MO or KS)")
	}

	zipDigits := digitsRe.ReplaceAllString(d.Zip, "")
	if len(zipDigits) != 5 && len(zipDigits) != 9 {
		errs = append(errs, "Zip code must be 5 or 9 digits")
	}

	return errs
}

func whichForm(source string) string {
	if source == "/free-estimate/ ads lander" {
		return source
	}
	return "Main Contact"
}

// appendAttribution stores a concise, allow-listed summary alongside the lead's
// comments. This keeps paid-click attribution available in Airtable without
// requiring a schema change to the existing Leads table.
func appendAttribution(message, raw string) string {
	if raw == "" {
		return message
	}

	var values map[string]string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return message
	}

	fields := []struct {
		key   string
		label string
	}{
		{"landing_page", "Landing page"},
		{"utm_source", "UTM source"},
		{"utm_medium", "UTM medium"},
		{"utm_campaign", "UTM campaign"},
		{"utm_content", "UTM content"},
		{"utm_term", "UTM term"},
		{"gclid", "Google click ID"},
		{"gbraid", "Google iOS click ID"},
		{"wbraid", "Google web-to-app ID"},
	}

	var lines []string
	for _, field := range fields {
		value := strings.TrimSpace(values[field.key])
		if value == "" {
			continue
		}
		value = strings.ReplaceAll(value, "\r", " ")
		value = strings.ReplaceAll(value, "\n", " ")
		if len(value) > 200 {
			value = value[:200]
		}
		lines = append(lines, fmt.Sprintf("%s: %s", field.label, value))
	}
	if len(lines) == 0 {
		return message
	}

	summary := "Lead attribution:\n" + strings.Join(lines, "\n")
	if message == "" {
		return summary
	}
	return message + "\n\n" + summary
}

// ── Client IP ──

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the last IP (closest to exe.dev proxy)
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	// Strip port
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i != -1 {
		return addr[:i]
	}
	return addr
}

// ── GeoIP Lookup ──

func countryForIP(ip string) (string, error) {
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,countryCode,message", ip)
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Status      string `json:"status"`
		CountryCode string `json:"countryCode"`
		Message     string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if result.Status != "success" {
		return "", fmt.Errorf("ip-api: %s", result.Message)
	}
	return result.CountryCode, nil
}

// ── Contact Handler ──

func handleContact(w http.ResponseWriter, r *http.Request) {
	// /contact/ is the Hugo page; keep this endpoint for form submissions only.
	// Redirect browser and crawler requests for the slashless URL to the canonical page.
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		http.Redirect(w, r, "/contact/", http.StatusMovedPermanently)
		return
	case http.MethodPost:
		// Continue to form processing below.
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead+", "+http.MethodPost)
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	ct := r.Header.Get("Content-Type")
	isForm := strings.Contains(ct, "application/x-www-form-urlencoded")
	isJSON := strings.Contains(ct, "application/json")

	var fd FormData

	if isJSON {
		if err := json.NewDecoder(r.Body).Decode(&fd); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, `{"error":"invalid form data"}`, http.StatusBadRequest)
			return
		}
		// Compute fill time from _loaded timestamp for no-JS submissions
		var fillTime float64
		if loaded, err := strconv.ParseInt(r.FormValue("_loaded"), 10, 64); err == nil && loaded > 0 {
			fillTime = float64(time.Now().UnixMilli()-loaded) / 1000.0
		}
		fd = FormData{
			FirstName:     strings.TrimSpace(r.FormValue("firstName")),
			LastName:      strings.TrimSpace(r.FormValue("lastName")),
			Email:         strings.TrimSpace(r.FormValue("email")),
			Phone:         strings.TrimSpace(r.FormValue("phone")),
			StreetAddress: strings.TrimSpace(r.FormValue("streetAddress")),
			City:          strings.TrimSpace(r.FormValue("city")),
			State:         strings.TrimSpace(r.FormValue("state")),
			Zip:           strings.TrimSpace(r.FormValue("zip")),
			Message:       strings.TrimSpace(r.FormValue("message")),
			FormSource:    strings.TrimSpace(r.FormValue("formSource")),
			Attribution:   strings.TrimSpace(r.FormValue("_attribution")),
			FillTime:      fillTime,
		}
	}

	ip := clientIP(r)

	// ── Spam checks ──

	// Timing check (0 means JS never ran — bot; <4s means filled too fast)
	if fd.FillTime < minFillTimeSeconds {
		log.Printf("REJECTED too_fast (%.1fs) from %s: %s %s", fd.FillTime, ip, fd.FirstName, fd.LastName)
		go recordSubmission(fd, ip, "too_fast", fmt.Sprintf("filled in %.1fs", fd.FillTime))
		// Still return a neutral success response to avoid tipping off bots.
		// It intentionally omits the lead-saved signal used by conversion tracking.
		respondSuccess(w, r, false)
		return
	}

	// Geo-IP check: reject non-US IPs
	if country, err := countryLookup(ip); err != nil {
		log.Printf("GeoIP lookup failed for %s: %v (allowing)", ip, err)
	} else if country != "US" {
		log.Printf("REJECTED non_us_ip from %s (country=%s): %s %s", ip, country, fd.FirstName, fd.LastName)
		go recordSubmission(fd, ip, "non_us_ip", fmt.Sprintf("country=%s", country))
		respondSuccess(w, r, false)
		return
	}

	// Rate limit check
	if !limiter.allow(ip) {
		log.Printf("REJECTED rate_limited from %s: %s %s", ip, fd.FirstName, fd.LastName)
		go recordSubmission(fd, ip, "rate_limited", fmt.Sprintf(">%d submissions/hour", maxSubmitsPerHour))
		respondSuccess(w, r, false)
		return
	}

	// ── Validation ──

	errs := validate(fd)

	if len(errs) > 0 {
		if isForm {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, errorHTML(fd, errs))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "errors": errs})
		return
	}

	fd.Message = appendAttribution(fd.Message, fd.Attribution)

	// ── Accepted ──

	log.Printf("Contact submission: %s %s <%s> %s | %s, %s, %s %s | msg: %s (%.1fs from %s)",
		fd.FirstName, fd.LastName, fd.Email, fd.Phone,
		fd.StreetAddress, fd.City, fd.State, fd.Zip, fd.Message,
		fd.FillTime, ip)

	if err := createLead(fd); err != nil {
		log.Printf("Airtable error: %v", err)
		respondAirtableFailure(w, r, fd)
		return
	}
	go recordSubmission(fd, ip, "accepted", "")

	respondSuccess(w, r, true)
}

func respondSuccess(w http.ResponseWriter, r *http.Request, leadSaved bool) {
	if leadSaved {
		// This header is the authoritative conversion signal for JavaScript clients.
		// It is set only after Airtable has confirmed lead creation.
		w.Header().Set("X-Lead-Saved", "true")
	}
	ct := r.Header.Get("Content-Type")
	isForm := strings.Contains(ct, "application/x-www-form-urlencoded")
	accept := r.Header.Get("Accept")

	if isForm {
		http.Redirect(w, r, "/thank-you/", http.StatusSeeOther)
		return
	}
	if strings.Contains(accept, "text/html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, thankYouHTML())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "Form submitted successfully"})
}

func respondAirtableFailure(w http.ResponseWriter, r *http.Request, fd FormData) {
	const message = "We couldn't save your request right now. Please try again in a moment."

	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/x-www-form-urlencoded") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, errorHTML(fd, []string{message}))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(map[string]any{"success": false, "error": message})
}

// ── Thank You HTML ──

func thankYouHTML() string {
	return `<div class="flow" style="--wrapper-max: 50rem">
    <section id="thank-you-next" class="thank-you-next region" aria-label="Next steps" style="scroll-margin-top: 6rem">
        <div class="wrapper flow" style="--wrapper-max: 50rem">
            <h2 data-balance>Thank you for your interest in our lighting!</h2>
            <p>Your request is in our inbox and we're probably already planning your installation. 🎄</p>
            <h3>What Happens Next</h3>
            <div class="thank-you-steps flow">
                <div class="thank-you-step">
                    <span class="thank-you-step__number">1</span>
                    <h4>We Review Your Request</h4>
                    <p>We'll carefully review all the details you provided about your property and project.</p>
                </div>
                <div class="thank-you-step">
                    <span class="thank-you-step__number">2</span>
                    <h4>We'll Reach Out Within 48 Hours</h4>
                    <p>Angela will contact you by phone and/or email with your custom estimate and next steps.</p>
                </div>
                <div class="thank-you-step">
                    <span class="thank-you-step__number">3</span>
                    <h4>Book Your Installation</h4>
                    <p>If you love the estimate, we'll get you in the scheduling queue and get you an invoice and schedule as soon as we can!</p>
                    <p style="margin-block-start: 1rem;">You'll also want to check out our <a href="/gallery/">gallery</a> to get an idea of what color combination you're going to want.</p>
                </div>
            </div>
        </div>
    </section>
    <section class="thank-you-links region bg-light" aria-label="Quick links">
        <div class="wrapper text-center flow" style="--wrapper-max: 50rem">
            <h3>In the Meantime</h3>
            <p>Check out our work and learn more about what we do:</p>
            <div class="thank-you-nav cluster" style="justify-content: center; --cluster-space: var(--space-s)">
                <a href="/gallery/" class="button">See Our Work</a>
                <a href="/services/" class="button">See Our Services</a>
                <a href="/faq/" class="button">Read FAQ</a>
            </div>
        </div>
    </section>
</div>`
}

// ── Airtable: Create Lead ──

func sendToAirtable(fd FormData) error {
	zipDigits := digitsRe.ReplaceAllString(fd.Zip, "")
	zipNum, _ := strconv.Atoi(zipDigits)

	payload := map[string]any{
		"typecast": true,
		"records": []map[string]any{
			{
				"fields": map[string]any{
					"fldZWX56UF9UNbZOW": whichForm(fd.FormSource), // Which Form
					"fldplXExIaztUlnVf": fd.FirstName,             // First Name
					"fldiVRdwdOumpsrCh": fd.LastName,              // Last Name
					"fldsvJF0WoUqKWOtq": fd.Email,                 // Email
					"fldGCBMLm7Ks1KD6N": fd.Phone,                 // Phone
					"flduHz8NtmIzaTakp": fd.StreetAddress,         // Street Address
					"fldRHrzLnl0dIEScW": fd.City,                  // City
					"flddThKQZrXUkq2LD": fd.State,                 // State
					"fldSe1UzJLxSb5yWW": zipNum,                   // Zip Code
					"fldGhAjDinMRV827I": fd.Message,               // Comments
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	url := fmt.Sprintf("%s/v0/%s/%s", airtableProxy, airtableBaseID, leadsTableID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("airtable returned %d: %s", resp.StatusCode, string(respBody))
	}

	log.Printf("Airtable lead created for %s %s", fd.FirstName, fd.LastName)
	return nil
}

// ── Airtable: Log Submission ──

func logSubmission(fd FormData, ip, status, reason string) {
	payload := map[string]any{
		"typecast": true,
		"records": []map[string]any{
			{
				"fields": map[string]any{
					"fldi3M07M8XzEdfDD": time.Now().UTC().Format(time.RFC3339), // Timestamp
					"fld5n4yX9VN3USNnH": ip,                                    // IP
					"fldEUrVMiJTBtyNnq": fd.FirstName,                          // First Name
					"fldpXJarQJ6fJD2EB": fd.LastName,                           // Last Name
					"fldoQv9Y8pkU2gYkE": fd.Email,                              // Email
					"fldt4QvDtl5rsUVKj": fd.Phone,                              // Phone
					"fldeB18affBf7eZER": fd.StreetAddress,                      // Street Address
					"fldRYgmzZvxbIUH9S": fd.City,                               // City
					"fldFUE3CyNQnp3N2j": fd.State,                              // State
					"fldDVWQ86NqW0Qobq": fd.Zip,                                // Zip
					"fldkHpBAQg67Ld1CM": fd.Message,                            // Message
					"fld0HcH3I7kqxUmI5": status,                                // Status
					"fldNklj8CcWdUuii4": reason,                                // Rejection Reason
					"fldXmciOEUg3mc0m8": fd.FillTime,                           // Fill Time Seconds
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Log marshal error: %v", err)
		return
	}

	url := fmt.Sprintf("%s/v0/%s/%s", airtableProxy, airtableBaseID, logTableID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("Log request error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("Log send error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("Log airtable error %d: %s", resp.StatusCode, string(respBody))
	}
}

// ── Error HTML ──

func errorHTML(d FormData, errs []string) string {
	var errItems strings.Builder
	for _, e := range errs {
		fmt.Fprintf(&errItems, "<li>%s</li>", html.EscapeString(e))
	}

	sel := func(val, opt string) string {
		if val == opt {
			return "selected"
		}
		return ""
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Form Errors — Tis The Season</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:system-ui,-apple-system,sans-serif;background:#f5f5f0;padding:2rem}
.c{max-width:50rem;margin:0 auto;background:#fff;padding:2rem;border-radius:.5rem;box-shadow:0 1px 3px rgba(0,0,0,.08)}
.eb{background:rgba(197,48,48,.1);border-left:4px solid #c53030;padding:1rem;margin-bottom:2rem;border-radius:.25rem}
.eb h2{color:#c53030;font-size:1.1rem;margin-bottom:.5rem}
.eb ul{list-style:none;margin-left:1rem}
.eb li:before{content:"✗ ";color:#c53030;font-weight:700;margin-right:.5rem}
form{display:flex;flex-direction:column;gap:1rem}
label{font-weight:600;font-size:.9rem}
input,select,textarea{padding:.625rem .75rem;border:1px solid #e0e0d8;border-radius:.25rem;font-size:1rem;font-family:inherit;width:100%%}
input:focus,select:focus,textarea:focus{outline:2px solid #69C4BB;outline-offset:1px;border-color:#69C4BB}
textarea{resize:vertical;min-height:6rem}
.r{display:grid;grid-template-columns:1fr 1fr;gap:1rem}
.ra{grid-template-columns:2fr 1fr 1fr}
btn,button[type=submit]{background:#69C4BB;color:#fff;border:none;padding:.875rem 1.5rem;border-radius:.25rem;font-weight:600;cursor:pointer;font-size:1rem}
btn:hover,button[type=submit]:hover{background:#52a89e}
.bl{display:block;margin-top:1rem;color:#69C4BB;text-decoration:none}
.bl:hover{text-decoration:underline}
</style></head><body>
<div class="c">
<div class="eb"><h2>Please Fix These Errors</h2><ul>%s</ul></div>
<form method="POST" action="/contact">
<div class="r"><div><label for="first-name">First Name</label><input type="text" id="first-name" name="firstName" required value="%s"></div>
<div><label for="last-name">Last Name</label><input type="text" id="last-name" name="lastName" required value="%s"></div></div>
<div class="r"><div><label for="email">Email</label><input type="email" id="email" name="email" required value="%s"></div>
<div><label for="phone">Phone Number</label><input type="tel" id="phone" name="phone" required value="%s"></div></div>
<div><label for="street-address">Street Address</label><input type="text" id="street-address" name="streetAddress" required value="%s"></div>
<div class="r ra"><div><label for="city">City</label><input type="text" id="city" name="city" required value="%s"></div>
<div><label for="state">State</label><select id="state" name="state" required><option value="">Select...</option><option value="MO" %s>MO</option><option value="KS" %s>KS</option></select></div>
<div><label for="zip">Zip Code</label><input type="text" id="zip" name="zip" required value="%s"></div></div>
<div><label for="message">Message</label><textarea id="message" name="message">%s</textarea></div>
<input type="hidden" name="_loaded" id="_loaded" value="">
<button type="submit">Submit Request</button>
<script>document.getElementById('_loaded').value = Date.now();</script>
<a href="/contact/" class="bl">← Back to Contact Form</a>
</form></div></body></html>`,
		errItems.String(),
		html.EscapeString(d.FirstName),
		html.EscapeString(d.LastName),
		html.EscapeString(d.Email),
		html.EscapeString(d.Phone),
		html.EscapeString(d.StreetAddress),
		html.EscapeString(d.City),
		sel(d.State, "MO"),
		sel(d.State, "KS"),
		html.EscapeString(d.Zip),
		html.EscapeString(d.Message),
	)
}

// ── Main + Static Serving ──

// wwwRedirect redirects www.tistheseasonkc.com → tistheseasonkc.com
func wwwRedirect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if strings.HasPrefix(host, "www.") {
			target := "https://" + strings.TrimPrefix(host, "www.") + r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func termsRedirect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/terms-and-conditions" {
			http.Redirect(w, r, "/terms/", http.StatusMovedPermanently)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// markdownNegotiationMiddleware intercepts requests for HTML pages and serves
// the corresponding markdown file (index.md) when the client sends
// Accept: text/markdown. This lets AI agents that support content negotiation
// pull clean markdown instead of parsing HTML. It also sets a Link header
// pointing at /llms.txt on every response so crawlers can discover it.
func markdownNegotiationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always advertise llms.txt
		w.Header().Set("Link", `</llms.txt>; rel="describedby"; type="text/markdown"`)

		// Only negotiate for GET/HEAD requests that accept markdown
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}

		accept := r.Header.Get("Accept")
		if !strings.Contains(accept, "text/markdown") {
			next.ServeHTTP(w, r)
			return
		}

		p := r.URL.Path
		// Skip non-page paths (assets, API routes, etc.)
		if strings.HasSuffix(p, ".css") || strings.HasSuffix(p, ".js") ||
			strings.HasSuffix(p, ".png") || strings.HasSuffix(p, ".jpg") ||
			strings.HasSuffix(p, ".jpeg") || strings.HasSuffix(p, ".webp") ||
			strings.HasSuffix(p, ".svg") || strings.HasSuffix(p, ".ico") ||
			strings.HasSuffix(p, ".xml") || strings.HasSuffix(p, ".json") ||
			strings.HasSuffix(p, ".pdf") || strings.HasSuffix(p, ".woff") ||
			strings.HasSuffix(p, ".woff2") || strings.HasSuffix(p, ".map") ||
			strings.HasPrefix(p, "/analytics/") || strings.HasPrefix(p, "/photos/") ||
			strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/internal") ||
			p == "/robots.txt" || p == "/llms.txt" || p == "/sitemap.xml" {
			next.ServeHTTP(w, r)
			return
		}

		// Look for index.md in the same directory
		mdPath := "public" + p
		if !strings.HasSuffix(mdPath, "/") {
			mdPath += "/"
		}
		mdPath += "index.md"

		if _, err := os.Stat(mdPath); err == nil {
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			http.ServeFile(w, r, mdPath)
			return
		}

		// No markdown available — fall through to HTML
		next.ServeHTTP(w, r)
	})
}

func main() {
	// Initialize Sentry first so panics during startup are captured.
	// No-op if SENTRY_DSN is unset (local dev). Flush on exit.
	services.InitSentry()
	defer services.FlushSentry(2 * time.Second)

	// Load config — reads APP_ENV and Stripe keys, logs active mode
	cfg := services.LoadConfig()

	loadCustom404()

	fs := http.FileServer(http.Dir("public"))

	mux := http.NewServeMux()

	// Existing routes
	mux.HandleFunc("/contact", handleContact)

	// Payment services
	mux.HandleFunc("/pay", services.CheckoutHandler(cfg))
	mux.HandleFunc("/stripe/webhook", services.WebhookHandler(cfg))
	mux.HandleFunc("/status", services.StatusHandler(cfg))

	// Estimate email (internal — protected by WEBHOOK_AUTH_KEY)
	mux.HandleFunc("/estimate/send", services.EstimateHandler(cfg))
	mux.HandleFunc("/confirmation/send", services.ConfirmationHandler(cfg))
	mux.HandleFunc("/oos/send", services.OOSHandler(cfg))

	// Sold sync + invoice automation (internal — protected by WEBHOOK_AUTH_KEY)
	mux.HandleFunc("/sold/sync", services.SoldSyncHandler(cfg))
	mux.HandleFunc("/invoice/create", services.InvoiceHandler(cfg))

	// Bot / AI-crawler crawl dashboard on the PUBLIC mux — still bearer-authed
	// so it's reachable via curl with the WEBHOOK_AUTH_KEY from anywhere.
	// (A token-free, one-click version runs on the internal listener below.)
	mux.HandleFunc("/internal/bots", services.BotDashboardHandler(cfg, true))

	// Serve downloaded estimate photos (permanent URLs embedded in emails)
	// Umami analytics (first-party): proxy the tracking script + collect
	// endpoint through the public server so visitor browsers can reach it.
	mux.Handle("/analytics/", services.UmamiProxyHandler())

	mux.HandleFunc("/photos/", services.PhotoHandler())

	// Static site (catch-all — must be last)
	mux.Handle("/", cacheMiddleware(notFoundMiddleware(fs)))

	log.Println("Serving on :8000")

	// Internal-only listener: a separate mux on a private port (default :3001)
	// for dashboards/tools meant only for the VM owner. exe.dev proxies alternate
	// ports privately — only users with VM access can reach them, gated by their
	// exe.dev login. That login gate IS the auth, so these handlers skip the
	// bearer-token check, keeping the dashboard a clean one-click bookmark with
	// no secret in the URL or logs. Set INTERNAL_PORT=0 to disable.
	internalPort := os.Getenv("INTERNAL_PORT")
	if internalPort == "" {
		internalPort = "3001"
	}
	if internalPort != "0" {
		imux := http.NewServeMux()
		imux.HandleFunc("/internal/bots", services.BotDashboardHandler(cfg, false))
		imux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><title>TTS Internal</title>"+
				"<style>body{font:15px/1.5 -apple-system,Segoe UI,Roboto,sans-serif;background:#0f1115;color:#e6e6e6;padding:40px}"+
				"h1{font-size:18px}a{color:#6fb6ff}code{background:#1a1d24;padding:2px 6px;border-radius:4px}</style>"+
				"<h1>TTS — Internal tools</h1>"+
				"<ul><li><a href=\"/internal/bots\">🤖 Bot Crawl Dashboard</a></li></ul>"+
				"<p style=color:#8a93a4;font-size:13px>Private to VM owner — gated by exe.dev login.</p>")
		})
		go func() {
			log.Printf("Internal listener on :%s (private — exe.dev-gated)", internalPort)
			if err := http.ListenAndServe(":"+internalPort, services.SentryRecovery(imux)); err != nil {
				log.Printf("[internal] listen :%s failed: %v", internalPort, err)
			}
		}()
	}

	log.Fatal(http.ListenAndServe(":8000",
		services.SentryRecovery(
			securityHeadersMiddleware(markdownNegotiationMiddleware(termsRedirect(wwwRedirect(services.BotTrackMiddleware(mux))))))))
}

// notFoundWriter buffers the FileServer's response just long enough to see
// whether it's about to emit a 404. If so, we discard its plain-text body
// and substitute Hugo's styled public/404.html instead — while keeping the
// real 404 status code intact, so bot-crawl gap detection still sees it.
type notFoundWriter struct {
	http.ResponseWriter
	status     int
	triggered  bool
	suppressed bool
}

func (w *notFoundWriter) WriteHeader(code int) {
	w.status = code
	if code == http.StatusNotFound {
		w.triggered = true
		// Don't forward the header yet — notFoundMiddleware will send its own
		// once the FileServer is done, after loading the custom 404 body.
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *notFoundWriter) Write(b []byte) (int, error) {
	if w.triggered {
		// Swallow the FileServer's default "404 page not found" body.
		w.suppressed = true
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}

var custom404Body []byte

func loadCustom404() {
	b, err := os.ReadFile("public/404.html")
	if err != nil {
		log.Printf("[404] no custom 404 page found at public/404.html: %v", err)
		return
	}
	custom404Body = b
}

// notFoundMiddleware serves Hugo's styled 404.html with a real HTTP 404
// status whenever the wrapped handler (the static FileServer) would
// otherwise return its bare-text 404. This keeps status codes truthful for
// bot-crawl gap detection while giving humans a real page with a way home.
func notFoundMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nfw := &notFoundWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(nfw, r)
		if nfw.triggered {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			if len(custom404Body) > 0 {
				w.Write(custom404Body)
			} else {
				w.Write([]byte("404 page not found"))
			}
		}
	})
}

func cacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, ".css"), strings.HasSuffix(p, ".js"),
			strings.HasSuffix(p, ".woff2"), strings.HasSuffix(p, ".woff"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case strings.HasSuffix(p, ".png"), strings.HasSuffix(p, ".jpg"),
			strings.HasSuffix(p, ".jpeg"), strings.HasSuffix(p, ".webp"),
			strings.HasSuffix(p, ".svg"), strings.HasSuffix(p, ".avif"):
			w.Header().Set("Cache-Control", "public, max-age=2592000")
		default:
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		next.ServeHTTP(w, r)
	})
}
