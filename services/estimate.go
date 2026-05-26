package services

// EstimateHandler — POST /estimate/send
//
// Triggered by an Airtable script (or curl) when Angela is ready to send
// an estimate to a lead. Flow:
//
//  1. Verify Bearer token (WEBHOOK_AUTH_KEY)
//  2. Parse JSON body: {"recordId": "recXXXX"}
//  3. Fetch lead from Airtable (leads.go)
//  4. Download photos from Airtable CDN → save to disk → build public URLs
//  5. Render estimate.html template
//  6. Send via Gmail SMTP
//  7. Return JSON response

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
)

const (
	// Where downloaded photos are stored on disk.
	// Created on first use if it doesn't exist.
	photoDir = "/var/lib/tts/photos"

	// Public base URL for serving photos.
	// Route GET /photos/{file} must be registered in server.go.
	photoBaseURL = "https://tistheseasonkc.com/photos"
)

// EstimateEmailData is passed to the estimate.html template.
type EstimateEmailData struct {
	FirstName   string
	Feet        float64
	PriceLED    float64
	PriceRehang float64
	PhotoURLs   []string // permanent public URLs, ready to embed in email
}

// estimateRequest is the JSON body we expect.
type estimateRequest struct {
	RecordID string `json:"recordId"`
}

// EstimateHandler returns the http.HandlerFunc for POST /estimate/send.
func EstimateHandler(cfg *Config) http.HandlerFunc {
	// Load and parse the template once at startup, not on every request.
	tmpl := template.Must(
		template.New("estimate").Funcs(template.FuncMap{
			// formatCurrency formats a float as "$1,234.56".
			// Available in the template as {{formatCurrency .PriceLED}}
			"formatCurrency": func(n float64) string {
				return fmt.Sprintf("$%.2f", n)
			},
		}).ParseFiles("services/email_templates/estimate.html"),
	)

	return func(w http.ResponseWriter, r *http.Request) {
		// ── 1. Method check ────────────────────────────────────────────────────
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// ── 2. Auth check ──────────────────────────────────────────────────────
		// TODO: read the Authorization header
		// TODO: compare to "Bearer " + os.Getenv("WEBHOOK_AUTH_KEY")
		// TODO: return 401 JSON if it doesn't match
		// hint: use strings.TrimPrefix(header, "Bearer ") to extract the token
		authHeader := r.Header.Get("Authorization")
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token != os.Getenv("WEBHOOK_AUTH_KEY") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		

		// ── 3. Parse request body ──────────────────────────────────────────────
		var req estimateRequest
		// TODO: json.NewDecoder(r.Body).Decode(&req)
		// TODO: return 400 if recordId is empty
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.RecordID == "" {
			http.Error(w, "recordId is required", http.StatusBadRequest)
			return
		}

		// ── 4. Fetch lead from Airtable ────────────────────────────────────────
		// TODO: call GetLeadByRecordID(req.RecordID)
		// TODO: return 500 on error
		lead, err := GetLeadByRecordID(req.RecordID)
		if err != nil {
			log.Printf("[estimate] fetch lead failed: %v", err)
			http.Error(w, "failed to fetch lead", http.StatusInternalServerError)
			return
		}
		if lead == nil {
			http.Error(w, "lead not found", http.StatusNotFound)
			return
		}

		// ── 5. Download photos → disk → public URLs ────────────────────────────
		publicURLs, err := stagePhotos(lead.Photos, lead.FirstName+" "+lead.LastName)
		if err != nil {
			// Non-fatal: log and continue with no photos rather than failing
			log.Printf("[estimate] photo staging failed: %v", err)
			publicURLs = nil
		}

		// ── 6. Render template ─────────────────────────────────────────────────
		data := EstimateEmailData{
			// TODO: populate from lead fields
			PhotoURLs: publicURLs,
			FirstName: lead.FirstName,
			Feet:      lead.Feet,
			PriceLED:  lead.PriceLED,
			PriceRehang: lead.PriceRehang,
		}

		var htmlBuf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&htmlBuf, "estimate.html", data); err != nil {
			log.Printf("[estimate] template render failed: %v", err)
			http.Error(w, `{"error":"template error"}`, http.StatusInternalServerError)
			return
		}

		// ── 7. Send email ──────────────────────────────────────────────────────
		// TODO: build subject line using lead.FirstName
		subject := fmt.Sprintf("Holiday Lighting Estimate - %s", lead.FirstName)
		// TODO: call sendHTMLEmail(lead.Email, subject, htmlBuf.String())
		// TODO: return 500 on error
		if err := sendHTMLEmail(lead.Email, subject, htmlBuf.String()); err != nil {
			log.Printf("[estimate] email send failed: %v", err)
			http.Error(w, `{"error":"email send failed"}`, http.StatusInternalServerError)
			return
		}
		

		// ── 8. Respond ─────────────────────────────────────────────────────────
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":   true,
			"recipient": lead.Email,
			"subject":   subject,
		})
	}
}

// stagePhotos downloads each photo from its temporary Airtable URL, saves it
// to photoDir with a content-hash filename, and returns the permanent public URLs.
func stagePhotos(photos []LeadPhoto, fullName string) ([]string, error) {
	if err := os.MkdirAll(photoDir, 0755); err != nil {
		return nil, fmt.Errorf("create photo dir: %w", err)
	}

	var publicURLs []string

	for _, photo := range photos {
		// ── Download ───────────────────────────────────────────────────────────
		// TODO: http.Get(photo.URL)
		// TODO: defer resp.Body.Close()
		resp, err := http.Get(photo.URL) //nolint:noctx
		if err != nil {
			log.Printf("[estimate] photo download failed: %v", err)
			continue
		}
		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close() // close immediately, not deferred — defer inside loop runs at function return
		if err != nil {
			log.Printf("[estimate] photo read failed: %v", err)
			continue
		}
		filename := photoFilename(bodyBytes, photo.Filename, fullName)

		// ── Save to disk ───────────────────────────────────────────────────────
		// TODO: os.WriteFile(filepath.Join(photoDir, filename), data, 0644)
		if err := os.WriteFile(filepath.Join(photoDir, filename), bodyBytes, 0644); err != nil {
			log.Printf("[estimate] photo save failed: %v", err)
			continue
		}

		// ── Build public URL ───────────────────────────────────────────────────
		// TODO: append photoBaseURL + "/" + filename to publicURLs
		publicURLs = append(publicURLs, photoBaseURL+"/"+filename)

	}

	return publicURLs, nil
}

// sendHTMLEmail sends an HTML email via Gmail SMTP.
// Credentials come from GMAIL_SEND_AS and GMAIL_APP_PASSWORD env vars.
func sendHTMLEmail(to, subject, htmlBody string) error {
	from := os.Getenv("GMAIL_SEND_AS")
	pass := os.Getenv("GMAIL_APP_PASSWORD")
	if from == "" || pass == "" {
		return fmt.Errorf("GMAIL_SEND_AS or GMAIL_APP_PASSWORD not set")
	}

	var buf bytes.Buffer
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("To: " + to + "\r\n")
	buf.WriteString("From: Tis The Season KC <" + from + ">\r\n")
	buf.WriteString("Subject: " + subject + "\r\n")
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(htmlBody)

	auth := smtp.PlainAuth("", from, pass, "smtp.gmail.com")
	return smtp.SendMail("smtp.gmail.com:587", auth, from, []string{to}, buf.Bytes())
}

// photoFilename builds a stable, human-readable filename for a downloaded photo:
// "{slug}-{hash16}{ext}" e.g. "angela-smith-a3f2b1c4d5e6f7a8.jpg"
func photoFilename(data []byte, originalName, fullName string) string {
	hash := sha256.Sum256(data)
	ext := filepath.Ext(originalName)
	if ext == "" {
		ext = ".jpg"
	}
	return fmt.Sprintf("%s-%x%s", slugify(fullName), hash[:8], ext)
}

// slugify converts a name to a lowercase hyphenated slug: "Angela Smith" → "angela-smith"
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.NewReplacer(" ", "-", "'", "", ".", "").Replace(s)
}

// PhotoHandler serves files from photoDir.
// Register as GET /photos/{file} in server.go.
func PhotoHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Strip /photos/ prefix to get just the filename
		filename := strings.TrimPrefix(r.URL.Path, "/photos/")
		if filename == "" || strings.Contains(filename, "/") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.ServeFile(w, r, filepath.Join(photoDir, filename))
	}
}
