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
	tableCustomers       = "tblBtIM0f6pCcR7XI"
	tableServices        = "tblNloDsEVME3OpPv"
	tableYearlyInvoicing = "tblXtuzgoKSAJwzDa"

	// Yearly Invoicing view: pre-filtered to current season only
	viewCurrentYear = "viwL8vbspkSX75DS3"

	// ── Customers table field IDs ────────────────────────────────────────────
	// returnFieldsByFieldId=true is added in atParams() so keys are always IDs.
	fieldCustomerFullName       = "fldw8mz8mEc3gVnaj" // formula
	fieldCustomerFirstName      = "fldkDGECRoNhY5PsR" // singleLineText
	fieldCustomerLastName       = "fldy5NPZ5ttw3y1GZ" // singleLineText
	fieldCustomerEmail          = "fldri14MY0YKwnPFw" // singleLineText
	fieldCustomerRecordID       = "fldgfXPisSW52URiL" // formula — RECORD_ID()
	fieldCustomerStripeID       = "fldOI8Qnn58hqTIDh" // singleLineText  e.g. cus_xxx
	fieldCustomerPaid           = "fldeQz412DaIiDeVN" // singleSelect    "Paid" | empty
	fieldCustomerReviewDiscount = "fldzCeTrLy9gdFD3m" // checkbox
	fieldCustomerDiscountCoupon = "fldgGTXCZdmSmc3I6" // linked coupon record in Services

	// ── Services table field IDs ──────────────────────────────────────────────
	fieldServiceStripeCouponIDProd = "fld03d7VLjq8Pvxk7" // live coupon ID
	fieldServiceStripeCouponIDDev  = "fldCt8m1scTZjtD1z" // sandbox coupon ID

	// ── Yearly Invoicing table field IDs ─────────────────────────────────────
	// (lookup fields return arrays — take index [0])
	fieldInvStripeCustomerID = "fldFgPy59G2zB9JxJ" // lookup: Stripe ID from Customer
	fieldInvCustomerRecordID = "fldNVumkFtJzdUKXR" // lookup: Record ID from Customer
	// Product ID fields — two versions, selected at runtime based on APP_ENV
	// dev  → Stripe TEST IDs (from Services Link)
	// prod → Stripe Product ID (from Services Link)
	fieldInvStripeProductIDProd = "fldliqzb96m5kYXjp" // lookup: Stripe Product ID (prod)
	fieldInvStripeProductIDDev  = "fldosP4xnAl4b6Hkr" // lookup: Stripe TEST IDs (dev/sandbox)
	fieldInvFinalValue          = "fldh8BoGKbwvdVxAq" // formula: quantity (linear feet or custom)
	fieldInvUnitCost            = "fldNUT5jTfj4CcN8y" // currency: price per unit
	fieldInvDescription         = "fldMGQd6Ggh8IZxD0" // singleLineText: line item description
	fieldInvStripeCouponID      = "fldDNR9C3fPdUwg3k" // legacy optional coupon field
	fieldInvTotalPrice          = "fldSTxRJQD5P449jv" // formula: total
	fieldInvLineItemDetail      = "fldSQkAsz0TULhHr7" // formula: human-readable summary
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
	CouponIDDev    string // sandbox coupon ID from linked Services coupon
	CouponIDProd   string // live coupon ID from linked Services coupon
}

// InvoiceLineItem holds one row from the Yearly Invoicing table.
// A customer may have multiple line items.
type InvoiceLineItem struct {
	AirtableID       string
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

// atParams returns a url.Values pre-seeded with returnFieldsByFieldId=true.
// All callers should use this instead of url.Values{} directly.
func atParams() url.Values {
	v := url.Values{}
	v.Set("returnFieldsByFieldId", "true")
	return v
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

// atGetRecord performs a GET request for one Airtable record.
func atGetRecord(rawURL string) (atRecord, error) {
	resp, err := http.Get(rawURL) //nolint:noctx
	if err != nil {
		return atRecord{}, fmt.Errorf("airtable GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var e atErrorResponse
		_ = json.Unmarshal(body, &e)
		return atRecord{}, fmt.Errorf("airtable %d: %s — %s", resp.StatusCode, e.Error.Type, e.Error.Message)
	}

	var record atRecord
	if err := json.Unmarshal(body, &record); err != nil {
		return atRecord{}, fmt.Errorf("airtable record decode: %w", err)
	}
	return record, nil
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
	params := atParams()
	params.Set("filterByFormula", fmt.Sprintf("{Record ID} = '%s'", recordID))
	params.Set("fields[]", fieldCustomerFullName)
	params.Add("fields[]", fieldCustomerFirstName)
	params.Add("fields[]", fieldCustomerLastName)
	params.Add("fields[]", fieldCustomerEmail)
	params.Add("fields[]", fieldCustomerRecordID)
	params.Add("fields[]", fieldCustomerStripeID)
	params.Add("fields[]", fieldCustomerPaid)
	params.Add("fields[]", fieldCustomerReviewDiscount)
	params.Add("fields[]", fieldCustomerDiscountCoupon)

	records, err := atGet(atURL(tableCustomers) + "?" + params.Encode())
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no customer found for record ID %q", recordID)
	}

	customer := parseCustomer(records[0])
	couponServiceID := lookupFirst(records[0].Fields[fieldCustomerDiscountCoupon])
	if couponServiceID != "" {
		devCoupon, prodCoupon, err := getServiceCouponIDs(couponServiceID)
		if err != nil {
			return nil, fmt.Errorf("load coupon for customer %q: %w", recordID, err)
		}
		customer.CouponIDDev = devCoupon
		customer.CouponIDProd = prodCoupon
	}

	return customer, nil
}

// GetCustomerByStripeID fetches a Customer by their Stripe customer ID (cus_...).
// Used by the webhook handler to identify who just paid.
func GetCustomerByStripeID(stripeCustomerID string) (*Customer, error) {
	params := atParams()
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

// getServiceCouponIDs loads both environment-specific coupon IDs from a linked
// Services record. Coupon records live in the Services table but are not invoice
// line items.
func getServiceCouponIDs(serviceRecordID string) (dev, prod string, err error) {
	params := atParams()
	params.Set("filterByFormula", fmt.Sprintf("RECORD_ID() = '%s'", serviceRecordID))
	params.Set("fields[]", fieldServiceStripeCouponIDDev)
	params.Add("fields[]", fieldServiceStripeCouponIDProd)

	records, err := atGet(atURL(tableServices) + "?" + params.Encode())
	if err != nil {
		return "", "", err
	}
	if len(records) == 0 {
		return "", "", fmt.Errorf("no Services record found for coupon %q", serviceRecordID)
	}

	return str(records[0].Fields[fieldServiceStripeCouponIDDev]), str(records[0].Fields[fieldServiceStripeCouponIDProd]), nil
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

	params := atParams()
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
		fieldCustomerPaid:           "Paid",
		fieldCustomerReviewDiscount: false,
	})
}

// ── Parsers ───────────────────────────────────────────────────────────────────

func parseCustomer(r atRecord) *Customer {
	f := r.Fields
	return &Customer{
		AirtableID:     r.ID,
		RecordID:       str(f[fieldCustomerRecordID]),
		FullName:       str(f[fieldCustomerFullName]),
		FirstName:      str(f[fieldCustomerFirstName]),
		LastName:       str(f[fieldCustomerLastName]),
		Email:          str(f[fieldCustomerEmail]),
		StripeID:       str(f[fieldCustomerStripeID]),
		Paid:           str(f[fieldCustomerPaid]) == "Paid",
		ReviewDiscount: boolField(f[fieldCustomerReviewDiscount]),
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
