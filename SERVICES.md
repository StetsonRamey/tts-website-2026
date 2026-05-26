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
│   ├── leads.go                 ← Airtable client for LEADS base (2026 Leads table)
│   ├── checkout.go              ← Stripe checkout page handler
│   ├── webhook.go               ← Stripe webhook handler
│   ├── estimate.go              ← estimate email sender ✅
│   └── email_templates/
│       └── estimate.html        ← Go HTML template for estimate email ✅
├── static/
│   └── images/
│       └── tts-logo.png         ← logo served at /images/tts-logo.png (Hugo static)
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
GMAIL_APP_PASSWORD="xxxx xxxx xxxx xxxx"  # 16-char Google App Password (quote it — has spaces)

# Webhook auth — shared with Airtable automation scripts
WEBHOOK_AUTH_KEY=<random>                # generate: openssl rand -base64 32 | tr '+/' '-_' | tr -d '='
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

When adding the live Stripe webhook secret, append to `/etc/tts/secrets.env`:

```
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

### 5. Leads Airtable Client

**File:** `services/leads.go`
**Status:** ✅ Done — tested

Reads from the **LEADS base** (`appg9012rLh2diVLq`), table **2026 Leads (forward)** (`tblui0E6mBFkHGWvZ`).

- `GetLeadByRecordID(recordID)` — fetch a single lead by Airtable record ID
- `Lead` struct: `AirtableID`, `RecordID`, `FirstName`, `LastName`, `Email`, `Feet`, `PriceLED`, `PriceRehang`, `Photos []LeadPhoto`
- `LeadPhoto` struct: `ID`, `URL` (temporary Airtable CDN link), `Filename`
- Uses `returnFieldsByFieldId=true` — field ID constants at top of file

**Field IDs (2026 Leads):**

| Field | ID |
|-------|----|---|
| First Name | `fldplXExIaztUlnVf` |
| Last Name | `fldiVRdwdOumpsrCh` |
| Email | `fldsvJF0WoUqKWOtq` |
| Feet | `fldelBDSYukmjqNbo` |
| Price LED | `fld5LWFylybQBCArw` |
| Price Rehang | `fldC1G0g2nhXxXvc5` |
| Photo | `fldPT5nBi1q8NsaoR` |
| RecordID formula | `fldjMi3L50dNSg2bV` |

---

### 6. Estimate Email

**Route:** `POST /estimate/send`
**Status:** ✅ Done — tested end-to-end

What it does:
1. Verifies `Authorization: Bearer WEBHOOK_AUTH_KEY` header
2. Parses JSON body: `{"recordId": "recXXX"}`
3. Fetches lead from 2026 Leads table via `GetLeadByRecordID()`
4. Downloads photos from Airtable CDN → saves to `/var/lib/tts/photos/`
5. Photo filenames: `{first-last}-{sha256[:16]}.{ext}` e.g. `stetson-ramey-748bd896badeb589.png`
6. Renders `services/email_templates/estimate.html` with lead data + public photo URLs
7. Sends via Gmail SMTP (`smtp.gmail.com:587`, STARTTLS) as `stetson@tts.lighting`

**Photo serving:**
- Stored at: `/var/lib/tts/photos/`
- Served at: `GET /photos/{filename}` → `https://tistheseasonkc.com/photos/{filename}`
- Photos are permanent (downloaded from expiring Airtable CDN links at send time)

**Email template data (`EstimateEmailData`):**

| Field | Type | Description |
|-------|------|-------------|
| `FirstName` | string | Lead's first name |
| `Feet` | float64 | Estimated linear feet |
| `PriceLED` | float64 | This year's LED price |
| `PriceRehang` | float64 | Next year's rehang price |
| `PhotoURLs` | []string | Public permanent photo URLs |

**Template func:** `{{formatCurrency .PriceLED}}` → `$425.00`

**To trigger (from Airtable automation or curl):**
```bash
curl -X POST https://tistheseasonkc.com/estimate/send \
  -H "Authorization: Bearer YOUR_WEBHOOK_AUTH_KEY" \
  -H "Content-Type: application/json" \
  -d '{"recordId": "recXXXXXXXXXXXXXX"}'
```

**Files:** `services/estimate.go`, `services/leads.go`, `services/email_templates/estimate.html`

---

## Route Map

| Method | Path | Service | Public? |
|--------|------|---------|--------|
| GET | `/pay` | Stripe Checkout | Yes — customer-facing |
| POST | `/stripe/webhook` | Stripe Webhook | Yes — Stripe only (sig verified) |
| POST | `/estimate/send` | Estimate Email | Internal — Bearer token required |
| GET | `/photos/{filename}` | Photo serving | Yes — linked from estimate emails |
| POST | `/contact` | Contact Form | Yes — existing, live |

---

## Remaining Work

- [ ] **Switch email sending to Angela's account** — blocked on Angela enabling 2FA on `angela@tts.lighting`
  - Once 2FA is on: generate App Password at myaccount.google.com/apppasswords
  - Verify `hello@tts.lighting` as a Send-As alias in Angela's Gmail settings
  - Update `/etc/tts/secrets.env`: `GMAIL_USER=angela@tts.lighting`, `GMAIL_SEND_AS=hello@tts.lighting`, new `GMAIL_APP_PASSWORD`
  - `sudo systemctl restart tts.service`
  - **Currently:** sending from `stetson@tts.lighting` for testing
- [ ] **Airtable automation** — wire up Airtable scripts to call `/estimate/send`, `/confirmation/send`, `/oos/send` with `WEBHOOK_AUTH_KEY`
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
