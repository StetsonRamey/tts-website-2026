/**
 * Progressive enhancement: submit form via fetch with graceful fallback
 * - With JS: fetch + instant redirect (no loading spinner)
 * - Without JS: native form POST + browser redirect
 */
(function () {
  "use strict";

  const form = document.querySelector(".contact-form__card");
  if (!form) return;

  const ENDPOINT = "/contact";

  form.addEventListener("submit", async (e) => {
    // Only hijack if form is valid (validation.js has already run)
    const fields = Array.from(form.querySelectorAll("input, select, textarea")).filter(
      (field) => field.hasAttribute("required") || field.hasAttribute("pattern"),
    );
    const isValid = fields.every((f) => f.validity.valid);

    if (!isValid) {
      // Let validation.js handle it (preventDefault already happened there)
      return;
    }

    // Prevent default form submission
    e.preventDefault();

    // Collect form data
    const formData = new FormData(form);
    const data = Object.fromEntries(formData);
    // Compute fill time in seconds and swap out the raw timestamp
    var loaded = parseInt(data._loaded) || 0;
    data._fillTime = loaded ? (Date.now() - loaded) / 1000 : 0;
    delete data._loaded;

    const submitBtn = form.querySelector(".contact-form__submit");
    const originalText = submitBtn.textContent;

    try {
      // Show subtle feedback (optional: add a disabled state to button)
      submitBtn.disabled = true;
      submitBtn.textContent = "Submitting...";

      // Send to Val Town endpoint
      const response = await fetch(ENDPOINT, {
        method: "POST",
        headers: { "Content-Type": "application/json", "Accept": "text/html" },
        body: JSON.stringify(data),
      });

      if (!response.ok) {
        // Try to parse as JSON for validation errors
        try {
          const result = await response.json();
          console.error("Form errors:", result.errors);
        } catch (_) {}
        submitBtn.disabled = false;
        submitBtn.textContent = originalText;
        return;
      }

      const html = await response.text();

      // ── Conversion tracking ──
      // Fire Google Ads conversion after the form is accepted. The site-wide
      // Google tag is loaded in layouts/partials/head.html.
      if (typeof gtag === "function") {
        try {
          gtag("event", "conversion", {
            send_to: "AW-17686347200/utHpCODyheocEMD7wPFB",
          });
        } catch (_) {}
      }

      // Fire a Umami custom event so conversion goals can be measured.
      // In Umami, create a goal of type "Custom event" with name "contact_form_submit".
      if (typeof umami === "object" && typeof umami.track === "function") {
        try { umami.track("contact_form_submit"); } catch (_) {}
      }

      var section = form.closest(".contact-form");
      if (section) {
        section.outerHTML = html;
      } else {
        form.outerHTML = html;
      }

      setTimeout(function () {
        var target = document.getElementById("thank-you-next");
        if (target) target.scrollIntoView({ behavior: "smooth", block: "start" });
      }, 50);
    } catch (error) {
      console.error("Form submission error:", error);
      // Restore button state
      const submitBtn = form.querySelector(".contact-form__submit");
      submitBtn.disabled = false;
      submitBtn.textContent = originalText;
      // Optionally show error message to user
    }
  });
})();
