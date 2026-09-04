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
