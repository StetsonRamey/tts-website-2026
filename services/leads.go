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
	"fmt"
)

const (
	leadsBase      = "appg9012rLh2diVLq"
	tableLeads2026 = "tblui0E6mBFkHGWvZ"

	// ── 2026 Leads (forward) field IDs ───────────────────────────────────────
	fieldLeadFirstName   = "fldplXExIaztUlnVf" // singleLineText
	fieldLeadLastName    = "fldLzoRuhDQ01XUCZ" // singleLineText
	fieldLeadEmail       = "fldsvJF0WoUqKWOtq" // singleLineText
	fieldLeadFeet        = "fldelBDSYukmjqNbo" // number
	fieldLeadPriceLED    = "fld5LWFylybQBCArw" // formula
	fieldLeadPriceRehang = "fldC1G0g2nhXxXvc5" // formula
	fieldLeadPhoto       = "fldPT5nBi1q8NsaoR" // multipleAttachments
	fieldLeadRecordID    = "fldjMi3L50dNSg2bV" // formula — RECORD_ID()
)

// Lead holds the fields we need from the 2026 Leads (forward) table.
type Lead struct {
	AirtableID  string // Airtable record ID (rec...)
	RecordID    string // same value, from the RECORD_ID() formula field
	FirstName   string
	LastName    string
	Email       string
	Feet        float64
	PriceLED    float64
	PriceRehang float64
	Photos      []LeadPhoto
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
	params.Add("fields[]", fieldLeadEmail)
	params.Add("fields[]", fieldLeadFeet)
	params.Add("fields[]", fieldLeadPriceLED)
	params.Add("fields[]", fieldLeadPriceRehang)
	params.Add("fields[]", fieldLeadPhoto)
	params.Add("fields[]", fieldLeadRecordID)

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
	return &Lead{
		AirtableID:  r.ID,
		RecordID:    str(f[fieldLeadRecordID]),
		FirstName:   str(f[fieldLeadFirstName]),
		LastName:    str(f[fieldLeadLastName]),
		Email:       str(f[fieldLeadEmail]),
		Feet:        numField(f[fieldLeadFeet]),
		PriceLED:    numField(f[fieldLeadPriceLED]),
		PriceRehang: numField(f[fieldLeadPriceRehang]),
		// TODO: fill in the remaining fields using the str(), numField() helpers
		// from airtable.go — they're in the same package so you can call them directly
		Photos: parseAttachments(f[fieldLeadPhoto]),
	}
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
