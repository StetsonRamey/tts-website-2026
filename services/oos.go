package services

// OOSHandler — POST /oos/send
//
// Triggered by an Airtable automation when a lead is marked out of service area.
// Sends a plain-text email letting them know we can't help.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"strings"
)

// OOSHandler returns the http.HandlerFunc for POST /oos/send.
func OOSHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if strings.TrimPrefix(authHeader, "Bearer ") != os.Getenv("WEBHOOK_AUTH_KEY") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}

		var req estimateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.RecordID == "" {
			http.Error(w, "recordId is required", http.StatusBadRequest)
			return
		}

		lead, err := GetLeadByRecordID(req.RecordID)
		if err != nil {
			log.Printf("[oos] fetch lead failed: %v", err)
			http.Error(w, "failed to fetch lead", http.StatusInternalServerError)
			return
		}
		if lead == nil {
			http.Error(w, "lead not found", http.StatusNotFound)
			return
		}

		if err := sendOOSEmail(lead.Email, lead.FirstName); err != nil {
			log.Printf("[oos] email send failed: %v", err)
			http.Error(w, `{"error":"email send failed"}`, http.StatusInternalServerError)
			return
		}

		log.Printf("[oos] sent to %s %s (%s)", lead.FirstName, lead.LastName, lead.Email)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}
}

func sendOOSEmail(to, firstName string) error {
	from := os.Getenv("GMAIL_SEND_AS")
	pass := os.Getenv("GMAIL_APP_PASSWORD")
	if from == "" || pass == "" {
		return fmt.Errorf("GMAIL_SEND_AS or GMAIL_APP_PASSWORD not set")
	}

	body := fmt.Sprintf(
		"Hi %s. Unfortunately you are out of our service area so we can't provide you with an estimate. "+
			"We appreciate you finding us and visiting our website. Have a great holiday!\r\n\r\nThank you \U0001f384",
		firstName,
	)

	var msg bytes.Buffer
	msg.WriteString("From: Tis The Season KC <" + from + ">\r\n")
	msg.WriteString("To: " + to + "\r\n")
	msg.WriteString("Subject: Thanks for Contacting Us!\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	auth := smtp.PlainAuth("", from, pass, "smtp.gmail.com")
	return smtp.SendMail("smtp.gmail.com:587", auth, from, []string{to}, msg.Bytes())
}
