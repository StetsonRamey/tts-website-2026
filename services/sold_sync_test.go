package services

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTextMagicPhone(t *testing.T) {
	cases := map[string]string{
		"(913) 707-4407":  "19137074407",
		"913-707-4407":    "19137074407",
		"+1 913 707 4407": "19137074407",
		"19137074407":     "19137074407",
		"707-4407":        "",
		"":                "",
	}
	for in, want := range cases {
		if got := textMagicPhone(in); got != want {
			t.Errorf("textMagicPhone(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCreateTextMagicContact(t *testing.T) {
	var gotBody string
	status, resp := http.StatusCreated, `{"id":1}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		if r.URL.Path != "/contacts" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(status)
		w.Write([]byte(resp))
	}))
	defer srv.Close()
	cfg := &Config{TextMagicBaseURL: srv.URL}
	lead := &Lead{FirstName: "Jane", LastName: "Doe", Phone: "(913) 555-0100", Email: "j@example.com"}

	if s, err := createTextMagicContact(cfg, lead); err != nil || s != "created" {
		t.Fatalf("created: %q %v", s, err)
	}
	if want := `{"email":"j@example.com","firstName":"Jane","lastName":"Doe","phone":"19135550100"}`; gotBody != want {
		t.Errorf("body = %s", gotBody)
	}

	// Phone already in TextMagic.
	status, resp = 400, `{"errors":{"fields":{"phone":["Phone number already exists in your contacts."]}}}`
	existing := `{"id":7,"phone":"19135550100","firstName":"19135550100","lastName":null,"email":null}`
	var putBody string
	lookupSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/contacts":
			w.WriteHeader(status)
			w.Write([]byte(resp))
		case r.Method == http.MethodGet && r.URL.Path == "/contacts/phone/19135550100":
			w.Write([]byte(existing))
		case r.Method == http.MethodPut && r.URL.Path == "/contacts/7":
			putBody = string(b)
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":7}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer lookupSrv.Close()
	cfg2 := &Config{TextMagicBaseURL: lookupSrv.URL}
	if s, err := createTextMagicContact(cfg2, lead); err != nil || s != "updated" {
		t.Fatalf("placeholder: %q %v", s, err)
	}
	if want := `{"email":"j@example.com","firstName":"Jane","lastName":"Doe","phone":"19135550100"}`; putBody != want {
		t.Errorf("put body = %s", putBody)
	}

	// Hand-entered name is left alone.
	existing, putBody = `{"id":7,"phone":"19135550100","firstName":"Janie","lastName":null,"email":null}`, ""
	if s, err := createTextMagicContact(cfg2, lead); err != nil || s != "exists" || putBody != "" {
		t.Fatalf("named: %q %v put=%q", s, err, putBody)
	}

	status, resp = 500, `boom`
	if _, err := createTextMagicContact(cfg, lead); err == nil {
		t.Fatal("expected error on 500")
	}
}
