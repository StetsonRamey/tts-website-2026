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
	"crypto/rand"
	"encoding/hex"
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

		ref := newRefCode()

		// ── 1. Fetch customer ─────────────────────────────────────────
		customer, err := GetCustomerByRecordID(recordID)
		if err != nil {
			log.Printf("[checkout] ref=%s GetCustomer(%s): %v", ref, recordID, err)
			cfg.sendErrorEmail(fmt.Sprintf("checkout: ref=%s GetCustomer(%s): %v", ref, recordID, err))
			renderCheckoutError(w, ref, "We couldn\u2019t look up your account.", friendlyCause(err))
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
			log.Printf("[checkout] ref=%s GetLineItems(%s): %v", ref, recordID, err)
			cfg.sendErrorEmail(fmt.Sprintf("checkout: ref=%s GetLineItems(%s) customer=%s: %v", ref, recordID, customer.FullName, err))
			renderCheckoutError(w, ref, "We couldn\u2019t load this year\u2019s invoice items.", friendlyCause(err))
			return
		}
		if len(items) == 0 {
			log.Printf("[checkout] ref=%s no line items for record %s", ref, recordID)
			cfg.sendErrorEmail(fmt.Sprintf("checkout: ref=%s no line items for %s (%s)", ref, customer.FullName, recordID))
			renderCheckoutError(w, ref, "There are no invoice items on file for this year yet.", "")
			return
		}

		// ── 4. Create Stripe Checkout session via proxy ───────────────
		sessURL, err := createCheckoutSession(cfg, customer, items)
		if err != nil {
			log.Printf("[checkout] ref=%s createCheckoutSession(%s): %v", ref, recordID, err)
			cfg.sendErrorEmail(fmt.Sprintf("checkout: ref=%s Stripe session failed for %s (%s): %v", ref, customer.FullName, recordID, err))
			renderCheckoutError(w, ref, "Our payment processor rejected the request.", friendlyCause(err))
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

	// Review discount coupon. A linked Services coupon takes precedence over
	// the legacy environment-level fallback. Discounts apply once per Checkout
	// Session, not as invoice line items.
	couponID := cfg.ReviewCouponID
	if cfg.Env == "dev" && customer.CouponIDDev != "" {
		couponID = customer.CouponIDDev
	}
	if cfg.Env == "prod" && customer.CouponIDProd != "" {
		couponID = customer.CouponIDProd
	}
	if customer.ReviewDiscount {
		if couponID == "" {
			return "", fmt.Errorf("review discount is enabled but no %s Stripe coupon is configured", cfg.Env)
		}
		params.Set("discounts[0][coupon]", couponID)
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

type checkoutErrorData struct {
	Reason string
	Detail string
	Ref    string
}

var checkoutErrorTmpl = template.Must(template.New("err").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Checkout Error — Tis The Season Holiday Lighting</title>
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
      padding: 2.5rem 2rem; max-width: 30rem;
    }
    h1 { color: #c0392b; font-size: 1.75rem; margin-bottom: 1rem; }
    p  { color: #4a4a5a; line-height: 1.6; margin-bottom: 1rem; }
    .reason { font-weight: 600; color: #2d2d3a; }
    .detail {
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      font-size: 0.85rem;
      background: #f7f4ec;
      border-radius: 0.375rem;
      padding: 0.6rem 0.75rem;
      color: #6a5a3a;
      word-break: break-word;
      text-align: left;
      margin-bottom: 1.25rem;
    }
    .ref {
      font-size: 0.8rem;
      color: #8a8a96;
      margin-bottom: 1.5rem;
    }
    .ref code {
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      background: #f0ede4;
      padding: 0.1rem 0.4rem;
      border-radius: 0.25rem;
      color: #4a4a5a;
    }
    a.home { display: inline-block; color: #69C4BB; text-decoration: none; font-weight: 600; }
    a.home:hover { text-decoration: underline; }
  </style>
</head>
<body>
  <div class="card">
    <h1>Checkout Unavailable</h1>
    <p class="reason">{{if .Reason}}{{.Reason}}{{else}}Something went wrong creating your checkout page.{{end}}</p>
    {{if .Detail}}<div class="detail">{{.Detail}}</div>{{end}}
    <p>Please text Tis The Season a screenshot of this page and we’ll get it sorted right away.</p>
    {{if .Ref}}<p class="ref">Reference: <code>{{.Ref}}</code></p>{{end}}
    <a class="home" href="https://tistheseasonkc.com/">Tis The Season Home Page &rarr;</a>
  </div>
</body>
</html>`))

func renderCheckoutError(w http.ResponseWriter, ref, reason, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_ = checkoutErrorTmpl.Execute(w, checkoutErrorData{Reason: reason, Detail: detail, Ref: ref})
}

// newRefCode returns a short reference code customers can quote to us.
func newRefCode() string {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "------"
	}
	return strings.ToUpper(hex.EncodeToString(b[:]))
}

// friendlyCause translates a raw error into a short customer-safe explanation.
// It deliberately avoids leaking IDs, secrets, or stack traces.
func friendlyCause(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "no such customer"):
		return "Your customer record needs to be re-linked on our end."
	case strings.Contains(lower, "no such coupon"), strings.Contains(lower, "no such price"), strings.Contains(lower, "no such product"):
		return "One of the items on your invoice is misconfigured."
	case strings.Contains(lower, "not_found"), strings.Contains(lower, "404"):
		return "We couldn\u2019t find your record \u2014 the payment link may be stale."
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "deadline"):
		return "The request timed out reaching our payment processor."
	case strings.Contains(lower, "unauthorized"), strings.Contains(lower, "forbidden"), strings.Contains(lower, "401"), strings.Contains(lower, "403"):
		return "We weren\u2019t able to authenticate with our payment processor."
	}
	return ""
}
