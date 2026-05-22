package services

// Stripe webhook handler — POST /stripe/webhook
//
// Listens for checkout.session.completed events from Stripe.
// When a payment completes:
//   1. Verify the Stripe-Signature header (prevents spoofed requests)
//   2. Find the customer in Airtable by their Stripe customer ID
//   3. Mark Paid? = "Paid" and clear Review Discount? in one PATCH
//
// Stripe expects a 200 response quickly — it will retry on anything else.
// Keep this handler fast: no expensive work, just Airtable updates.

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
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

		event, err := webhook.ConstructEvent(body, sig, cfg.StripeWebhookSecret)
		if err != nil {
			log.Printf("[webhook] signature verification failed: %v", err)
			http.Error(w, "invalid signature", http.StatusBadRequest)
			return
		}

		// ── Handle events ───────────────────────────────────────────
		switch event.Type {

		case stripe.EventTypeCheckoutSessionCompleted:
			var sess stripe.CheckoutSession
			if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
				log.Printf("[webhook] unmarshal session: %v", err)
				cfg.sendErrorEmail(fmt.Sprintf("webhook: unmarshal checkout.session.completed: %v", err))
				// Still return 200 — Stripe shouldn't retry a bad payload
				w.WriteHeader(http.StatusOK)
				return
			}

			if err := handlePaymentCompleted(cfg, &sess); err != nil {
				log.Printf("[webhook] handlePaymentCompleted: %v", err)
				cfg.sendErrorEmail(fmt.Sprintf("webhook: payment completed but Airtable update failed: %v", err))
				// Return 200 anyway — don't cause Stripe to retry since the
				// payment succeeded; the error email will alert you.
			}

		default:
			// Ignore other event types — Stripe sends many we don't need
			log.Printf("[webhook] ignored event type: %s", event.Type)
		}

		w.WriteHeader(http.StatusOK)
	}
}

func handlePaymentCompleted(cfg *Config, sess *stripe.CheckoutSession) error {
	stripeCustomerID := sess.Customer.ID
	if stripeCustomerID == "" {
		return fmt.Errorf("checkout.session.completed has no customer ID (session %s)", sess.ID)
	}

	log.Printf("[webhook] payment completed for Stripe customer %s (session %s)", stripeCustomerID, sess.ID)

	// Find the customer in Airtable
	customer, err := GetCustomerByStripeID(stripeCustomerID)
	if err != nil {
		return fmt.Errorf("GetCustomerByStripeID(%s): %w", stripeCustomerID, err)
	}

	// Mark paid and clear the review discount checkbox in one call
	if err := MarkCustomerPaid(customer.AirtableID); err != nil {
		return fmt.Errorf("MarkCustomerPaid(%s / %s): %w", customer.FullName, customer.AirtableID, err)
	}

	log.Printf("[webhook] marked paid: %s (airtable %s)", customer.FullName, customer.AirtableID)
	return nil
}
