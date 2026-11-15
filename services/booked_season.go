package services

// BookedSeasonHandler — POST /booked-season
//
// Triggered by an Airtable automation when we are booked for the season. The
// automation supplies the lead's email; the message is fixed plain text.

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

type bookedSeasonRequest struct {
	Email string `json:"email"`
}

const bookedSeasonEmailSubject = "Bad news......"

const bookedSeasonEmailBody = "We're booked for this season.  We appreciate you finding us and requesting an estimate.  If you don't find someone this year, reach out to us in September or October next year and we'll be able to help you!\r\n\r\n🎄 'Tis the Season"

func BookedSeasonHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !RequireBearerAuth(w, r) {
			return
		}

		var req bookedSeasonRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Email) == "" {
			http.Error(w, "email is required", http.StatusBadRequest)
			return
		}
		address, err := mail.ParseAddress(strings.TrimSpace(req.Email))
		if err != nil {
			http.Error(w, "invalid email address", http.StatusBadRequest)
			return
		}

		recipient := resolveRecipient(address.Address)
		if err := sendBookedSeasonEmail(recipient); err != nil {
			log.Printf("[booked-season] email send failed to %s: %v", recipient, err)
			cfg.sendErrorEmail(fmt.Sprintf("booked-season: send to %s failed: %v", recipient, err))
			http.Error(w, `{"error":"email send failed"}`, http.StatusInternalServerError)
			return
		}

		log.Printf("[booked-season] sent to %s", recipient)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data": map[string]string{
				"recipient": recipient,
				"subject":   bookedSeasonEmailSubject,
			},
		})
	}
}

func sendBookedSeasonEmail(to string) error {
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
	msg.WriteString("Subject: " + bookedSeasonEmailSubject + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(bookedSeasonEmailBody)

	auth := smtp.PlainAuth("", user, pass, "smtp.gmail.com")
	return smtp.SendMail("smtp.gmail.com:587", auth, from, []string{to}, msg.Bytes())
}
