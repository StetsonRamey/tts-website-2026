package services

// Airtable client for the LEADS base.
//
// Requests go through the same exe.dev proxy as airtable.go:
//   https://airtable.int.exe.xyz/v0/{baseID}/{tableID}
// No API key needed — the proxy injects it.
//
// We read from "2026 Leads (forward)" which is where new estimates live.
// Field IDs are used instead of names for stability.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	leadsBase      = "appg9012rLh2diVLq"
	tableLeads2026 = "tblui0E6mBFkHGWvZ"

	// ── 2026 Leads (forward) field IDs ───────────────────────────────────────
	fieldLeadFirstName     = "fldplXExIaztUlnVf" // singleLineText
	fieldLeadLastName      = "fldiVRdwdOumpsrCh" // singleLineText
	fieldLeadFullName      = "fldASesUdamRf6QsC" // formula — "First Last"
	fieldLeadEmail         = "fldsvJF0WoUqKWOtq" // singleLineText
	fieldLeadPhone         = "fldGCBMLm7Ks1KD6N" // phoneNumber / singleLineText
	fieldLeadStreetAddress = "flduHz8NtmIzaTakp" // singleLineText
	fieldLeadCity          = "fldRHrzLnl0dIEScW" // singleLineText
	fieldLeadState         = "flddThKQZrXUkq2LD" // singleSelect (MO / KS)
	fieldLeadZip           = "fldSe1UzJLxSb5yWW" // number
	fieldLeadFeet          = "fldelBDSYukmjqNbo" // number
	fieldLeadPriceLED      = "fld5LWFylybQBCArw" // formula
	fieldLeadPriceRehang   = "fldC1G0g2nhXxXvc5" // formula
	fieldLeadPhoto         = "fldPT5nBi1q8NsaoR" // multipleAttachments
	fieldLeadRecordID      = "fldjMi3L50dNSg2bV" // formula — RECORD_ID()

	// ── Sold sync / invoice workflow fields ──────────────────────────────────
	fieldLeadStatus            = "fldfEuftFl2Y56jzx" // singleSelect — "Sold" triggers sync
	fieldLeadCompanyCamID      = "fldMVhdPoyhFk4rZ2" // singleLineText — CompanyCam project ID
	fieldLeadStripeID          = "fldU6bJBjtQvd6dAX" // singleLineText — Stripe customer (cus_...)
	fieldLeadStripeInvoiceLink = "fld2JkST42hZMHltf" // url — hosted_invoice_url
	fieldLeadMakeInvoice       = "fldskscCdCGUlxhXq" // checkbox — triggers invoice creation
	fieldLeadSendInvoice       = "fldmPoHcq6HBInZXC" // checkbox — (future: triggers send)
)

// Lead holds the fields we need from the 2026 Leads (forward) table.
type Lead struct {
	AirtableID    string // Airtable record ID (rec...)
	RecordID      string // same value, from the RECORD_ID() formula field
	FirstName     string
	LastName      string
	FullName      string
	Email         string
	Phone         string
	StreetAddress string
	City          string
	State         string
	Zip           string
	Feet          float64
	PriceLED      float64
	PriceRehang   float64
	Photos        []LeadPhoto

	// Workflow fields
	Status            string // singleSelect — "Sold" etc.
	CompanyCamID      string // CompanyCam project ID (set after sold/sync)
	StripeID          string // Stripe customer ID (cus_...)
	StripeInvoiceLink string // Stripe hosted invoice URL
}

// LeadPhoto is one attachment from the Photo field.
// URL is the temporary Airtable CDN link — callers must re-host before emailing.
type LeadPhoto struct {
	ID       string
	URL      string // expires in a few hours — download before use
	Filename string
}

// leadsURL builds the base URL for the LEADS Airtable proxy.
func leadsURL() string {
	return fmt.Sprintf("%s/%s/%s", airtableBase, leadsBase, tableLeads2026)
}

// GetLeadByRecordID fetches a single lead from the 2026 Leads (forward) table
// by its Airtable record ID (the rec... string).
func GetLeadByRecordID(recordID string) (*Lead, error) {
	params := atParams() // returnFieldsByFieldId=true is set for us

	// TODO: add a filterByFormula param to match the recordID formula field
	// hint: same pattern as GetCustomerByRecordID in airtable.go
	// params.Set(...)
	params.Set("filterByFormula", fmt.Sprintf("RECORD_ID() = '%s'", recordID))

	// TODO: request only the fields we need — add each fieldLead* constant
	// hint: first field uses params.Set("fields[]", ...), rest use params.Add
	params.Set("fields[]", fieldLeadFirstName)
	params.Add("fields[]", fieldLeadLastName)
	params.Add("fields[]", fieldLeadFullName)
	params.Add("fields[]", fieldLeadEmail)
	params.Add("fields[]", fieldLeadPhone)
	params.Add("fields[]", fieldLeadStreetAddress)
	params.Add("fields[]", fieldLeadCity)
	params.Add("fields[]", fieldLeadState)
	params.Add("fields[]", fieldLeadZip)
	params.Add("fields[]", fieldLeadFeet)
	params.Add("fields[]", fieldLeadPriceLED)
	params.Add("fields[]", fieldLeadPriceRehang)
	params.Add("fields[]", fieldLeadPhoto)
	params.Add("fields[]", fieldLeadRecordID)
	params.Add("fields[]", fieldLeadStatus)
	params.Add("fields[]", fieldLeadCompanyCamID)
	params.Add("fields[]", fieldLeadStripeID)
	params.Add("fields[]", fieldLeadStripeInvoiceLink)

	records, err := atGet(leadsURL() + "?" + params.Encode())
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no lead found for record ID %q", recordID)
	}

	return parseLead(records[0]), nil
}

// parseLead converts a raw Airtable record into a Lead struct.
func parseLead(r atRecord) *Lead {
	f := r.Fields
	zip := str(f[fieldLeadZip])
	if zip == "" {
		// Zip is stored as a number field — render integers without decimals.
		if n := numField(f[fieldLeadZip]); n != 0 {
			zip = fmt.Sprintf("%d", int64(n))
		}
	}
	return &Lead{
		AirtableID:        r.ID,
		RecordID:          str(f[fieldLeadRecordID]),
		FirstName:         str(f[fieldLeadFirstName]),
		LastName:          str(f[fieldLeadLastName]),
		FullName:          str(f[fieldLeadFullName]),
		Email:             str(f[fieldLeadEmail]),
		Phone:             str(f[fieldLeadPhone]),
		StreetAddress:     str(f[fieldLeadStreetAddress]),
		City:              str(f[fieldLeadCity]),
		State:             str(f[fieldLeadState]),
		Zip:               zip,
		Feet:              numField(f[fieldLeadFeet]),
		PriceLED:          numField(f[fieldLeadPriceLED]),
		PriceRehang:       numField(f[fieldLeadPriceRehang]),
		Photos:            parseAttachments(f[fieldLeadPhoto]),
		Status:            str(f[fieldLeadStatus]),
		CompanyCamID:      str(f[fieldLeadCompanyCamID]),
		StripeID:          str(f[fieldLeadStripeID]),
		StripeInvoiceLink: str(f[fieldLeadStripeInvoiceLink]),
	}
}

// PatchLeadFields updates one or more fields on a lead record in the LEADS base.
// Pass field IDs as keys (e.g. fieldLeadStripeID).
//
// Mirrors atPatch in airtable.go but targets the LEADS base instead of the
// Customers base.
func PatchLeadFields(airtableID string, fields map[string]interface{}) error {
	payload, _ := json.Marshal(map[string]interface{}{"fields": fields})
	req, _ := http.NewRequest(http.MethodPatch,
		fmt.Sprintf("%s/%s", leadsURL(), airtableID),
		bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("airtable PATCH leads/%s: %w", airtableID, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		var e atErrorResponse
		_ = json.Unmarshal(body, &e)
		return fmt.Errorf("airtable PATCH leads %d: %s — %s", resp.StatusCode, e.Error.Type, e.Error.Message)
	}
	return nil
}

// parseAttachments converts a multipleAttachments field value into []LeadPhoto.
// Airtable returns attachments as a JSON array of objects:
//
//	[{"id": "attXXX", "url": "https://...", "filename": "house.jpg", ...}, ...]
func parseAttachments(v interface{}) []LeadPhoto {
	if v == nil {
		return nil
	}

	// TODO: cast v to []interface{}
	x, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var photos []LeadPhoto
	for _, item := range x {
		y, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		photos = append(photos, LeadPhoto{
			ID: str(y["id"]),
			URL:        str(y["url"]),
			Filename:   str(y["filename"]),
		})
	}
	// TODO: extract "id", "url", "filename" from each map using str()
	// TODO: append to result slice and return
	return photos
}
