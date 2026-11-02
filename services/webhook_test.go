package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebhookMarksCustomerPaidForPaymentIntentSucceeded(t *testing.T) {
	oldBase := airtableBase
	defer func() { airtableBase = oldBase }()

	var patchedFields map[string]interface{}
	airtable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/"+baseID+"/"+tableCustomers:
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"records":[{"id":"recSandraHay00001","fields":{%q:"Sandra Hay",%q:"cus_phone_payment"}}]}`,
				fieldCustomerFullName, fieldCustomerStripeID)
		case r.Method == http.MethodPatch && r.URL.Path == "/v0/"+baseID+"/"+tableCustomers+"/recSandraHay00001":
			var body struct {
				Fields map[string]interface{} `json:"fields"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode Airtable patch: %v", err)
			}
			patchedFields = body.Fields
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{}`)
		default:
			http.Error(w, "unexpected Airtable request", http.StatusNotFound)
		}
	}))
	defer airtable.Close()
	airtableBase = airtable.URL + "/v0"

	const secret = "webhook-test-secret"
	payload := []byte(`{"type":"payment_intent.succeeded","data":{"object":{"id":"pi_phone_payment","customer":"cus_phone_payment"}}}`)
	timestamp := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d.%s", timestamp, payload)))
	signature := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/stripe/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=%s", timestamp, signature))
	res := httptest.NewRecorder()

	WebhookHandler(&Config{WebhookSecret: secret})(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	if got := patchedFields[fieldCustomerPaid]; got != "Paid" {
		t.Fatalf("Paid field = %#v, want %q", got, "Paid")
	}
	if got := patchedFields[fieldCustomerReviewDiscount]; got != false {
		t.Fatalf("Review Discount field = %#v, want false", got)
	}
}

// fakeLeadsAirtable serves the Customers and Leads tables for webhook tests.
// leadRecords is a JSON array body fragment; customers answers every customer query.
func fakeLeadsAirtable(t *testing.T, customerRecords, leadRecords string, patched map[string]map[string]interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v0/"+baseID+"/"+tableCustomers:
			fmt.Fprintf(w, `{"records":[%s]}`, customerRecords)
		case r.Method == http.MethodGet && r.URL.Path == "/v0/"+leadsBase+"/"+tableLeads2026:
			fmt.Fprintf(w, `{"records":[%s]}`, leadRecords)
		case r.Method == http.MethodPatch:
			var body struct {
				Fields map[string]interface{} `json:"fields"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			patched[r.URL.Path] = body.Fields
			fmt.Fprint(w, `{}`)
		default:
			http.Error(w, "unexpected Airtable request", http.StatusNotFound)
		}
	}))
}

func postSignedWebhook(t *testing.T, payload string) *httptest.ResponseRecorder {
	t.Helper()
	const secret = "webhook-test-secret"
	timestamp := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d.%s", timestamp, payload)))
	req := httptest.NewRequest(http.MethodPost, "/stripe/webhook", strings.NewReader(payload))
	req.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil))))
	res := httptest.NewRecorder()
	WebhookHandler(&Config{WebhookSecret: secret})(res, req)
	return res
}

func leadRec(id, name, status, link, paid string) string {
	f := fmt.Sprintf(`%q:%q,%q:%q,%q:"cus_lead"`, fieldLeadFullName, name, fieldLeadStatus, status, fieldLeadStripeID)
	if link != "" {
		f += fmt.Sprintf(`,%q:%q`, fieldLeadStripeInvoiceLink, link)
	}
	if paid != "" {
		f += fmt.Sprintf(`,%q:%q`, fieldLeadPaid, paid)
	}
	return fmt.Sprintf(`{"id":%q,"fields":{%s}}`, id, f)
}

func TestWebhookInvoicePaidMarksLeadPaid(t *testing.T) {
	oldBase := airtableBase
	defer func() { airtableBase = oldBase }()
	patched := map[string]map[string]interface{}{}
	at := fakeLeadsAirtable(t, "", leadRec("recLead1", "Kim Stacks", "Sold", "https://invoice.stripe.com/i/x", ""), patched)
	defer at.Close()
	airtableBase = at.URL + "/v0"

	res := postSignedWebhook(t, `{"type":"invoice.paid","data":{"object":{"id":"in_1","customer":"cus_lead"}}}`)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d", res.Code)
	}
	got := patched["/v0/"+leadsBase+"/"+tableLeads2026+"/recLead1"]
	if got[fieldLeadPaid] != "Paid" {
		t.Fatalf("lead patch = %#v, want Paid?=Paid", got)
	}
	if len(patched) != 1 {
		t.Fatalf("unexpected extra patches: %#v", patched)
	}
}

func TestWebhookInvoicePaidAmbiguousLeadsChangesNothing(t *testing.T) {
	oldBase := airtableBase
	defer func() { airtableBase = oldBase }()
	patched := map[string]map[string]interface{}{}
	leads := leadRec("recA", "Elizabeth Bell", "Sold", "https://invoice.stripe.com/i/a", "") + "," +
		leadRec("recB", "Elizabeth Bell (2 of 2)", "Sold", "https://invoice.stripe.com/i/b", "")
	at := fakeLeadsAirtable(t, "", leads, patched)
	defer at.Close()
	airtableBase = at.URL + "/v0"

	postSignedWebhook(t, `{"type":"invoice.paid","data":{"object":{"id":"in_1","customer":"cus_lead"}}}`)
	if len(patched) != 0 {
		t.Fatalf("ambiguous match must not patch, got %#v", patched)
	}
}

func TestWebhookInvoicePaidPicksSoldUnpaidAmongSharedStripeID(t *testing.T) {
	oldBase := airtableBase
	defer func() { airtableBase = oldBase }()
	patched := map[string]map[string]interface{}{}
	leads := leadRec("recOld", "Old", "Sold", "https://invoice.stripe.com/i/a", "Paid") + "," +
		leadRec("recNew", "New", "Sold", "https://invoice.stripe.com/i/b", "") + "," +
		leadRec("recDead", "Dead", "Dead", "", "")
	at := fakeLeadsAirtable(t, "", leads, patched)
	defer at.Close()
	airtableBase = at.URL + "/v0"

	postSignedWebhook(t, `{"type":"invoice.paid","data":{"object":{"id":"in_1","customer":"cus_lead"}}}`)
	if _, ok := patched["/v0/"+leadsBase+"/"+tableLeads2026+"/recNew"]; !ok || len(patched) != 1 {
		t.Fatalf("patched = %#v, want only recNew", patched)
	}
}

func TestWebhookPaymentIntentForLeadOnlyCustomerIsNotAnError(t *testing.T) {
	oldBase := airtableBase
	defer func() { airtableBase = oldBase }()
	patched := map[string]map[string]interface{}{}
	at := fakeLeadsAirtable(t, "", leadRec("recLead1", "Kim Stacks", "Sold", "https://invoice.stripe.com/i/x", ""), patched)
	defer at.Close()
	airtableBase = at.URL + "/v0"

	// With no customer row, handlePaymentCompleted must return nil (no alert, no patch).
	if err := handlePaymentCompleted(&Config{}, "cus_lead", "pi_1"); err != nil {
		t.Fatalf("lead-only payment_intent returned error: %v", err)
	}
	if len(patched) != 0 {
		t.Fatalf("unexpected patches: %#v", patched)
	}
}
