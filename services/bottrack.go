package services

// AI / crawler tracking.
//
// Purpose: client-side analytics (Umami/Plausible/PostHog) cannot see bots —
// bots don't execute JavaScript. The only place AI-crawler traffic exists is
// in this server's incoming HTTP requests (User-Agent + path). This module
// records those hits to a JSONL file and exposes a bearer-auth-protected
// dashboard at /internal/bots so the site owner can see:
//
//   - which AI bots are crawling, and how often
//   - which paths each bot requests most
//   - 404s returned to AI bots  →  LLM-SEO content gaps to fill
//
// The file is append-only JSONL, one record per line:
//   {"ts":"2026-07-13T20:11:00Z","bot":"GPTBot","ua":"...","path":"/faq","status":200,"ip":"1.2.3.4"}
//
// Non-AI bots (Googlebot, Bingbot, DuckDuckBot) are recorded too but tagged
// distinctly so the dashboard can separate "AI training/search bots" from
// "classic search indexers".

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ── Bot signature table ───────────────────────────────────────────────────

type botKind int

const (
	botNone  botKind = iota
	botAI    // AI training / answer engines: OpenAI, Anthropic, Google AI, Perplexity, etc.
	botSearch // Classic search indexers: Googlebot, Bingbot, DuckDuckBot, etc.
	botOther  // Other known crawlers / archivers
)

type botSig struct {
	match string // case-insensitive substring of User-Agent
	name  string // human-readable label
	kind  botKind
}

// Order matters: first match wins. More specific signatures first.
var botSignatures = []botSig{
	// ── AI / answer-engine crawlers (the ones we care most about) ──
	{"ClaudeBot", "ClaudeBot", botAI},
	{"anthropic-ai", "Anthropic", botAI},
	{"Claude-Web", "Claude-Web", botAI},
	{"GPTBot", "GPTBot", botAI},
	{"OAI-SearchBot", "OAI-SearchBot", botAI},
	{"ChatGPT-User", "ChatGPT-User", botAI},
	{"Google-Extended", "Google-Extended", botAI}, // Google's AI training crawler
	{"PerplexityBot", "PerplexityBot", botAI},
	{"Perplexity-User", "Perplexity-User", botAI},
	{"CCBot", "CCBot", botAI}, // Common Crawl — feeds many models
	{"Bytespider", "Bytespider", botAI},
	{"Amazonbot", "Amazonbot", botAI},
	{"Applebot-Extended", "Applebot-Extended", botAI},
	{"FacebookBot", "MetaBot", botAI},
	{"meta-externalagent", "Meta-ExternalAgent", botAI},
	{"Diffbot", "Diffbot", botAI},
	{"ImagesiftBot", "ImagesiftBot", botAI},
	{"Omgilibot", "Omgili", botAI},
	{"Omgili", "Omgili", botAI},
	{"YouBot", "YouBot", botAI},
	{"Cotoyogi", "Cotoyogi", botAI},
	{"Timpibot", "Timpibot", botAI},
	{"PiplBot", "PiplBot", botAI},

	// ── Classic search-engine indexers (tracked, but separate bucket) ──
	{"Googlebot", "Googlebot", botSearch},
	{"Bingbot", "Bingbot", botSearch},
	{"Slurp", "Slurp (Yahoo)", botSearch},
	{"DuckDuckBot", "DuckDuckBot", botSearch},
	{"Baiduspider", "Baiduspider", botSearch},
	{"YandexBot", "YandexBot", botSearch},
	{"Sogou", "Sogou", botSearch},
	{"Exabot", "Exabot", botSearch},
	{"facebot", "Facebot", botSearch},
	{"facebookexternalhit", "Facebook External Hit", botSearch},

	// ── Other known crawlers / archivers ──
	{"archive.org_bot", "Archive.org", botOther},
	{"WaybackMachine", "Wayback", botOther},
	{"Wget", "Wget", botOther},
}

// classifyBot returns (name, kind). kind == botNone means "not a known bot".
func classifyBot(ua string) (string, botKind) {
	if ua == "" {
		return "", botNone
	}
	l := strings.ToLower(ua)
	for _, s := range botSignatures {
		if strings.Contains(l, strings.ToLower(s.match)) {
			return s.name, s.kind
		}
	}
	return "", botNone
}

// ── Tracker ───────────────────────────────────────────────────────────────

type botHit struct {
	TS     string `json:"ts"`
	Bot    string `json:"bot"`
	Kind   string `json:"kind"` // "ai" | "search" | "other"
	UA     string `json:"ua"`
	Path   string `json:"path"`
	Status int    `json:"status"`
	IP     string `json:"ip"`
}

var (
	botMu       sync.Mutex
	botFile     string
	botFileOnce sync.Once
)

// botDataPath resolves the JSONL log location. We put it under the user's
// local share dir so it survives reboots and is owned by the service user.
func botDataPath() string {
	botFileOnce.Do(func() {
		dir := filepath.Join(os.Getenv("HOME"), ".local", "share", "tts")
		_ = os.MkdirAll(dir, 0o755)
		botFile = filepath.Join(dir, "bot-crawls.jsonl")
	})
	return botFile
}

func kindLabel(k botKind) string {
	switch k {
	case botAI:
		return "ai"
	case botSearch:
		return "search"
	case botOther:
		return "other"
	default:
		return ""
	}
}

// recordBotHit appends one structured record to the JSONL log.
// Cheap: one file open + write per hit, under a mutex. Bot traffic is low
// volume (even a busy day is a few thousand hits), so this is fine.
func recordBotHit(name string, kind botKind, ua, path, ip string, status int) {
	hit := botHit{
		TS:     time.Now().UTC().Format(time.RFC3339),
		Bot:    name,
		Kind:   kindLabel(kind),
		UA:     truncate(ua, 400),
		Path:   path,
		Status: status,
		IP:     ip,
	}
	line, err := json.Marshal(hit)
	if err != nil {
		return
	}
	line = append(line, '\n')

	botMu.Lock()
	defer botMu.Unlock()

	f, err := os.OpenFile(botDataPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("[bottrack] write failed: %v", err)
		return
	}
	defer f.Close()
	if _, err := f.Write(line); err != nil {
		log.Printf("[bottrack] write failed: %v", err)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// readBotHits loads all records from the JSONL log. Malformed lines are
// skipped silently (the file is append-only and trusted).
func readBotHits() []botHit {
	botMu.Lock()
	path := botDataPath()
	botMu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var hits []botHit
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var h botHit
		if json.Unmarshal([]byte(line), &h) == nil {
			hits = append(hits, h)
		}
	}
	return hits
}

// ── Middleware ────────────────────────────────────────────────────────────

// BotTrackMiddleware classifies the request's User-Agent and, if it's a known
// bot, records the hit after the downstream handler runs. It uses a wrapping
// status recorder to capture the response code. Non-bot requests pass through
// untouched with effectively zero overhead (a single UA substring scan).
func BotTrackMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, kind := classifyBot(r.UserAgent())
		if kind == botNone {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		recordBotHit(name, kind, r.UserAgent(), r.URL.Path, clientIP(r), rec.status)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func clientIP(r *http.Request) string {
	// exe.dev proxy sets X-Forwarded-For; first hop is the real client.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	return r.RemoteAddr
}

// ── Dashboard aggregation types ──────────────────────────────────────────

type pathStat struct {
	path   string
	count  int
	status int // last seen status for this bot+path
}

type botStat struct {
	name     string
	kind     string
	total    int
	notFound int // 404 count
	paths    map[string]*pathStat
}

// ── Dashboard handler ────────────────────────────────────────────────────

// BotDashboardHandler returns an HTML summary of recent bot crawling.
//
// auth controls how the endpoint is gated:
//   - true  → require Authorization: Bearer WEBHOOK_AUTH_KEY (used when the
//             endpoint is exposed on the public-facing mux).
//   - false → no bearer check; intended for the internal-only listener whose
//             access is already gated by exe.dev's private-proxy login. This
//             avoids putting a secret in the URL while keeping the dashboard
//             a one-click bookmark for the VM owner.
//
// Pass ?days=N to change the window (default 7, max 90).
func BotDashboardHandler(cfg *Config, auth bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth && !RequireBearerAuth(w, r) {
			return
		}

		days := 7
		if d := r.URL.Query().Get("days"); d != "" {
			if n, err := strconv.Atoi(d); err == nil && n > 0 && n <= 90 {
				days = n
			}
		}
		cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)

		hits := readBotHits()

		bots := map[string]*botStat{}
		var recent []botHit
		totalAI, totalSearch, totalOther := 0, 0, 0

		for _, h := range hits {
			t, err := time.Parse(time.RFC3339, h.TS)
			if err != nil {
				continue
			}
			if t.Before(cutoff) {
				continue
			}
			b := bots[h.Bot]
			if b == nil {
				b = &botStat{name: h.Bot, kind: h.Kind, paths: map[string]*pathStat{}}
				bots[h.Bot] = b
			}
			b.total++
			if h.Status == 404 {
				b.notFound++
			}
			ps := b.paths[h.Path]
			if ps == nil {
				ps = &pathStat{path: h.Path, status: h.Status}
				b.paths[h.Path] = ps
			}
			ps.count++
			ps.status = h.Status

			switch h.Kind {
			case "ai":
				totalAI++
			case "search":
				totalSearch++
			default:
				totalOther++
			}

			recent = append(recent, h)
		}

		// Sort bots by total desc
		var botList []*botStat
		for _, b := range bots {
			botList = append(botList, b)
		}
		sort.Slice(botList, func(i, j int) bool { return botList[i].total > botList[j].total })

		// Recent: last 50, newest first
		if len(recent) > 50 {
			recent = recent[len(recent)-50:]
		}
		for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
			recent[i], recent[j] = recent[j], recent[i]
		}

		renderBotDashboard(w, days, botList, recent, totalAI, totalSearch, totalOther)
	}
}

func renderBotDashboard(w http.ResponseWriter, days int, bots []*botStat, recent []botHit, totalAI, totalSearch, totalOther int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var sb strings.Builder
	sb.WriteString(`<!doctype html><html><head><meta charset=utf-8>`)
	sb.WriteString(`<meta name=viewport content="width=device-width,initial-scale=1">`)
	sb.WriteString(`<title>TTS — Bot Crawl Dashboard</title>`)
	sb.WriteString(`<style>
		body{font:14px/1.5 -apple-system,Segoe UI,Roboto,sans-serif;margin:0;background:#0f1115;color:#e6e6e6}
		.wrap{max-width:980px;margin:0 auto;padding:24px}
		h1{font-size:20px;margin:0 0 4px}
		.sub{color:#8a93a4;margin:0 0 20px}
		.summary{display:flex;gap:12px;flex-wrap:wrap;margin-bottom:24px}
		.card{background:#1a1d24;border:1px solid #262b34;border-radius:8px;padding:14px 18px;min-width:120px}
		.card .n{font-size:26px;font-weight:700}
		.card .l{font-size:12px;color:#8a93a4;text-transform:uppercase;letter-spacing:.5px}
		.bot{background:#1a1d24;border:1px solid #262b34;border-radius:8px;padding:16px 18px;margin-bottom:14px}
		.bot h2{margin:0 0 10px;font-size:16px}
		.badge{display:inline-block;font-size:11px;padding:2px 8px;border-radius:10px;margin-left:8px;vertical-align:middle}
		.badge.ai{background:#2d4b8a;color:#a9c4ff}
		.badge.search{background:#2d5b3e;color:#a9e6c4}
		.badge.other{background:#5b5530;color:#ffe9a9}
		.meta{color:#8a93a4;font-size:12px;margin-bottom:8px}
		table{width:100%;border-collapse:collapse;font-size:13px}
		td,th{text-align:left;padding:5px 8px;border-bottom:1px solid #262b34}
		th{color:#8a93a4;font-weight:600}
		.nf{color:#ff8b8b}
		.ok{color:#8bd0a0}
		.feed{background:#1a1d24;border:1px solid #262b34;border-radius:8px;padding:14px 18px;margin-top:18px}
		.feed td{font-family:ui-monospace,Consolas,monospace;font-size:12px}
		a{color:#6fb6ff}
		.nav{margin-bottom:18px}
		.nav a{margin-right:10px;color:#8a93a4;text-decoration:none}
		.nav a.on{color:#fff;border-bottom:2px solid #6fb6ff}
	</style>`)
	sb.WriteString(`</head><body><div class=wrap>`)

	sb.WriteString(`<h1>🤖 Bot Crawl Dashboard</h1>`)
	sb.WriteString(fmt.Sprintf(`<p class=sub>Last %d days · Tis The Season KC</p>`, days))

	// day nav
	sb.WriteString(`<div class=nav>`)
	for _, d := range []int{1, 7, 30, 90} {
		cls := ""
		if d == days {
			cls = "on"
		}
		sb.WriteString(fmt.Sprintf(`<a class="%s" href="?days=%d">%dd</a>`, cls, d, d))
	}
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class=summary>`)
	sb.WriteString(fmt.Sprintf(`<div class=card><div class=n>%d</div><div class=l>AI bots</div></div>`, totalAI))
	sb.WriteString(fmt.Sprintf(`<div class=card><div class=n>%d</div><div class=l>Search bots</div></div>`, totalSearch))
	sb.WriteString(fmt.Sprintf(`<div class=card><div class=n>%d</div><div class=l>Other bots</div></div>`, totalOther))
	sb.WriteString(`</div>`)

	if len(bots) == 0 {
		sb.WriteString(`<p>No bot traffic recorded in this window yet.</p>`)
	} else {
		for _, b := range bots {
			sb.WriteString(fmt.Sprintf(`<div class=bot><h2>%s <span class="badge %s">%s</span></h2>`,
				html.EscapeString(b.name), html.EscapeString(b.kind), html.EscapeString(b.kind)))
			sb.WriteString(fmt.Sprintf(`<div class=meta>%d requests · %d returned 404</div>`, b.total, b.notFound))

			// top paths
			var psList []*pathStat
			for _, ps := range b.paths {
				psList = append(psList, ps)
			}
			sort.Slice(psList, func(i, j int) bool { return psList[i].count > psList[j].count })
			if len(psList) > 12 {
				psList = psList[:12]
			}
			sb.WriteString(`<table><tr><th>Path</th><th>Hits</th><th>Last status</th></tr>`)
			for _, ps := range psList {
				statusClass := ""
				if ps.status == 404 {
					statusClass = "nf"
				} else if ps.status < 400 {
					statusClass = "ok"
				}
				sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%d</td><td class="%s">%d</td></tr>`,
					html.EscapeString(ps.path), ps.count, statusClass, ps.status))
			}
			sb.WriteString(`</table>`)
			sb.WriteString(`</div>`)
		}
	}

	// recent feed
	sb.WriteString(`<div class=feed><h2 style="margin-top:8px">Recent hits (last 50)</h2>`)
	sb.WriteString(`<table><tr><th>Time</th><th>Bot</th><th>Path</th><th>Status</th></tr>`)
	for _, h := range recent {
		statusClass := ""
		if h.Status == 404 {
			statusClass = "nf"
		} else if h.Status < 400 {
			statusClass = "ok"
		}
		t := h.TS
		if parsed, err := time.Parse(time.RFC3339, h.TS); err == nil {
			t = parsed.Local().Format("01-02 15:04")
		}
		sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td><td class="%s">%d</td></tr>`,
			html.EscapeString(t), html.EscapeString(h.Bot), html.EscapeString(h.Path), statusClass, h.Status))
	}
	sb.WriteString(`</table></div>`)

	sb.WriteString(`<p style="color:#8a93a4;font-size:12px;margin-top:24px">Raw log: <code>~/.local/share/tts/bot-crawls.jsonl</code></p>`)
	sb.WriteString(`</div></body></html>`)

	_, _ = w.Write([]byte(sb.String()))
}
