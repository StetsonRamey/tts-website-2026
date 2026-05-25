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
	Env              string // "dev" or "prod"
	StripeBaseURL    string // proxy URL selected based on Env
	WebhookSecret    string // still needed for signature verification (set in systemd)
	ReviewCouponID   string // Stripe coupon ID for review discounts (optional)
	ErrorEmailTo     string // address to notify on errors
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

	return &Config{
		Env:            env,
		StripeBaseURL:  stripeBaseURL,
		WebhookSecret:  webhookSecret,
		ReviewCouponID: os.Getenv("STRIPE_REVIEW_COUPON_ID"),
		ErrorEmailTo:   getEnvOr("ERROR_EMAIL_TO", "stetson@tts.lighting"),
	}
}

// sendErrorEmail sends a plain-text alert when something goes wrong.
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
