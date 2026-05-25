package services

// Airtable client for Tis The Season KC
//
// All requests go through the exe.dev HTTP proxy integration at
// https://airtable.int.exe.xyz — the PAT is injected automatically.
// No Airtable API keys are stored on this server.
//
// Field IDs are used instead of field names for stability — Airtable
// field names can be renamed without breaking anything here.
// To look up a field ID: shelley skill cat airtable, then run the schema query.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const (
	// Base URL for the exe.dev Airtable proxy
	airtableBase = "https://airtable.int.exe.xyz/v0"

	// Master Customer DB base ID
	baseID = "appuKBlgPC6igwAMO"

	// Table IDs
	tableCustomers        = "tblBtIM0f6pCcR7XI"
	tableYearlyInvoicing  = "tblXtuzgoKSAJwzDa"

	// Yearly Invoicing view: pre-filtered to current season only
	viewCurrentYear = "viwL8vbspkSX75DS3"

	// ── Customers table field names ──────────────────────────────────────────
	// NOTE: Airtable REST API returns field *names* as keys, not field IDs.
	fieldCustomerFullName       = "Full Name"       // formula
	fieldCustomerFirstName      = "First Name"      // singleLineText
	fieldCustomerLastName       = "Last Name"       // singleLineText
	fieldCustomerEmail          = "Email"           // singleLineText
	fieldCustomerRecordID       = "Record ID"       // formula — RECORD_ID()
	fieldCustomerStripeID       = "Stripe ID"       // singleLineText  e.g. cus_xxx
	fieldCustomerPaid           = "Paid?"           // singleSelect    "Paid" | empty
	fieldCustomerReviewDiscount = "Review Discount?" // checkbox

	// ── Yearly Invoicing table field names ───────────────────────────────────
	// (lookup fields return arrays — take index [0])
	fieldInvStripeCustomerID    = "Stripe ID (from Customer Link)"          // lookup
	fieldInvCustomerRecordID    = "Record ID (from Customer Link)"          // lookup
	// Product ID fields — two versions, selected at runtime based on APP_ENV
	// dev  → Stripe TEST IDs (from Services Link)
	// prod → Stripe Product ID (from Services Link)
	fieldInvStripeProductIDProd = "Stripe Product ID (from Services Link)" // lookup: prod
	fieldInvStripeProductIDDev  = "Stripe TEST IDs (from Services Link)"   // lookup: dev/sandbox
	fieldInvFinalValue          = "Final Value"      // formula: quantity (linear feet or custom)
	fieldInvUnitCost            = "Unit Cost"        // currency: price per unit
	fieldInvDescription         = "Description"      // singleLineText: line item description
	fieldInvStripeCouponID      = "Stripe Coupon ID" // singleLineText: optional Stripe coupon
	fieldInvTotalPrice          = "Total Price"      // formula: total
	fieldInvLineItemDetail      = "Line Item Detail" // formula: human-readable summary
)

// ── Structs ───────────────────────────────────────────────────────────────────

// Customer holds the fields we need from the Customers table.
type Customer struct {
	AirtableID     string // Airtable record ID (rec...)
	RecordID       string // same value, from the formula field
	FullName       string
	FirstName      string
	LastName       string
	Email          string
	StripeID       string // Stripe customer ID (cus_...)
	Paid           bool   // true if Paid? == "Paid"
	ReviewDiscount bool   // true if Review Discount? checkbox is checked
}

// InvoiceLineItem holds one row from the Yearly Invoicing table.
// A customer may have multiple line items.
type InvoiceLineItem struct {
	AirtableID      string
	StripeCustomerID string  // lookup from Customer
	StripeProductID  string  // lookup from Services
	FinalValue       float64 // quantity
	UnitCost         float64 // price per unit in dollars
	TotalPrice       float64
	Description      string
	StripeCouponID   string // optional — set on the item row if a coupon applies
}

// ── Airtable API helpers ──────────────────────────────────────────────────────

type atRecord struct {
	ID     string                 `json:"id"`
	Fields map[string]interface{} `json:"fields"`
}

type atListResponse struct {
	Records []atRecord `json:"records"`
	Offset  string     `json:"offset"`
}

type atErrorResponse struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func atURL(table string) string {
	return fmt.Sprintf("%s/%s/%s", airtableBase, baseID, table)
}

// atGet performs a GET request to the Airtable proxy.
func atGet(rawURL string) ([]atRecord, error) {
	resp, err := http.Get(rawURL) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("airtable GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		var e atErrorResponse
		_ = json.Unmarshal(body, &e)
		return nil, fmt.Errorf("airtable %d: %s — %s", resp.StatusCode, e.Error.Type, e.Error.Message)
	}

	var list atListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("airtable decode: %w", err)
	}
	return list.Records, nil
}

// atPatch updates specific fields on an Airtable record.
func atPatch(table, recordID string, fields map[string]interface{}) error {
	payload, _ := json.Marshal(map[string]interface{}{"fields": fields})
	req, _ := http.NewRequest(http.MethodPatch,
		fmt.Sprintf("%s/%s", atURL(table), recordID),
		bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("airtable PATCH %s/%s: %w", table, recordID, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		var e atErrorResponse
		_ = json.Unmarshal(body, &e)
		return fmt.Errorf("airtable PATCH %d: %s — %s", resp.StatusCode, e.Error.Type, e.Error.Message)
	}
	return nil
}

// ── Public functions ──────────────────────────────────────────────────────────

// GetCustomerByRecordID fetches a single Customer record by its Airtable Record ID
// (the rec... string stored in the Record ID formula field and used in payment URLs).
func GetCustomerByRecordID(recordID string) (*Customer, error) {
	params := url.Values{}
	params.Set("filterByFormula", fmt.Sprintf("{Record ID} = '%s'", recordID))
	params.Set("fields[]", fieldCustomerFullName)
	params.Add("fields[]", fieldCustomerFirstName)
	params.Add("fields[]", fieldCustomerLastName)
	params.Add("fields[]", fieldCustomerEmail)
	params.Add("fields[]", fieldCustomerRecordID)
	params.Add("fields[]", fieldCustomerStripeID)
	params.Add("fields[]", fieldCustomerPaid)
	params.Add("fields[]", fieldCustomerReviewDiscount)

	records, err := atGet(atURL(tableCustomers) + "?" + params.Encode())
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no customer found for record ID %q", recordID)
	}

	return parseCustomer(records[0]), nil
}

// GetCustomerByStripeID fetches a Customer by their Stripe customer ID (cus_...).
// Used by the webhook handler to identify who just paid.
func GetCustomerByStripeID(stripeCustomerID string) (*Customer, error) {
	params := url.Values{}
	params.Set("filterByFormula", fmt.Sprintf("{Stripe ID} = '%s'", stripeCustomerID))
	params.Set("fields[]", fieldCustomerFullName)
	params.Add("fields[]", fieldCustomerRecordID)
	params.Add("fields[]", fieldCustomerStripeID)
	params.Add("fields[]", fieldCustomerPaid)
	params.Add("fields[]", fieldCustomerReviewDiscount)

	records, err := atGet(atURL(tableCustomers) + "?" + params.Encode())
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no customer found for Stripe ID %q", stripeCustomerID)
	}

	return parseCustomer(records[0]), nil
}

// GetCurrentYearLineItems returns all Yearly Invoicing rows for a given customer
// in the current season (uses the "Current Year" view which pre-filters by year).
// env should be "dev" or "prod" — selects the correct Stripe product ID field.
func GetCurrentYearLineItems(customerRecordID string, env string) ([]InvoiceLineItem, error) {
	// Pick sandbox or live product ID field based on environment
	productIDField := fieldInvStripeProductIDProd
	if env == "dev" {
		productIDField = fieldInvStripeProductIDDev
	}

	params := url.Values{}
	params.Set("view", viewCurrentYear)
	params.Set("filterByFormula", fmt.Sprintf("{Record ID (from Customer Link)} = '%s'", customerRecordID))
	params.Set("fields[]", fieldInvStripeCustomerID)
	params.Add("fields[]", fieldInvCustomerRecordID)
	params.Add("fields[]", productIDField) // dev=TEST IDs, prod=live IDs
	params.Add("fields[]", fieldInvFinalValue)
	params.Add("fields[]", fieldInvUnitCost)
	params.Add("fields[]", fieldInvDescription)
	params.Add("fields[]", fieldInvStripeCouponID)
	params.Add("fields[]", fieldInvTotalPrice)
	params.Add("fields[]", fieldInvLineItemDetail)

	records, err := atGet(atURL(tableYearlyInvoicing) + "?" + params.Encode())
	if err != nil {
		return nil, err
	}

	items := make([]InvoiceLineItem, 0, len(records))
	for _, r := range records {
		items = append(items, parseLineItem(r))
	}
	return items, nil
}

// MarkCustomerPaid sets Paid? = "Paid" and clears Review Discount? in one PATCH.
// Called by the Stripe webhook handler on checkout.session.completed.
func MarkCustomerPaid(airtableRecordID string) error {
	return atPatch(tableCustomers, airtableRecordID, map[string]interface{}{
		"Paid?":            "Paid",
		"Review Discount?": false,
	})
}

// ── Parsers ───────────────────────────────────────────────────────────────────

func parseCustomer(r atRecord) *Customer {
	f := r.Fields
	return &Customer{
		AirtableID:      r.ID,
		RecordID:        str(f[fieldCustomerRecordID]),
		FullName:        str(f[fieldCustomerFullName]),
		FirstName:       str(f[fieldCustomerFirstName]),
		LastName:        str(f[fieldCustomerLastName]),
		Email:           str(f[fieldCustomerEmail]),
		StripeID:        str(f[fieldCustomerStripeID]),
		Paid:            str(f[fieldCustomerPaid]) == "Paid",
		ReviewDiscount:  boolField(f[fieldCustomerReviewDiscount]),
	}
}

func parseLineItem(r atRecord) InvoiceLineItem {
	f := r.Fields
	// One of these two fields will be populated depending on which was requested
	productID := lookupFirst(f[fieldInvStripeProductIDProd])
	if productID == "" {
		productID = lookupFirst(f[fieldInvStripeProductIDDev])
	}
	return InvoiceLineItem{
		AirtableID:       r.ID,
		StripeCustomerID: lookupFirst(f[fieldInvStripeCustomerID]),
		StripeProductID:  productID,
		FinalValue:       numField(f[fieldInvFinalValue]),
		UnitCost:         numField(f[fieldInvUnitCost]),
		TotalPrice:       numField(f[fieldInvTotalPrice]),
		Description:      str(f[fieldInvDescription]),
		StripeCouponID:   str(f[fieldInvStripeCouponID]),
	}
}

// ── Field extraction helpers ──────────────────────────────────────────────────

// str safely casts an interface{} to string.
func str(v interface{}) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// numField safely extracts a float64 from number/currency/formula fields.
func numField(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

// boolField safely extracts a checkbox value.
func boolField(v interface{}) bool {
	if v == nil {
		return false
	}
	b, _ := v.(bool)
	return b
}

// lookupFirst extracts the first element from a multipleLookupValues array.
func lookupFirst(v interface{}) string {
	if v == nil {
		return ""
	}
	arr, ok := v.([]interface{})
	if !ok || len(arr) == 0 {
		return ""
	}
	s, _ := arr[0].(string)
	return s
}
