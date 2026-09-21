# Tis The Season KC — Backend Services Runbook

This document describes the Go backend that serves the public Hugo build and powers customer-facing and operational workflows. It covers the implemented routes, configuration, integrations, and deployment expectations.

For the site structure and local build workflow, see `README.md`. For agent working conventions, see `AGENTS.md`.

> This repository does **not** version a systemd unit or production secrets. The commands and paths below describe the expected VM deployment, which must be verified on the target host.

---

## Architecture and Code Map

The public Go server starts in `server.go`. It serves Hugo's generated `public/` directory, registers backend handlers from `services/`, and applies redirects, security headers, caching, markdown content negotiation for AI agents, bot tracking, Sentry recovery, and custom 404 handling.

```text
TTS/
├── server.go                       # Public server, middleware, route registration, internal listener
├── go.mod                          # Go module; includes Sentry SDK dependencies
├── .env.example                    # Environment-variable reference; never contains real secrets
├── services/
│   ├── config.go                   # Runtime config, bearer auth, status, error-email helper
│   ├── airtable.go                 # Customers and yearly-invoicing Airtable client
│   ├── leads.go                    # Leads Airtable client and patch helpers
│   ├── checkout.go                 # GET /pay
│   ├── webhook.go                  # POST /stripe/webhook
│   ├── estimate.go                 # POST /estimate/send and GET /photos/*
│   ├── confirmation.go             # POST /confirmation/send
│   ├── oos.go                      # POST /oos/send
│   ├── emailtest.go                # EMAIL_TEST_TO recipient override for customer emails
│   ├── sold_sync.go                # POST /sold/sync
│   ├── invoice.go                  # POST /invoice/create
│   ├── bottrack.go                 # Crawler logging and /internal/bots dashboard
│   ├── umamiproxy.go               # First-party /analytics/* proxy
│   ├── sentry.go                   # Sentry initialization and recovery middleware
│   └── email_templates/
│       ├── estimate.html
│       ├── confirmation.html
│       └── tts-logo.png
├── public/                         # Hugo build output served by the Go server
└── SERVICES.md                     # This runbook
```

## Listeners

- **Public listener:** `:8000`. Serves the static site and all public/backend routes.
- **Internal listener:** `INTERNAL_PORT`, default `:3001`; set `INTERNAL_PORT=0` to disable it. It exposes owner-only tools through the exe.dev VM proxy. Currently it offers `/internal/bots` without a bearer token; access is gated by exe.dev login.

The internal listener is not a replacement for route-level authorization on the public listener.

---

## Environment and Configuration

All runtime configuration comes from environment variables. Never hardcode secrets or commit an environment file containing real credentials.

`.env.example` is the versioned reference for variable names. The Go server reads its process environment directly; it does not parse `.env` files.

On the production VM, a typical systemd deployment uses an external root-readable-only `EnvironmentFile=`, for example `/etc/tts/secrets.env`, referenced by an externally managed `tts.service` unit. Verify the actual unit and paths before changing production configuration.

### Modes

`APP_ENV` selects Stripe mode:

| Value | Behavior |
|---|---|
| `dev` (default) | Uses the sandbox Stripe proxy and `*_DEV` configuration |
| `prod` | Uses the live Stripe proxy and `*_PROD` configuration |

Startup logs identify the selected mode. Do not switch modes casually: validate required webhook/product configuration first.

### Environment-variable Reference

| Variable | Purpose |
|---|---|
| `APP_ENV` | `dev` or `prod`; selects Stripe proxy and environment-specific values |
| `STRIPE_WEBHOOK_SECRET_DEV` / `STRIPE_WEBHOOK_SECRET_PROD` | Inbound Stripe webhook signature verification |
| `STRIPE_REVIEW_COUPON_ID` | Optional coupon applied for eligible review discounts |
| `STRIPE_INVOICE_PRODUCT_ID_DEV` / `STRIPE_INVOICE_PRODUCT_ID_PROD` | Stripe product used for invoice creation; the product's default price determines per-foot cost |
| `WEBHOOK_AUTH_KEY` | Required bearer token for internal automation routes and the public bot dashboard |
| `COMPANYCAM_BASE_URL` | Optional CompanyCam API base URL override; defaults to the exe.dev CompanyCam proxy |
| `COMPANYCAM_API_TOKEN` | Optional direct CompanyCam token for local/non-proxy development |
| `COMPANYCAM_USER_EMAIL` | Optional CompanyCam user identity header for direct/non-proxy development |
| `GMAIL_USER` | Optional SMTP authentication account; defaults to `GMAIL_SEND_AS` when unset |
| `GMAIL_SEND_AS` | SMTP envelope/from address and visible sender alias |
| `GMAIL_APP_PASSWORD` | Gmail App Password used for SMTP |
| `EMAIL_TEST_TO` | Optional test-recipient override; when set, all automated customer emails (estimate, confirmation, out-of-service) go to this address instead of the lead's email |
| `ERROR_EMAIL_TO` | Recipient for backend error alerts; defaults to `stetson@tts.lighting` |
| `SENTRY_DSN` | Optional Sentry DSN; unset disables Sentry reporting |
| `INTERNAL_PORT` | Internal owner-only listener port; defaults to `3001`, or `0` disables it |
| `META_PIXEL_ID` | Meta dataset / Pixel ID for the Conversions API (`1737341487496114`, "TTS Website"). Must match `params.metaPixelId` in `hugo.toml`, which embeds the public Pixel |
| `META_CAPI_ACCESS_TOKEN` | Server-only Meta Conversions API system-user token. Never placed in Hugo assets, Git, or logs. Both this and `META_PIXEL_ID` are required or CAPI stays disabled |
| `META_GRAPH_API_VERSION` | Optional explicit Graph API version for CAPI; defaults to `v25.0` |
| `META_TEST_EVENT_CODE` | Optional Events Manager test code applied to every CAPI event. Honored only when `APP_ENV` is not `prod`; in prod it is logged and ignored so real visitors are never tagged as test events. Use `/internal/meta-test` for production checks instead |
| `UMIAMI_URL` | Umami upstream URL; intentionally matches the spelling used by the code, defaults to `http://127.0.0.1:3000` |

Outbound Stripe and Airtable calls use exe.dev proxy integrations in the deployed environment, so Stripe API keys and Airtable PATs are not stored in this repository or expected as normal backend environment variables.

### Changing Configuration on a VM

1. Confirm the active systemd unit and environment-file path with `systemctl cat tts.service`.
2. Edit the external secret/config source, never this repository's `.env.example` with real values.
3. Restart the service and inspect logs:

```bash
sudo systemctl daemon-reload
sudo systemctl restart tts.service
journalctl -u tts.service -n 50 --no-pager
```

---

## Services

### Contact Intake

**Routes:** `GET|HEAD /contact`, `POST /contact`
**Code:** `server.go`

- `GET` and `HEAD` redirect to the canonical Hugo page at `/contact/`.
- `POST` accepts HTML form submissions and JSON submissions.
- The handler validates data, rate-limits by IP, rejects implausibly fast submissions, checks non-US traffic, records outcomes, and writes accepted leads to Airtable before acknowledging them as saved.
- Airtable-confirmed saves set `X-Lead-Saved: true` and `X-Meta-Event-Id: <id>`; `assets/js/form-submit.js` fires the Google Ads, Meta Pixel, and Umami conversion events only when the saved signal is present. Neutral responses for spam rejections intentionally omit both headers, and Airtable failures return a retryable error without a thank-you redirect.
- After the Airtable save the handler also sends a server-side Meta `Lead` through the Conversions API (see [Meta Pixel and Conversions API](#meta-pixel-and-conversions-api)). It runs in the background and never changes the form response.
- Both forms submit an allow-listed `_attribution` JSON snapshot (Google click IDs, Meta `fbclid`/`_fbc`/`_fbp`, UTMs, landing page). Opaque click and browser identifiers are retained in full; the human-readable UTM/path summary is capped separately. The handler stores those values plus the Meta event ID in the lead's Airtable `Tracking Metrics` field (`fldqlM7utGqqVnVa7`); the `Comments` field carries only the customer's own form notes. The paid landing page (`/free-estimate/`) additionally sets the Lead record's `Which Form` single select to `/free-estimate/ ads lander`. This enables later reconciliation of paid traffic with qualified and sold jobs.
- Lead source labeling: the exact printed van-QR combination (`utm_source=van`, `utm_medium=qr`, `utm_campaign=van_wrap` carried in the session attribution) maps the `Which Form` field to the literal option `QR Code` (`server.go` `leadSource`/`isVanQR`). The original UTMs remain in the lead's `Tracking Metrics` field via the attribution summary, so van leads stay distinguishable from future QR campaigns. Missing or mismatched tags never produce `QR Code`; ordinary, Google Ads, Meta, and non-van free-estimate leads keep their existing values.
- Standard HTML form posts redirect to `/thank-you/`; JavaScript-enhanced HTML requests receive a thank-you fragment; JSON clients receive JSON.

### Airtable Clients

**Code:** `services/airtable.go`, `services/leads.go`

The backend has separate typed clients for the customer/yearly-invoicing data and the leads data. They support checkout/webhook state changes, lead lookups, photo metadata, sold-job sync, invoice-link updates, and coupon catalog lookups. Airtable requests use the exe.dev proxy integration in the deployed environment.

The customer `Discount/Coupon` link points to a coupon catalog record in the Services table. Coupon catalog records use `Catalog Type = Coupon`, `Stripe TEST Coupon ID`, and `Stripe Coupon ID`; they are intentionally separate from the customer's `Products/Services` line-item links. The 2026 Google Review Drawing coupon is configured as a one-time $75 discount with a five-redemption cap in both Stripe modes. The payment handler selects the sandbox or live coupon based on `APP_ENV` and only applies it when `Review Discount?` is checked.

### Stripe Checkout
**Code:** `services/checkout.go`

The handler loads the customer and the Yearly Invoicing rows linked directly from that customer whose raw `Year` matches the customer’s raw `Invoice Build Year`. This deliberately avoids mutable view/formula/lookup filters and avoids scanning the full Yearly Invoicing table; Airtable list pagination is still followed for correctness. It prevents duplicate payment when the customer is already paid, creates a Stripe Checkout Session through the selected exe.dev proxy, and redirects to Stripe. Checkout prices and Adaptive Pricing are explicitly set to USD/off for every session, so a Stripe Dashboard default cannot localize the amount into a customer’s currency. If the customer's `Review Discount?` checkbox is set, the handler applies the linked environment-specific coupon from the customer's `Discount/Coupon` Services link. The legacy `STRIPE_REVIEW_COUPON_ID` remains an optional fallback when no linked coupon is configured. A coupon is applied once to the Checkout Session, not as a Yearly Invoicing line item.

### Stripe Webhook

**Route:** `POST /stripe/webhook`
**Code:** `services/webhook.go`

Receives `checkout.session.completed` for website Checkout payments and `payment_intent.succeeded` for successful manually-entered phone payments. It verifies the `Stripe-Signature` header with the mode-specific webhook secret, finds the customer through the Stripe customer ID, and marks the customer paid in Airtable. Both events can be emitted for a Checkout payment; applying the same paid update twice is safe.

The Stripe Dashboard endpoint must subscribe to both event types in the applicable mode.

Configure Stripe Dashboard endpoints for the actual deployed public hostname and mode. This is deployment state, not a repository guarantee.

### Payment Policy and Unexpected-charge Investigation

Customer payments must remain customer-initiated: customers open their payment link and confirm payment on Stripe's hosted page. Signup and reminder delivery are not authorization to charge a saved payment method.

- `services/checkout.go` explicitly uses `mode=payment` with one-time line items; it does not create subscriptions or request `setup_future_usage` for later off-session charges. Creating a Checkout Session is not itself payment confirmation.
- `services/invoice.go` explicitly uses `collection_method=send_invoice` and `auto_advance=false`, then finalizes the invoice to obtain the customer-facing payment URL. Do not change this to `charge_automatically`.
- Do not introduce subscription renewals, off-session payment confirmation, or automation that charges a saved card without an explicit change to this business policy. Dashboard actions and external automations are separate from this repository and must be audited separately if suspected.

For a reported unexpected charge, investigate read-only before changing billing or CRM state:

1. Retrieve the exact PaymentIntent from the appropriate Stripe mode, expanding `latest_charge`. Confirm its actual `customer`, amount/currency, creation time, status, invoice relationship, billing name, and wallet type. Do not assume the supplied customer ID owns the supplied payment ID.
2. Look up Checkout Sessions with `GET /v1/checkout/sessions?payment_intent=<id>`. Inspect the session's customer, mode, payment status, line items, and any subscription/invoice relationship. If there is an invoice or subscription, inspect its collection settings and event history rather than assuming Checkout initiated it.
3. Find the Airtable Customers record by the PaymentIntent's actual Stripe customer ID. Compare it with the reported Airtable record and invoice amount. Stripe display names can be stale, especially when properties or contacts change; distinguish customer identity from the charge's billing name. A billing name alone does not prove who authorized payment.
4. Correlate the Checkout Session and PaymentIntent IDs with `journalctl -u tts.service` and Stripe events. Server timestamps are UTC; convert explicitly to `America/Chicago` for customer discussions. Look for both `[checkout] session ... created` and `[webhook] marked paid` on the actual record. An unpaid *different* record does not establish that the webhook failed.
5. Separately list charges, invoices, and subscriptions (`status=all`) for the reported customer, following pagination. This distinguishes a misidentified payment from a real unexpected renewal or another payment. A missing Stripe `payment_link` object does not mean the customer never opened our `/pay` link, which creates a Checkout Session directly.
6. State what the evidence establishes and what remains unknown (for example, how the person obtained the link). Confirm property ownership/contact details before proposing customer renames, refunds, payment reassignment, or CRM paid-state changes. Keep private API responses and customer identifiers out of Git.

### Estimate Email and Photo Hosting

**Routes:** `POST /estimate/send`, `GET /photos/{filename}`
**Code:** `services/estimate.go`, `services/email_templates/estimate.html`

The authenticated estimate endpoint accepts `{"recordId":"rec..."}`, loads a lead from Airtable, downloads its photo attachments before their Airtable CDN URLs expire, writes permanent copies under `/var/lib/tts/photos/`, renders the estimate template, and sends it by Gmail SMTP. The photo route serves those staged copies at the public site hostname for use in the email.

### Confirmation and Out-of-Service Emails

**Routes:** `POST /confirmation/send`, `POST /oos/send`
**Code:** `services/confirmation.go`, `services/oos.go`

Both authenticated endpoints accept an Airtable record ID, fetch the lead, and send Gmail SMTP communication. The confirmation flow renders `services/email_templates/confirmation.html`; the out-of-service flow sends a plain-text notice.

All three customer-facing email handlers (estimate, confirmation, oos) route the recipient through `resolveRecipient` (`services/emailtest.go`): when `EMAIL_TEST_TO` is set, mail is delivered to that address instead of the lead's email and the redirect is logged. Unset/empty means normal delivery.

All three handlers alert on failure: if lead fetch, template render, or Gmail SMTP delivery fails (or a requested lead is not found), they send an error email to `ERROR_EMAIL_TO` (default `stetson@tts.lighting`) and report the message to Sentry via `cfg.sendErrorEmail` (which wraps `CaptureMessage`) — the same alert path used by checkout/webhook/invoice/sold-sync. Photo-staging failures are non-fatal (the email still sends without photos) but still trigger the alert so re-hosting problems are visible. Successful sends are logged with the recipient.

Because requests arrive through the exe.dev edge proxy (which can return transient `503 Service Unavailable` before the request reaches this VM — that response body carries a `trace:` id and `x-trace-id` header, and never appears in `journalctl`), the calling Airtable automation should retry on 5xx/network errors with a short backoff (e.g. 3 attempts, ~5 s apart) and surface the final `trace:` id if it still fails.

### Sold Sync

**Route:** `POST /sold/sync`
**Code:** `services/sold_sync.go`

An authenticated Airtable automation calls this after a lead is marked sold. The handler creates a CompanyCam project and uploads lead photos when a project ID is missing, creates a Stripe customer when needed, and writes resulting IDs back to Airtable. It is designed to be idempotent: existing IDs are not recreated.

### Invoice Creation

**Route:** `POST /invoice/create`
**Code:** `services/invoice.go`

An authenticated Airtable automation calls this after sold sync. The handler verifies the lead has a Stripe customer and valid feet amount, gets the configured environment-specific Stripe product's default price, creates/finalizes a Stripe invoice, and stores the hosted invoice URL in Airtable. The Stripe product controls the unit price.

### Bot Crawl Tracking

**Route:** `GET /internal/bots`
**Code:** `services/bottrack.go`

Middleware records known AI, search, and other crawler requests to `~/.local/share/tts/bot-crawls.jsonl`, including status code and requested path. The public route requires `Authorization: Bearer WEBHOOK_AUTH_KEY`. The internal listener exposes the same dashboard without bearer auth, relying on exe.dev VM-owner access control.

### Umami Analytics Proxy

**Routes:** `GET /analytics/script.js`, `POST /analytics/send`, `POST /analytics/api/send`
**Code:** `services/umamiproxy.go`

The public Go server proxies the self-hosted Umami tracker and collection endpoint to the configured local Umami upstream. This keeps browser tracking first-party while leaving the Umami dashboard itself private. The tracker script builds its collect URL from `data-host-url` + `/api/send`, producing `/analytics/api/send`; both that and the legacy `/analytics/send` path are proxied to Umami's `/api/send`. Unknown `/analytics/*` paths return 404.

**Google Ads conversion tracking:** The global Google tag (`AW-17686347200`) is loaded once in `layouts/partials/head.html`. `assets/js/form-submit.js` fires the Google Ads `Submit lead form` conversion (`AW-17686347200/utHpCODyheocEMD7wPFB`) only when `/contact` returns `X-Lead-Saved: true`, which the server sets after Airtable confirms lead creation. Phone calls from Google Ads are tracked separately through the Google Ads call asset/conversion action and do not use this website event.

**Umami conversion tracking:** The contact form fires a sequence of Umami custom events so conversion goals and the Funnel report can measure engagement and drop-off:

| Event | Fired from | When |
|---|---|---|
| `free_estimate_view` | `assets/js/landing-tracking.js` | A visit to the paid `/free-estimate/` landing page; records UTM source when available. |
| `free_estimate_cta` (property: `placement`) | `assets/js/landing-tracking.js` | Clicks on the paid landing page's header or final estimate CTA. |
| `contact_form_start` | `assets/js/form-tracking.js` | First field focus |
| `contact_form_field` (property: `field`) | `assets/js/form-tracking.js` | Each field's first focus (once per field) |
| `contact_form_submit` | `assets/js/form-submit.js` | Airtable-confirmed lead creation (`X-Lead-Saved: true`) |

In the Umami dashboard, a goal of type "Custom event" with name `contact_form_submit` measures conversions. The Funnel report can chain `/free-estimate/` → `free_estimate_view` → `contact_form_start` → `contact_form_submit` to measure paid-landing engagement and conversion; the existing `/contact/` funnel remains useful for general site traffic. The Events → Properties tab and Breakdown report show which specific fields people reach.

The landing page sets the existing Airtable `Which Form` single-select field to `/free-estimate/ ads lander`. Session attribution (`assets/js/attribution.js`) runs on every page and holds the allow-listed `gclid`, `gbraid`, `wbraid`, `fbclid`, and standard UTM values in memory for the current page; it also writes them to `sessionStorage` for cross-page handoff when available. A URL that carries any of those keys **replaces** the stored/current attribution (never merges), so a new campaign visit cannot inherit identifiers from an earlier one. Pages without campaign parameters carry stored session values forward, so a visitor who lands on `/free-estimate/` and submits on `/contact/` keeps the landing attribution when storage works. If browser storage is unavailable, the landing-page submission still has its current-page attribution but it intentionally cannot persist through a navigation. At submit time the Pixel's complete `_fbc`/`_fbp` cookies (when present) and the form page path are added. Opaque identifiers are never clipped; only the human-readable UTM/path CRM summary is length-limited. The server writes the allow-listed values and the Meta event ID into the lead's Airtable `Tracking Metrics` field. This supports manual campaign/lead-quality reconciliation now and preserves identifiers needed for later offline-conversion import workflows.

### Meta Pixel and Conversions API

**Code:** `services/meta.go`, `server.go` (`metaLeadEvent`, `deliverMetaLead`), `layouts/partials/meta-pixel.html`, `assets/js/form-submit.js`, `assets/js/attribution.js`
**Dataset:** `1737341487496114` ("TTS Website"), ad account `1432154495518650`

Browser side:

- `layouts/partials/meta-pixel.html` (included by `analytics.html`) loads the Pixel and fires `PageView` when `params.metaPixelId` is set in `hugo.toml`. It is omitted on the 404 page and on pages whose front matter sets `tracking.metaPixel: false` (`/payment-success/`, `/create-fix-request/`, `/review/`). Internal tools on the Go listeners never include it.
- `form-submit.js` fires `fbq('track', 'Lead', {}, { eventID })` only when `/contact` returned `X-Lead-Saved: true`, using the server's `X-Meta-Event-Id`. No customer data is attached to the browser event. Loading `/thank-you/` never fires a Lead.
- If the Pixel script is blocked, the form still works and the server event is the only one Meta receives.

Server side (`POST /contact`, after Airtable confirms the save):

- One `Lead` event with `event_id` (the same ID returned to the browser), the actual save time, `event_source_url` (canonical host + referring page path, falling back to the attribution's `page`, then `/contact/`), and `action_source: website`.
- `user_data`: email and phone normalized per Meta's rules (trimmed/lowercased email; digits-only phone with `1` country code) and SHA-256 hashed; `client_ip_address`, `client_user_agent`, `fbc`, `fbp` unhashed. `fbc`/`fbp` come from the request's `_fbc`/`_fbp` cookies first, then the complete attribution snapshot, then — for `fbc` only — `fb.1.<landingMs>.<fbclid>` built from a real `fbclid` the visitor arrived with. No click ID is ever synthesized. Inquiry text, quote details, and addresses are never sent (no `custom_data`).
- Delivery uses a 5 s HTTP timeout with one retry on transport/5xx errors (same event ID), a 12 s overall bound, and logs only `event_id` plus a sanitized Meta error (token redacted). Failures never affect the saved lead or the form response. There is no persistent queue: an event that fails both attempts is lost from Meta reporting but remains reconcilable via the `Meta event ID` line in the Airtable `Tracking Metrics` field.
- Missing `META_PIXEL_ID`/`META_CAPI_ACCESS_TOKEN` disables CAPI; lead intake is unaffected.

Deduplication: Meta matches browser and server events on `event_name` + `event_id`. Both channels send `Lead` with the same `tts-lead-…` ID.

Verification without real leads: on the internal listener, `GET /internal/meta-test` (no bearer token; exe.dev-gated) sends one synthetic server Lead tagged with a required `TEST…` code from Events Manager → Test events and renders a page that fires the matching browser Lead with the same ID. Expect a Server row and a Browser row with one marked deduplicated. Keep the Events Manager Test events tab open in the same browser so the browser event is labelled as a test. Booking/status conversions and offline conversion uploads are not implemented.

### AEO / Markdown Content Negotiation

**Routes:** all page paths (e.g., `/about/`, `/faq/`, `/service-areas/lees-summit/northpark-village/`)
**Code:** `server.go` (`markdownNegotiationMiddleware`), `layouts/partials/markdown-page.html`, `layouts/_default/*.md`

Hugo generates both HTML and Markdown versions of every page. The Go server's `markdownNegotiationMiddleware` serves the markdown version when a client sends `Accept: text/markdown`, enabling AI agents that support content negotiation to pull clean markdown instead of parsing HTML. Markdown files are also accessible directly at `{path}/index.md`.

Every response includes a `Link: </llms.txt>; rel="describedby"; type="text/markdown"` header advertising the site's `llms.txt` file. Each HTML page also includes `<link rel="alternate" type="text/markdown">` in its `<head>` pointing to the corresponding markdown file.

The `llms.txt` file at `/llms.txt` (served from `static/llms.txt`) provides a human- and machine-readable summary of the site's content, services, service areas, and important page URLs.

### Sentry and Status

**Routes:** `GET /status`
**Code:** `services/sentry.go`, `services/config.go`

`SENTRY_DSN` enables panic recovery and backend error reporting; unset means the Sentry integration is a no-op. `/status` returns a simple health response and selected Stripe mode/proxy information. Treat it as operational metadata, not a public customer page.

---

## Route Map

| Method | Path | Authentication / audience |
|---|---|---|
| `GET`, `HEAD` | `/contact` | Public; redirects to `/contact/` |
| `POST` | `/contact` | Public contact intake |
| `GET` | `/pay` | Public customer checkout |
| `POST` | `/stripe/webhook` | Stripe signature required |
| `GET` | `/status` | Public operational status |
| `POST` | `/estimate/send` | Bearer token required |
| `POST` | `/confirmation/send` | Bearer token required |
| `POST` | `/oos/send` | Bearer token required |
| `POST` | `/sold/sync` | Bearer token required |
| `POST` | `/invoice/create` | Bearer token required |
| `GET` | `/photos/{filename}` | Public permanent email-photo URLs |
| `GET` | `/internal/bots` | Bearer token on public listener; no bearer token on internal listener |
| `GET` | `/internal/meta-test` | Internal listener only; synthetic Meta Lead test (requires `?code=TEST…`) |
| `GET` | `/analytics/script.js` | Public Umami script proxy |
| `POST` | `/analytics/send` | Public Umami collection proxy (legacy path) |
| `POST` | `/analytics/api/send` | Public Umami collection proxy (tracker default) |

The static Hugo site is the public catch-all and must remain registered after specific routes.

---

## Build, Test, and Service Management

The Makefile runs Hugo tasks only:

```bash
make dev          # Hugo development server on :1313
make build        # Hugo build to public/
make build-prod   # Minified production Hugo build to public/
make clean        # Removes Hugo generated artifacts
```

Build and test the backend separately:

```bash
go test ./...
node --test assets/js/attribution_test.js
go build -o tts-server .
```

Typical VM commands, after verifying the service name:

```bash
sudo systemctl status tts.service
sudo journalctl -u tts.service -f
make build-prod && go build -o tts-server .
sudo systemctl restart tts.service
```

## Seasonal Startup Checklist

```text
□ Confirm the Hugo build and Go binary are current
□ Confirm the service is running and logs are clean
□ Verify APP_ENV is correct before customer traffic
□ Verify the matching Stripe webhook secret and invoice product are configured
□ Confirm the Stripe webhook endpoint subscribes to `checkout.session.completed` and `payment_intent.succeeded`
□ Make a controlled payment/webhook test in the intended mode
□ Check Airtable schemas and automations for seasonal changes
□ Verify Gmail sender/authentication and CompanyCam integration
□ Verify Sentry, Umami, and the internal bot dashboard as applicable
□ Rotate credentials that are due for rotation
```

## Deployment Notes and Follow-up Work

Live operational status belongs in deployment tickets or the VM's runbook, not as permanent implementation claims in this file. Before a season or release, verify:

- Stripe Dashboard webhook URLs for the actual public hostname.
- Airtable automations that call estimate, confirmation, out-of-service, sold-sync, and invoice endpoints.
- Gmail sender/account configuration (`GMAIL_USER`, `GMAIL_SEND_AS`, and app password).
- CompanyCam proxy/direct credentials as applicable.
- DNS and any external payment hostname configuration.
