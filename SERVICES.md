# Tis The Season KC — Backend Services Runbook

This document covers all backend API services that power the customer-facing and
operational tools for Tis The Season KC. It is the single source of truth for
what each service does, how to run it, what it needs, and what's left to build.

> **Site infrastructure** (Hugo build, Cloudflare Pages deployment) lives in
> `PLAN.md`. This doc is only about the Go backend services.

---

## Where This Code Lives

All backend services run inside the existing Go HTTP server (`server.go`) that
already powers the contact form and serves the static site. New services are
added as route handlers and organized in the `services/` directory.

The server runs as a persistent systemd service (`tts.service`) on this VM.

```
TTS/
├── server.go                    ← main entry point, registers all routes
├── go.mod                       ← Go module (zero external dependencies)
├── .env.example                 ← documents every required env var (no secrets)
├── services/
│   ├── config.go                ← LoadConfig(), env var validation, error emails
│   ├── airtable.go              ← shared Airtable API client, typed structs
│   ├── checkout.go              ← Stripe checkout page handler
│   ├── webhook.go               ← Stripe webhook handler
│   ├── estimate.go              ← estimate email sender (not yet built)
│   └── email_templates/
│       └── estimate.html        ← Go HTML template for estimate email (not yet built)
└── SERVICES.md                  ← this file
```

---

## Environment & Configuration

All secrets and config live in environment variables. **Never hardcode keys.**

The server is controlled by a single `APP_ENV` flag:

| Value | Behavior |
|-------|----------|
| `dev` | Uses Stripe sandbox proxy, Stripe TEST product IDs from Airtable |
| `prod` | Uses Stripe live proxy, live Stripe product IDs from Airtable |

The server logs the active mode unmistakably on startup:
```
⚡ TTS Server starting
🟡 STRIPE MODE: SANDBOX (dev)
```
or
```
⚡ TTS Server starting
🟢 STRIPE MODE: LIVE (prod)
```

To switch modes: change `APP_ENV` in the systemd unit and restart. No code changes needed.

### Required Environment Variables

See `.env.example` for the full list. These live in `/etc/systemd/system/tts.service`
under `[Service]` as `Environment=` lines.

```
APP_ENV=dev                              # or: prod

# Stripe webhook signing secrets (outbound Stripe calls use exe.dev proxy — no keys needed)
STRIPE_WEBHOOK_SECRET_DEV=whsec_...      # from Stripe Dashboard → Webhooks (sandbox)
STRIPE_WEBHOOK_SECRET_PROD=whsec_...     # from Stripe Dashboard → Webhooks (live) — add when going live

# Gmail SMTP (for estimate emails and server error alerts)
GMAIL_SEND_AS=stetson@tts.lighting
GMAIL_APP_PASSWORD=xxxx-xxxx-xxxx-xxxx   # 16-char Google App Password
```

**Note:** Stripe API calls go through `stripe-test-mode.int.exe.xyz` (dev) and
`stripe-live-mode.int.exe.xyz` (prod) — exe.dev proxy integrations inject the keys,
so no Stripe secret key is ever stored on this VM.

Airtable calls go through `airtable.int.exe.xyz` — same deal, no Airtable PAT on VM.

### Where Secrets Live

Secrets are stored in a dedicated environment file, separate from the service config:

```
/etc/tts/secrets.env
```

- Owned by `root`, mode `600` — only root can read it
- Never commit this file or its contents to git
- The service config (`/etc/systemd/system/tts.service`) references it via `EnvironmentFile=` — it contains no secrets itself

### How to Add or Rotate a Secret

```bash
# Edit the secrets file
sudo nano /etc/tts/secrets.env

# Reload and restart to pick up changes
sudo systemctl daemon-reload && sudo systemctl restart tts.service

# Confirm it started correctly
journalctl -u tts.service -n 5
```

### Adding Secrets for Future Services

When adding Gmail or the live Stripe webhook secret, append to `/etc/tts/secrets.env`:

```
GMAIL_SEND_AS=stetson@tts.lighting
GMAIL_APP_PASSWORD=xxxx-xxxx-xxxx-xxxx
STRIPE_WEBHOOK_SECRET_PROD=whsec_...
```

---

## Services

### 1. Airtable Client

**File:** `services/airtable.go`
**Status:** ✅ Done — tested

Shared client used by all other services. Key details:
- All requests include `returnFieldsByFieldId=true` so map keys are stable field IDs (not names)
- Field ID constants defined at top of file for both Customers and Yearly Invoicing tables
- `GetCustomerByRecordID()` — fetch by Airtable record ID (used by checkout)
- `GetCustomerByStripeID()` — fetch by Stripe `cus_` ID (used by webhook)
- `GetCurrentYearLineItems()` — fetch Yearly Invoicing rows for a customer, selects dev or prod Stripe product ID field based on `APP_ENV`
- `MarkCustomerPaid()` — PATCH: sets `Paid? = "Paid"` and clears `Review Discount?` checkbox

---

### 2. Stripe Checkout

**Route:** `GET /pay?q={airtableRecordID}`
**Status:** ✅ Done — tested end-to-end

What it does:
1. Reads `?q=` record ID, fetches customer from Airtable
2. If already paid → returns a friendly "already paid" message
3. Fetches current-year line items from Yearly Invoicing table
4. Builds a Stripe Checkout Session via the exe.dev proxy (form-encoded POST)
5. If `Review Discount?` is checked on the customer → applies coupon `wdxZW6X2`
6. Redirects customer to Stripe-hosted checkout page
7. Stripe redirects to `https://tistheseasonkc.com/payment-success` on success

**File:** `services/checkout.go`

---

### 3. Stripe Webhook

**Route:** `POST /stripe/webhook`
**Status:** ✅ Done — tested end-to-end

What it does:
1. Receives `checkout.session.completed` event from Stripe
2. Verifies `Stripe-Signature` header with HMAC-SHA256 (manual — no stripe-go SDK)
3. Looks up customer in Airtable by Stripe `cus_` ID
4. Patches Airtable: `Paid? = "Paid"` + `Review Discount? = false`

**Stripe Dashboard setup:**
- Sandbox endpoint: `https://pi-vm.exe.xyz:8000/stripe/webhook` ✅ registered
- Live endpoint: `https://tistheseasonkc.com/stripe/webhook` — add when going live
- Event: `checkout.session.completed` only

**File:** `services/webhook.go`

---

### 4. Payment Success Page

**URL:** `https://tistheseasonkc.com/payment-success`
**Status:** ✅ Done

Hugo static page. Customers land here after completing Stripe checkout.
Layout: `layouts/payment-success/single.html`
Content: `content/payment-success/index.md`

---

### 5. Estimate Email

**Route:** `POST /estimate/send`
**Status:** 🔴 Not started

What it will do:
1. Accept an Airtable record ID
2. Fetch customer + line items from Airtable
3. Render `services/email_templates/estimate.html`
4. Send via Gmail SMTP as `stetson@tts.lighting`

**To build this, still needed:**
- `GMAIL_APP_PASSWORD` added to systemd env
- Decide: what fields go in the estimate email (line items, total, pay link, etc.)
- Decide: does it include the `/pay?q=` link?
- Design the email template

**File:** `services/estimate.go` + `services/email_templates/estimate.html`

---

## Route Map

| Method | Path | Service | Public? |
|--------|------|---------|--------|
| GET | `/pay` | Stripe Checkout | Yes — customer-facing |
| POST | `/stripe/webhook` | Stripe Webhook | Yes — Stripe only (sig verified) |
| POST | `/estimate/send` | Estimate Email | Internal only |
| POST | `/contact` | Contact Form | Yes — existing, live |

---

## Remaining Work

- [ ] **Estimate email** — build `services/estimate.go` + HTML template
- [ ] **Gmail App Password** — add `GMAIL_SEND_AS` + `GMAIL_APP_PASSWORD` to systemd env
- [ ] **Live Stripe webhook** — register `https://tistheseasonkc.com/stripe/webhook` in Stripe live dashboard + add `STRIPE_WEBHOOK_SECRET_PROD`
- [ ] **DNS cutover** — point `payments.tistheseasonkc.com` to this VM
- [ ] **Flip to prod** — change `APP_ENV=prod` when ready for real customers
- [ ] **Archive `tts-contact-form/`** — old Val.town TypeScript, safe to remove once everything is live

---

## Seasonal Startup Checklist (Every September)

```
□ Check server is running:      systemctl status tts.service
□ Check logs look clean:        journalctl -u tts.service -n 50
□ Confirm APP_ENV is correct:   should be prod
□ Verify Stripe mode is live:   look for 🟢 LIVE in the startup log
□ Test a sandbox payment:       flip APP_ENV=dev, test, flip back to prod
□ Confirm Airtable is current:  check for any schema changes from off-season
□ Rotate any keys that are >1 year old
```

---

## Quick Reference — Server Management

```bash
# Check status
systemctl status tts.service

# View live logs
journalctl -u tts.service -f

# Rebuild + restart
go build -o tts-server . && sudo systemctl restart tts.service

# Edit env vars
sudo systemctl edit tts.service
```
