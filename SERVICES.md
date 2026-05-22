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
├── go.mod                       ← Go dependencies
├── .env.example                 ← documents every required env var (no secrets)
├── services/
│   ├── airtable.go              ← shared Airtable API client
│   ├── checkout.go              ← Stripe checkout page handler
│   ├── webhook.go               ← Stripe webhook handler
│   ├── estimate.go              ← estimate email sender
│   └── email_templates/
│       └── estimate.html        ← Go HTML template for estimate email
└── SERVICES.md                  ← this file
```

---

## Environment & Configuration

All secrets and config live in environment variables. **Never hardcode keys.**

The server is controlled by a single `APP_ENV` flag:

| Value | Behavior |
|-------|----------|
| `dev` | Uses Stripe sandbox keys, logs verbosely |
| `prod` | Uses Stripe live keys, minimal logs |

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

To switch modes: change `APP_ENV` in the systemd environment file and restart
the service. No code changes needed.

### Required Environment Variables

See `.env.example` for the full list. Summary:

```
# Environment selector — the only thing you change to switch modes
APP_ENV=dev                         # or: prod

# Stripe — both sets of keys always present, server picks based on APP_ENV
STRIPE_SECRET_KEY_DEV=sk_test_...
STRIPE_WEBHOOK_SECRET_DEV=whsec_...
STRIPE_SECRET_KEY_PROD=sk_live_...
STRIPE_WEBHOOK_SECRET_PROD=whsec_...

# Airtable
AIRTABLE_API_KEY=pat...
AIRTABLE_BASE_ID=app...

# Gmail API (Google Workspace service account)
GMAIL_SERVICE_ACCOUNT_JSON='{...}'  # full JSON key contents as a single env var
GMAIL_SEND_AS=hello@tistheseasonkc.com
```

### How to Update Environment Variables

```bash
# Edit the environment file
sudo systemctl edit tts.service

# Restart to pick up changes
sudo systemctl restart tts.service

# Confirm it started correctly
sudo journalctl -u tts.service -n 30
```

---

## Services

### 1. Stripe Checkout Page

**Route:** `GET /pay/{recID}`  
**Public URL:** `payments.tistheseasonkc.com/{recID}` (DNS points here)

**What it does:**
1. Extracts the Airtable record ID from the URL
2. Fetches the customer record from Airtable
3. If already paid → shows a "payment already received" page
4. If not paid → reads line items from Airtable, creates a Stripe Checkout
   Session, redirects customer to Stripe's hosted checkout page
5. On success, Stripe redirects customer to a confirmation page

**File:** `services/checkout.go`  
**Status:** 🟡 Built — needs Stripe keys + end-to-end test

**Open questions — YOU need to answer these before we build:**
- [ ] What Airtable table holds customer records? What's it called?
- [ ] What field indicates a customer is already paid? (checkbox? field name?)
- [ ] Where do the line items / products come from? Same record? A linked table?
- [ ] What fields make up a line item? (name, price, quantity?)
- [ ] What URL should Stripe redirect to on success? (e.g. a thank-you page)
- [ ] What URL on cancel/back?
- [ ] Is the Stripe price created dynamically (from Airtable dollar amounts) or
      do you have fixed Stripe Price IDs that you reference?

**Your task when ready:**
- Fill in the Airtable field names in `services/airtable.go`
- Test a checkout flow end-to-end against Stripe sandbox

---

### 2. Stripe Webhook

**Route:** `POST /stripe/webhook`  
**Configured in:** Stripe Dashboard → Webhooks

**What it does:**
1. Receives event from Stripe
2. Verifies the `Stripe-Signature` header (prevents spoofing)
3. On `checkout.session.completed` event:
   - Reads the Airtable record ID from the session metadata
   - Updates the Airtable customer record to mark as paid
4. Returns `200 OK` to Stripe quickly (Stripe retries if it gets anything else)

**File:** `services/webhook.go`  
**Status:** 🟡 Built — needs webhook URL registered in Stripe + test

**Open questions — YOU need to answer these before we build:**
- [ ] What Airtable field(s) do you update when a payment completes?
      (e.g. a "Paid" checkbox, a "Payment Date" date field, a "Stripe Session ID"
      text field — tell me all of them)
- [ ] Do you want an email notification to yourself when a payment comes in?

**Your task when ready:**
- Register the webhook URL in Stripe Dashboard (both sandbox + live endpoints)
- Copy the webhook signing secrets into your env vars
- Run a test payment through Stripe sandbox and confirm Airtable updates

---

### 3. Estimate Email

**Route:** `POST /estimate/send`  
**Auth:** Bearer token (internal use only — called from your tools, not public)

**What it does:**
1. Accepts an Airtable record ID in the request body
2. Fetches customer data and line items from Airtable
3. Renders `services/email_templates/estimate.html` with that data
4. Sends the HTML email via Gmail API as your business address
5. Customer receives a normal email → replies land in your Gmail inbox
   naturally → you can reply from your Sent folder

**Sending:** Gmail API with a Google Workspace service account. No Resend,
no SMTP, no third-party email service. Keeps deliverability strong and
everything in your existing Gmail.

**File:** `services/estimate.go` + `services/email_templates/estimate.html`  
**Status:** 🔴 Not started

**Open questions — YOU need to answer these before we build:**
- [ ] What Airtable fields go into the estimate email?
      (customer name, address, line items, prices, totals, deposit amount,
      install date estimate, anything else?)
- [ ] What should the email subject line be?
- [ ] Does the email need a "pay now" link? (pointing to `/pay/{recID}`)
- [ ] Any legal/terms language at the bottom?
- [ ] What's the "from" name? (e.g. "Tis The Season KC" vs your personal name)
- [ ] Does this endpoint need to be callable from a simple script/tool you run,
      or do you want a minimal internal web UI to trigger it from?

**Your task when ready:**
- Set up Gmail API: create a Google Cloud project, enable Gmail API, create a
  service account, grant domain-wide delegation (there's a step-by-step in the
  Gmail Setup section below)
- Design the estimate email template (we'll do this together)

---

## Gmail API Setup (Do This Once)

This is the one-time setup to let the server send email as your Gmail address.
I'll walk you through each step when we get here, but the high-level sequence is:

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project (e.g. "TTS Backend")
3. Enable the **Gmail API**
4. Create a **Service Account**
5. Download the service account JSON key
6. In **Google Workspace Admin** → grant the service account
   domain-wide delegation with scope `https://mail.google.com/`
7. Put the JSON key contents into your `GMAIL_SERVICE_ACCOUNT_JSON` env var

**Status:** 🔴 Not done yet

---

## Route Map

| Method | Path | Service | Public? |
|--------|------|---------|--------|
| GET | `/pay/{recID}` | Stripe Checkout | Yes — customer-facing |
| POST | `/stripe/webhook` | Stripe Webhook | Yes — Stripe only (verified by signature) |
| POST | `/estimate/send` | Estimate Email | No — internal, bearer token required |
| POST | `/contact` | Contact Form | Yes — existing, already live |

---

## Build Order & Task Ownership

We build in this order so each piece is testable before the next depends on it.

### Step 1 — Airtable Client
**Owner: Shelley builds skeleton → You fill in field names**

- [x] Shelley: create `services/airtable.go` with typed structs and fetch functions
- [x] Field names confirmed via Airtable API schema pull (exe.dev integration)
- [x] Airtable accessed via exe.dev HTTP proxy — no API keys in env needed
- [ ] Test: visit `/status` after server restarts to confirm it boots

### Step 2 — Stripe Checkout
**Owner: You add keys + test**

- [x] Shelley: created `services/checkout.go` and `services/config.go`
- [ ] You: add Stripe keys to systemd env (see `.env.example`)
- [ ] You: `sudo systemctl restart tts.service`
- [ ] You: confirm 🟡 SANDBOX log line appears (`journalctl -u tts.service -n 5`)
- [ ] You: visit `https://pi-vm.exe.xyz:8000/pay?q={a real customer recID}` and complete a test payment
- [ ] Confirm Airtable `Paid?` field updates to "Paid" after webhook fires

### Step 3 — Stripe Webhook
**Owner: You register in Stripe Dashboard**

- [x] Shelley: created `services/webhook.go`
- [ ] You: in Stripe Dashboard (sandbox) → Webhooks → Add endpoint
        URL: `https://payments.tistheseasonkc.com/stripe/webhook`
        Event: `checkout.session.completed`
- [ ] You: copy the webhook signing secret → add as `STRIPE_WEBHOOK_SECRET_DEV` in env
- [ ] You: repeat for live endpoint with `STRIPE_WEBHOOK_SECRET_PROD`
- [ ] Test: trigger test event from Stripe Dashboard, confirm Airtable updates

### Step 4 — Gmail Setup
**Owner: You do the Google Cloud setup → Shelley helps if you get stuck**

- [ ] You: follow the Gmail API Setup steps above
- [ ] You: add `GMAIL_SERVICE_ACCOUNT_JSON` and `GMAIL_SEND_AS` to env
- [ ] Test: send a test email to yourself

### Step 5 — Estimate Email
**Owner: Build together**

- [ ] You: answer the open questions above (what goes in the email)
- [ ] Shelley: create `services/estimate.go` and `estimate.html` template skeleton
- [ ] You: review and adjust the email design/content
- [ ] Test: call `POST /estimate/send` with a real record ID and check your inbox

### Step 6 — DNS Cutover
**Owner: You**

- [ ] Repoint `payments.tistheseasonkc.com` DNS to this VM (same as main site)
- [ ] Confirm `/pay/{recID}` is reachable from that domain
- [ ] Remove Val.town endpoints one by one as each service goes live

---

## Seasonal Startup Checklist (Every September)

Coming back after the off-season? Run through this:

```
□ Check server is running:     systemctl status tts.service
□ Check logs look clean:       journalctl -u tts.service -n 50
□ Confirm APP_ENV is correct:  grep APP_ENV in your env (should be prod)
□ Verify Stripe keys are live: look for 🟢 LIVE in the startup log
□ Test a sandbox payment:      flip APP_ENV=dev, test, flip back
□ Confirm Airtable is current: open base, check any schema changes from off-season
□ Confirm Gmail API still works: POST /estimate/send with a test record
□ Rotate any keys that are >1 year old
```

---

## Quick Reference — Server Management

```bash
# Check status
systemctl status tts.service

# View live logs
journalctl -u tts.service -f

# Restart (after env changes or deploys)
sudo systemctl restart tts.service

# Build new server binary
go build -o tts-server .

# Switch to sandbox mode
# → change APP_ENV=dev in systemd env, then restart

# Switch to production mode  
# → change APP_ENV=prod in systemd env, then restart
```

---

## Dependencies (Go packages to add)

| Package | Purpose |
|---------|--------|
| `github.com/stripe/stripe-go/v82` | Stripe API client |
| `google.golang.org/api/gmail/v1` | Gmail API |
| `golang.org/x/oauth2/google` | Google service account auth |

All added to `go.mod` as we build each service.
