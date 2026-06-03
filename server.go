package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
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
	FillTime      float64 `json:"_fillTime"`
}

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
var digitsRe = regexp.MustCompile(`\D`)

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
	if r.Method != http.MethodPost {
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
			FillTime:      fillTime,
		}
	}

	ip := clientIP(r)

	// ── Spam checks ──

	// Timing check (0 means JS never ran — bot; <4s means filled too fast)
	if fd.FillTime < minFillTimeSeconds {
		log.Printf("REJECTED too_fast (%.1fs) from %s: %s %s", fd.FillTime, ip, fd.FirstName, fd.LastName)
		go logSubmission(fd, ip, "too_fast", fmt.Sprintf("filled in %.1fs", fd.FillTime))
		// Still return success to not tip off bots
		respondSuccess(w, r)
		return
	}

	// Geo-IP check: reject non-US IPs
	if country, err := countryForIP(ip); err != nil {
		log.Printf("GeoIP lookup failed for %s: %v (allowing)", ip, err)
	} else if country != "US" {
		log.Printf("REJECTED non_us_ip from %s (country=%s): %s %s", ip, country, fd.FirstName, fd.LastName)
		go logSubmission(fd, ip, "non_us_ip", fmt.Sprintf("country=%s", country))
		respondSuccess(w, r)
		return
	}

	// Rate limit check
	if !limiter.allow(ip) {
		log.Printf("REJECTED rate_limited from %s: %s %s", ip, fd.FirstName, fd.LastName)
		go logSubmission(fd, ip, "rate_limited", fmt.Sprintf(">%d submissions/hour", maxSubmitsPerHour))
		respondSuccess(w, r)
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

	// ── Accepted ──

	log.Printf("Contact submission: %s %s <%s> %s | %s, %s, %s %s | msg: %s (%.1fs from %s)",
		fd.FirstName, fd.LastName, fd.Email, fd.Phone,
		fd.StreetAddress, fd.City, fd.State, fd.Zip, fd.Message,
		fd.FillTime, ip)

	go func() {
		if err := sendToAirtable(fd); err != nil {
			log.Printf("Airtable error: %v", err)
		}
	}()
	go logSubmission(fd, ip, "accepted", "")

	respondSuccess(w, r)
}

func respondSuccess(w http.ResponseWriter, r *http.Request) {
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
					"fldZWX56UF9UNbZOW": "Main Contact",  // Which Form
					"fldplXExIaztUlnVf": fd.FirstName,     // First Name
					"fldiVRdwdOumpsrCh": fd.LastName,      // Last Name
					"fldsvJF0WoUqKWOtq": fd.Email,         // Email
					"fldGCBMLm7Ks1KD6N": fd.Phone,         // Phone
					"flduHz8NtmIzaTakp": fd.StreetAddress, // Street Address
					"fldRHrzLnl0dIEScW": fd.City,          // City
					"flddThKQZrXUkq2LD": fd.State,         // State
					"fldSe1UzJLxSb5yWW": zipNum,           // Zip Code
					"fldGhAjDinMRV827I": fd.Message,       // Comments
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
					"fld5n4yX9VN3USNnH": ip,                                   // IP
					"fldEUrVMiJTBtyNnq": fd.FirstName,                         // First Name
					"fldpXJarQJ6fJD2EB": fd.LastName,                          // Last Name
					"fldoQv9Y8pkU2gYkE": fd.Email,                             // Email
					"fldt4QvDtl5rsUVKj": fd.Phone,                             // Phone
					"fldeB18affBf7eZER": fd.StreetAddress,                     // Street Address
					"fldRYgmzZvxbIUH9S": fd.City,                              // City
					"fldFUE3CyNQnp3N2j": fd.State,                             // State
					"fldDVWQ86NqW0Qobq": fd.Zip,                               // Zip
					"fldkHpBAQg67Ld1CM": fd.Message,                           // Message
					"fld0HcH3I7kqxUmI5": status,                               // Status
					"fldNklj8CcWdUuii4": reason,                               // Rejection Reason
					"fldXmciOEUg3mc0m8": fd.FillTime,                          // Fill Time Seconds
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

func main() {
	// Load config — reads APP_ENV and Stripe keys, logs active mode
	cfg := services.LoadConfig()

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

	// Serve downloaded estimate photos (permanent URLs embedded in emails)
	mux.HandleFunc("/photos/", services.PhotoHandler())

	// Static site (catch-all — must be last)
	mux.Handle("/", cacheMiddleware(fs))

	log.Println("Serving on :8000")
	log.Fatal(http.ListenAndServe(":8000", securityHeadersMiddleware(termsRedirect(wwwRedirect(mux)))))
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
