# Tis The Season KC — Site Rebuild Plan

## Ignore

Ignore the folder `/pen-designs`

## Overview

Rebuild [tistheseasonkc.com](https://tistheseasonkc.com/) as a fast, SEO-optimized static site using:

| Tool | Role |
|------|------|
| **Hugo** (Go) | Static site generation, templating, content management |
| **CUBE CSS** | Styling methodology (Composition → Utility → Block → Exception) |
| **Vanilla JS** | Progressive enhancement for form UX and analytics |
| **Cloudflare Pages** | Hosting, redirects, deployment |
| **Val Town** | Form backend and PRG-compatible submission flow |

### Locked Decisions

- **Hosting:** Cloudflare Pages
- **Form backend:** Val Town
- **Primary nav:** `Home`, `Services`, `Areas`, `About`, `Contact`
- **Keep `/holiday-lighting/`:** yes, but not in the primary nav
- **Service area pages at launch:** Lee's Summit, MO and Raymore, MO only
- **Images:** self-host originals and process them with Hugo
- **Fonts:** system fonts for now
- **Fix request page:** keep as a utility page linked from the footer, not the main nav
- **Analytics:** build with analytics hooks in mind, but ship analytics after launch

### Workflow Conventions

- **Canonical command runner:** use `make`, not `package.json`/Bun, for top-level local workflows
- **Canonical local commands:** `make dev`, `make build`, `make build-prod`, and `make clean`
- **JS tooling policy:** do not add Bun, npm, pnpm, or yarn unless we later introduce real JS dependencies that justify them
- **Hugo workflow:** keep site generation Hugo-native; Make targets should call Hugo directly rather than wrap another package manager
- **Go helper tooling:** prefer small Go programs for non-trivial helpers so the project stays Go-leaning and doubles as Go practice
- **Go helper layout:** put helper tools under `cmd/<tool>/main.go`
- **Go module policy:** add `go.mod` only when the first real Go helper tool lands; until then this remains a plain Hugo site repo
- **Agent-friendly repo contract:** keep one documented entrypoint for humans and agents, with stable Make targets and concise repo docs

### Canonical Business Inputs

- Use `.agents/business-facts.md` as the source of truth for approved claims, contact details, service areas, trust points, and future brand colors
- Use `/business-logos` as the source folder for approved logo SVGs during implementation

---

## Site Pages & Content Map

Launch IA and SEO priorities:

| Page | URL | Purpose | SEO Priority |
|------|-----|---------|-------------|
| Home | `/` | Hero, value prop, services overview, CTA | Highest |
| Holiday Lighting | `/holiday-lighting/` | Existing high-value service landing page | Highest |
| Services | `/services/` | Services overview hub with residential and commercial lighting paths | Highest |
| Service Areas | `/service-areas/` | Local SEO hub for covered cities | Highest |
| Lee's Summit | `/service-areas/lees-summit/` | Dedicated area landing page | Highest |
| Raymore | `/service-areas/raymore/` | Dedicated area landing page | Highest |
| About | `/about/` | Company story, trust signals, team | High |
| Gallery | `/gallery/` | Photo portfolio of installations | High |
| Contact | `/contact/` | Contact form, phone, service area map | High |
| FAQ | `/faq/` | Common questions (pricing, timing, etc.) | Medium |
| Fix Request | `/create-fix-request/` | Existing utility page for current customers | Low |
| Terms of Service | `/terms/` | Legal | Low |

### Navigation Strategy

- **Primary nav:** `Home`, `Services`, `Areas`, `About`, `Contact`
- **Secondary/footer links:** `Holiday Lighting`, `Gallery`, `Fix Request`, `FAQ`, `Terms`
- **Mobile nav behavior:** no hamburger menu; keep the nav compact enough to remain a horizontal list on mobile
- **Holiday Lighting page role:** keep as a dedicated SEO landing page and link to it prominently from the services hub, homepage, and footer

### Service Positioning

- Treat **Residential Holiday Lighting** and **Commercial Holiday Lighting** as the two main service audiences
- Reflect both audiences on the homepage, services page, and in internal linking
- Keep the initial launch IA simple: start with strong sections for each audience before deciding whether they need dedicated URLs later

### Service Area Strategy

- Launch with dedicated pages for **Lee's Summit, MO** and **Raymore, MO** only
- HOA and neighborhood names can be incorporated as on-page sections, FAQs, and supporting copy
- Do **not** create separate URLs for individual HOAs at launch unless we later have enough unique content and search intent to justify them

### Redirect Map

| Current URL | Action | Destination | Notes |
|-------------|--------|-------------|-------|
| `/` | Keep | `/` | Canonical home page |
| `/about` | Canonicalize | `/about/` | Preserve content and trailing-slash convention |
| `/contact` | Canonicalize | `/contact/` | Preserve content and trailing-slash convention |
| `/holiday-lighting` | Canonicalize | `/holiday-lighting/` | Keep this page live as a first-class landing page |
| `/terms-and-conditions` | 301 redirect | `/terms/` | Clean up legacy URL |
| `/create-fix-request` | Canonicalize | `/create-fix-request/` | Utility page remains live |
| `/holiday-lighting#led-section` | Preserve anchor | `/holiday-lighting/#led-section` | Fragments are not server-redirectable; keep matching anchor IDs |

---

## Content Strategy: Hugo Content Files (Markdown + Front Matter)

**Recommendation: Markdown files with YAML front matter.** This is Hugo's native, simplest approach. No CMS, no JSON files needed for a site this size. Each page is a single `.md` file with structured front matter for metadata and markdown body for content.

Why not JSON/CMS:
- Small site footprint — a CMS adds deployment complexity for no benefit
- Markdown is easy to edit and version-controlled
- Front matter handles all structured data (SEO meta, hero text, service lists)
- If you ever want a CMS later, headless CMS tools (CloudCannon, Decap) can bolt onto Hugo markdown files with zero refactoring

---

## SEO Scheme

### Technical SEO (baked into templates)
1. **Semantic HTML5** — `<header>`, `<nav>`, `<main>`, `<article>`, `<section>`, `<footer>`
2. **Schema.org structured data** (JSON-LD) — `LocalBusiness`, `Service`, `FAQPage`
3. **Meta tags** — title, description, og:image, og:type, twitter:card per page via front matter
4. **Canonical URLs** — auto-generated by Hugo
5. **XML Sitemap** — Hugo built-in (`hugo.toml` config)
6. **robots.txt** — Hugo built-in
7. **Performance** — static HTML, minimal JS, optimized images via Hugo's image processing

### Local SEO (critical for service-area business)
1. **Business info consistency** — company name, approved phone/email, and service-area wording in the footer and structured data without inventing a public office address
2. **Service area pages** — dedicated pages for Lee's Summit and Raymore
3. **Google Business Profile** alignment — match site content to GBP listing
4. **Local keywords** in titles/headings: "Christmas Lighting Lee's Summit", "Holiday Lights Raymore", "Holiday Lights Kansas City Metro"
5. **Neighborhood / HOA mentions** — incorporate HOA names into copy where useful, but keep them on the city pages rather than spinning up thin pages

### Content SEO
1. **Title tags** — `{Page Title} | Tis The Season Holiday Lighting` (< 60 chars)
2. **Meta descriptions** — unique per page, action-oriented (< 160 chars)
3. **H1 per page** — one, keyword-rich
4. **Internal linking** — every page links to Contact/Estimate
5. **Image alt text** — descriptive, keyword-aware
6. **FAQ page** with `FAQPage` schema
7. **Service intent coverage** — include residential and commercial wording where relevant without stuffing pages unnaturally

### Front Matter SEO Pattern
```yaml
---
title: "Christmas Lighting Installation in Lee's Summit"
description: "Professional holiday lighting installation in Lee's Summit, MO. Free estimates, LED lights, maintenance, takedown & storage included."
seo:
  og_image: "/images/og/home.jpg"
  og_type: "website"
  schema_type: "LocalBusiness"
keywords:
  - "christmas lights lee's summit"
  - "holiday lighting kansas city"
  - "christmas light installation"
---
```

---

## Project File Structure

```
tts-go/
├── Makefile                     # Canonical dev/build/clean entrypoint
├── hugo.toml                    # Hugo config (baseURL, title, menus, params)
├── README.md                    # Runbook: setup, commands, workflow conventions
├── content/                     # 📝 All page content (Markdown + front matter)
│   ├── _index.md                # Home page
│   ├── about/
│   │   └── index.md
│   ├── services/
│   │   └── index.md
│   ├── holiday-lighting/
│   │   └── index.md
│   ├── service-areas/
│   │   ├── _index.md            # Service areas overview
│   │   ├── lees-summit/
│   │   │   └── index.md
│   │   └── raymore/
│   │       └── index.md
│   ├── gallery/
│   │   └── index.md
│   ├── contact/
│   │   └── index.md
│   ├── create-fix-request/
│   │   └── index.md
│   ├── faq/
│   │   └── index.md
│   ├── terms/
│   │   └── index.md
│   └── thank-you/
│       └── index.md            # PRG success page after form submission
│
├── layouts/                     # 🏗️ Hugo templates
│   ├── _default/
│   │   ├── baseof.html          # Base layout (html, head, body shell)
│   │   ├── home.html            # Home page template
│   │   ├── single.html          # Default single page template
│   │   └── list.html            # Default list page template
│   ├── partials/
│   │   ├── head.html            # <head> with SEO meta, OG tags, canonical, preload hints
│   │   ├── header.html          # Site header + compact nav
│   │   ├── footer.html          # Site footer (contact info, service-area language, links)
│   │   ├── schema.html          # JSON-LD structured data
│   │   ├── image.html           # Reusable responsive image partial with blur-up support
│   │   ├── analytics.html       # Deferred analytics snippet (launch later)
│   │   └── cta.html             # Reusable call-to-action block
│   ├── contact/
│   │   └── single.html          # Contact page with form
│   ├── create-fix-request/
│   │   └── single.html          # Utility form page shell
│   ├── gallery/
│   │   └── single.html          # Gallery layout
│   ├── faq/
│   │   └── single.html          # FAQ with schema
│   └── service-areas/
│       ├── list.html            # Service areas overview
│       └── single.html          # Individual area page
│
├── assets/                      # 🎨 Processed by Hugo Pipes
│   ├── css/
│   │   ├── global.css           # CUBE: Global/reset styles, custom properties
│   │   ├── composition.css      # CUBE: Layout primitives (flow, wrapper, grid, cluster, sidebar)
│   │   ├── utilities.css        # CUBE: Utility classes (colors, spacing, typography)
│   │   ├── blocks.css           # CUBE: Component styles (card, button, nav, hero, form, gallery)
│   │   └── exceptions.css       # CUBE: Data-attribute state overrides
│   └── js/
│       ├── form.js              # Validation, phone formatting, submit UX
│       ├── address-autocomplete.js
│       └── analytics.js         # Custom analytics tracker (post-launch)
│
├── static/                      # 📦 Copied as-is to public/
│   ├── _redirects               # Cloudflare Pages redirect rules
│   ├── images/
│   │   ├── originals/           # Source images for Hugo processing
│   │   ├── og/                  # Open Graph images per page
│   │   └── logo.svg
│   └── favicon.ico
│
├── data/                        # 📊 Hugo data files (optional structured data)
│   ├── services.yaml            # Service list (name, description, icon)
│   ├── testimonials.yaml        # Customer testimonials
│   └── service-areas.yaml       # City and HOA names used in templates
│
├── cmd/                         # Optional Go helper tools (add when needed)
│   └── ...
│
└── archetypes/                  # Hugo content templates
    └── default.md
```

---

## Design System

Treat the tokens below as a placeholder starting point until the final design arrives.

### Color Tokens (CSS Custom Properties)

Based on the current site's holiday/Christmas theme:

```css
:root {
  /* Core brand */
  --color-primary: #69C4BB;       /* Brand Teal Color */
  --color-secondary: #F066A1;     /* Brand Pink Color */
  --color-accent: #d4af37;        /* Gold / warm lights */

  /* Neutrals */
  --color-dark: #1a1a2e;          /* Near-black for text */
  --color-mid: #4a4a5a;           /* Secondary text */
  --color-light: #f5f5f0;         /* Off-white background */
  --color-white: #ffffff;

  /* Semantic */
  --color-text: var(--color-dark);
  --color-text-muted: var(--color-mid);
  --color-bg: var(--color-white);
  --color-bg-accent: var(--color-light);
  --color-surface: var(--color-white);
  --color-border: #e0e0d8;

  /* States */
  --color-success: #2d8a4e;
  --color-error: #c53030;
  --color-focus: var(--color-accent);
}
```

### Spacing Scale (CUBE CSS approach — fluid)

Using Andy Bell's Utopia-inspired fluid spacing with `clamp()`:

```css
:root {
  /* Fluid spacing scale — min @ 320px, max @ 1140px */
  --space-3xs: clamp(0.25rem, 0.23rem + 0.09vw, 0.3125rem);
  --space-2xs: clamp(0.5rem, 0.46rem + 0.19vw, 0.625rem);
  --space-xs:  clamp(0.75rem, 0.69rem + 0.28vw, 0.9375rem);
  --space-s:   clamp(1rem, 0.93rem + 0.37vw, 1.25rem);
  --space-m:   clamp(1.5rem, 1.39rem + 0.56vw, 1.875rem);
  --space-l:   clamp(2rem, 1.85rem + 0.74vw, 2.5rem);
  --space-xl:  clamp(3rem, 2.78rem + 1.11vw, 3.75rem);
  --space-2xl: clamp(4rem, 3.7rem + 1.48vw, 5rem);
  --space-3xl: clamp(6rem, 5.56rem + 2.22vw, 7.5rem);

  /* One-up pairs (for margin between sections) */
  --space-s-m: clamp(1rem, 0.76rem + 1.2vw, 1.875rem);
  --space-m-l: clamp(1.5rem, 1.13rem + 1.85vw, 2.5rem);
  --space-l-xl: clamp(2rem, 1.39rem + 3.06vw, 3.75rem);

  /* Global gutter */
  --gutter: var(--space-s-m);
}
```

### Responsive Typography (Fluid, CUBE approach)

```css
:root {
  /* Fluid type scale — min @ 320px, max @ 1140px */
  --step--2: clamp(0.6944rem, 0.6547rem + 0.1984vw, 0.8333rem);
  --step--1: clamp(0.8333rem, 0.7754rem + 0.2893vw, 1rem);
  --step-0:  clamp(1rem, 0.913rem + 0.4348vw, 1.25rem);       /* body */
  --step-1:  clamp(1.2rem, 1.0739rem + 0.6304vw, 1.5625rem);
  --step-2:  clamp(1.44rem, 1.2522rem + 0.9391vw, 1.9531rem);
  --step-3:  clamp(1.728rem, 1.4478rem + 1.4009vw, 2.4414rem);
  --step-4:  clamp(2.0736rem, 1.6563rem + 2.0870vw, 3.0518rem);
  --step-5:  clamp(2.4883rem, 1.8689rem + 3.0972vw, 3.8147rem);

  /* Apply to body */
  font-size: var(--step-0);
  line-height: 1.5;
}

h1 { font-size: var(--step-5); line-height: 1.1; }
h2 { font-size: var(--step-4); line-height: 1.2; }
h3 { font-size: var(--step-3); line-height: 1.2; }
h4 { font-size: var(--step-2); line-height: 1.3; }
```

### CUBE Composition Primitives

These are reusable layout classes (inspired by Every Layout):

| Primitive | Purpose |
|-----------|---------|
| `.flow` | Vertical rhythm via `* + *` lobotomized owl |
| `.wrapper` | Centered max-width container with gutter padding |
| `.grid` | Auto-fit responsive grid |
| `.cluster` | Flexbox horizontal grouping with wrap |
| `.sidebar` | Two-column layout with sidebar/main |
| `.switcher` | Flexbox that switches from row to column at a threshold |
| `.cover` | Full-height section (hero) with centered content |
| `.region` | Vertical padding for page sections |

### CUBE Utility Classes

```css
/* Color utilities */
.bg-primary    { background-color: var(--color-primary); }
.bg-secondary  { background-color: var(--color-secondary); }
.bg-accent     { background-color: var(--color-accent); }
.bg-light      { background-color: var(--color-bg-accent); }
.bg-dark       { background-color: var(--color-dark); }
.color-primary { color: var(--color-primary); }
.color-light   { color: var(--color-light); }

/* Spacing utilities */
.gap-s  { gap: var(--space-s); }
.gap-m  { gap: var(--space-m); }
.gap-l  { gap: var(--space-l); }

/* Typography utilities */
.text-center  { text-align: center; }
.font-heading { font-family: var(--font-heading); }
.uppercase    { text-transform: uppercase; }
.weight-bold  { font-weight: 700; }

/* Layout utilities */
.visually-hidden { /* sr-only pattern */ }
```

---

## Image Strategy

### Hosting and Processing

- Use `https://placehold.co/` placeholder images during markup and styling work
- Self-host image originals in the repo
- Use Hugo image processing to generate responsive variants
- Output modern formats where practical (`webp`, `avif`) plus a fallback when needed
- Reserve explicit `width` and `height` on all rendered images to reduce layout shift

### Development Workflow

- Use placeholder images with intentional aspect ratios and dimensions during the build
- Choose placeholders that reflect the intended image shape for each slot (hero, gallery tile, testimonial image, etc.)
- Swap placeholders out for real self-hosted originals before launch
- Do not ship external placeholder URLs in production

### Loading Strategy

- **Hero image:** eager load, high priority, no lazy loading
- **Gallery and below-the-fold images:** lazy load
- **Responsive delivery:** use `srcset` and `sizes`
- **Blur-up UX:** generate a tiny low-quality placeholder and fade into the full image once loaded

### SEO and Accessibility

- Store meaningful filenames before import
- Write descriptive alt text for every content image
- Keep gallery images visually strong, but avoid image-only content sections without supporting copy

---

## Build Phases

### Phase 0: Project Setup ✦ CURRENT
- [ x ] Install Hugo
- [ x ] Scaffold Hugo project (`hugo new site`)
- [ x ] Create `Makefile` with `dev`, `build`, `build-prod`, and `clean` as the canonical local interface
- [ x ] Set up `hugo.toml` with baseURL, menus, params
- [ x ] Add `README.md` with setup steps, workflow conventions, and canonical Make targets
- [ x ] Keep the repo free of `package.json`/Bun unless future JS tooling creates a real need
- [ x ] Add Cloudflare Pages support files (`static/_redirects`, optional `_headers`)
- [ x ] Create CSS file structure (global, composition, utilities, blocks, exceptions)
- [ x ] Create design tokens (colors, spacing, typography custom properties)
- [ x ] Set up Hugo Pipes to bundle CSS
- [ x ] Commit changes

### Phase 1: Raw Pages (Content + Minimal Templates)
- [ x ] Create `baseof.html` with HTML shell
- [ x ] Create content `.md` files for all pages with front matter (SEO meta, structured data)
- [ x ] Create bare-minimum `single.html` / `home.html` templates
- [ x ] Add page shells for `holiday-lighting`, `service-areas`, `gallery`, and `create-fix-request`
- [ x ] Use `placehold.co` image slots so page structure and styling can be built before final asset selection
- [ x ] Pages render with content, zero styling
- [ x ] Verify all routes work (`make dev`)
- [ x ] Commit changes
- [ x ] Update PLAN.md

### Phase 2: Reusable Components (Partials)
- [ ] `head.html` — full `<head>` with SEO meta tags, OG tags, canonical, favicon
- [ ] `schema.html` — JSON-LD structured data (LocalBusiness, Service, FAQPage)
- [ ] `header.html` — logo and compact horizontal nav
- [ ] `footer.html` — approved contact info, service-area language, links, copyright
- [ ] `cta.html` — reusable "Get a Free Quote" call-to-action
- [ ] `image.html` — reusable responsive image partial with blur-up support
- [ ] Navigation menu from `hugo.toml` menu config
- [ ] Commit changes
- [ ] Update PLAN.md

### Phase 3: Semantic HTML Scaffolding (SEO-first markup)
- [ ] All pages use proper semantic elements (`<header>`, `<main>`, `<article>`, `<section>`, `<footer>`)
- [ ] Proper heading hierarchy per page (single H1, logical H2-H4 flow)
- [ ] Landmark roles where needed
- [ ] Skip-to-content link
- [ ] Image elements with proper `alt`, `width`, `height`, `loading="lazy"`
- [ ] Internal links between pages (every page → Contact)
- [ ] Breadcrumbs partial (optional, good for SEO)
- [ ] Commit changes
- [ ] Update PLAN.md

### Phase 4: Styling (CUBE CSS)
- [ ] `global.css` — reset/normalize, custom properties, base element styles, global typography
- [ ] `composition.css` — flow, wrapper, grid, cluster, sidebar, switcher, cover, region
- [ ] `utilities.css` — color, spacing, typography, visibility utilities from tokens
- [ ] `blocks.css` — nav, hero, card, button, footer, form, gallery-grid
- [ ] `exceptions.css` — data-attribute states (active nav, reversed card, etc.)
- [ ] Responsive behavior via fluid type/space (no breakpoint-heavy media queries)
- [ ] Test across viewport sizes, with the nav staying usable as a horizontal list on mobile
- [ ] Commit changes
- [ ] Update PLAN.md

### Phase 5: Forms + Minimal JS
- [ ] Build progressive-enhancement JS for phone formatting (`(XXX) XXX-XXXX`)
- [ ] Integrate Google Maps Places autocomplete for address entry
- [ ] Add client-side validation on input and on submit
- [ ] Add matching server-side validation in the Val Town endpoint
- [ ] Wire contact form submission to Val Town
- [ ] Use PRG (`POST` → `303` redirect → thank-you page) to avoid duplicate submissions on refresh
- [ ] Reuse the form shell on `/create-fix-request/` with the new site chrome
- [ ] Commit changes
- [ ] Update PLAN.md

### Phase 6: Gallery + Image Pipeline
- [ ] Build gallery page with Hugo-processed responsive images
- [ ] Support a strong browsing experience without requiring a heavy JS lightbox at launch
- [ ] Use blur-up placeholders for gallery thumbnails and major editorial images
- [ ] Confirm image output sizes, lazy loading, and CLS-safe rendering
- [ ] Commit changes
- [ ] Update PLAN.md

### Phase 7: Custom Analytics (Post-Launch)
- [ ] **Lightweight tracker** (`analytics.js`) — no third-party dependencies
- [ ] **What to track:**
  - Page views (path, timestamp)
  - Referrer (document.referrer — captures ad campaign sources)
  - UTM parameters (utm_source, utm_medium, utm_campaign from URL)
  - Viewport / device type (mobile vs desktop)
  - Session duration (approximate via visibilitychange)
  - Outbound link clicks (phone number, email links)
- [ ] **How it works:**
  - On page load: collect data → send `POST` to a tiny endpoint
  - Endpoint options:
    - **Preferred:** lightweight Val Town endpoint or another small collector
    - **Alternative:** Cloudflare Worker / D1 if analytics later moves closer to the site infra
    - **Alternative:** Plausible Analytics self-hosted (open source, privacy-friendly)
  - Use `navigator.sendBeacon()` for reliable fire-and-forget
  - Respect `Do Not Track` header
- [ ] **Dashboard:**
  - Start with raw event collection only
  - Add a lightweight dashboard later if reporting needs justify it
- [ ] **Campaign tracking pattern:**
  - Use UTM links in ads: `tistheseasonkc.com/?utm_source=facebook&utm_campaign=fall2026`
  - Analytics script parses and stores UTM params with pageview
  - You can filter/group by campaign in your data store
- [ ] Commit changes
- [ ] Update PLAN.md

---

## Agent Task Breakdown

Each phase can be executed by an agent with clear inputs/outputs:

| Agent Task | Phase | Depends On | Description |
|------------|-------|------------|-------------|
| `setup-project` | 0 | — | Install Hugo, scaffold project, create config, and add Cloudflare Pages support files |
| `create-design-tokens` | 0 | — | Create all CSS files with custom properties, spacing, typography |
| `create-content` | 1 | Phase 0 | Write all markdown content files with SEO front matter |
| `create-base-templates` | 1 | Phase 0 | baseof.html, home.html, single.html, list.html |
| `create-partials` | 2 | Phase 1 | head, header, footer, schema, cta, analytics partials |
| `semantic-markup` | 3 | Phase 2 | Refine all templates with proper semantic HTML, heading hierarchy, landmarks |
| `style-global` | 4 | Phase 0 | global.css reset, element defaults, custom properties |
| `style-composition` | 4 | Phase 0 | composition.css layout primitives |
| `style-utilities` | 4 | Phase 0 | utilities.css from tokens |
| `style-blocks` | 4 | Phase 3 | blocks.css component styles |
| `style-exceptions` | 4 | Phase 4 blocks | exceptions.css state overrides |
| `build-contact-form` | 5 | Phase 4 | Contact form templates, progressive JS, Val Town wiring, PRG flow |
| `build-gallery` | 6 | Phase 4 | Hugo image processing, responsive gallery, blur-up placeholders |
| `build-analytics` | 7 | Phase 3 | analytics.js tracker + endpoint setup |

---

## Agent Working Preferences

The workflow below will help future implementation stay accurate and low-friction:

1. **Prefer canonical commands** — use the repo's Make targets first so humans and agents share the same entrypoints
2. **Keep docs close to reality** — when commands or structure change, update the Makefile and README in the same pass
3. **Use Go for meaningful helpers** — if we add generators, audits, or content transforms, prefer small Go tools in `cmd/` over JS scripts
4. **Avoid speculative tooling** — do not introduce JS package managers, bundlers, or extra build layers without a concrete need
5. **Preserve Hugo-first simplicity** — content in markdown, templates in Hugo, assets via Hugo Pipes, and only minimal progressive JS where the site actually needs it

---

## Remaining Inputs To Gather

1. **Approved image folder** — can come after markup/styling, but needed before launch for final processing
2. **Approved business facts additions** — add any missing HOA / neighborhood names and future approved claims to `.agents/business-facts.md` as they become confirmed
3. **Brand colors** — add these to `.agents/business-facts.md` under a new `## Brand Colors` section so implementation can use one canonical source alongside the approved logos in `/business-logos`
4. **Copy inputs** — hero copy, service descriptions, FAQs, testimonials, legal text
5. **Val Town form contract** — endpoint URL, required fields, spam protection, and redirect target

---

## Ready to Start?

Say the word and we'll begin with **Phase 0: Project Setup** — install Hugo, scaffold the project, create the file structure, wire in Cloudflare Pages redirects, add the canonical Make targets, and get `make dev` running with a blank site.
