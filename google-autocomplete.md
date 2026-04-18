# Google Maps Autocomplete — Implementation Plan

## Overview

Add Google Places Autocomplete to the Street Address field on the contact form. As users type, they'll see suggestions from Google Maps. Selecting a suggestion will auto-fill the Street Address, City, State, and Zip Code fields.

This is a **progressive enhancement** — the form works fully without JS or if the Google API fails to load.

---

## What Changes

### 1. New JS file: `assets/js/address-autocomplete.js`

Handles:
- Loading the Google Maps Places library via `<script>` tag
- Initializing autocomplete on the `#street-address` input
- On place selection:
  - Extract and fill street address
  - Extract and fill city
  - Extract and fill state (must map full state name → abbreviation)
  - Extract and fill zip code
- Graceful fallback if Google API fails to load or user denies access

### 2. Update: `layouts/contact/single.html`

Add the Google Maps API script tag with your API key (loaded conditionally via Hugo params to keep the key out of source control).

---

## API Key Handling

The Google Places API requires an API key. This will be stored as a Hugo site parameter:

```toml
# hugo.toml or config.toml
[params]
  googleMapsApiKey = "YOUR_KEY_HERE"
```

The template will conditionally render the Google script only when the key is present:

```html
{{ with site.Params.googleMapsApiKey }}
<script src="https://maps.googleapis.com/maps/api/js?key={{ . }}&libraries=places&callback=initAddressAutocomplete" async defer></script>
{{ end }}
```

### API Key Security

- The Places API key **must** have HTTP referrer restrictions in Google Cloud Console
- Restrict to: `https://tistheseasonkc.com/*` and `localhost:*` (for dev)
- Restrict the key to only **Places API** (not Maps JavaScript API, etc.)

### Cost

- Google Places Autocomplete: **$0.017 per session** (first 1,000 sessions/month free)
- A "session" = one user typing and selecting a place (or abandoning)
- Estimated cost: **Free** for this site's traffic level

---

## Address Component Mapping

Google returns structured address components. We need to map them:

| Form Field    | Google Component               | Notes                              |
|---------------|--------------------------------|------------------------------------|
| Street Address| `street_number` + `route`      | Combine these                      |
| City          | `locality`                     | Sometimes `postal_town` in UK      |
| State         | `administrative_area_level_1`  | Must map "Missouri" → "MO"         |
| Zip Code      | `postal_code`                  | May be partial in some countries   |

### State Name → Abbreviation Mapping

Since Google returns full state names, we'll need a lookup object:

```js
const STATE_MAP = {
  "Missouri": "MO",
  "Kansas": "KS",
  // ... other states if needed
};
```

Only MO and KS are in the current dropdown, so the map can be minimal.

---

## UX Considerations

1. **Keyboard navigation**: Users can arrow-down through suggestions and press Enter to select
2. **Click-away**: If user types but doesn't select a suggestion, nothing auto-fills (their typed address stays)
3. **Manual override**: After auto-fill, user can still edit any field manually
4. **No auto-focus**: Don't force focus to the dropdown; let user control when to engage
5. **Mobile**: Google's autocomplete works on mobile; the dropdown adjusts to screen width

---

## Graceful Degradation

| Scenario                    | Behavior                              |
|-----------------------------|---------------------------------------|
| JS disabled                 | Form works normally, no autocomplete  |
| Google API fails to load    | Form works normally, no autocomplete  |
| User denies location access | Not applicable (this is address lookup, not geolocation) |
| Invalid API key             | Console error, form still works       |
| Network error               | Form still works, just no suggestions |

---

## File Checklist

- [ ] `assets/js/address-autocomplete.js` — new file
- [ ] `layouts/contact/single.html` — add Google API script tag
- [ ] `hugo.toml` — add `googleMapsApiKey` param (or `.env` / build variable)
- [ ] Google Cloud Console — create & restrict API key

---

## Testing Checklist

- [ ] Type address → suggestions appear
- [ ] Select suggestion → all 4 fields populate correctly
- [ ] Edit field after auto-fill → user edits are preserved
- [ ] Tab through fields → works normally
- [ ] Keyboard-only navigation → can select from dropdown
- [ ] Disable JS → form still submits with typed address
- [ ] Mobile viewport → dropdown fits screen, usable
- [ ] Slow network → graceful timeout, no broken state

---

## Questions / Open Items

1. **API key storage**: Should the key go in `hugo.toml`, a `.env` file, or be injected at build time (e.g., CI/CD variable)?
2. **State mapping**: Should we support all 50 states or just MO/KS for now?
3. **Country restriction**: Restrict autocomplete suggestions to `country:us` to avoid international addresses?
