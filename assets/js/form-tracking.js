/**
 * Contact-form engagement tracking for Umami.
 *
 * Fires custom events at key points so the Umami Funnel report can show
 * where visitors drop off before submitting:
 *
 *   /contact/  (pageview, automatic)
 *     → contact_form_start   (first meaningful interaction)
 *     → contact_form_submit  (successful submission, fired in form-submit.js)
 *
 * Each tracked field also fires a `contact_form_field` event with a `field`
 * property (first-name, email, phone, …) so the Events → Properties tab
 * and Breakdown report can show which fields people reach and where they stop.
 *
 * All calls are guarded so they no-op if the Umami tracker is unavailable.
 * Each event fires at most once per field per page load.
 */
(function () {
  "use strict";

  var form = document.querySelector(".contact-form__card");
  if (!form) return;

  var started = false;
  var seenFields = {};

  function track(name, data) {
    if (typeof umami === "object" && typeof umami.track === "function") {
      try { umami.track(name, data); } catch (_) {}
    }
  }

  function startForm() {
    if (started) return;
    started = true;
    track("contact_form_start");
  }

  function fieldEngaged(field) {
    if (seenFields[field]) return;
    seenFields[field] = true;
    startForm();
    track("contact_form_field", { field: field });
  }

  // Map each trackable field's id to a readable slug for the `field` property.
  var fieldMap = {
    "first-name": "first-name",
    "last-name": "last-name",
    email: "email",
    phone: "phone",
    "street-address": "street-address",
    city: "city",
    state: "state",
    zip: "zip",
    message: "message",
  };

  function slugFor(el) {
    return fieldMap[el.id] || el.name || el.id || "unknown";
  }

  // Fire on first focus (user intent to fill) for inputs/select/textarea.
  form.querySelectorAll("input, select, textarea").forEach(function (el) {
    el.addEventListener("focus", function () {
      fieldEngaged(slugFor(el));
    }, { once: true });
  });
})();
