package services

// Config holds all runtime configuration for the backend services.
//
// Stripe is accessed via exe.dev HTTP proxy integrations — no secret keys
// on this server. The proxy injects the Authorization header automatically.
//
//   dev  → https://stripe-test-mode.int.exe.xyz  (sandbox, livemode=false)
//   prod → https://stripe-live-mode.int.exe.xyz   (live, livemode=true)
//        └─ add this integration when ready to go live
//
// To switch modes: change APP_ENV in the systemd service and restart.
// No other changes needed.

import (
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"strings"
)

const (
	// Stripe proxy base URLs — the exe.dev integration injects the API key
	StripeProxyDev  = "https://stripe-test-mode.int.exe.xyz"
	StripeProxyProd = "https://stripe-live-mode.int.exe.xyz"
)

// Config is passed to every service handler.
type Config struct {
	Env            string // "dev" or "prod"
	StripeBaseURL  string // proxy URL selected based on Env
	WebhookSecret  string // still needed for signature verification (set in systemd)
	ReviewCouponID string // Stripe coupon ID for review discounts (optional)
	ErrorEmailTo   string // address to notify on errors

	// InvoiceProductID is the Stripe Product used by the "Make Invoice"
	// automation. The product owns the unit cost per linear foot; the Go
	// backend just passes Airtable's Feet value as the quantity.
	// Selected from STRIPE_INVOICE_PRODUCT_ID_DEV / _PROD based on Env.
	InvoiceProductID string

	// CompanyCam — used by /sold/sync to create projects and upload photos.
	// CompanyCamBaseURL defaults to https://api.companycam.com/v2 but can be
	// overridden to point at an exe.dev proxy that injects auth.
	CompanyCamBaseURL   string
	CompanyCamAPIToken  string // blank when a proxy injects auth
	CompanyCamUserEmail string // sent as X-CompanyCam-User header
}

// LoadConfig reads APP_ENV and sets up the correct Stripe proxy URL.
// Call once at startup.
func LoadConfig() *Config {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if env == "" {
		env = "dev" // safe default — never accidentally go live
	}

	var stripeBaseURL string
	switch env {
	case "prod":
		stripeBaseURL = StripeProxyProd
		log.Println("⚡ TTS Server starting")
		log.Println("🟢 STRIPE MODE: LIVE (prod)")
	default:
		env = "dev"
		stripeBaseURL = StripeProxyDev
		log.Println("⚡ TTS Server starting")
		log.Println("🟡 STRIPE MODE: SANDBOX (dev)")
	}

	// Webhook secret still comes from env — it's used for signature
	// verification only (never sent outbound, so the proxy can't help here)
	webhookSecretKey := "STRIPE_WEBHOOK_SECRET_" + strings.ToUpper(env)
	webhookSecret := os.Getenv(webhookSecretKey)
	if webhookSecret == "" {
		log.Printf("WARNING: %s not set — webhook signature verification will fail", webhookSecretKey)
	}

	// Stripe Product used for the "Make Invoice" automation.
	// Dev/prod use different product IDs (test products vs live products).
	invoiceProductKey := "STRIPE_INVOICE_PRODUCT_ID_" + strings.ToUpper(env)
	invoiceProductID := os.Getenv(invoiceProductKey)
	if invoiceProductID == "" {
		log.Printf("WARNING: %s not set — /invoice/create will fail", invoiceProductKey)
	}

	return &Config{
		Env:            env,
		StripeBaseURL:  stripeBaseURL,
		WebhookSecret:  webhookSecret,
		ReviewCouponID: os.Getenv("STRIPE_REVIEW_COUPON_ID"),
		ErrorEmailTo:   getEnvOr("ERROR_EMAIL_TO", "stetson@tts.lighting"),

		InvoiceProductID: invoiceProductID,

		CompanyCamBaseURL:   getEnvOr("COMPANYCAM_BASE_URL", "https://companycam.int.exe.xyz/v2"),
		CompanyCamAPIToken:  os.Getenv("COMPANYCAM_API_TOKEN"),
		CompanyCamUserEmail: os.Getenv("COMPANYCAM_USER_EMAIL"),
	}
}

// RequireBearerAuth validates the Authorization: Bearer <WEBHOOK_AUTH_KEY> header.
// Writes a 401 response and returns false if the token is missing/wrong, or if
// WEBHOOK_AUTH_KEY is empty in the environment (fail-closed — we never want an
// internal endpoint to be open in dev because the secret wasn't set).
func RequireBearerAuth(w http.ResponseWriter, r *http.Request) bool {
	expected := os.Getenv("WEBHOOK_AUTH_KEY")
	if expected == "" {
		log.Printf("[auth] WEBHOOK_AUTH_KEY is empty — rejecting request to %s", r.URL.Path)
		http.Error(w, `{"error":"server auth not configured"}`, http.StatusUnauthorized)
		return false
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token != expected {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return false
	}
	return true
}

// sendErrorEmail sends a plain-text alert when something goes wrong.
func (cfg *Config) sendErrorEmail(message string) {
	// Also report to Sentry (no-op if disabled). We pass nil for the request
	// because this helper is called from background paths without one; the
	// env tag set at InitSentry still attaches. This means every existing
	// error-alert call site (checkout, webhook, invoice, sold-sync, ...) is
	// automatically reported to Sentry without scattering CaptureError calls.
	CaptureMessage(message, nil)

	smtpHost := "smtp.gmail.com"
	smtpPort := "587"
	user := os.Getenv("GMAIL_USER")
	from := os.Getenv("GMAIL_SEND_AS")
	pass := os.Getenv("GMAIL_APP_PASSWORD")
	if user == "" {
		user = from // backward compat
	}

	if from == "" || pass == "" {
		log.Printf("[error-email] GMAIL_SEND_AS or GMAIL_APP_PASSWORD not set; cannot send alert: %s", message)
		return
	}

	body := fmt.Sprintf("To: %s\r\nFrom: %s\r\nSubject: TTS Backend Error [%s]\r\n\r\n%s",
		cfg.ErrorEmailTo, from, cfg.Env, message)

	auth := smtp.PlainAuth("", user, pass, smtpHost)
	if err := smtp.SendMail(
		smtpHost+":"+smtpPort,
		auth,
		from,
		[]string{cfg.ErrorEmailTo},
		[]byte(body),
	); err != nil {
		log.Printf("[error-email] send failed: %v (original error: %s)", err, message)
	}
}

// StatusHandler returns a simple health-check for GET /status
func StatusHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mode := "🟢 LIVE (prod)"
		if cfg.Env == "dev" {
			mode = "🟡 SANDBOX (dev)"
		}
		fmt.Fprintf(w, "TTS Backend OK\nStripe mode: %s\nStripe proxy: %s\n", mode, cfg.StripeBaseURL)
	}
}

func getEnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
