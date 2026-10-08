package services

// Fix request intake: POST /fix-request
//
// Replaces the old embedded Fillout form on /create-fix-request/. Creates a
// row in the Master Customer DB "Fix Tickets" table. The Photo field is an
// attachment field, and the exe.dev Airtable proxy cannot reach Airtable's
// upload endpoint, so photos are staged in photoDir (served at /photos/) and
// attached to the record by URL; Airtable copies them to its own storage, after
// which the staged files are deleted. The ticket is linked to the matching
// Customers record by phone number (then name) when that can be done
// unambiguously; otherwise the Customers link is left for manual entry.

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	tableFixTickets = "tblVqvNWOelGUyGII"

	// ── Fix Tickets table field IDs ──────────────────────────────────────────
	fieldFixFullName    = "fld7U7kwa3gXekM8q" // singleLineText
	fieldFixType        = "fldjCJhXtGpf7y9NW" // multipleSelects
	fieldFixPhone       = "fldQysqsfYxchBAn3" // phoneNumber
	fieldFixDescription = "fldnehQTx8NGSqCex" // multilineText
	fieldFixPhoto       = "fldN702My6jqUB56P" // multipleAttachments
	fieldFixCustomers   = "fld57YoQGFuCWSOGB" // multipleRecordLinks → Customers

	// ── Customers table fields used for matching ─────────────────────────────
	fieldCustomerPhoneDigits = "fldI8p4WTPgV3FnmC" // formula: 10-digit number
	fieldCustomerActive      = "fldVyL33NVLa3qmdT" // singleSelect; "Active" = current customer

	fixMaxPhotos     = 6
	fixMaxPhotoBytes = 10 << 20 // per photo; the browser downsizes before upload
	fixMaxBodyBytes  = 40 << 20

	fixMaxPerHour   = 5
	fixStagedMaxAge = 24 * time.Hour // safety-net sweep for photos never cleaned up
)

// Existing options on the "Type of Fix" multi-select. Do not add new ones in code.
var fixTypes = map[string]bool{
	"Bulb(s) Out":       true,
	"Bulb(s) unclipped": true,
	"Other Issue":       true,
}

var fixDigitsRe = regexp.MustCompile(`\D`)

// FixRequestHandler accepts multipart/form-data fix requests. fetch clients
// get JSON; plain form posts are redirected to the page's success anchor.
//
// clientIP and countryFor are supplied by server.go (shared with /contact).
func FixRequestHandler(cfg *Config, clientIP func(*http.Request) string, countryFor func(ip string) (string, error)) http.HandlerFunc {
	limiter := &fixLimiter{hits: map[string][]time.Time{}}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/create-fix-request/", http.StatusSeeOther)
			return
		}
		wantsJSON := strings.Contains(r.Header.Get("Accept"), "application/json")
		fail := func(status int, msg string) {
			if wantsJSON {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				json.NewEncoder(w).Encode(map[string]any{"success": false, "error": msg})
				return
			}
			http.Error(w, msg, status)
		}

		r.Body = http.MaxBytesReader(w, r.Body, fixMaxBodyBytes)
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			fail(http.StatusBadRequest, "We couldn't read your submission. Photos may be too large — try fewer or smaller photos.")
			return
		}
		defer r.MultipartForm.RemoveAll()

		// Spam: honeypot, non-US, rate limit. Rejections get a neutral success
		// response so bots learn nothing.
		ip := clientIP(r)
		neutral := func(reason string) {
			log.Printf("fix request REJECTED %s from %s", reason, ip)
			succeed(w, r, wantsJSON)
		}
		if r.FormValue("website") != "" {
			neutral("honeypot")
			return
		}
		if country, err := countryFor(ip); err == nil && country != "US" {
			neutral("non_us_ip (" + country + ")")
			return
		}
		if !limiter.allow(ip) {
			neutral("rate_limited")
			return
		}

		name := strings.TrimSpace(r.FormValue("fullName"))
		phone := strings.TrimSpace(r.FormValue("phone"))
		desc := strings.TrimSpace(r.FormValue("description"))
		var types []string
		for _, t := range r.Form["fixType"] {
			if fixTypes[t] {
				types = append(types, t)
			}
		}

		var errs []string
		if len(name) < 2 {
			errs = append(errs, "Please enter your full name.")
		}
		if len(fixDigitsRe.ReplaceAllString(phone, "")) != 10 {
			errs = append(errs, "Phone number must be 10 digits.")
		}
		if len(types) == 0 {
			errs = append(errs, "Please choose the type of fix.")
		}
		if desc == "" {
			errs = append(errs, "Please describe what needs fixing.")
		}
		files := r.MultipartForm.File["photos"]
		if len(files) > fixMaxPhotos {
			errs = append(errs, fmt.Sprintf("Please attach no more than %d photos.", fixMaxPhotos))
		}
		if len(errs) > 0 {
			fail(http.StatusBadRequest, strings.Join(errs, " "))
			return
		}

		sweepStagedFixPhotos()
		staged, err := stageFixPhotos(files)
		if err != nil {
			log.Printf("fix request: photo error: %v", err)
			fail(http.StatusBadRequest, err.Error())
			return
		}

		fields := map[string]any{
			fieldFixFullName:    name,
			fieldFixType:        types,
			fieldFixPhone:       phone,
			fieldFixDescription: desc,
		}
		if len(staged) > 0 {
			atts := make([]map[string]string, len(staged))
			for i, f := range staged {
				atts[i] = map[string]string{"url": f.url}
			}
			fields[fieldFixPhoto] = atts
		}
		customerID, err := matchFixCustomer(name, phone)
		if err != nil {
			log.Printf("fix request: customer match failed (saving unlinked): %v", err)
		}
		if customerID != "" {
			fields[fieldFixCustomers] = []string{customerID}
		}
		rec, err := createFixTicket(fields)
		if err != nil {
			removeStaged(staged)
			log.Printf("fix request: airtable error: %v", err)
			cfg.sendErrorEmail(fmt.Sprintf("Fix request from %s (%s) could not be saved to Airtable: %v\n\nType: %s\nDescription: %s",
				name, phone, err, strings.Join(types, ", "), desc))
			fail(http.StatusServiceUnavailable, "We couldn't save your request right now. Please call or text us instead.")
			return
		}
		log.Printf("Fix request saved: %s %s [%s] photos=%d customer=%q", name, phone, strings.Join(types, ","), len(staged), customerID)
		if len(staged) > 0 {
			go cleanupStagedWhenCopied(rec.ID, staged)
		}

		succeed(w, r, wantsJSON)
	}
}

func succeed(w http.ResponseWriter, r *http.Request, wantsJSON bool) {
	if !wantsJSON {
		http.Redirect(w, r, "/create-fix-request/#fix-received", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true})
}

// ── Rate limit ───────────────────────────────────────────────────────────────

type fixLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func (l *fixLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-time.Hour)
	var recent []time.Time
	for _, t := range l.hits[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= fixMaxPerHour {
		l.hits[ip] = recent
		return false
	}
	l.hits[ip] = append(recent, time.Now())
	return true
}

// ── Photo staging and cleanup ────────────────────────────────────────────────

type stagedPhoto struct {
	path string
	url  string
}

func removeStaged(photos []stagedPhoto) {
	for _, p := range photos {
		os.Remove(p.path)
	}
}

// sweepStagedFixPhotos deletes fix-* files older than fixStagedMaxAge, a
// safety net for photos whose cleanup never ran (e.g. a restart mid-copy).
func sweepStagedFixPhotos() {
	entries, err := os.ReadDir(photoDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "fix-") {
			continue
		}
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > fixStagedMaxAge {
			os.Remove(filepath.Join(photoDir, e.Name()))
		}
	}
}

// cleanupStagedWhenCopied deletes staged files once every attachment on the
// record is served from Airtable's own storage instead of our /photos/ URL.
// Airtable normally copies during the create call; polling covers any delay.
// If it never confirms, the age-based sweep removes the files later.
func cleanupStagedWhenCopied(recordID string, photos []stagedPhoto) {
	for _, wait := range []time.Duration{0, 5 * time.Second, 30 * time.Second, 2 * time.Minute, 5 * time.Minute} {
		time.Sleep(wait)
		rec, err := atGetRecord(fmt.Sprintf("%s/%s?returnFieldsByFieldId=true", atURL(tableFixTickets), recordID))
		if err != nil {
			continue
		}
		if attachmentsCopied(rec.Fields[fieldFixPhoto]) {
			removeStaged(photos)
			return
		}
	}
	log.Printf("fix request: photos for %s not confirmed copied; leaving for age sweep", recordID)
}

func attachmentsCopied(v interface{}) bool {
	list, _ := v.([]interface{})
	if len(list) == 0 {
		return false
	}
	for _, item := range list {
		m, _ := item.(map[string]interface{})
		if u, _ := m["url"].(string); u == "" || strings.HasPrefix(u, photoBaseURL+"/") {
			return false
		}
	}
	return true
}

// stageFixPhotos validates each upload as an image, writes it to photoDir under
// an unguessable name, and returns the public URLs.
func stageFixPhotos(files []*multipart.FileHeader) ([]stagedPhoto, error) {
	var staged []stagedPhoto
	fail := func(err error) ([]stagedPhoto, error) {
		removeStaged(staged)
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(photoDir, 0755); err != nil {
		return fail(fmt.Errorf("photo storage unavailable"))
	}
	for _, fh := range files {
		if fh.Size > fixMaxPhotoBytes {
			return fail(fmt.Errorf("%q is too large (10 MB max per photo).", fh.Filename))
		}
		f, err := fh.Open()
		if err != nil {
			return fail(fmt.Errorf("couldn't read %q.", fh.Filename))
		}
		data, err := io.ReadAll(io.LimitReader(f, fixMaxPhotoBytes+1))
		f.Close()
		if err != nil {
			return fail(fmt.Errorf("couldn't read %q.", fh.Filename))
		}
		ext := imageExt(data)
		if ext == "" {
			return fail(fmt.Errorf("%q isn't a supported image (JPEG, PNG, HEIC, or WebP).", fh.Filename))
		}
		token := make([]byte, 12)
		if _, err := rand.Read(token); err != nil {
			return fail(fmt.Errorf("photo storage unavailable"))
		}
		filename := "fix-" + hex.EncodeToString(token) + ext
		path := filepath.Join(photoDir, filename)
		if err := os.WriteFile(path, data, 0644); err != nil {
			return fail(fmt.Errorf("photo storage unavailable"))
		}
		staged = append(staged, stagedPhoto{path: path, url: photoBaseURL + "/" + filename})
	}
	return staged, nil
}

// imageExt sniffs the file signature and returns a file extension, or "".
func imageExt(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("\xff\xd8\xff")):
		return ".jpg"
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return ".png"
	case len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return ".webp"
	case len(b) >= 12 && string(b[4:8]) == "ftyp" && isHEIFBrand(string(b[8:12])):
		return ".heic"
	}
	return ""
}

func isHEIFBrand(b string) bool {
	switch b {
	case "heic", "heix", "hevc", "hevx", "heim", "heis", "mif1", "msf1":
		return true
	}
	return false
}

func createFixTicket(fields map[string]any) (atRecord, error) {
	payload, _ := json.Marshal(map[string]any{"typecast": true, "fields": fields})
	resp, err := http.Post(atURL(tableFixTickets), "application/json", bytes.NewReader(payload))
	if err != nil {
		return atRecord{}, fmt.Errorf("airtable POST: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e atErrorResponse
		_ = json.Unmarshal(body, &e)
		return atRecord{}, fmt.Errorf("airtable %d: %s — %s", resp.StatusCode, e.Error.Type, e.Error.Message)
	}
	var rec atRecord
	_ = json.Unmarshal(body, &rec)
	return rec, nil
}

// ── Customer matching ────────────────────────────────────────────────────────

type fixCandidate struct {
	ID     string
	First  string
	Last   string
	Active bool
}

var nonLetterRe = regexp.MustCompile(`[^a-z]+`)

func normName(s string) string { return nonLetterRe.ReplaceAllString(strings.ToLower(s), "") }

// matchFixCustomer returns the Customers record ID for a submission, or ""
// when no single customer can be identified (staff then link it manually).
// Phone is the primary key; name only breaks ties or stands in when the phone
// is unknown. Never guesses between multiple plausible customers.
func matchFixCustomer(fullName, phone string) (string, error) {
	digits := fixDigitsRe.ReplaceAllString(phone, "")
	if len(digits) != 10 {
		return "", nil
	}
	cands, err := fetchCustomerCandidates(fmt.Sprintf("{%s}=%s", fieldCustomerPhoneDigits, digits))
	if err != nil {
		return "", err
	}
	if len(cands) > 0 {
		return pickFixCustomer(cands, fullName), nil
	}

	// Unknown phone (new number, spouse's phone…): exact first + last name.
	toks := strings.Fields(fullName)
	if len(toks) < 2 {
		return "", nil
	}
	first, last := strings.ToLower(toks[0]), strings.ToLower(toks[len(toks)-1])
	formula := fmt.Sprintf(`AND(LOWER(TRIM({%s}))="%s",LOWER(TRIM({%s}))="%s")`,
		fieldCustomerFirstName, escFormula(first), fieldCustomerLastName, escFormula(last))
	cands, err = fetchCustomerCandidates(formula)
	if err != nil {
		return "", err
	}
	return pickFixCustomer(cands, fullName), nil
}

// pickFixCustomer narrows candidates to exactly one, or returns "".
func pickFixCustomer(cands []fixCandidate, fullName string) string {
	if len(cands) == 1 {
		return cands[0].ID
	}
	// Several properties on one phone: narrow by last name, then active status.
	submitted := normName(fullName)
	var byName []fixCandidate
	for _, c := range cands {
		if l := normName(c.Last); l != "" && strings.Contains(submitted, l) {
			byName = append(byName, c)
		}
	}
	if len(byName) == 1 {
		return byName[0].ID
	}
	if len(byName) > 1 {
		cands = byName
	}
	var active []fixCandidate
	for _, c := range cands {
		if c.Active {
			active = append(active, c)
		}
	}
	if len(active) == 1 {
		return active[0].ID
	}
	return ""
}

func fetchCustomerCandidates(formula string) ([]fixCandidate, error) {
	q := atParams()
	q.Set("filterByFormula", formula)
	q.Set("maxRecords", "10")
	for _, f := range []string{fieldCustomerFirstName, fieldCustomerLastName, fieldCustomerActive} {
		q.Add("fields[]", f)
	}
	recs, err := atGet(atURL(tableCustomers) + "?" + q.Encode())
	if err != nil {
		return nil, err
	}
	out := make([]fixCandidate, 0, len(recs))
	for _, r := range recs {
		out = append(out, fixCandidate{
			ID:     r.ID,
			First:  str(r.Fields[fieldCustomerFirstName]),
			Last:   str(r.Fields[fieldCustomerLastName]),
			Active: str(r.Fields[fieldCustomerActive]) == "Active",
		})
	}
	return out, nil
}

func escFormula(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}
