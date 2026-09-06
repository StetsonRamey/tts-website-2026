# Tis The Season KC — Agent Guide

This is the starting point for anyone working in this repository. Read it before changing code. Keep it current when the architecture, workflows, tooling, or ownership boundaries change.

## Product and Architecture

Tis The Season KC is a holiday-lighting website at `tistheseasonkc.com`.

- **Hugo** renders the public website from `content/`, `layouts/`, `assets/`, `data/`, and `static/`.
- **Go** (`server.go` and `services/`) serves Hugo's `public/` output and provides runtime features: contact intake, Stripe payments, Airtable/CompanyCam workflows, email, analytics proxying, bot tracking, and operational tools.
- The deployed public Go server listens on **port 8000**. An optional owner-only internal listener uses `INTERNAL_PORT` (default **3001**).
- Production deployment runs on an exe.dev VM. Runtime secrets and service-unit configuration are external to the repository.

## Source-of-Truth Map

| Need | Start here |
|---|---|
| Site overview, local build workflow, directory map | `README.md` |
| Backend routes, integrations, configuration, operations | `SERVICES.md` |
| Static business facts used in templates/content | `.agents/business-facts.md` |
| Hugo configuration, menus, params, outputs | `hugo.toml` |
| Hugo build commands | `Makefile` |
| Public-server routing and middleware | `server.go` |
| Backend integrations and handlers | `services/` |
| Page content | `content/` |
| Page layouts and shared SEO/UI partials | `layouts/` |
| Paid Google Ads estimate landing page | `content/free-estimate/`, `layouts/free-estimate/`, `assets/js/landing-tracking.js` |
| CSS and progressive-enhancement JavaScript | `assets/css/`, `assets/js/` |
| Passthrough public assets | `static/` |
| Generated site output — do not hand-edit | `public/` |
| Legacy, unused Val Town contact-form code | `tts-contact-form/` |

`tts-contact-form/` is retained only as historical reference. Do not implement production features there.

## Working Principles

1. **Understand the current implementation first.** Read the relevant source and documentation before editing. Search for existing patterns before adding a new one.
2. **Make the smallest change that makes the requested feature work.** Prefer direct, readable code and existing project conventions over abstraction, framework additions, or speculative refactors.
3. **Do not add speculative guardrails by default.** After the minimal implementation works, identify worthwhile validation, error handling, monitoring, tests, rate limiting, or other defensive measures. Recommend them clearly and ask the human whether they want them implemented before adding them.
4. **Keep scope disciplined.** Do not mix unrelated cleanup or behavior changes into a feature/fix unless necessary for correctness or explicitly requested.
5. **Protect production data and secrets.** Never commit credentials, generated secret files, API tokens, or real customer data. Use environment variables and update `.env.example` only with variable names/example values.
6. **Preserve public behavior intentionally.** The Go server's route ordering, redirects, caching, security headers, and static-file fallback are significant. The Hugo output in `public/` must be rebuilt rather than edited directly.

## Implementation Boundaries

### Hugo and frontend changes

- Put editable page content in `content/`; use front matter and existing data files where appropriate.
- Put reusable rendering/SEO/UI work in `layouts/partials/` or the applicable layout.
- City service-area pages use branch bundles at `content/service-areas/<city>/_index.md`; nested neighborhood pages use `content/service-areas/<city>/<neighborhood>/index.md`. Keep city/neighborhood listings in `data/service-areas.yaml`, adding a neighborhood `slug` only when its detail page is published.
- Maintain CUBE CSS: `assets/css/main.css` imports global, composition, utilities, blocks, and exceptions layers. Add component styles under `assets/css/blocks/` when that is the established fit.
- Frontend JavaScript lives in `assets/js/` and is bundled by Hugo. There is no Node/npm build step.
- Run `make build` or `make build-prod` after applicable Hugo/template/asset changes.

### Go/backend changes

- Register routes in `server.go`; keep specific routes before the static-site catch-all.
- Put focused handlers and external-service logic in `services/`; follow existing authorization and error-reporting patterns.
- Stripe review discounts use coupon catalog records in the Airtable `Services` table, linked separately from customer line-item services through `Customers.Discount/Coupon`; checkout selects the sandbox/live coupon by `APP_ENV` when `Review Discount?` is checked.
- The paid Google Ads landing page is `/free-estimate/`. It uses the same `/contact` form handler and Google Ads conversion event as `/contact/`, and saves allow-listed Google click IDs/UTMs in the lead comments for later quality reconciliation. A browser conversion may fire only after `/contact` returns `X-Lead-Saved: true`, which is sent after Airtable confirms creation. Keep it `noindex` unless its purpose is intentionally expanded to organic search.
- Use configuration through environment variables. If adding/changing one, update `.env.example`, `SERVICES.md`, and this file if it changes the architecture map.
- Build/test with `go test ./...` and `go build -o tts-server .` for backend changes.

## Common Commands

```bash
# Hugo
make dev
make build
make build-prod
make clean

# Go backend
go test ./...
go build -o tts-server .
go run server.go

# Typical deployed-service inspection (verify actual deployment first)
sudo systemctl status tts.service
sudo journalctl -u tts.service -f
```

`make dev` serves Hugo only on port `1313`; it does not run Go routes. `go run server.go` runs the public Go server on port `8000` and serves the existing `public/` build.

## Documentation Rules

Documentation is part of the deliverable:

- Update `README.md` when site structure, local workflows, frontend architecture, or high-level runtime behavior changes.
- Update `SERVICES.md` when routes, backend behavior, integrations, environment variables, ports, authentication, monitoring, or operations change.
- Update `.env.example` whenever environment-variable names, defaults, or expected configuration change; never put real secrets in it.
- Update this `AGENTS.md` when a new agent needs different orientation, conventions, source-of-truth locations, or completion steps.
- Preserve `.agents/business-facts.md` as static reference material unless the human explicitly requests a factual/domain update.

## Required End-of-Session Checklist

This applies to every agent and subagent session that changes the repository:

1. **Update relevant documentation** (`README.md`, `SERVICES.md`, `.env.example`, and/or other `*.md` files) so it matches the implemented changes.
2. **Update `AGENTS.md`** when the changes affect future-agent context, architecture, conventions, tooling, or this checklist.
3. **Validate the work** with the smallest relevant checks (for example `go test ./...`, `go build -o tts-server .`, and/or `make build`). State any checks that could not be run and why.
4. **Review the diff** for accidental files, generated output, secrets, or unrelated changes.
5. **Commit all intended changes** with a clear, imperative commit message.
6. **Push the commit to the configured GitHub remote** before returning to the human. If pushing fails, report the exact failure and leave the commit in place.

Do not claim a change is complete until these steps have been attempted.
