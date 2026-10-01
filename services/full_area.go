package services

// FullAreaHandler — POST /full
//
// Triggered by an Airtable automation when a lead's area is full. The
// automation supplies firstName and email from the Leads record.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"strings"
)

type fullAreaRequest struct {
	FirstName string `json:"firstName"`
	Email     string `json:"email"`
}

const fullAreaEmailSubject = "Thanks for Contacting Us!"

func FullAreaHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !RequireBearerAuth(w, r) {
			return
		}

		var req fullAreaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		req.FirstName = strings.TrimSpace(req.FirstName)
		if req.FirstName == "" || strings.TrimSpace(req.Email) == "" {
			http.Error(w, "firstName and email are required", http.StatusBadRequest)
			return
		}

		address, err := mail.ParseAddress(strings.TrimSpace(req.Email))
		if err != nil {
			http.Error(w, "invalid email address", http.StatusBadRequest)
			return
		}
		recipient := resolveRecipient(address.Address)
		if err := sendFullAreaEmail(recipient, req.FirstName); err != nil {
			log.Printf("[full] email send failed to %s: %v", recipient, err)
			cfg.sendErrorEmail(fmt.Sprintf("full: send to %s failed: %v", recipient, err))
			http.Error(w, `{"error":"email send failed"}`, http.StatusInternalServerError)
			return
		}

		log.Printf("[full] sent to %s (%s)", req.FirstName, recipient)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data": map[string]string{
				"recipient": recipient,
				"subject":   fullAreaEmailSubject,
			},
		})
	}
}

func fullAreaEmailBody(firstName string) string {
	return fmt.Sprintf(
		"Hi %s,\r\n\r\nWe are not taking new clients in your neighborhood. Sorry that we cannot help, but thanks for reaching out to us!\r\n\r\nThank You🌲",
		firstName,
	)
}

func sendFullAreaEmail(to, firstName string) error {
	user := os.Getenv("GMAIL_USER")
	from := os.Getenv("GMAIL_SEND_AS")
	pass := os.Getenv("GMAIL_APP_PASSWORD")
	if user == "" {
		user = from
	}
	if from == "" || pass == "" {
		return fmt.Errorf("GMAIL_SEND_AS or GMAIL_APP_PASSWORD not set")
	}

	var msg bytes.Buffer
	msg.WriteString("From: Tis The Season KC <" + from + ">\r\n")
	msg.WriteString("To: " + to + "\r\n")
	msg.WriteString("Subject: " + fullAreaEmailSubject + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(fullAreaEmailBody(firstName))

	auth := smtp.PlainAuth("", user, pass, "smtp.gmail.com")
	return smtp.SendMail("smtp.gmail.com:587", auth, from, []string{to}, msg.Bytes())
}
