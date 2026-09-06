/**
 * Progressive enhancement: submit form via fetch with graceful fallback.
 * Browser conversion events require the server's Airtable-confirmed lead signal.
 */
(function () {
  "use strict";

  const form = document.querySelector(".contact-form__card");
  if (!form) return;

  const ENDPOINT = "/contact";

  function restoreSubmitButton(submitBtn, originalText) {
    submitBtn.disabled = false;
    submitBtn.textContent = originalText;
  }

  function showSubmissionError(message) {
    let error = form.querySelector(".contact-form__submission-error");
    if (!error) {
      error = document.createElement("p");
      error.className = "contact-form__submission-error";
      error.setAttribute("role", "alert");
      const submitBtn = form.querySelector(".contact-form__submit");
      submitBtn.insertAdjacentElement("beforebegin", error);
    }
    error.textContent = message;
  }

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

      // Send only after browser validation succeeds.
      const response = await fetch(ENDPOINT, {
        method: "POST",
        headers: { "Content-Type": "application/json", "Accept": "text/html" },
        body: JSON.stringify(data),
      });

      if (!response.ok) {
        let message = "";
        try {
          const result = await response.json();
          if (result.error) message = result.error;
          if (result.errors) console.error("Form errors:", result.errors);
        } catch (_) {
          message = "We couldn't save your request right now. Please try again in a moment.";
        }
        if (message) showSubmissionError(message);
        restoreSubmitButton(submitBtn, originalText);
        return;
      }

      const html = await response.text();
      const leadSaved = response.headers.get("X-Lead-Saved") === "true";

      // ── Conversion tracking ──
      // The server supplies X-Lead-Saved only after Airtable confirms creation.
      if (leadSaved && typeof gtag === "function") {
        try {
          gtag("event", "conversion", {
            send_to: "AW-17686347200/utHpCODyheocEMD7wPFB",
          });
        } catch (_) {}
      }

      // Fire a Umami custom event only for a lead Airtable confirmed as saved.
      if (leadSaved && typeof umami === "object" && typeof umami.track === "function") {
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
      restoreSubmitButton(submitBtn, originalText);
      showSubmissionError("We couldn't save your request right now. Please check your connection and try again.");
    }
  });
})();
