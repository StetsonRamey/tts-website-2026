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

* `global.css`
* `composition.css`
* `utilities.css`
* `blocks.css`
* `exceptions.css`

Guidance:

* put broad site-wide rules in `global.css`
* put layout patterns in `composition.css`
* put small utility classes in `utilities.css`
* put component styling in `blocks.css`
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

## Build quality checklist

Before finalizing implementation:

* templates are modular
* repeated UI is abstracted
* content is editable where appropriate
* CSS changes fit the existing architecture
* mobile layout is clean
* no unnecessary JS was introduced
* SEO-critical elements are present
