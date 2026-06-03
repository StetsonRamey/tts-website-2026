package services

// POST /invoice/create
//
// Triggered by an Airtable automation when "Make Invoice" is checked.
// Requires that /sold/sync has already run (stripeID must exist).
//
// Stripe owns the unit cost per linear foot — we just pass the Airtable Feet
// value as the quantity. The product's default price is looked up on the fly,
// so the env vars only need to point at a Product ID.
//
// Flow (idempotent — re-runs short-circuit when invoice already exists):
//   1. Fetch the lead from Airtable
//   2. If stripeInvoiceLink already set → return success without duplicating
//   3. Look up product default_price (Stripe owns the unit cost)
//   4. POST /v1/invoiceitems  (customer, price, quantity=Feet)
//   5. POST /v1/invoices      (customer, collection_method=send_invoice)
//   6. POST /v1/invoices/{id}/finalize
//   7. Patch hosted_invoice_url back to Airtable

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

type invoiceRequest struct {
	RecordID string `json:"recordId"`
}

// InvoiceHandler returns the http.HandlerFunc for POST /invoice/create.
func InvoiceHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if !RequireBearerAuth(w, r) {
			return
		}

		var req invoiceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.RecordID) == "" {
			http.Error(w, `{"error":"recordId is required"}`, http.StatusBadRequest)
			return
		}
		if cfg.InvoiceProductID == "" {
			log.Printf("[invoice] STRIPE_INVOICE_PRODUCT_ID_%s is empty", strings.ToUpper(cfg.Env))
			http.Error(w, `{"error":"server invoice product not configured"}`, http.StatusInternalServerError)
			return
		}

		lead, err := GetLeadByRecordID(req.RecordID)
		if err != nil {
			log.Printf("[invoice] fetch lead %s: %v", req.RecordID, err)
			cfg.sendErrorEmail(fmt.Sprintf("invoice: fetch lead %s: %v", req.RecordID, err))
			http.Error(w, `{"error":"failed to fetch lead"}`, http.StatusInternalServerError)
			return
		}

		// ── Guardrails ────────────────────────────────────────────────────────
		if lead.StripeID == "" {
			http.Error(w, `{"error":"stripeID missing — run /sold/sync first"}`, http.StatusBadRequest)
			return
		}
		if lead.Feet <= 0 {
			http.Error(w, `{"error":"Feet must be > 0 to create invoice"}`, http.StatusBadRequest)
			return
		}
		if lead.StripeInvoiceLink != "" {
			log.Printf("[invoice] %s already has invoice link, skipping", lead.RecordID)
			respondJSON(w, map[string]interface{}{
				"success":           true,
				"skipped":           true,
				"stripeInvoiceLink": lead.StripeInvoiceLink,
			})
			return
		}

		// ── 1. Look up product's default price ────────────────────────────────
		priceID, err := getProductDefaultPrice(cfg, cfg.InvoiceProductID)
		if err != nil {
			log.Printf("[invoice] get default_price for %s: %v", cfg.InvoiceProductID, err)
			cfg.sendErrorEmail(fmt.Sprintf("invoice: get default_price for product %s: %v", cfg.InvoiceProductID, err))
			http.Error(w, `{"error":"failed to look up Stripe price"}`, http.StatusInternalServerError)
			return
		}

		// ── 2. Create the invoice item ────────────────────────────────────────
		feetQty := int64(lead.Feet)
		if feetQty < 1 {
			feetQty = 1
		}
		if err := createStripeInvoiceItem(cfg, lead.StripeID, priceID, feetQty); err != nil {
			log.Printf("[invoice] create invoice item for %s: %v", lead.RecordID, err)
			cfg.sendErrorEmail(fmt.Sprintf("invoice: create item for %s: %v", lead.RecordID, err))
			http.Error(w, `{"error":"failed to create invoice item"}`, http.StatusInternalServerError)
			return
		}

		// ── 3. Create the invoice ─────────────────────────────────────────────
		invoiceID, err := createStripeInvoice(cfg, lead.StripeID)
		if err != nil {
			log.Printf("[invoice] create invoice for %s: %v", lead.RecordID, err)
			cfg.sendErrorEmail(fmt.Sprintf("invoice: create invoice for %s: %v", lead.RecordID, err))
			http.Error(w, `{"error":"failed to create invoice"}`, http.StatusInternalServerError)
			return
		}

		// ── 4. Finalize ───────────────────────────────────────────────────────
		hostedURL, err := finalizeStripeInvoice(cfg, invoiceID)
		if err != nil {
			log.Printf("[invoice] finalize invoice %s for %s: %v", invoiceID, lead.RecordID, err)
			cfg.sendErrorEmail(fmt.Sprintf("invoice: finalize %s for %s: %v", invoiceID, lead.RecordID, err))
			http.Error(w, `{"error":"failed to finalize invoice"}`, http.StatusInternalServerError)
			return
		}

		// ── 5. Patch Airtable ─────────────────────────────────────────────────
		if err := PatchLeadFields(lead.AirtableID, map[string]interface{}{
			fieldLeadStripeInvoiceLink: hostedURL,
		}); err != nil {
			log.Printf("[invoice] patch invoice link for %s: %v", lead.RecordID, err)
			cfg.sendErrorEmail(fmt.Sprintf("invoice: patch link for %s (invoice=%s url=%s): %v", lead.RecordID, invoiceID, hostedURL, err))
			// Still report success to the caller — the invoice exists; this just
			// needs manual cleanup in Airtable. Include the URL in the response.
		}

		log.Printf("[invoice] created record=%s invoice=%s quantity=%d", lead.RecordID, invoiceID, feetQty)
		respondJSON(w, map[string]interface{}{
			"success":           true,
			"recordId":          lead.RecordID,
			"invoiceId":         invoiceID,
			"stripeInvoiceLink": hostedURL,
			"quantity":          feetQty,
		})
	}
}

// ── Stripe helpers ───────────────────────────────────────────────────────────

// getProductDefaultPrice returns the default_price string for a product.
// GET /v1/products/{id}  → { default_price: "price_..." }
func getProductDefaultPrice(cfg *Config, productID string) (string, error) {
	resp, err := http.Get(cfg.StripeBaseURL + "/v1/products/" + url.PathEscape(productID))
	if err != nil {
		return "", fmt.Errorf("GET product: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("stripe product %d: %s", resp.StatusCode, string(body))
	}
	// default_price can be a string ID or an expanded object — handle both.
	var raw struct {
		DefaultPrice json.RawMessage `json:"default_price"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", fmt.Errorf("decode product: %w", err)
	}
	if len(raw.DefaultPrice) == 0 || string(raw.DefaultPrice) == "null" {
		return "", fmt.Errorf("product %s has no default_price set in Stripe", productID)
	}
	var asString string
	if err := json.Unmarshal(raw.DefaultPrice, &asString); err == nil && asString != "" {
		return asString, nil
	}
	var asObject struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw.DefaultPrice, &asObject); err == nil && asObject.ID != "" {
		return asObject.ID, nil
	}
	return "", fmt.Errorf("could not parse default_price for product %s", productID)
}

// createStripeInvoiceItem adds a single line to the customer's pending items.
// POST /v1/invoiceitems  customer=, price=, quantity=
func createStripeInvoiceItem(cfg *Config, stripeCustomerID, priceID string, quantity int64) error {
	params := url.Values{}
	params.Set("customer", stripeCustomerID)
	params.Set("price", priceID)
	params.Set("quantity", fmt.Sprintf("%d", quantity))

	resp, err := http.Post(
		cfg.StripeBaseURL+"/v1/invoiceitems",
		"application/x-www-form-urlencoded",
		strings.NewReader(params.Encode()),
	)
	if err != nil {
		return fmt.Errorf("POST /v1/invoiceitems: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("stripe invoiceitem %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// createStripeInvoice creates a draft invoice and returns its ID.
// POST /v1/invoices  customer=, collection_method=send_invoice, days_until_due=
func createStripeInvoice(cfg *Config, stripeCustomerID string) (string, error) {
	params := url.Values{}
	params.Set("customer", stripeCustomerID)
	params.Set("collection_method", "send_invoice")
	params.Set("days_until_due", "30")
	// auto_advance lets Stripe try to handle the lifecycle automatically — but
	// we explicitly finalize below for predictable behaviour and so we can
	// surface the hosted URL right away.
	params.Set("auto_advance", "false")

	resp, err := http.Post(
		cfg.StripeBaseURL+"/v1/invoices",
		"application/x-www-form-urlencoded",
		strings.NewReader(params.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("POST /v1/invoices: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("stripe invoice %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decode invoice: %w", err)
	}
	if out.ID == "" {
		return "", fmt.Errorf("stripe returned empty invoice id")
	}
	return out.ID, nil
}

// finalizeStripeInvoice transitions a draft invoice to open and returns the
// hosted_invoice_url that customers use to pay.
// POST /v1/invoices/{id}/finalize
func finalizeStripeInvoice(cfg *Config, invoiceID string) (string, error) {
	resp, err := http.Post(
		cfg.StripeBaseURL+"/v1/invoices/"+url.PathEscape(invoiceID)+"/finalize",
		"application/x-www-form-urlencoded",
		strings.NewReader(""),
	)
	if err != nil {
		return "", fmt.Errorf("POST finalize: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("stripe finalize %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		HostedInvoiceURL string `json:"hosted_invoice_url"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decode finalize: %w", err)
	}
	if out.HostedInvoiceURL == "" {
		return "", fmt.Errorf("stripe finalize returned empty hosted_invoice_url")
	}
	return out.HostedInvoiceURL, nil
}

// respondJSON writes a small JSON body with Content-Type set.
func respondJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
