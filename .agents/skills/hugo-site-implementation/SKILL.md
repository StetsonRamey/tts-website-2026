---
name: hugo-site-implementation
description: This skill gives the agents guidance for the architecture and specifics of building the hugo site
---

# Skill: Hugo Site Implementation

Build and edit this project using Hugo best practices. Respect the existing project structure and avoid introducing unnecessary complexity.

## Project structure awareness

Key directories in this project:

* `content/` for page content
* `layouts/_default/` for base templates
* `layouts/partials/` for shared UI
* `layouts/contact/`, `layouts/gallery/`, `layouts/faq/`, `layouts/service-areas/` for section-specific templates
* `assets/css/` for CSS architecture
* `assets/js/` for JavaScript
* `data/` for structured reusable content
* `static/images/` for static image assets

## Implementation goals

When changing the site:

* preserve Hugo conventions
* keep templates modular
* reuse partials
* avoid duplicating repeated markup
* prefer data-driven content where appropriate
* maintain clean separation between content, presentation, and data

## Templating rules

Prefer:

* `baseof.html` for shared shell structure
* partials for reusable sections
* section-specific templates only when page type truly needs custom presentation
* Hugo variables and conditionals for content-driven rendering

Avoid:

* hardcoding repeated CTA blocks across multiple templates
* duplicating header or footer markup
* putting page-specific business copy directly into shared templates unless intentionally defaulted
* mixing large chunks of logic into every template

## Partial usage guidance

Use partials for reusable pieces such as:

* CTA sections
* trust bars
* testimonials blocks
* gallery previews
* service area summaries
* FAQ previews
* schema output
* image rendering helpers

If a section appears more than once or is likely to be reused, extract it into a partial.

## Data file guidance

Use `data/` files for structured content that may be reused across pages.

Current data files:

* `data/services.yaml`
* `data/testimonials.yaml`
* `data/service-areas.yaml`

Prefer data files for:

* service cards
* service area lists
* testimonials
* repeated trust points
* FAQ collections when reused across pages

Avoid creating data files for one-off content that belongs naturally in a single page file.

## Content and front matter rules

Content belongs in markdown files under `content/`.

Use front matter for:

* title
* description
* draft state
* page-specific metadata
* hero configuration if the template supports it
* optional CTA text if needed

Do not bury important editable marketing copy inside templates when editors should be able to change it in content files.

## CSS architecture rules

Respect the existing CSS structure:

* `global.css` — reset, custom properties, base element styles
* `composition.css` — layout primitives (flow, wrapper, region, grid, cluster, switcher, sidebar, cover)
* `utilities.css` — small single-purpose utility classes
* `assets/css/blocks/` — component styles, one file per block (see below)
* `exceptions.css` — rare overrides

### Block CSS file convention

Component styles live in `assets/css/blocks/` as individual files. Each file wraps its rules in `@layer blocks { }`. The bundle partial (`css-bundle.html`) auto-discovers all files via `resources.Match "css/blocks/*.css"`.

Existing block files:

* `button.css`, `overline.css`, `text-link.css`, `data-balance.css`
* `site-header.css`, `breadcrumbs.css`, `site-footer.css`
* `hero.css`, `trust-bar.css`, `service-cards.css`, `process.css`
* `gallery.css`, `testimonials.css`, `area-cards.css`, `neighborhood-list.css`, `cta-band.css`
* `feature-card.css`, `why-card.css`, `faq-preview.css`, `led-options.css`
* `contact.css`

When creating a new page with new block-level component styles:

* First check if an existing block file already covers the component — reuse it.
* If the component is genuinely new, create a new file in `assets/css/blocks/` named after the component (e.g., `pricing-table.css`).
* Always wrap rules in `@layer blocks { }`.
* Do NOT add new component styles to `global.css`, `composition.css`, or `utilities.css`.
* Do NOT create a monolithic `blocks.css` file — keep blocks split by component.

Guidance:

* put broad site-wide rules in `global.css`
* put layout patterns in `composition.css`
* put small utility classes in `utilities.css`
* put component styling in `assets/css/blocks/<component>.css`
* put rare overrides in `exceptions.css`

Avoid:

* dumping all new styles into a single file
* adding one-off styles without checking whether a reusable block already exists
* inline styles unless absolutely necessary

## JavaScript rules

Use JavaScript sparingly.

Current JS files include:

* `form.js`
* `address-autocomplete.js`
* `analytics.js`

Only add JavaScript when it materially improves UX.

Avoid JS for:

* purely decorative interactions
* behavior that can be handled with HTML and CSS
* features that increase fragility without improving conversion

## Image handling rules

Use images intentionally.

Prefer:

* meaningful filenames
* descriptive alt text
* compressed assets
* consistent gallery presentation
* hero images that support trust and service clarity

Avoid:

* oversized unoptimized images
* decorative stock photos that reduce credibility
* using the same image repeatedly across key sections

## Page-specific implementation notes

For homepage:

* use modular partials
* surface main services and trust early
* add clear CTAs

For service area pages:

* localize headings and copy
* keep structure consistent
* avoid thin or duplicated pages

For gallery:

* prioritize image clarity and simple navigation

For contact and fix request pages:

* keep forms prominent
* reduce distractions
* keep submission flow clear

## Schema and SEO implementation

Use `layouts/partials/schema.html` consistently.

When outputting structured data:

* match visible content
* include local business details if available
* support service and location relevance
* avoid misleading markup

## Page layout and styling conventions

These conventions are established across the existing pages and must be followed when creating new pages.

### Text alignment

* All headings (h1–h3), overlines, and body text are **left-aligned by default**. Do NOT center-justify them.
* `text-center` on a wrapper is an intentional design choice made by the designer — agents should not apply it. If centering is needed, the designer will add it.
* Cards (testimonial-card, why-card, contact-card, feature-card) must remain `text-align: left` internally, even if their parent wrapper uses `text-center`.

### Section structure pattern

Every content section follows this structure:

```html
<section class="region [bg-light|bg-dark]" aria-label="Section description">
    <div class="wrapper flow">
        <p class="overline">Overline Text</p>
        <h2>Section Heading</h2>
        <!-- section content -->
    </div>
</section>
```

* Always include `aria-label` on sections.
* Use `.region` for vertical padding.
* Use `.wrapper` for centered max-width container.
* Use `.flow` for vertical rhythm between child elements.

### Overline + heading pattern

* Sections consistently lead with `<p class="overline">` followed by an `<h2>`.
* Do not skip the overline or invent a different pattern for section introductions.

### data-balance attribute

* Use `data-balance` on `h1` elements and select `h2` elements that benefit from `text-wrap: balance`.
* Do not apply it to every heading — use it where long headings would look awkward without balancing.

### Background alternation

* Sections alternate between no background class, `bg-light`, and `bg-dark`.
* Never stack two consecutive sections with the same background treatment.

### Layout primitives

* Use `.flow` for vertical spacing — never add manual margins between sibling elements.
* Use `.grid` with `--grid-min-item-size` custom property for responsive card grids (set via inline style).
* Use `.switcher` for two-column content/image layouts on inner pages, not grid.
* Use `.cluster` for horizontal flex groupings (CTAs, trust points).
* Only inline styles allowed are CSS custom property overrides: `--grid-min-item-size`, `--wrapper-max`, `--flow-space`.

### Page structure conventions

* Inner pages (not homepage) include breadcrumbs: `{{ partial "breadcrumbs.html" . }}`
* Every page ends with the CTA partial: `{{ partial "cta.html" (dict "page" .) }}`
* The CTA partial accepts optional `heading`, `text`, `buttonLabel`, `buttonURL` overrides.

### Hero patterns

Two hero patterns exist:

1. **Overlay hero** (homepage, holiday-lighting): Uses `.hero__inner` with `.hero__image` and `.hero__content` in a grid overlay layout.
2. **Side-by-side hero** (services, contact): Uses `.hero .region` with a `.switcher` or centered `.flow` inside `.wrapper`.

Choose the pattern that matches the page type. Do not invent new hero layouts.

### Process/steps sections

* Use the established `.process` / `.process-step` pattern with `.process__grid`.
* Steps use `.process-step__number` and `.process-step__title`.
* Always rendered inside a `bg-dark` section.

## Build quality checklist

Before finalizing implementation:

* templates are modular
* repeated UI is abstracted
* content is editable where appropriate
* CSS changes fit the existing architecture
* mobile layout is clean
* no unnecessary JS was introduced
* SEO-critical elements are present
* text is left-aligned by default — no centering unless explicitly directed
* sections follow the wrapper/flow/region pattern
* backgrounds alternate — no two consecutive sections share the same background
* existing composition primitives (flow, grid, switcher, cluster) are used instead of custom layouts
