package services

// Sentry integration.
//
// Initialized once at startup via InitSentry() using the SENTRY_DSN env var
// (kept in /etc/tts/secrets.env — never in the repo). Once initialized,
// CaptureError/CaptureMessage forward to Sentry in addition to the existing
// log + email-alert paths. If SENTRY_DSN is unset, these are no-ops, so the
// server runs fine without Sentry configured (e.g. local dev).
//
// SentryRecovery is installed as the outermost middleware so any unexpected
// panic in a handler is captured with a stack trace before the process
// crashes (the systemd unit's Restart=always brings it back).

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
)

// sentryEnabled is set by InitSentry based on whether a DSN was provided.
var sentryEnabled bool

// InitSentry configures the global Sentry client. Safe to call when no DSN
// is set — it logs a message and leaves capture as no-ops.
func InitSentry() {
	dsn := strings.TrimSpace(os.Getenv("SENTRY_DSN"))
	if dsn == "" {
		log.Println("⚪ SENTRY: no DSN set — error capture disabled (set SENTRY_DSN to enable)")
		sentryEnabled = false
		return
	}

	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if env == "" {
		env = "dev"
	}

	if err := sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      env,
		AttachStacktrace: true,
		// Light performance visibility without burning quota. Default 1.0 is
		// noisy for a low-traffic site; 0.1 surfaces slow endpoints occasionally.
		TracesSampleRate: 0.1,
		// Ignore benign noise so it doesn't fill the free-tier quota.
		IgnoreErrors: []string{
			"context canceled",
			"client disconnected",
		},
	}); err != nil {
		log.Printf("⚠️ SENTRY init failed: %v (continuing without error capture)", err)
		sentryEnabled = false
		return
	}
	sentryEnabled = true
	log.Printf("🟣 SENTRY enabled (env=%s)", env)
}

// FlushSentry should be deferred at program exit so buffered events are sent.
func FlushSentry(timeout time.Duration) {
	if sentryEnabled {
		sentry.Flush(timeout)
	}
}

// CaptureError reports an error to Sentry (with request context as tags when
// r is non-nil). No-op if Sentry is off or err is nil.
func CaptureError(err error, r *http.Request) {
	if !sentryEnabled || err == nil {
		return
	}
	hub := sentry.CurrentHub()
	if r != nil {
		if h := sentry.GetHubFromContext(r.Context()); h != nil {
			hub = h
		}
		hub.ConfigureScope(func(scope *sentry.Scope) {
			scope.SetTag("http.method", r.Method)
			scope.SetTag("http.path", r.URL.Path)
			scope.SetTag("http.user_agent", r.UserAgent())
			scope.SetTag("http.remote_ip", clientIPFromRequest(r))
		})
	}
	hub.CaptureException(err)
}

// CaptureMessage reports a message to Sentry at Info level. No-op if off.
func CaptureMessage(msg string, r *http.Request) {
	if !sentryEnabled {
		return
	}
	hub := sentry.CurrentHub()
	if r != nil {
		if h := sentry.GetHubFromContext(r.Context()); h != nil {
			hub = h
		}
		hub.ConfigureScope(func(scope *sentry.Scope) {
			scope.SetTag("http.path", r.URL.Path)
		})
	}
	hub.CaptureMessage(msg)
}

// SentryRecovery is a middleware that recovers from panics, reports them to
// Sentry with a stack trace, and re-panics so systemd's Restart=always handles
// the crash. Re-panicking (vs. returning a 500) keeps behavior consistent
// whether or not Sentry is enabled, and a clean restart is safer for a
// stateless API than continuing in a possibly-wedged state.
func SentryRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				err, ok := rec.(error)
				if !ok {
					err = fmt.Errorf("panic: %v", rec)
				}
				log.Printf("[panic] %s %s: %v", r.Method, r.URL.Path, err)
				CaptureError(err, r)
				panic(rec)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// clientIPFromRequest returns the first X-Forwarded-For hop (set by exe.dev)
// or the remote address, for Sentry tags.
func clientIPFromRequest(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	return r.RemoteAddr
}
