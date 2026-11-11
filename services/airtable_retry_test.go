package services

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDoAirtableRetriesDialErrors(t *testing.T) {
	var sleeps int
	old := airtableSleep
	airtableSleep = func(time.Duration) { sleeps++ }
	defer func() { airtableSleep = old }()

	var bodies []string
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.WriteHeader(200)
	}))
	addr := srv.Listener.Addr().String()
	srv.Listener.Close() // port refuses connections until we start the server

	// Bring the server up after the first (refused) attempt.
	airtableSleep = func(time.Duration) {
		sleeps++
		if sleeps == 1 {
			l, err := net.Listen("tcp", addr)
			if err != nil {
				t.Fatal(err)
			}
			srv.Listener = l
			srv.Start()
		}
	}
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPatch, "http://"+addr, strings.NewReader("payload"))
	resp, err := DoAirtable(req)
	if err != nil {
		t.Fatalf("expected success after retry: %v", err)
	}
	resp.Body.Close()
	if sleeps != 1 || len(bodies) != 1 || bodies[0] != "payload" {
		t.Fatalf("sleeps=%d bodies=%v", sleeps, bodies)
	}
}

func TestDoAirtableGivesUpAndRetriesGET503(t *testing.T) {
	old := airtableSleep
	airtableSleep = func(time.Duration) {}
	defer func() { airtableSleep = old }()

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(503)
	}))
	defer srv.Close()

	resp, err := doAirtableGet(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if calls != 4 || resp.StatusCode != 503 {
		t.Fatalf("calls=%d status=%d", calls, resp.StatusCode)
	}

	calls = 0
	req, _ := http.NewRequest(http.MethodPatch, srv.URL, strings.NewReader("x"))
	resp, _ = DoAirtable(req)
	resp.Body.Close()
	if calls != 1 {
		t.Fatalf("PATCH 503 must not retry, calls=%d", calls)
	}
}
