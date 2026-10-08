package services

// Fix request intake: POST /fix-request
//
// Replaces the old embedded Fillout form on /create-fix-request/. Creates a
// row in the Master Customer DB "Fix Tickets" table. The Photo field is an
// attachment field, and the exe.dev Airtable proxy cannot reach Airtable's
// upload endpoint, so photos are staged in photoDir (served at /photos/) and
// attached to the record by URL; Airtable copies them to its own storage.

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
)

const (
	tableFixTickets = "tblVqvNWOelGUyGII"

	// ── Fix Tickets table field IDs ──────────────────────────────────────────
	fieldFixFullName    = "fld7U7kwa3gXekM8q" // singleLineText
	fieldFixType        = "fldjCJhXtGpf7y9NW" // multipleSelects
	fieldFixPhone       = "fldQysqsfYxchBAn3" // phoneNumber
	fieldFixDescription = "fldnehQTx8NGSqCex" // multilineText
	fieldFixPhoto       = "fldN702My6jqUB56P" // multipleAttachments

	fixMaxPhotos     = 6
	fixMaxPhotoBytes = 10 << 20 // per photo; the browser downsizes before upload
	fixMaxBodyBytes  = 40 << 20
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
func FixRequestHandler(cfg *Config) http.HandlerFunc {
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

		urls, err := stageFixPhotos(files)
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
		if len(urls) > 0 {
			atts := make([]map[string]string, len(urls))
			for i, u := range urls {
				atts[i] = map[string]string{"url": u}
			}
			fields[fieldFixPhoto] = atts
		}
		if err := createFixTicket(fields); err != nil {
			log.Printf("fix request: airtable error: %v", err)
			cfg.sendErrorEmail(fmt.Sprintf("Fix request from %s (%s) could not be saved to Airtable: %v\n\nType: %s\nDescription: %s",
				name, phone, err, strings.Join(types, ", "), desc))
			fail(http.StatusServiceUnavailable, "We couldn't save your request right now. Please call or text us instead.")
			return
		}
		log.Printf("Fix request saved: %s %s [%s] photos=%d", name, phone, strings.Join(types, ","), len(urls))

		if !wantsJSON {
			http.Redirect(w, r, "/create-fix-request/#fix-received", http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	}
}

// stageFixPhotos validates each upload as an image, writes it to photoDir under
// an unguessable name, and returns the public URLs.
func stageFixPhotos(files []*multipart.FileHeader) ([]string, error) {
	var urls []string
	if len(files) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(photoDir, 0755); err != nil {
		return nil, fmt.Errorf("photo storage unavailable")
	}
	for _, fh := range files {
		if fh.Size > fixMaxPhotoBytes {
			return nil, fmt.Errorf("%q is too large (10 MB max per photo).", fh.Filename)
		}
		f, err := fh.Open()
		if err != nil {
			return nil, fmt.Errorf("couldn't read %q.", fh.Filename)
		}
		data, err := io.ReadAll(io.LimitReader(f, fixMaxPhotoBytes+1))
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("couldn't read %q.", fh.Filename)
		}
		ext := imageExt(data)
		if ext == "" {
			return nil, fmt.Errorf("%q isn't a supported image (JPEG, PNG, HEIC, or WebP).", fh.Filename)
		}
		token := make([]byte, 12)
		if _, err := rand.Read(token); err != nil {
			return nil, fmt.Errorf("photo storage unavailable")
		}
		filename := "fix-" + hex.EncodeToString(token) + ext
		if err := os.WriteFile(filepath.Join(photoDir, filename), data, 0644); err != nil {
			return nil, fmt.Errorf("photo storage unavailable")
		}
		urls = append(urls, photoBaseURL+"/"+filename)
	}
	return urls, nil
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

func createFixTicket(fields map[string]any) error {
	payload, _ := json.Marshal(map[string]any{"typecast": true, "fields": fields})
	resp, err := http.Post(atURL(tableFixTickets), "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("airtable POST: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e atErrorResponse
		_ = json.Unmarshal(body, &e)
		return fmt.Errorf("airtable %d: %s — %s", resp.StatusCode, e.Error.Type, e.Error.Message)
	}
	return nil
}
