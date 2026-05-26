package services

// ConfirmationHandler — POST /confirmation/send
//
// Triggered by an Airtable automation when a lead is marked "Sold".
// Sends a confirmation email to the lead with onboarding instructions.
//
//  1. Verify Bearer token (WEBHOOK_AUTH_KEY)
//  2. Parse JSON body: {"recordId": "recXXXX"}
//  3. Fetch lead from Airtable (leads.go)
//  4. Render confirmation.html template
//  5. Send via Gmail SMTP
//  6. Return JSON response

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"strings"
)

// ConfirmationEmailData is passed to the confirmation.html template.
type ConfirmationEmailData struct {
	FirstName string
}

// ConfirmationHandler returns the http.HandlerFunc for POST /confirmation/send.
func ConfirmationHandler(cfg *Config) http.HandlerFunc {
	tmpl := template.Must(
		template.New("confirmation").ParseFiles("services/email_templates/confirmation.html"),
	)

	return func(w http.ResponseWriter, r *http.Request) {
		// ── 1. Method check ────────────────────────────────────────────────────
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// ── 2. Auth check ──────────────────────────────────────────────────────
		// TODO: same pattern as EstimateHandler — check Authorization: Bearer WEBHOOK_AUTH_KEY
		authHeader := r.Header.Get("Authorization")
		if strings.TrimPrefix(authHeader, "Bearer ") != os.Getenv("WEBHOOK_AUTH_KEY") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}

		// ── 3. Parse body ──────────────────────────────────────────────────────
		// TODO: decode JSON body into a struct with a RecordID field
		// hint: same pattern as EstimateHandler
		var req estimateRequest // reuse the same struct — same shape
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.RecordID == "" {
			http.Error(w, "recordId is required", http.StatusBadRequest)
			return
		}

		// ── 4. Fetch lead ──────────────────────────────────────────────────────
		// TODO: call GetLeadByRecordID and handle errors
		lead, err := GetLeadByRecordID(req.RecordID)
		if err != nil {
			log.Printf("[confirmation] fetch lead failed: %v", err)
			http.Error(w, "failed to fetch lead", http.StatusInternalServerError)
			return
		}
		if lead == nil {
			http.Error(w, "lead not found", http.StatusNotFound)
			return
		}

		// ── 5. Render template ─────────────────────────────────────────────────
		data := ConfirmationEmailData{
			FirstName: lead.FirstName,
		}
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "confirmation.html", data); err != nil {
			log.Printf("[confirmation] template render failed: %v", err)
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}

		// ── 6. Send email ──────────────────────────────────────────────────────
		// TODO: call sendConfirmationEmail
		if err := sendConfirmationEmail(lead.Email, lead.FirstName, buf.Bytes()); err != nil {
			log.Printf("[confirmation] email send failed: %v", err)
			http.Error(w, `{"error":"email send failed"}`, http.StatusInternalServerError)
			return
		}

		log.Printf("[confirmation] sent to %s %s (%s)", lead.FirstName, lead.LastName, lead.Email)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}
}

// sendConfirmationEmail sends the rendered HTML via Gmail SMTP.
func sendConfirmationEmail(to, firstName string, body []byte) error {
	user := os.Getenv("GMAIL_USER")
	from := os.Getenv("GMAIL_SEND_AS")
	pass := os.Getenv("GMAIL_APP_PASSWORD")
	if user == "" {
		user = from // backward compat
	}
	if from == "" || pass == "" {
		return fmt.Errorf("GMAIL_SEND_AS or GMAIL_APP_PASSWORD not set")
	}

	subject := fmt.Sprintf("\U0001f384 You're Confirmed, %s! \U0001f384", firstName)

	var msg bytes.Buffer
	msg.WriteString("From: Tis The Season KC <" + from + ">\r\n")
	msg.WriteString("To: " + to + "\r\n")
	msg.WriteString("Subject: " + subject + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	msg.WriteString("\r\n")
	msg.Write(body)

	auth := smtp.PlainAuth("", user, pass, "smtp.gmail.com")
	return smtp.SendMail("smtp.gmail.com:587", auth, from, []string{to}, msg.Bytes())
}
