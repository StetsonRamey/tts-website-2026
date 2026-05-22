package services

// Config holds all runtime configuration for the backend services.
// It is created once at startup in server.go and passed to each handler.
//
// To switch between sandbox and production:
//   Set APP_ENV=dev  → uses STRIPE_SECRET_KEY_DEV + STRIPE_WEBHOOK_SECRET_DEV
//   Set APP_ENV=prod → uses STRIPE_SECRET_KEY_PROD + STRIPE_WEBHOOK_SECRET_PROD
//
// Everything else is the same in both environments.

import (
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"strings"

	stripe "github.com/stripe/stripe-go/v82"
)

// Config is passed to every service handler.
type Config struct {
	Env                 string // "dev" or "prod"
	StripeWebhookSecret string
	ReviewCouponID      string // Stripe coupon ID for review discounts
	ErrorEmailTo        string // address to notify on errors
}

// LoadConfig reads environment variables and initialises Stripe.
// Call once at startup. Logs clearly which mode is active.
func LoadConfig() *Config {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if env == "" {
		env = "dev" // safe default — never accidentally go live
	}

	var secretKey, webhookSecret string
	switch env {
	case "prod":
		secretKey = requireEnv("STRIPE_SECRET_KEY_PROD")
		webhookSecret = requireEnv("STRIPE_WEBHOOK_SECRET_PROD")
		log.Println("⚡ TTS Server starting")
		log.Println("🟢 STRIPE MODE: LIVE (prod)")
	default:
		env = "dev"
		secretKey = requireEnv("STRIPE_SECRET_KEY_DEV")
		webhookSecret = requireEnv("STRIPE_WEBHOOK_SECRET_DEV")
		log.Println("⚡ TTS Server starting")
		log.Println("🟡 STRIPE MODE: SANDBOX (dev)")
	}

	// Initialise the Stripe client globally (stripe-go uses a package-level key)
	stripe.Key = secretKey

	return &Config{
		Env:                 env,
		StripeWebhookSecret: webhookSecret,
		ReviewCouponID:      os.Getenv("STRIPE_REVIEW_COUPON_ID"), // optional
		ErrorEmailTo:        getEnvOr("ERROR_EMAIL_TO", "stetson@tts.lighting"),
	}
}

// sendErrorEmail sends a plain-text alert when something goes wrong.
// Uses Gmail SMTP with app password — configured via env vars.
// If email fails, logs to stderr (don't panic over a failed error email).
func (cfg *Config) sendErrorEmail(message string) {
	smtpHost := "smtp.gmail.com"
	smtpPort := "587"
	from := os.Getenv("GMAIL_SEND_AS")
	pass := os.Getenv("GMAIL_APP_PASSWORD")

	if from == "" || pass == "" {
		log.Printf("[error-email] GMAIL_SEND_AS or GMAIL_APP_PASSWORD not set; cannot send alert: %s", message)
		return
	}

	body := fmt.Sprintf("To: %s\r\nFrom: %s\r\nSubject: TTS Backend Error [%s]\r\n\r\n%s",
		cfg.ErrorEmailTo, from, cfg.Env, message)

	auth := smtp.PlainAuth("", from, pass, smtpHost)
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

// ── Helpers ─────────────────────────────────────────────────────────

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required environment variable %q is not set", key)
	}
	return v
}

func getEnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// StatusHandler returns a simple health-check handler for GET /status
// Shows the active Stripe mode so you can confirm at a glance.
func StatusHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mode := "🟢 LIVE (prod)"
		if cfg.Env == "dev" {
			mode = "🟡 SANDBOX (dev)"
		}
		fmt.Fprintf(w, "TTS Backend OK\nStripe mode: %s\n", mode)
	}
}
