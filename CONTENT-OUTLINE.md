# Tis The Season KC — IA And Content Outline

## Purpose

This document defines the launch information architecture and the content outline for each page so the site can be scaffolded before final copy is written.

It is intentionally structural rather than final-marketing-copy-ready.

Use `.agents/business-facts.md` as the canonical source for approved company details, contact information, service areas, trust points, and future brand colors.

---

## Do We Need A `schema.json`?

No standalone `schema.json` file is required for the site build.

For this project, structured data should be rendered as JSON-LD inside Hugo templates and partials, using page front matter and shared site params as inputs.

Recommended implementation:

- Put reusable business data in `hugo.toml` or a data file
- Put page-specific schema switches in front matter
- Render JSON-LD in `layouts/partials/schema.html`

That means the schema plan is already captured in [PLAN.md](file:///Users/stetson/workspace/github.com/StetsonRamey/tts-go/PLAN.md), and later we will implement it in template partials rather than maintain a single static JSON file.

Launch schema targets:

- `LocalBusiness` on the site generally
- `Service` on service-focused pages like `/holiday-lighting/`
- `FAQPage` on `/faq/` and possibly on city pages if those FAQs are page-specific
- `BreadcrumbList` if we decide to add breadcrumbs

---

## Image Placeholder Workflow

- Use `https://placehold.co/` image placeholders while building markup and styling
- Match placeholder dimensions to the intended final aspect ratio for each image slot
- Keep image positions flexible during design implementation, then replace placeholders once real selections are made
- Replace all placeholder URLs before launch so production images are self-hosted and processed through Hugo

---

## Primary IA

### Primary Navigation

- Home
- Services
- Areas
- About
- Contact

### Secondary / Footer Navigation

- Holiday Lighting
- Gallery
- Fix Request
- FAQ
- Terms

### URL Map

- `/`
- `/services/`
- `/holiday-lighting/`
- `/service-areas/`
- `/service-areas/lees-summit/`
- `/service-areas/raymore/`
- `/gallery/`
- `/about/`
- `/contact/`
- `/thank-you/`
- `/faq/`
- `/create-fix-request/`
- `/terms/`

---

## Internal Linking Strategy

- Home should link prominently to `Services`, `Holiday Lighting`, `Service Areas`, `Gallery`, and `Contact`
- `Services` should link strongly to `/holiday-lighting/`
- `Services` should give clear paths into both residential and commercial lighting content
- `Service Areas` should link strongly to both city pages
- Each city page should link to `Contact`, `Gallery`, and `Holiday Lighting`
- Gallery should link to `Contact` and `Services`
- FAQ should link to `Contact`
- Footer should reinforce all utility and SEO-important destinations without overloading the header

---

## Page-by-Page Outline

## Home `/`

### Goal

Convert visitors quickly, establish trust, and route users into the best next page.

### Primary SEO Theme

- Christmas light installation
- Holiday lighting
- Lee's Summit and Raymore service coverage

### Suggested Sections

1. Hero
2. Quick value proposition / trust bar
3. Residential and commercial snapshot
4. Why choose us
5. How it works
6. Featured photos
7. Trust / testimonials
8. Service areas snapshot
9. FAQ preview
10. Primary CTA band

### Section Notes

- **Hero:** clear headline, subhead, primary CTA, secondary phone or text CTA, and trust points
- **Quick value proposition / trust bar:** free quote, install, maintenance, takedown, storage, and approved trust signals from `.agents/business-facts.md`
- **Residential and commercial snapshot:** concise cards or list, with a path into `/services/` and clear audience labeling
- **Why choose us:** surface trust early with insured, local, and experience-oriented proof points
- **How it works:** 3 to 5 simple steps to reduce friction
- **Featured photos:** a few strong examples linking into the gallery
- **Trust / testimonials:** short proof points, customer quotes, repeat-customer angle
- **Service areas snapshot:** Lee's Summit and Raymore with links
- **FAQ preview:** answer a few high-friction questions and link to `/faq/`
- **Primary CTA band:** repeat the main conversion action with contact options nearby

---

## Services `/services/`

### Goal

Act as the service hub and create a clean SEO path into the more focused holiday lighting page.

### Primary SEO Theme

- holiday lighting services
- christmas light installation services
- residential holiday lighting
- commercial holiday lighting

### Suggested Sections

1. Intro / overview
2. Residential lighting section
3. Commercial lighting section
4. Shared service inclusions
5. Holiday lighting feature block
6. Process / what is included
7. Why choose us
8. CTA

### Section Notes

- **Intro / overview:** what the company does and who it serves
- **Residential lighting section:** speak to homeowners, curb appeal, hassle-free installation, maintenance, takedown, storage
- **Commercial lighting section:** speak to storefronts, offices, entrances, common areas, and professional presentation
- **Shared service inclusions:** estimate, installation, maintenance, takedown, storage where applicable
- **Holiday lighting feature block:** strong internal link to `/holiday-lighting/`
- **Process / what is included:** set expectations clearly
- **Why choose us:** communication, experience, ease of scheduling
- **CTA:** request estimate

---

## Holiday Lighting `/holiday-lighting/`

### Goal

Be the strongest individual service landing page and absorb existing SEO value from the live site.

### Primary SEO Theme

- christmas light installation
- holiday lighting installation
- LED holiday lighting
- residential holiday lighting
- commercial holiday lighting

### Suggested Sections

1. Hero
2. Service detail overview
3. LED options / design choices
4. What's included
5. Installation process
6. FAQ snippet
7. Photo support
8. CTA

### Section Notes

- **Hero:** keep it tightly focused on the flagship offering
- **Service detail overview:** what the service covers and how it works
- **Service detail overview:** what the service covers and how it works for both homes and commercial properties
- **LED options / design choices:** colors, bulb styles, customization notes
- **What's included:** estimate, install, maintenance, takedown, storage
- **Installation process:** make it feel easy and low-friction
- **FAQ snippet:** price framing, timing, service area, maintenance expectations
- **Photo support:** examples of real installs
- **CTA:** estimate/contact

---

## Service Areas `/service-areas/`

### Goal

Serve as the local SEO hub and route users to the city pages.

### Primary SEO Theme

- christmas light installation near me
- holiday lighting Lee's Summit
- holiday lighting Raymore

### Suggested Sections

1. Intro
2. Areas served grid
3. Residential and commercial local service notes
4. FAQ snippet
5. CTA

### Section Notes

- **Intro:** explain the coverage area simply
- **Areas served grid:** Lee's Summit and Raymore cards linking to their pages
- **Residential and commercial local service notes:** clarify that both homeowners and commercial properties can inquire where applicable
- **FAQ snippet:** not sure if you're in range, how to ask, what happens if outside area
- **CTA:** contact/estimate

---

## Lee's Summit `/service-areas/lees-summit/`

### Goal

Rank locally and reassure Lee's Summit homeowners that you regularly work in their area.

### Primary SEO Theme

- christmas lighting Lee's Summit MO
- holiday light installation Lee's Summit

### Suggested Sections

1. Local hero
2. Service in Lee's Summit overview
3. Neighborhoods / HOA mentions
4. Relevant service details
5. Local testimonials or proof
6. FAQ
7. CTA

### Section Notes

- **Local hero:** city-specific headline and subhead
- **Service in Lee's Summit overview:** who the page is for and what to expect
- **Neighborhoods / HOA mentions:** list supported neighborhoods and HOA familiarity where appropriate
- **Relevant service details:** timing, install style, maintenance, removal
- **Local testimonials or proof:** city-tagged quotes if available
- **FAQ:** city-specific questions
- **CTA:** estimate/contact

---

## Raymore `/service-areas/raymore/`

### Goal

Rank locally and reassure Raymore homeowners that the service area is active and intentional.

### Primary SEO Theme

- christmas lighting Raymore MO
- holiday light installation Raymore

### Suggested Sections

1. Local hero
2. Service in Raymore overview
3. Neighborhoods / HOA mentions
4. Relevant service details
5. Local testimonials or proof
6. FAQ
7. CTA

### Section Notes

- Same structure as Lee's Summit, but with genuinely Raymore-specific copy
- Avoid near-duplicate text between city pages

---

## Gallery `/gallery/`

### Goal

Sell the work visually and support conversions with proof of quality.

### Primary SEO Theme

- christmas light installation photos
- holiday lighting gallery

### Suggested Sections

1. Intro
2. Gallery grid
3. Optional category filters later
4. Supporting proof / caption copy
5. CTA

### Section Notes

- **Intro:** short setup, not heavy copy
- **Gallery grid:** strong images, optimized loading, descriptive alt text
- **Optional category filters later:** rooflines, wreaths, trees, warm white, multicolor, etc.
- **Supporting proof / caption copy:** avoid making this a page with only images
- **CTA:** estimate/contact

---

## About `/about/`

### Goal

Build trust through story, values, and team credibility.

### Primary SEO Theme

- holiday lighting company
- christmas light installers near Lee's Summit

### Suggested Sections

1. Intro / story
2. Values
3. Team
4. Why customers trust us
5. Local roots / service area credibility
6. CTA

### Section Notes

- **Intro / story:** why the company exists and how it operates
- **Values:** communication, ease, reliability, fun, craftsmanship
- **Team:** owners and roles
- **Why customers trust us:** experience, responsiveness, professionalism
- **Local roots / service area credibility:** reinforce Lee's Summit-area roots and nearby service coverage without implying a public storefront
- **CTA:** contact/estimate

---

## Contact `/contact/`

### Goal

Get qualified estimate requests with as little friction as possible.

### Primary SEO Theme

- holiday lighting contact
- christmas lighting estimate

### Suggested Sections

1. Intro
2. Contact options
3. Estimate form
4. Service-area guidance
5. Testimonials / reassurance
6. Submission follow-up expectations

### Form Inputs

1. Name (required)
2. Phone Number (required)
3. Email (required)
4. Service Address (required)
5. Project Type (Residential or Commercial) (required)
6. Message (optional)

### Section Notes

- **Intro:** who should fill this out
- **Contact options:** main phone, text number, and email from `.agents/business-facts.md`
- **Estimate form:** include phone formatting, address autocomplete, validation
- **Service-area guidance:** Lee's Summit and Raymore only, with a graceful note for uncertain addresses
- **Testimonials / reassurance:** a few short trust signals can help conversion
- **Submission follow-up expectations:** explain what happens next without promising a response-time SLA unless it is later approved

---

## Thank You `/thank-you/`

### Goal

Complete the PRG flow after successful form submission.

### Suggested Sections

1. Confirmation message
2. What happens next
3. Return links

### Section Notes

- Keep this page simple
- Include a clear confirmation state so refreshes do not resubmit data

---

## FAQ `/faq/`

### Goal

Answer common objections and support both SEO and conversion.

### Primary SEO Theme

- holiday lighting FAQ
- christmas light installation questions

### Suggested Sections

1. Intro
2. Pricing questions
3. Scheduling questions
4. Service-area questions
5. Installation / maintenance questions
6. Contact CTA

### Section Notes

- Keep answers short, clear, and customer-facing
- This page is a strong candidate for `FAQPage` schema

---

## Create Fix Request `/create-fix-request/`

### Goal

Preserve the existing utility workflow for current customers without elevating it in the IA.

### Suggested Sections

1. Intro
2. Form embed or form shell
3. Support expectations

### Section Notes

- New site chrome should wrap the existing function
- Footer-only discoverability is fine

---

## Terms `/terms/`

### Goal

Provide legal coverage and preserve a clean replacement for the current terms URL.

### Suggested Sections

1. Terms content
2. Contact reference if needed

---

## Content Inputs Still Needed

- neighborhood / HOA names for Lee's Summit and Raymore
- testimonials with permission and city tags
- gallery image set
- legal copy for terms
- optional brand colors to add under a new `## Brand Colors` section in `.agents/business-facts.md`

Image selection can happen after markup and styling, since placeholder images will be used during implementation.

---

## Recommended Next Step

Use this document to scaffold page front matter and section placeholders first, then write copy section by section during the build.
