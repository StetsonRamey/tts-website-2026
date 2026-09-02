package services

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAtGetFollowsPagination(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Query().Get("keep") != "yes" {
			http.Error(w, "missing existing query parameter", http.StatusBadRequest)
			return
		}
		switch r.URL.Query().Get("offset") {
		case "":
			fmt.Fprint(w, `{"records":[{"id":"recAAAAAAAAAAAAAA","fields":{}}],"offset":"next page"}`)
		case "next page":
			fmt.Fprint(w, `{"records":[{"id":"recBBBBBBBBBBBBBB","fields":{}}]}`)
		default:
			http.Error(w, "unexpected offset", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	records, err := atGet(server.URL + "?keep=yes")
	if err != nil {
		t.Fatalf("atGet: %v", err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
	got := []string{records[0].ID, records[1].ID}
	want := []string{"recAAAAAAAAAAAAAA", "recBBBBBBBBBBBBBB"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("record IDs = %v, want %v", got, want)
	}
}

func TestRecordIDsFilter(t *testing.T) {
	filter, err := recordIDsFilter([]string{
		"recAAAAAAAAAAAAAA",
		"recBBBBBBBBBBBBBB",
		"recAAAAAAAAAAAAAA",
	})
	if err != nil {
		t.Fatalf("recordIDsFilter: %v", err)
	}
	want := "OR(RECORD_ID() = 'recAAAAAAAAAAAAAA',RECORD_ID() = 'recBBBBBBBBBBBBBB')"
	if filter != want {
		t.Fatalf("filter = %q, want %q", filter, want)
	}

	if _, err := recordIDsFilter([]string{"not-a-record-id"}); err == nil {
		t.Fatal("recordIDsFilter accepted an invalid record ID")
	}
}

func TestGetCustomerByRecordIDRejectsFormulaInjection(t *testing.T) {
	if _, err := GetCustomerByRecordID("rec')OR(TRUE())"); err == nil {
		t.Fatal("GetCustomerByRecordID accepted invalid public input")
	}
}

func TestCheckoutRejectsMalformedRecordIDWithoutAirtableLookup(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/pay?q=rec')OR(TRUE())", nil)
	res := httptest.NewRecorder()

	CheckoutHandler(nil)(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusBadRequest)
	}
}

func TestGetInvoiceBuildYearLineItemsWithoutBuildMetadataReturnsEmpty(t *testing.T) {
	customer := &Customer{AirtableID: "recCUSTOMERAAAAAA"}
	items, err := GetInvoiceBuildYearLineItems(customer, "prod")
	if err != nil {
		t.Fatalf("GetInvoiceBuildYearLineItems: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %#v, want empty", items)
	}
}

func TestParseCustomerIncludesRawInvoiceMetadata(t *testing.T) {
	customer := parseCustomer(atRecord{
		ID: "recCUSTOMERAAAAAA",
		Fields: map[string]interface{}{
			fieldCustomerInvoiceBuildYear: 2026.0,
			fieldCustomerInvoices: []interface{}{
				"recITEMCURRENTAAA",
				"recITEMHISTORYAAA",
			},
		},
	})
	if customer.InvoiceBuildYear != 2026 {
		t.Fatalf("InvoiceBuildYear = %d, want 2026", customer.InvoiceBuildYear)
	}
	wantIDs := []string{"recITEMCURRENTAAA", "recITEMHISTORYAAA"}
	if !reflect.DeepEqual(customer.InvoiceLineItemIDs, wantIDs) {
		t.Fatalf("InvoiceLineItemIDs = %v, want %v", customer.InvoiceLineItemIDs, wantIDs)
	}
}

func TestInvoiceLookupUsesCustomerBuildYearAndRawLinks(t *testing.T) {
	const customerID = "recCUSTOMERAAAAAA"
	oldBase := airtableBase
	defer func() { airtableBase = oldBase }()

	requests := make(chan *http.Request, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v0/" + baseID + "/" + tableCustomers:
			fmt.Fprintf(w, `{"records":[{"id":%q,"fields":{%q:"Test Customer",%q:"cus_test",%q:2025,%q:["recITEM2025AAAAAA","recITEM2026AAAAAA"]}}]}`,
				customerID, fieldCustomerFullName, fieldCustomerStripeID,
				fieldCustomerInvoiceBuildYear, fieldCustomerInvoices)
		case "/v0/" + baseID + "/" + tableYearlyInvoicing:
			fmt.Fprintf(w, `{"records":[
				{"id":"recITEM2025AAAAAA","fields":{%q:"2025",%q:[%q],%q:["prod_live"],%q:10,%q:2.25}},
				{"id":"recITEM2026AAAAAA","fields":{%q:"2026",%q:[%q],%q:["prod_live"],%q:20,%q:2.25}}
			]}`,
				fieldInvYear, fieldInvCustomerLink, customerID, fieldInvStripeProductIDProd, fieldInvFinalValue, fieldInvUnitCost,
				fieldInvYear, fieldInvCustomerLink, customerID, fieldInvStripeProductIDProd, fieldInvFinalValue, fieldInvUnitCost)
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer server.Close()
	airtableBase = server.URL + "/v0"

	customer, err := GetCustomerByRecordID(customerID)
	if err != nil {
		t.Fatalf("GetCustomerByRecordID: %v", err)
	}
	items, err := GetInvoiceBuildYearLineItems(customer, "prod")
	if err != nil {
		t.Fatalf("GetInvoiceBuildYearLineItems: %v", err)
	}
	if len(items) != 1 || items[0].AirtableID != "recITEM2025AAAAAA" {
		t.Fatalf("items = %#v, want only the 2025 build-year row", items)
	}

	customerReq := <-requests
	if got := customerReq.URL.Query().Get("filterByFormula"); got != "RECORD_ID() = '"+customerID+"'" {
		t.Fatalf("customer filter = %q", got)
	}
	invoiceReq := <-requests
	filter := invoiceReq.URL.Query().Get("filterByFormula")
	if !strings.Contains(filter, "recITEM2025AAAAAA") || !strings.Contains(filter, "recITEM2026AAAAAA") {
		t.Fatalf("invoice filter = %q, want both raw linked record IDs", filter)
	}
	fields := invoiceReq.URL.Query()["fields[]"]
	if !containsString(fields, fieldInvYear) || !containsString(fields, fieldInvCustomerLink) {
		t.Fatalf("invoice fields = %v, want raw Year and Customer Link", fields)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestLineItemsForCustomerYearUsesRawFields(t *testing.T) {
	customerID := "recCUSTOMERAAAAAA"
	records := []atRecord{
		{
			ID: "recITEMCURRENTAAA",
			Fields: map[string]interface{}{
				fieldInvYear:         "2026",
				fieldInvCustomerLink: []interface{}{customerID},
				fieldInvUnitCost:     12.5,
			},
		},
		{
			ID: "recITEMHISTORYAAA",
			Fields: map[string]interface{}{
				fieldInvYear:         "2025",
				fieldInvCustomerLink: []interface{}{customerID},
			},
		},
		{
			ID: "recITEMOTHERAAAAA",
			Fields: map[string]interface{}{
				fieldInvYear:         "2026",
				fieldInvCustomerLink: []interface{}{"recSOMEONEELSEAAA"},
			},
		},
	}

	items := lineItemsForCustomerYear(records, customerID, "2026")
	if len(items) != 1 || items[0].AirtableID != "recITEMCURRENTAAA" {
		t.Fatalf("items = %#v, want only current-year item", items)
	}
}
