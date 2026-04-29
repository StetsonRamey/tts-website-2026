package main

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"regexp"
	"strings"
)

type FormData struct {
	FirstName     string `json:"firstName"`
	LastName      string `json:"lastName"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	StreetAddress string `json:"streetAddress"`
	City          string `json:"city"`
	State         string `json:"state"`
	Zip           string `json:"zip"`
	Message       string `json:"message"`
}

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
var digitsRe = regexp.MustCompile(`\D`)

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
		}
	}

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

	// Log the submission (Airtable integration goes here later)
	log.Printf("Contact submission: %s %s <%s> %s | %s, %s, %s %s | msg: %s",
		fd.FirstName, fd.LastName, fd.Email, fd.Phone,
		fd.StreetAddress, fd.City, fd.State, fd.Zip, fd.Message)

	if isForm {
		http.Redirect(w, r, "/thank-you/", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "Form submitted successfully"})
}

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
<button type="submit">Submit Request</button>
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

func main() {
	// Static file server with cache headers
	fs := http.FileServer(http.Dir("public"))

	mux := http.NewServeMux()
	mux.HandleFunc("/contact", handleContact)
	mux.Handle("/", cacheMiddleware(fs))

	log.Println("Serving on :8000")
	log.Fatal(http.ListenAndServe(":8000", mux))
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
