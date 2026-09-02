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
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	// Master Customer DB base ID
	baseID = "appuKBlgPC6igwAMO"

	// Table IDs
	tableCustomers       = "tblBtIM0f6pCcR7XI"
	tableServices        = "tblNloDsEVME3OpPv"
	tableYearlyInvoicing = "tblXtuzgoKSAJwzDa"

	// ── Customers table field IDs ────────────────────────────────────────────
	// returnFieldsByFieldId=true is added in atParams() so keys are always IDs.
	fieldCustomerFullName         = "fldw8mz8mEc3gVnaj" // formula
	fieldCustomerFirstName        = "fldkDGECRoNhY5PsR" // singleLineText
	fieldCustomerLastName         = "fldy5NPZ5ttw3y1GZ" // singleLineText
	fieldCustomerEmail            = "fldri14MY0YKwnPFw" // singleLineText
	fieldCustomerRecordID         = "fldgfXPisSW52URiL" // formula — RECORD_ID()
	fieldCustomerStripeID         = "fldOI8Qnn58hqTIDh" // singleLineText  e.g. cus_xxx
	fieldCustomerPaid             = "fldeQz412DaIiDeVN" // singleSelect    "Paid" | empty
	fieldCustomerReviewDiscount   = "fldzCeTrLy9gdFD3m" // checkbox
	fieldCustomerDiscountCoupon   = "fldgGTXCZdmSmc3I6" // linked coupon record in Services
	fieldCustomerInvoices         = "fldgsZDOSayIYUtwG" // raw reciprocal links to Yearly Invoicing
	fieldCustomerInvoiceBuildYear = "fldGKPjdAYrqRCaR1" // number: season built by automation

	// ── Services table field IDs ──────────────────────────────────────────────
	fieldServiceStripeCouponIDProd = "fld03d7VLjq8Pvxk7" // live coupon ID
	fieldServiceStripeCouponIDDev  = "fldCt8m1scTZjtD1z" // sandbox coupon ID

	// ── Yearly Invoicing table field IDs ─────────────────────────────────────
	// (lookup fields return arrays — take index [0])
	fieldInvStripeCustomerID = "fldFgPy59G2zB9JxJ" // lookup: Stripe ID from Customer
	fieldInvCustomerLink     = "fldIELNteHoebkqxE" // multipleRecordLinks: raw link to Customers
	fieldInvYear             = "flddSd62O3sMmMq7C" // singleSelect: invoice year, written by build automation
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

var airtableBase = "https://airtable.int.exe.xyz/v0"

// ── Structs ───────────────────────────────────────────────────────────────────

// Customer holds the fields we need from the Customers table.
type Customer struct {
	AirtableID         string // Airtable record ID (rec...)
	RecordID           string // same value, from the formula field
	FullName           string
	FirstName          string
	LastName           string
	Email              string
	StripeID           string   // Stripe customer ID (cus...)
	Paid               bool     // true if Paid? == "Paid"
	ReviewDiscount     bool     // true if Review Discount? checkbox is checked
	CouponIDDev        string   // sandbox coupon ID from linked Services coupon
	CouponIDProd       string   // live coupon ID from linked Services coupon
	InvoiceBuildYear   int      // raw season selected by the invoice-build automation
	InvoiceLineItemIDs []string // raw reciprocal links to Yearly Invoicing rows
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

// atGet performs a GET request to the Airtable proxy, following Airtable's
// offset-based pagination until all pages are consumed.
func atGet(rawURL string) ([]atRecord, error) {
	// Follow pagination — Airtable returns max 100 records per page.
	// Without this, any query matching >100 rows silently misses records.
	var all []atRecord
	offset := ""
	for {
		var list atListResponse
		pageURL := rawURL
		if offset != "" {
			sep := "&"
			if !strings.Contains(rawURL, "?") {
				sep = "?"
			}
			pageURL = rawURL + sep + "offset=" + url.QueryEscape(offset)
		}
		resp, err := http.Get(pageURL) //nolint:noctx
		if err != nil {
			return nil, fmt.Errorf("airtable GET %s: %w", pageURL, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			var e atErrorResponse
			_ = json.Unmarshal(body, &e)
			return nil, fmt.Errorf("airtable %d: %s — %s", resp.StatusCode, e.Error.Type, e.Error.Message)
		}
		if err := json.Unmarshal(body, &list); err != nil {
			return nil, fmt.Errorf("airtable decode: %w", err)
		}
		all = append(all, list.Records...)
		if list.Offset == "" {
			break
		}
		offset = list.Offset
	}
	return all, nil
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

// GetCustomerByRecordID fetches a single Customer record by its Airtable record ID
// (the rec... value used in payment URLs). RECORD_ID() is intrinsic Airtable
// metadata, so this lookup does not depend on the Customer table's Record ID
// formula field being recalculated.
func GetCustomerByRecordID(recordID string) (*Customer, error) {
	if !isAirtableRecordID(recordID) {
		return nil, fmt.Errorf("invalid Airtable customer record ID %q", recordID)
	}

	params := atParams()
	params.Set("filterByFormula", fmt.Sprintf("RECORD_ID() = '%s'", recordID))
	params.Set("fields[]", fieldCustomerFullName)
	params.Add("fields[]", fieldCustomerFirstName)
	params.Add("fields[]", fieldCustomerLastName)
	params.Add("fields[]", fieldCustomerEmail)
	params.Add("fields[]", fieldCustomerRecordID)
	params.Add("fields[]", fieldCustomerStripeID)
	params.Add("fields[]", fieldCustomerPaid)
	params.Add("fields[]", fieldCustomerReviewDiscount)
	params.Add("fields[]", fieldCustomerDiscountCoupon)
	params.Add("fields[]", fieldCustomerInvoices)
	params.Add("fields[]", fieldCustomerInvoiceBuildYear)

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

// GetInvoiceBuildYearLineItems returns a customer's Yearly Invoicing rows for
// their raw Invoice Build Year.
//
// The customer record contains raw reciprocal links to its Yearly Invoicing
// rows. We query only those exact record IDs, then inspect the raw Year and
// Customer Link fields in Go. This avoids the mutable formula, lookup, and view
// filters that intermittently hid valid invoice rows, while also avoiding a
// full scan of every invoice row for the year.
//
// env should be "dev" or "prod" — selects the correct Stripe product ID field.
func GetInvoiceBuildYearLineItems(customer *Customer, env string) ([]InvoiceLineItem, error) {
	if customer == nil {
		return nil, fmt.Errorf("customer is required")
	}
	if customer.InvoiceBuildYear == 0 || len(customer.InvoiceLineItemIDs) == 0 {
		return []InvoiceLineItem{}, nil
	}

	// Pick sandbox or live product ID field based on environment
	productIDField := fieldInvStripeProductIDProd
	if env == "dev" {
		productIDField = fieldInvStripeProductIDDev
	}

	invoiceYear := strconv.Itoa(customer.InvoiceBuildYear)
	filter, err := recordIDsFilter(customer.InvoiceLineItemIDs)
	if err != nil {
		return nil, fmt.Errorf("build Yearly Invoicing record filter: %w", err)
	}

	params := atParams()
	params.Set("filterByFormula", filter)
	params.Set("fields[]", fieldInvCustomerLink)
	params.Add("fields[]", fieldInvYear)
	params.Add("fields[]", fieldInvStripeCustomerID)
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

	items := lineItemsForCustomerYear(records, customer.AirtableID, invoiceYear)
	if len(items) == 0 {
		log.Printf("[checkout] no matching raw invoice rows: customer=%s build_year=%s linked=%d fetched=%d",
			customer.AirtableID, invoiceYear, len(customer.InvoiceLineItemIDs), len(records))
	}
	return items, nil
}

func lineItemsForCustomerYear(records []atRecord, customerRecordID, year string) []InvoiceLineItem {
	items := make([]InvoiceLineItem, 0, len(records))
	for _, r := range records {
		if strings.TrimSpace(str(r.Fields[fieldInvYear])) != year {
			continue
		}
		if !linkContains(r.Fields[fieldInvCustomerLink], customerRecordID) {
			continue
		}
		items = append(items, parseLineItem(r))
	}
	return items
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
		AirtableID:         r.ID,
		RecordID:           str(f[fieldCustomerRecordID]),
		FullName:           str(f[fieldCustomerFullName]),
		FirstName:          str(f[fieldCustomerFirstName]),
		LastName:           str(f[fieldCustomerLastName]),
		Email:              str(f[fieldCustomerEmail]),
		StripeID:           str(f[fieldCustomerStripeID]),
		Paid:               str(f[fieldCustomerPaid]) == "Paid",
		ReviewDiscount:     boolField(f[fieldCustomerReviewDiscount]),
		InvoiceBuildYear:   int(numField(f[fieldCustomerInvoiceBuildYear])),
		InvoiceLineItemIDs: stringSlice(f[fieldCustomerInvoices]),
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

// stringSlice extracts string values from Airtable link and lookup arrays.
func stringSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// isAirtableRecordID accepts rec-prefixed alphanumeric Airtable IDs.
// Validating public /pay input also prevents filterByFormula injection.
func isAirtableRecordID(id string) bool {
	if len(id) <= 3 || len(id) > 64 || !strings.HasPrefix(id, "rec") {
		return false
	}
	for _, r := range id[3:] {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// recordIDsFilter builds a stable Airtable list filter from raw linked record
// IDs. It depends only on Airtable's intrinsic RECORD_ID(), not mutable fields.
func recordIDsFilter(ids []string) (string, error) {
	clauses := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if !isAirtableRecordID(id) {
			return "", fmt.Errorf("invalid linked record ID %q", id)
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		clauses = append(clauses, fmt.Sprintf("RECORD_ID() = '%s'", id))
	}
	if len(clauses) == 0 {
		return "", fmt.Errorf("no linked record IDs")
	}
	if len(clauses) == 1 {
		return clauses[0], nil
	}
	return "OR(" + strings.Join(clauses, ",") + ")", nil
}

// linkContains reports whether a multipleRecordLinks field value (array of
// Airtable record IDs) contains the given record ID.
func linkContains(v interface{}, recordID string) bool {
	arr, ok := v.([]interface{})
	if !ok {
		return false
	}
	for _, item := range arr {
		if s, _ := item.(string); s == recordID {
			return true
		}
	}
	return false
}
