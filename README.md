# Tis The Season KC — Hugo Site

Professional holiday lighting website for [tistheseasonkc.com](https://tistheseasonkc.com/).

Built with **Hugo**, styled with **CUBE CSS**, hosted on **Cloudflare Pages**.

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
tts-go/
├── Makefile          # Canonical dev/build/clean commands
├── hugo.toml         # Hugo configuration
├── content/          # Markdown pages with front matter
├── layouts/          # HTML templates and partials
├── assets/css/       # CUBE CSS (global, composition, utilities, blocks, exceptions)
├── static/           # Static files copied as-is (images, _redirects)
├── data/             # YAML data files (services, testimonials, areas)
└── business-logos/   # Approved logo SVGs
```

## Workflow Conventions

- **Use `make` targets** as the canonical interface — not npm/bun/yarn.
- **Hugo-first**: content in markdown, templates in Hugo, assets via Hugo Pipes.
- **No JS package manager** unless real JS dependencies justify it.
- **Go helpers** go in `cmd/<tool>/main.go` when needed.

## CSS Architecture (CUBE CSS)

| File | Role |
|------|------|
| `global.css` | Reset, custom properties, base element styles |
| `composition.css` | Layout primitives (flow, wrapper, grid, cluster, etc.) |
| `utilities.css` | Utility classes (colors, spacing, typography) |
| `blocks.css` | Component styles (nav, hero, card, button, form) |
| `exceptions.css` | Data-attribute state overrides |

## Deployment

Production builds deploy to **Cloudflare Pages**. Redirects are managed in `static/_redirects`.
