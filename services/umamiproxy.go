package services

// Reverse proxy for self-hosted Umami analytics.
//
// Public visitors' browsers cannot reach the private Umami port (exe.dev gates
// it to the VM owner). So we expose the tracking script and the event-collect
// endpoint through the public Go server under /analytics/*, proxying to the
// local Umami container on 127.0.0.1:3000. This makes the tracker first-party
// (served from tistheseasonkc.com), which is more accurate and resistant to
// ad blockers, and avoids any cross-origin/CORS issues.
//
// The Umami dashboard itself stays private — reached only via the exe.dev
// private port (https://pi-vm.exe.xyz:3000/), gated to the owner's login.
//
// Rewrites:
//   /analytics/script.js  ->  /script.js        (the tracker)
//   /analytics/send       ->  /api/send         (event ingest, POST)
//
// UMIAMI_URL is configurable (default http://127.0.0.1:3000) so tests/dev
// can point elsewhere; unset falls back to the default.

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

var umamiProxy *httputil.ReverseProxy

func initUmamiProxy() {
	raw := strings.TrimSpace(os.Getenv("UMIAMI_URL"))
	if raw == "" {
		raw = "http://127.0.0.1:3000"
	}
	target, err := url.Parse(raw)
	if err != nil {
		log.Printf("[umami-proxy] bad UMIAMI_URL %q: %v (proxy disabled)", raw, err)
		return
	}
	umamiProxy = httputil.NewSingleHostReverseProxy(target)
	// Preserve the visitor's real IP / referrer so Umami attributes correctly.
	orig := umamiProxy.Director
	umamiProxy.Director = func(req *http.Request) {
		orig(req)
		// Set the real client IP so Umami records geo / dedupe correctly.
		if ip := clientIPFromRequest(req); ip != "" {
			req.Header.Set("X-Forwarded-For", ip)
		}
		// Umami reads the origin host to attribute events to the right website.
		req.Host = target.Host
	}
}

// UmamiProxyHandler returns a handler that rewrites /analytics/* paths and
// proxies to Umami. Returns 404 for unknown sub-paths so it can't be used to
// probe the Umami API surface beyond the two intended endpoints.
func UmamiProxyHandler() http.Handler {
	if umamiProxy == nil {
		initUmamiProxy()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if umamiProxy == nil {
			http.Error(w, `{"error":"analytics unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case "/analytics/script.js":
			r.URL.Path = "/script.js"
			r.URL.RawPath = ""
			// Cache the tracker script for an hour; it's stable per Umami version.
			w.Header().Set("Cache-Control", "public, max-age=3600")
			umamiProxy.ServeHTTP(w, r)
		case "/analytics/send":
			r.URL.Path = "/api/send"
			r.URL.RawPath = ""
			umamiProxy.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}
