package services

// Stripe Checkout handler — GET /pay?q={customerRecordID}
//
// Flow:
//   1. Read customer record ID from query string
//   2. Fetch customer from Airtable — check if already paid
//   3. Fetch their current-year line items from Yearly Invoicing
//   4. Build Stripe Checkout session with dynamic pricing
//   5. Apply review discount coupon if Review Discount? is checked
//   6. Redirect customer to Stripe-hosted checkout page
//
// The Review Discount? checkbox is NOT cleared here — it is cleared
// by the webhook handler when payment actually completes. This means
// the customer can open their link multiple times without losing the
// discount before they pay.

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	stripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"
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

		// ── 2. Already paid? ────────────────────────────────────────
		if customer.Paid {
			renderAlreadyPaid(w)
			return
		}

		// ── 3. Fetch line items ─────────────────────────────────────
		items, err := GetCurrentYearLineItems(recordID)
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

		// ── 4. Build Stripe line items ───────────────────────────────
		var lineItems []*stripe.CheckoutSessionLineItemParams
		for _, item := range items {
			// Skip negative amounts — these are old-style discounts no longer used
			if item.UnitCost < 0 {
				continue
			}
			if item.StripeProductID == "" {
				log.Printf("[checkout] skipping line item with no Stripe product ID: %s", item.Description)
				continue
			}
			lineItems = append(lineItems, &stripe.CheckoutSessionLineItemParams{
				Quantity: stripe.Int64(int64(item.FinalValue)),
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency:   stripe.String("usd"),
					UnitAmount: stripe.Int64(int64(item.UnitCost * 100)), // cents
					Product:    stripe.String(item.StripeProductID),
				},
			})
		}
		if len(lineItems) == 0 {
			log.Printf("[checkout] all line items filtered out for %s", recordID)
			cfg.sendErrorEmail(fmt.Sprintf("checkout: all line items filtered for %s (%s)", customer.FullName, recordID))
			http.Error(w, checkoutErrorMsg, http.StatusInternalServerError)
			return
		}

		// ── 5. Build session params ─────────────────────────────────
		params := &stripe.CheckoutSessionParams{
			Customer:   stripe.String(customer.StripeID),
			LineItems:  lineItems,
			Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
			SuccessURL: stripe.String("https://tistheseasonkc.com/payment-success"),
		}

		// Apply review discount coupon if the checkbox is checked
		if customer.ReviewDiscount {
			params.Discounts = []*stripe.CheckoutSessionDiscountParams{
				{Coupon: stripe.String(cfg.ReviewCouponID)},
			}
			log.Printf("[checkout] applying review discount to %s", customer.FullName)
		}

		// ── 6. Create session and redirect ───────────────────────────
		sess, err := session.New(params)
		if err != nil {
			log.Printf("[checkout] Stripe session.New(%s): %v", recordID, err)
			cfg.sendErrorEmail(fmt.Sprintf("checkout: Stripe session.New for %s (%s): %v", customer.FullName, recordID, err))
			http.Error(w, checkoutErrorMsg, http.StatusInternalServerError)
			return
		}

		log.Printf("[checkout] created session %s for %s (env=%s)", sess.ID, customer.FullName, cfg.Env)
		http.Redirect(w, r, sess.URL, http.StatusSeeOther)
	}
}

// ── Already-paid page ─────────────────────────────────────────────────

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
    a {
      display: inline-block; color: #69C4BB;
      text-decoration: none; font-weight: 600;
    }
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
