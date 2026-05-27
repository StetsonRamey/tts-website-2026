# Tis The Season KC — Hugo + Go Site

Professional holiday lighting website for [tistheseasonkc.com](https://tistheseasonkc.com/).

Built with **Hugo** (static content), **Go** (backend server), styled with **CUBE CSS**, hosted on **exe.dev VM**.

## Prerequisites

- [Hugo Extended](https://gohugo.io/installation/) (v0.120+)
- GNU Make

## Quick Start

```bash
make dev          # Start dev server at http://localhost:1313
make build        # Development build → public/
make build-prod   # Production build (minified) → public/
make clean        # Remove generated files
```

## Project Structure

```
TTS/
├── Makefile          # Canonical dev/build/clean commands
├── hugo.toml         # Hugo configuration
├── server.go         # Go HTTP server (forms, payments, static serving)
├── services/         # Go packages (Stripe, email, photo handling)
├── content/          # Markdown pages with front matter
├── layouts/          # HTML templates and partials
├── assets/css/       # CUBE CSS (global, composition, utilities, blocks, exceptions)
├── static/           # Static assets (images, robots.txt)
├── public/           # Hugo build output (served by Go server)
├── data/             # YAML data files (services, testimonials, areas)
└── business-logos/   # Approved logo SVGs
```

## Workflow Conventions

- **Hugo for content**: Markdown pages, templates, static generation
- **Go for backend**: HTTP server, form handling, payment processing, email services
- **Use `make` targets**: `dev`, `build`, `build-prod`, `clean`
- **No JS package manager** — Hugo handles frontend assets via Pipes
- **Environment**: `.env` file for API keys (Stripe, Airtable, etc.)

## CSS Architecture (CUBE CSS)

| File | Purpose |
|------|----------|
| `global.css` | Reset, custom properties, base element styles |
| `composition.css` | Layout primitives (flow, wrapper, grid, cluster, etc.) |
| `utilities.css` | Utility classes (colors, spacing, typography) |
| `blocks.css` | Component styles (nav, hero, card, button, form) |
| `exceptions.css` | Data-attribute state overrides |

## Building & Running

### Development
```bash
make dev              # Start Hugo dev server at http://localhost:1313
```

### Production Build & Deploy
```bash
make build-prod       # Production build (minified, optimized)
go run server.go      # Start Go server on :8000
```

### Server Features
- **Contact Form Handler** (`/contact`): Validates submissions, rate-limits by IP, geo-blocks non-US, logs to Airtable
- **Stripe Integration** (`/pay`, `/stripe/webhook`): Payment processing and webhook handling
- **Email Services** (`/estimate/send`, `/confirmation/send`, `/oos/send`): Customer communication (protected by auth key)
- **Static File Serving**: Smart cache headers (1-year for immutable assets, 1-hour for HTML)
- **Security Headers**: X-Frame-Options, X-Content-Type-Options, Referrer-Policy
- **URL Redirects**: Legacy Cloudflare redirects (e.g., `/terms-and-conditions` → `/terms/`)
- **www Redirect**: Forces canonical domain without www
