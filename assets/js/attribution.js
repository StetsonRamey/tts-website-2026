/**
 * Session-scoped campaign attribution.
 *
 * Runs on every page. When the current URL carries campaign identifiers
 * (Google/Meta click IDs or UTM values) the stored attribution is REPLACED,
 * never merged, so a fresh campaign visit can't inherit stale identifiers
 * from an earlier one. Otherwise the stored session values carry forward, so a
 * visitor who lands on /free-estimate/ and then submits on /contact/ keeps
 * the original landing attribution.
 *
 * Only allow-listed keys are kept. Meta's _fbc/_fbp cookies are read at
 * submit time (form-submit.js) because the Pixel sets them after page load.
 * Nothing here ever invents a click ID.
 */
(function () {
  "use strict";

  var KEY = "tts_ad_attribution";
  var CLICK_IDS = ["gclid", "gbraid", "wbraid", "fbclid"];
  var UTMS = ["utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content"];

  function read() {
    try {
      var stored = sessionStorage.getItem(KEY);
      return stored ? JSON.parse(stored) || {} : {};
    } catch (_) {
      return {};
    }
  }

  function write(attribution) {
    try { sessionStorage.setItem(KEY, JSON.stringify(attribution)); } catch (_) {}
  }

  function cookie(name) {
    var match = document.cookie.match(new RegExp("(?:^|; )" + name + "=([^;]*)"));
    return match ? decodeURIComponent(match[1]).slice(0, 200) : "";
  }

  var params = new URLSearchParams(window.location.search);
  var incoming = {};
  CLICK_IDS.concat(UTMS).forEach(function (key) {
    var value = params.get(key);
    if (value) incoming[key] = value.slice(0, 200);
  });

  var attribution = read();
  if (Object.keys(incoming).length) {
    // New campaign arrival: start clean so identifiers never cross campaigns.
    attribution = incoming;
    attribution.landing_page = window.location.pathname;
    if (incoming.fbclid) attribution.fbclid_ts = String(Date.now());
    write(attribution);
  } else if (!attribution.landing_page && Object.keys(attribution).length === 0) {
    // Organic/direct visit with nothing stored: record where the session began.
    attribution.landing_page = window.location.pathname;
    write(attribution);
  }

  /**
   * Snapshot for submission: stored session attribution plus the Pixel's own
   * first-party cookies (when present) and the page the form lives on.
   */
  function snapshot() {
    var out = {};
    var current = read();
    Object.keys(current).forEach(function (k) { out[k] = current[k]; });
    var fbc = cookie("_fbc");
    var fbp = cookie("_fbp");
    if (fbc) out.fbc = fbc;
    if (fbp) out.fbp = fbp;
    out.page = window.location.pathname;
    return out;
  }

  function fill() {
    document.querySelectorAll("input[name='_attribution']").forEach(function (field) {
      field.value = JSON.stringify(snapshot());
    });
  }

  fill();
  // Refresh right before a native (no-fetch) submit so late-set cookies are included.
  document.querySelectorAll("form.contact-form__card").forEach(function (form) {
    form.addEventListener("submit", fill, true);
  });

  window.ttsAttribution = { read: read, snapshot: snapshot };
})();
