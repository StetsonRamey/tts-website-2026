/**
 * Paid-landing-page attribution and engagement tracking.
 *
 * Keeps only Google Ads click identifiers and standard UTM values for the
 * current browser session. The submitted summary is attached to the lead so
 * campaign source can be reconciled with qualified and sold jobs later.
 */
(function () {
  "use strict";

  var page = document.body && document.body.getAttribute("data-landing-page");
  if (!page) return;

  var allowed = ["gclid", "gbraid", "wbraid", "utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content"];
  var attribution = {};
  var params = new URLSearchParams(window.location.search);

  try {
    var stored = sessionStorage.getItem("tts_ad_attribution");
    if (stored) attribution = JSON.parse(stored) || {};
  } catch (_) {}

  allowed.forEach(function (key) {
    var value = params.get(key);
    if (value) attribution[key] = value.slice(0, 200);
  });
  attribution.landing_page = window.location.pathname;

  try {
    sessionStorage.setItem("tts_ad_attribution", JSON.stringify(attribution));
  } catch (_) {}

  var field = document.querySelector("input[name='_attribution']");
  if (field) field.value = JSON.stringify(attribution);

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
