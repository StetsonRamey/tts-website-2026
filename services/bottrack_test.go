package services

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestClassifyBot(t *testing.T) {
	cases := []struct {
		ua   string
		name string
		kind botKind
	}{
		{"Mozilla/5.0 (compatible; GPTBot/1.0; +https://openai.com/gptbot)", "GPTBot", botAI},
		{"ClaudeBot/1.0 (+https://anthropic.com/claudebot)", "ClaudeBot", botAI},
		{"anthropic-ai/1.0", "Anthropic", botAI},
		{"Mozilla/5.0 (compatible; Googlebot/2.1)", "Googlebot", botSearch},
		{"Mozilla/5.0 (compatible; PerplexityBot/1.0)", "PerplexityBot", botAI},
		{"Mozilla/5.0 (Macintosh) Chrome/120", "", botNone},
		{"", "", botNone},
	}
	for _, c := range cases {
		name, kind := classifyBot(c.ua)
		if name != c.name || kind != c.kind {
			t.Errorf("classifyBot(%q) = (%q,%d), want (%q,%d)", c.ua, name, kind, c.name, c.kind)
		}
	}
}

func TestBotTrackingAndDashboard(t *testing.T) {
	// Isolate log file to a temp dir + set the auth key the dashboard checks.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("WEBHOOK_AUTH_KEY", "testkey123")
	botFileOnce = sync.Once{}
	botFile = ""

	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/missing", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/internal/bots", BotDashboardHandler(&Config{Env: "dev"}, true))
	wrapped := BotTrackMiddleware(mux)

	srv := httptest.NewServer(wrapped)
	defer srv.Close()

	do := func(ua, path string) {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		if ua != "" {
			req.Header.Set("User-Agent", ua)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	do("GPTBot/1.0", "/ok")
	do("GPTBot/1.0", "/ok")
	do("GPTBot/1.0", "/missing") // 404 → LLM-SEO gap
	do("ClaudeBot/1.0", "/ok")
	do("Googlebot/2.1", "/ok")
	do("Mozilla/5.0 Chrome/120", "/ok") // browser — must NOT be recorded

	hits := readBotHits()
	if len(hits) != 5 {
		t.Fatalf("expected 5 recorded hits (browser excluded), got %d", len(hits))
	}

	// Unauthed dashboard → 401
	resp, err := http.Get(srv.URL + "/internal/bots")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 {
		t.Errorf("unauthed dashboard = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	// Authed dashboard content checks
	req, _ := http.NewRequest("GET", srv.URL+"/internal/bots?days=7", nil)
	req.Header.Set("Authorization", "Bearer testkey123")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	bs := string(body)
	for _, want := range []string{"GPTBot", "ClaudeBot", "Googlebot", "/missing", "AI bots", "404"} {
		if !strings.Contains(bs, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
	if strings.Contains(bs, "Chrome") {
		t.Errorf("dashboard should not mention non-bot browser")
	}
}

func TestBotDataPathIsolatesByHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	botFileOnce = sync.Once{}
	botFile = ""
	p := botDataPath()
	want := filepath.Join(tmp, ".local", "share", "tts", "bot-crawls.jsonl")
	if p != want {
		t.Errorf("botDataPath() = %q, want %q", p, want)
	}
}
