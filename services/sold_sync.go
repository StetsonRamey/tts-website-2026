package services

// POST /sold/sync
//
// Triggered by an Airtable automation when a lead is marked Sold.
// Idempotent: skips work that has already been completed (CompanyCam project
// already exists, Stripe customer already exists).
//
// Each external ID is patched back to Airtable IMMEDIATELY after creation
// (before the next external call) — so a partial failure never causes a
// duplicate project or customer on retry.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// soldSyncRequest is the JSON body — just the Airtable record ID.
type soldSyncRequest struct {
	RecordID string `json:"recordId"`
}

// SoldSyncHandler returns the http.HandlerFunc for POST /sold/sync.
func SoldSyncHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if !RequireBearerAuth(w, r) {
			return
		}

		var req soldSyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.RecordID) == "" {
			http.Error(w, `{"error":"recordId is required"}`, http.StatusBadRequest)
			return
		}

		lead, err := GetLeadByRecordID(req.RecordID)
		if err != nil {
			log.Printf("[sold-sync] fetch lead %s: %v", req.RecordID, err)
			cfg.sendErrorEmail(fmt.Sprintf("sold-sync: fetch lead %s: %v", req.RecordID, err))
			http.Error(w, `{"error":"failed to fetch lead"}`, http.StatusInternalServerError)
			return
		}

		result := map[string]interface{}{
			"recordId": lead.RecordID,
			"name":     leadDisplayName(lead),
		}

		// ── CompanyCam ────────────────────────────────────────────────────────
		ccamID := lead.CompanyCamID
		if ccamID != "" {
			result["companyCamID"] = ccamID
			result["companyCamSkipped"] = true
		} else {
			id, err := createCompanyCamProject(cfg, lead)
			if err != nil {
				log.Printf("[sold-sync] companycam create for %s: %v", lead.RecordID, err)
				cfg.sendErrorEmail(fmt.Sprintf("sold-sync: companycam project create for %s: %v", lead.RecordID, err))
				// Continue to Stripe anyway — failures here shouldn't block customer creation.
			} else {
				// Patch IMMEDIATELY so a retry doesn't duplicate the project.
				if err := PatchLeadFields(lead.AirtableID, map[string]interface{}{
					fieldLeadCompanyCamID: id,
				}); err != nil {
					log.Printf("[sold-sync] patch CompanyCam ID for %s: %v", lead.RecordID, err)
					cfg.sendErrorEmail(fmt.Sprintf("sold-sync: patch CompanyCam ID for %s (id=%s): %v — manual cleanup required", lead.RecordID, id, err))
				}
				ccamID = id
				result["companyCamID"] = id
				result["companyCamCreated"] = true

				// Upload photos — non-fatal, project creation alone is "success".
				uploaded, failed := uploadCompanyCamPhotos(cfg, id, lead.Photos)
				result["photosUploaded"] = uploaded
				result["photosFailed"] = failed
			}
		}

		// ── Stripe customer ───────────────────────────────────────────────────
		stripeID := lead.StripeID
		if stripeID != "" {
			result["stripeID"] = stripeID
			result["stripeSkipped"] = true
		} else {
			id, err := createStripeCustomer(cfg, lead)
			if err != nil {
				log.Printf("[sold-sync] stripe create customer for %s: %v", lead.RecordID, err)
				cfg.sendErrorEmail(fmt.Sprintf("sold-sync: stripe customer create for %s: %v", lead.RecordID, err))
				http.Error(w, `{"error":"failed to create Stripe customer"}`, http.StatusInternalServerError)
				return
			}
			if err := PatchLeadFields(lead.AirtableID, map[string]interface{}{
				fieldLeadStripeID: id,
			}); err != nil {
				log.Printf("[sold-sync] patch Stripe ID for %s: %v", lead.RecordID, err)
				cfg.sendErrorEmail(fmt.Sprintf("sold-sync: patch Stripe ID for %s (id=%s): %v — manual cleanup required", lead.RecordID, id, err))
			}
			stripeID = id
			result["stripeID"] = id
			result["stripeCreated"] = true
		}

		log.Printf("[sold-sync] done record=%s ccam=%s stripe=%s", lead.RecordID, ccamID, stripeID)

		result["success"] = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}
}

// ── CompanyCam ───────────────────────────────────────────────────────────────

// createCompanyCamProject POSTs to CompanyCam /v2/projects.
// Returns the new project ID.
//
// Docs: https://docs.companycam.com/reference/createproject
func createCompanyCamProject(cfg *Config, lead *Lead) (string, error) {
	name := leadDisplayName(lead)
	body := map[string]interface{}{
		"name": name,
		"address": map[string]interface{}{
			"street_address_1": lead.StreetAddress,
			"city":             lead.City,
			"state":            lead.State,
			"postal_code":      lead.Zip,
			"country":          "US",
		},
	}
	// primary_contact requires a name; only attach if we have one.
	if name != "" {
		contact := map[string]interface{}{"name": name}
		if lead.Email != "" {
			contact["email"] = lead.Email
		}
		if lead.Phone != "" {
			contact["phone_number"] = lead.Phone
		}
		body["primary_contact"] = contact
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := companyCamRequest(cfg, http.MethodPost, "/projects", body, &resp); err != nil {
		return "", err
	}
	if resp.ID == "" {
		return "", fmt.Errorf("CompanyCam create project returned empty id")
	}
	return resp.ID, nil
}

// uploadCompanyCamPhotos uploads each Airtable photo URL to the CompanyCam
// project. Returns counts of successes and failures.
//
// Docs: https://docs.companycam.com/reference/createprojectphoto
func uploadCompanyCamPhotos(cfg *Config, projectID string, photos []LeadPhoto) (int, int) {
	uploaded, failed := 0, 0
	for _, p := range photos {
		if p.URL == "" {
			continue
		}
		body := map[string]interface{}{
			"photo": map[string]interface{}{
				"uri":         p.URL,
				"captured_at": time.Now().Unix(),
			},
		}
		var resp struct {
			ID string `json:"id"`
		}
		if err := companyCamRequest(cfg, http.MethodPost,
			fmt.Sprintf("/projects/%s/photos", projectID), body, &resp); err != nil {
			log.Printf("[sold-sync] photo upload failed (project=%s file=%s): %v", projectID, p.Filename, err)
			failed++
			continue
		}
		uploaded++
	}
	return uploaded, failed
}

// companyCamRequest performs a JSON request to the CompanyCam API.
// Sets Authorization: Bearer ... when COMPANYCAM_API_TOKEN is set,
// and X-CompanyCam-User when COMPANYCAM_USER_EMAIL is set.
func companyCamRequest(cfg *Config, method, path string, body interface{}, out interface{}) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, cfg.CompanyCamBaseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cfg.CompanyCamAPIToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.CompanyCamAPIToken)
	}
	if cfg.CompanyCamUserEmail != "" {
		req.Header.Set("X-CompanyCam-User", cfg.CompanyCamUserEmail)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("companycam %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 300 {
		return fmt.Errorf("companycam %s %s: %d %s", method, path, resp.StatusCode, string(respBody))
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// ── Stripe ───────────────────────────────────────────────────────────────────

// createStripeCustomer POSTs to /v1/customers via the exe.dev proxy.
// Returns the cus_... ID.
func createStripeCustomer(cfg *Config, lead *Lead) (string, error) {
	params := url.Values{}
	if name := leadDisplayName(lead); name != "" {
		params.Set("name", name)
	}
	if lead.Email != "" {
		params.Set("email", lead.Email)
	}
	if lead.Phone != "" {
		params.Set("phone", lead.Phone)
	}
	if lead.StreetAddress != "" {
		params.Set("address[line1]", lead.StreetAddress)
	}
	if lead.City != "" {
		params.Set("address[city]", lead.City)
	}
	if lead.State != "" {
		params.Set("address[state]", lead.State)
	}
	if lead.Zip != "" {
		params.Set("address[postal_code]", lead.Zip)
	}
	params.Set("address[country]", "US")
	// Reverse-lookup aid in Stripe dashboard.
	params.Set("metadata[airtable_record_id]", lead.RecordID)

	resp, err := http.Post(
		cfg.StripeBaseURL+"/v1/customers",
		"application/x-www-form-urlencoded",
		strings.NewReader(params.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("POST /v1/customers: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("stripe customer %d: %s", resp.StatusCode, string(body))
	}

	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decode customer response: %w", err)
	}
	if out.ID == "" {
		return "", fmt.Errorf("stripe returned empty customer id")
	}
	return out.ID, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

// leadDisplayName prefers the formula Full Name, falling back to "First Last".
func leadDisplayName(lead *Lead) string {
	if s := strings.TrimSpace(lead.FullName); s != "" {
		return s
	}
	return strings.TrimSpace(lead.FirstName + " " + lead.LastName)
}
