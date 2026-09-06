/**
 * Paid-landing-page engagement tracking (Umami).
 *
 * Session attribution capture lives in attribution.js and runs on every page;
 * this file only emits the landing-page engagement events.
 */
(function () {
  "use strict";

  var page = document.body && document.body.getAttribute("data-landing-page");
  if (!page) return;

  var attribution = (window.ttsAttribution && window.ttsAttribution.read()) || {};

  function track(name, data) {
    if (typeof umami === "object" && typeof umami.track === "function") {
      try { umami.track(name, data); } catch (_) {}
    }
  }

  track("free_estimate_view", { source: attribution.utm_source || "direct" });

  document.querySelectorAll("[data-landing-cta]").forEach(function (cta) {
    cta.addEventListener("click", function () {
      track("free_estimate_cta", { placement: cta.getAttribute("data-landing-cta") });
    });
  });
})();
