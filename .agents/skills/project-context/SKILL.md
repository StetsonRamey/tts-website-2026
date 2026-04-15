---
name: project-context
description: This skill gives the agents project specific context for the build
---

# Skill: Hugo Site Implementation

Use this skill when editing Hugo templates, content structure, partials, CSS, or reusable site architecture.

## Canonical source of truth

Use `.agents/business-facts.md` as the source of truth for all reusable business information.

If a template, partial, or content block needs company facts, use only approved facts from that file.

Do not hardcode invented claims into templates.

## Project structure awareness

Key directories:

* `content/`
* `layouts/_default/`
* `layouts/partials/`
* `layouts/contact/`
* `layouts/create-fix-request/`
* `layouts/gallery/`
* `layouts/faq/`
* `layouts/service-areas/`
* `assets/css/`
* `assets/js/`
* `data/`
* `static/images/`

## Implementation goals

Prefer:

* modular templates
* reusable partials
* data-driven repeated content
* clean Hugo conventions
* content that remains editable in markdown or data files

Avoid:

* repeated hardcoded CTA sections
* duplicate layout logic
* business facts scattered inconsistently across templates
* unnecessary complexity

## Partial guidance

Use partials for repeated sections such as:

* CTA bands
* trust bars
* testimonial blocks
* gallery previews
* service area lists
* FAQ previews
* schema output

If a section is reused or likely to be reused, extract it into a partial.

## Data guidance

Use `data/` files for reusable collections such as:

* services
* testimonials
* service areas

If a reusable business-facts data file is created later, ensure it matches `.agents/business-facts.md`.

## Content guidance

Put editable marketing content in `content/` where possible.

Use front matter for:

* title
* description
* hero settings if supported
* page-specific metadata
* optional CTA labels if needed

Do not bury important page copy inside layout templates unless it is a true default.

## CSS guidance

Respect the existing CSS architecture:

* `global.css`
* `composition.css`
* `utilities.css`
* `blocks.css`
* `exceptions.css`

Prefer:

* reusable block styles
* minimal one-off overrides
* clean mobile-first styling

Avoid:

* inline styles
* style duplication
* dumping everything into one file

## JavaScript guidance

Use JavaScript only when it improves UX materially.

Avoid JS for decorative effects.

Focus JS on:

* forms
* address autocomplete
* analytics
* simple conversion-related enhancements

## SEO and schema guidance

Use visible content and approved business facts consistently.

Do not output schema fields that invent:

* office addresses
* storefront location
* unsupported business details

If location information is needed, use service-area language rather than making up a physical office.

## Output expectations

Implementation should be:

* modular
* conversion-aware
* editable
* factually accurate
* aligned with Hugo best practices
