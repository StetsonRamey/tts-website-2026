# LLM-SEO Content Plan

**Date:** 2026-07-30
**Source data:** 17 days of bot-crawl tracking (2,442 hits, Jul 13–30) + Umami analytics setup

## Context

Bot tracking revealed that AI crawlers (ClaudeBot, Amazonbot, Bytespider, PerplexityBot, OAI-SearchBot) are actively indexing the site. AI bots account for 63% of all crawler traffic (1,549 of 2,442 hits). The bots crawl real content pages repeatedly — the opportunity is deepening existing content, not adding new top-level pages.

Key findings from the data:
- `/holiday-lighting/` is the most-crawled content page (42 hits)
- `/service-areas/lees-summit/` and `/service-areas/raymore/` get strong crawl attention (8 and 6 hits)
- `/faq/` is crawled by answer-engine bots (PerplexityBot, Amazonbot, ClaudeBot) — the strongest LLM-SEO asset
- All non-existent paths bots hit were security probes, not content gaps

## Scope constraints (from owner)

- **No new service areas.** The business serves only Lee's Summit, MO and Raymore, MO. No city expansion.
- Neighborhood/HOA-level pages are in scope and desirable.

---

## Plan Item 1: Neighborhood / HOA Service-Area Pages

**Goal:** Create individual pages for known neighborhoods/HOAs within Lee's Summit and Raymore to capture hyper-local search intent and give AI bots more geographic specificity to index.

**Why:** Bots crawl service-area pages more than services or terms pages. The business already names Winterset Valley (Lee's Summit) and Creekmoor (Raymore) as neighborhoods it serves. These are HOA-level communities where the business has existing familiarity. Creating dedicated pages for them gives AI answer engines specific content to cite for queries like "holiday lighting Winterset Valley" or "Christmas lights Creekmoor HOA."

**Pages to create:**

| Path | Neighborhood | City |
|-----|-------------|------|
| `/service-areas/lees-summit/winterset-valley/` | Winterset Valley | Lee's Summit, MO |
| `/service-areas/raymore/creekmoor/` | Creekmoor | Raymore, MO |

**Content for each page:**
- H1: Neighborhood name + "Holiday Lighting"
- Reference to the parent city service area with a link back
- Mention HOA familiarity and willingness to work within community guidelines
- Service overview (installation, maintenance, takedown, storage) tailored to residential/neighborhood context
- CTA: free estimate (phone/text/contact form)
- SEO: `schema_type: Service`, neighborhood name in title/description/keywords

**Implementation approach:**
- Add neighborhood entries to `data/service-areas.yaml` under each city
- Create content files under `content/service-areas/{city}/{neighborhood}/index.md`
- Use the existing `service-areas/{neighborhood}` layout or extend the existing service-area layout if it doesn't support nested pages
- Add new pages to `sitemap.xml` (Hugo should handle automatically if they're content pages)
- Verify the service-areas index page links to the new neighborhood pages

**Open questions for owner:**
- Are there other neighborhoods/HOAs beyond Winterset Valley and Creekmoor that the business has worked in and wants pages for?
- Any specific HOA rules or restrictions worth mentioning on the neighborhood pages?
- Are there photos from installations in these specific neighborhoods we could use?

---

## Plan Item 2: Expand FAQ with Conversational / Long-Tail Questions

**Goal:** Add more FAQ entries phrased the way people ask AI assistants and voice search, to improve visibility in AI-generated answers.

**Why:** FAQ pages are the primary format AI answer engines scrape for Q&A content. The current FAQ has 13 solid questions. The bots that crawl it (PerplexityBot, Amazonbot, ClaudeBot) feed answer engines. Adding questions phrased as natural-language queries — especially with geographic qualifiers — directly improves the odds of being cited in AI responses.

**Questions to add:**

1. **"Do you install Christmas lights in Lee's Summit, MO?"**
   - Reinforce primary service area with explicit yes; mention neighborhoods and HOA experience

2. **"Do you install holiday lights in Raymore, MO?"**
   - Explicit yes; mention Creekmoor and HOA coordination

3. **"What's the difference between C7 and C9 holiday lights?"**
   - Explain bulb size, brightness, and visual effect; note C9 is most common in photos

4. **"Can I choose custom colors for my holiday lighting?"**
   - Warm white, pure white, multicolor, custom combinations; design consultation

5. **"Do you install lights on trees and bushes, or just rooflines?"**
   - Clarify scope: rooflines, wreaths, and what else is available

6. **"How do I get a quote for holiday lighting?"**
   - Walk through the estimate process: call, text, or online form; free, no-hassle, guaranteed first-year pricing

7. **"What happens if my lights stop working during the season?"**
   - In-season maintenance is included; contact us and we'll fix it; link to terms for warranty details

8. **"Do you offer commercial holiday lighting for businesses?"**
   - Yes; storefronts, office buildings, custom designs; link to services page

9. **"How far in advance should I schedule my holiday light installation?"**
   - October or first week of November for Thanksgiving installation; schedule fills fast

10. **"Do you remove and store the lights after the holidays?"**
    - Yes, takedown and off-season storage included; wreath storage not available; lights returned ready for next season

**Implementation approach:**
- Add entries to the `faq_items` array in `content/faq/index.md`
- Maintain the existing front-matter structure (question + answer fields)
- Keep answers concise (2–4 sentences) — answer engines prefer scannable, direct responses
- Where relevant, link answers to related pages (services, terms, contact)
- Rebuild Hugo after changes

**Open questions for owner:**
- Are there questions you get asked frequently by customers that aren't in the list above or in the current FAQ?
- Any answers that need to be adjusted based on current business practices?

---

## Plan Item 3: Deepen the Holiday Lighting Page

**Goal:** Expand `/holiday-lighting/` with more detailed content on bulb types, color options, and the design process — the page AI bots crawl most.

**Why:** At 42 bot hits (combined trailing-slash variants), `/holiday-lighting/` is the most-crawled content page. It's the flagship service page. Adding structured detail gives AI bots more extractable content for answer queries about lighting types, color options, and what the design/installation process looks like.

**Content to add or expand:**

### Section: Bulb Types and Options
- **C7 bulbs:** Smaller, more delicate look. Good for subtle accent lighting.
- **C9 bulbs:** Larger, brighter, most popular. The style shown in most gallery photos.
- **LED technology:** Energy efficient, vibrant, long-lasting. All lights are commercial-grade LED.
- **Wreaths:** Available with lighting; note that wreath storage is not offered.

### Section: Color Choices
- **Warm white:** Classic, cozy traditional look
- **Pure white:** Clean, modern, bright
- **Multicolor:** Festive red, green, blue, gold
- **Custom combinations:** Two-tone and themed displays (e.g., red + green, blue + white)
- Note: C9 is the most common style shown in gallery photos

### Section: The Design Process (step-by-step)
1. **Free consultation & estimate** — We assess your property, discuss your vision, and provide guaranteed first-year pricing
2. **Custom design** — We design a display tailored to your home's architecture and your preferences
3. **Professional installation** — Insured crew installs in under an hour for most homes
4. **In-season maintenance** — If anything goes out, we fix it at no extra charge
5. **Post-season takedown** — We remove everything after the holidays
6. **Off-season storage** — We store your lights and have them ready for next year

**Implementation approach:**
- Edit `content/holiday-lighting/index.md` to add or expand the sections above
- Use existing image assets from `static/images/originals/` where applicable
- Keep the existing front matter and SEO metadata; add keywords if new terms (C7, C9, LED) aren't already captured
- Ensure the page links to the gallery, FAQ, and contact page where relevant
- Rebuild Hugo after changes

**Open questions for owner:**
- Is the design process description accurate? Any steps to add or change?
- Are there additional bulb types or services (e.g., tree wrapping, gutter lighting) not listed above?
- Any specific gallery photos that should be referenced inline on this page?

---

## Implementation Order

1. **Plan Item 2 (FAQ expansion)** — fastest to implement, highest LLM-SEO ROI, no layout changes needed
2. **Plan Item 3 (Holiday lighting page)** — content expansion on existing page, no structural changes
3. **Plan Item 1 (Neighborhood pages)** — requires new content files and possibly layout work for nested service-area pages

## Validation

After all changes:
- Run `make build` or `make build-prod` to rebuild Hugo output
- Verify new pages appear in `sitemap.xml`
- Verify pages render correctly by checking `public/` output
- Run `go test ./...` and `go build -o tts-server .` (no backend changes expected, but validate)
- Check the bot dashboard in the following weeks to see if new pages get crawled
- Check Umami (now fixed) for real visitor data on the new pages

## Post-Implementation Monitoring

Now that both tracking systems are fixed:
- **Bot tracking** (status codes now correct) will show if AI bots crawl the new neighborhood pages and whether any return 404s
- **Umami analytics** (proxy now fixed) will show real human visitor engagement on the expanded pages
- Revisit this data in 2–4 weeks to measure impact