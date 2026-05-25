package services

// Stripe Checkout handler — GET /pay?q={customerRecordID}
//
// Calls Stripe via the exe.dev HTTP proxy integration — no API keys on server.
//
// Flow:
//   1. Read customer record ID from query string
//   2. Fetch customer from Airtable — check if already paid
//   3. Fetch their current-year line items from Yearly Invoicing
//   4. POST to Stripe /v1/checkout/sessions via proxy
//   5. Apply review discount coupon if Review Discount? checkbox is set
//   6. Redirect customer to Stripe-hosted checkout page

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"strings"
)

// CheckoutHandler returns an http.HandlerFunc for GET /pay
func CheckoutHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recordID := strings.TrimSpace(r.URL.Query().Get("q"))
		if recordID == "" {
			http.Error(w, "Payment link required. Please use the link from your invoice.", http.StatusBadRequest)
			return
		}

		// ── 1. Fetch customer ─────────────────────────────────────────
		customer, err := GetCustomerByRecordID(recordID)
		if err != nil {
			log.Printf("[checkout] GetCustomer(%s): %v", recordID, err)
			cfg.sendErrorEmail(fmt.Sprintf("checkout: GetCustomer(%s): %v", recordID, err))
			http.Error(w, checkoutErrorMsg, http.StatusInternalServerError)
			return
		}

		// ── 2. Already paid? ──────────────────────────────────────
		if customer.Paid {
			renderAlreadyPaid(w)
			return
		}

		// ── 3. Fetch line items ───────────────────────────────────
		items, err := GetCurrentYearLineItems(recordID, cfg.Env)
		if err != nil {
			log.Printf("[checkout] GetLineItems(%s): %v", recordID, err)
			cfg.sendErrorEmail(fmt.Sprintf("checkout: GetLineItems(%s) customer=%s: %v", recordID, customer.FullName, err))
			http.Error(w, checkoutErrorMsg, http.StatusInternalServerError)
			return
		}
		if len(items) == 0 {
			log.Printf("[checkout] no line items for record %s", recordID)
			cfg.sendErrorEmail(fmt.Sprintf("checkout: no line items for %s (%s)", customer.FullName, recordID))
			http.Error(w, checkoutErrorMsg, http.StatusInternalServerError)
			return
		}

		// ── 4. Create Stripe Checkout session via proxy ───────────────
		sessURL, err := createCheckoutSession(cfg, customer, items)
		if err != nil {
			log.Printf("[checkout] createCheckoutSession(%s): %v", recordID, err)
			cfg.sendErrorEmail(fmt.Sprintf("checkout: Stripe session failed for %s (%s): %v", customer.FullName, recordID, err))
			http.Error(w, checkoutErrorMsg, http.StatusInternalServerError)
			return
		}

		log.Printf("[checkout] session created for %s (env=%s)", customer.FullName, cfg.Env)
		http.Redirect(w, r, sessURL, http.StatusSeeOther)
	}
}

// createCheckoutSession POSTs to Stripe via the exe.dev proxy.
// Stripe's checkout.sessions.create uses form-encoded body params.
func createCheckoutSession(cfg *Config, customer *Customer, items []InvoiceLineItem) (string, error) {
	params := url.Values{}
	params.Set("customer", customer.StripeID)
	params.Set("mode", "payment")
	params.Set("success_url", "https://tistheseasonkc.com/payment-success")

	// Line items — Stripe uses indexed form params for arrays
	i := 0
	for _, item := range items {
		if item.UnitCost < 0 || item.StripeProductID == "" {
			continue // skip negative/empty (old-style discounts)
		}
		prefix := fmt.Sprintf("line_items[%d]", i)
		params.Set(prefix+"[quantity]", fmt.Sprintf("%d", int64(item.FinalValue)))
		params.Set(prefix+"[price_data][currency]", "usd")
		params.Set(prefix+"[price_data][product]", item.StripeProductID)
		params.Set(prefix+"[price_data][unit_amount]", fmt.Sprintf("%d", int64(math.Round(item.UnitCost*100))))
		i++
	}
	if i == 0 {
		return "", fmt.Errorf("no valid line items after filtering")
	}

	// Review discount coupon
	if customer.ReviewDiscount && cfg.ReviewCouponID != "" {
		params.Set("discounts[0][coupon]", cfg.ReviewCouponID)
		log.Printf("[checkout] applying review discount coupon to %s", customer.FullName)
	}

	resp, err := http.Post(
		cfg.StripeBaseURL+"/v1/checkout/sessions",
		"application/x-www-form-urlencoded",
		strings.NewReader(params.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("POST /v1/checkout/sessions: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("stripe %d: %s", resp.StatusCode, string(body))
	}

	var sess struct {
		URL string `json:"url"`
		ID  string `json:"id"`
	}
	if err := json.Unmarshal(body, &sess); err != nil {
		return "", fmt.Errorf("decode session response: %w", err)
	}
	if sess.URL == "" {
		return "", fmt.Errorf("stripe returned empty session URL")
	}

	log.Printf("[checkout] session %s created", sess.ID)
	return sess.URL, nil
}

// ── Already-paid page ────────────────────────────────────────────────

var alreadyPaidTmpl = template.Must(template.New("paid").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Already Paid — Tis The Season Holiday Lighting</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      font-family: system-ui, -apple-system, sans-serif;
      background: #f5f5f0;
      display: flex; flex-direction: column;
      align-items: center; justify-content: center;
      min-height: 100vh; padding: 2rem;
      text-align: center;
    }
    .card {
      background: white; border-radius: 0.5rem;
      box-shadow: 0 1px 3px rgba(0,0,0,0.08);
      padding: 2.5rem 2rem; max-width: 28rem;
    }
    h1 { color: #2d8a4e; font-size: 1.75rem; margin-bottom: 1rem; }
    p { color: #4a4a5a; line-height: 1.6; margin-bottom: 1.5rem; }
    a { display: inline-block; color: #69C4BB; text-decoration: none; font-weight: 600; }
    a:hover { text-decoration: underline; }
  </style>
</head>
<body>
  <div class="card">
    <h1>✓ Already Paid!</h1>
    <p>Our records show your invoice has been paid. If you think this is incorrect,
    please send us a text and we’ll double check right away.</p>
    <a href="https://tistheseasonkc.com/">Tis The Season Home Page &rarr;</a>
  </div>
</body>
</html>`))

func renderAlreadyPaid(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = alreadyPaidTmpl.Execute(w, nil)
}

const checkoutErrorMsg = "An error occurred creating your checkout page. " +
	"Please text Tis The Season and we\u2019ll get it corrected and resend your link."
