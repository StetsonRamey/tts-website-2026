# Tis The Season KC — Hugo + Go Site

Professional holiday-lighting website for [tistheseasonkc.com](https://tistheseasonkc.com/).

The site is rendered and built with **Hugo**. A small **Go** server serves the built site and owns the runtime features: contact intake, payments, operational automations, email, analytics proxying, and observability. Styling follows **CUBE CSS**. It is deployed on an exe.dev VM.

## Prerequisites

- [Hugo Extended](https://gohugo.io/installation/)
- GNU Make
- Go 1.26+

## Quick Start

```bash
make dev          # Hugo development server only: http://localhost:1313
make build        # Hugo development build → public/
make build-prod   # Production Hugo build → public/
go run server.go  # Go server: public/ plus backend routes on :8000
make clean        # Remove generated Hugo artifacts
```

> `make dev` is for frontend/content work only. It does not run backend routes such as `/contact`, `/pay`, `/analytics/*`, or the internal tools.

For runtime configuration and backend operations, see `SERVICES.md`. Environment-variable names are documented in `.env.example`; the Go server reads environment variables directly and does **not** load a `.env` file itself.

## Project Structure

```text
TTS/
├── AGENTS.md             # Working map and conventions for coding agents
├── README.md             # This site/application overview
├── SERVICES.md           # Backend architecture and operations runbook
├── Makefile              # Hugo dev/build/clean commands
├── hugo.toml             # Hugo configuration, menus, params, outputs
├── server.go             # Go server, routes, middleware, static serving
├── services/             # Backend handlers and third-party integrations
├── content/              # Markdown pages with front matter
├── layouts/              # Hugo layouts, partials, SEO/schema output
├── assets/css/           # CUBE CSS entry point, layers, and component styles
├── assets/js/            # Progressive enhancement for forms and UI behavior
├── assets/images/        # Hugo-managed image source assets
├── static/               # Passthrough assets: favicons, logos, llms.txt, etc.
├── data/                 # YAML data for site content
├── business-logos/       # Approved logo SVGs
├── .agents/              # Static business/domain facts used for templates
└── public/               # Generated Hugo output, served by the Go server
```

## Responsibilities

- **Hugo** — page content, page rendering, templates, SEO/meta tags, schema, generated `robots.txt`, and asset bundling.
- **Go** — contact form intake, Stripe checkout/webhooks, email and internal automation endpoints, staged-photo hosting, first-party analytics proxy, markdown content negotiation for AI agents (AEO), bot-crawl tracking, redirects, caching, security headers, and custom 404 handling.
- **`services/`** — focused backend integrations for Airtable, Stripe, Gmail, CompanyCam, Sentry, Umami, and bot tracking.

## Workflow Conventions

- Use Hugo for content, rendering, and frontend asset changes; use Go only for server/runtime behavior. Hugo outputs both HTML and Markdown for every page.
- Use `make dev`, `make build`, `make build-prod`, and `make clean` for Hugo tasks. The Makefile does not build or run the Go backend.
- There is no Node/npm build step. Hugo Pipes builds CSS and JavaScript from `assets/`.
- Keep secrets out of the repository. Runtime configuration is supplied by environment variables; see `.env.example` and `SERVICES.md`.
- Before changing a subsystem, read its source and the relevant documentation. `AGENTS.md` is the starting point for agents.

### Service-area content structure

City service-area pages are Hugo branch bundles at `content/service-areas/<city>/_index.md`. Neighborhood pages live beneath their city at `content/service-areas/<city>/<neighborhood>/index.md`, which preserves the city relationship in URLs, breadcrumbs, and the sitemap. `data/service-areas.yaml` supplies city display data and neighborhood lists; a neighborhood entry with a `slug` links to its published detail page.

## CSS Architecture (CUBE CSS)

| Path | Purpose |
|---|---|
| `assets/css/main.css` | CSS entry point that imports the layers below |
| `assets/css/global.css` | Reset, custom properties, and base element styles |
| `assets/css/composition.css` | Layout primitives such as flow, wrapper, grid, and cluster |
| `assets/css/utilities.css` | Utility classes for colors, spacing, and typography |
| `assets/css/blocks/*.css` | Component-level styles such as header, hero, cards, forms, and footer |
| `assets/css/exceptions.css` | State-specific and data-attribute overrides |

## Frontend Behavior

Frontend JavaScript is progressively enhanced through Hugo-bundled files in `assets/js/`:

- form validation and asynchronous contact submission
- phone-number formatting
- address autocomplete
- theme toggle

Shared Hugo partials also provide canonical URLs, Open Graph/Twitter metadata, schema.org JSON-LD, analytics markup, and the CSS/JS bundles.

## Special-Purpose Pages

- `/thank-you/` — post-contact confirmation with next steps (`content/thank-you/`, custom layout).
- `/review/ — customer-facing Google review solicitation page for the annual review drawing (5 winners, $75 off install). Linked from the Fillout signup form; `noindex` and excluded from the sitemap since it is not organic-search content (`content/review/`, custom layout).

## Running the Production Server

```bash
make build-prod
go build -o tts-server .
./tts-server
```

The public Go listener runs on port `8000`. It serves `public/` and backend routes. When enabled, a second owner-only internal listener runs on `INTERNAL_PORT` (default `3001`). Deployment/service management details are in `SERVICES.md`.

### Server Features

- **Contact intake** (`/contact`) — accepts HTML form and JSON POSTs; validates fields, applies anti-spam checks, logs submissions, and writes accepted leads to Airtable.
- **Payments** (`/pay`, `/stripe/webhook`) — Stripe Checkout and signed webhook handling.
- **Operational automations** (`/estimate/send`, `/confirmation/send`, `/oos/send`, `/sold/sync`, `/invoice/create`) — authenticated email, CRM, CompanyCam, and invoicing workflows.
- **Analytics and observability** — first-party Umami proxy (`/analytics/*`), Sentry error reporting, and a bot/crawler dashboard (`/internal/bots`).
- **Photo hosting** (`/photos/*`) — permanent serving of staged photos used in estimate emails.
- **Static-site middleware** — canonical `www` and legacy URL redirects, security headers, a Hugo-styled 404 response, and cache headers: one year immutable for CSS/JS/fonts, 30 days for images, and one hour for other responses.
