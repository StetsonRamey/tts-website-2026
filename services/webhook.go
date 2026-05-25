package services

// Stripe webhook handler — POST /stripe/webhook
//
// Listens for checkout.session.completed events from Stripe.
// When a payment completes:
//   1. Verify the Stripe-Signature header (prevents spoofed requests)
//   2. Find the customer in Airtable by their Stripe customer ID
//   3. Mark Paid? = "Paid" and clear Review Discount? in one PATCH
//
// Stripe expects a 200 response quickly — it retries on anything else.
// Signature verification is done manually (no SDK) since we use the
// proxy for outbound calls but webhooks are inbound from Stripe directly.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// WebhookHandler returns an http.HandlerFunc for POST /stripe/webhook
func WebhookHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Read body first — must be raw bytes for signature verification
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("[webhook] read body: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// ── Verify Stripe signature ──────────────────────────────────
		sig := r.Header.Get("Stripe-Signature")
		if sig == "" {
			log.Println("[webhook] missing Stripe-Signature header")
			http.Error(w, "missing signature", http.StatusBadRequest)
			return
		}

		if err := verifyStripeSignature(body, sig, cfg.WebhookSecret); err != nil {
			log.Printf("[webhook] signature verification failed: %v", err)
			http.Error(w, "invalid signature", http.StatusBadRequest)
			return
		}

		// Parse just enough of the event to get type + customer ID
		var event struct {
			Type string `json:"type"`
			Data struct {
				Object struct {
					ID       string `json:"id"`
					Customer string `json:"customer"`
				} `json:"object"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &event); err != nil {
			log.Printf("[webhook] json parse: %v", err)
			w.WriteHeader(http.StatusOK) // don't let Stripe retry a bad payload
			return
		}

		// ── Handle events ───────────────────────────────────────────
		switch event.Type {

		case "checkout.session.completed":
			if err := handlePaymentCompleted(cfg, event.Data.Object.Customer, event.Data.Object.ID); err != nil {
				log.Printf("[webhook] handlePaymentCompleted: %v", err)
				cfg.sendErrorEmail(fmt.Sprintf("webhook: payment completed but Airtable update failed: %v", err))
				// Return 200 anyway — payment succeeded, don't cause retries.
				// The error email will alert you to fix manually if needed.
			}

		default:
			log.Printf("[webhook] ignored event type: %s", event.Type)
		}

		w.WriteHeader(http.StatusOK)
	}
}

func handlePaymentCompleted(cfg *Config, stripeCustomerID, sessionID string) error {
	if stripeCustomerID == "" {
		return fmt.Errorf("checkout.session.completed has no customer ID (session %s)", sessionID)
	}

	log.Printf("[webhook] payment completed for Stripe customer %s (session %s)", stripeCustomerID, sessionID)

	customer, err := GetCustomerByStripeID(stripeCustomerID)
	if err != nil {
		return fmt.Errorf("GetCustomerByStripeID(%s): %w", stripeCustomerID, err)
	}

	if err := MarkCustomerPaid(customer.AirtableID); err != nil {
		return fmt.Errorf("MarkCustomerPaid(%s / %s): %w", customer.FullName, customer.AirtableID, err)
	}

	log.Printf("[webhook] marked paid: %s (airtable %s)", customer.FullName, customer.AirtableID)
	return nil
}

// verifyStripeSignature implements Stripe's webhook signature verification.
// See: https://stripe.com/docs/webhooks/signatures
func verifyStripeSignature(payload []byte, sigHeader, secret string) error {
	if secret == "" {
		return fmt.Errorf("webhook secret not configured")
	}

	// Parse t= and v1= from the Stripe-Signature header
	var timestamp string
	var signatures []string
	for _, part := range strings.Split(sigHeader, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "t=") {
			timestamp = strings.TrimPrefix(part, "t=")
		} else if strings.HasPrefix(part, "v1=") {
			signatures = append(signatures, strings.TrimPrefix(part, "v1="))
		}
	}
	if timestamp == "" || len(signatures) == 0 {
		return fmt.Errorf("malformed Stripe-Signature header")
	}

	// Reject events older than 5 minutes (replay attack protection)
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp in signature")
	}
	if time.Since(time.Unix(ts, 0)) > 5*time.Minute {
		return fmt.Errorf("webhook timestamp too old")
	}

	// Compute expected signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + string(payload)))
	expected := hex.EncodeToString(mac.Sum(nil))

	for _, sig := range signatures {
		if hmac.Equal([]byte(sig), []byte(expected)) {
			return nil
		}
	}
	return fmt.Errorf("signature mismatch")
}
