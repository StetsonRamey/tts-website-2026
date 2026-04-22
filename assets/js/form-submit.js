/**
 * Progressive enhancement: submit form via fetch with graceful fallback
 * - With JS: fetch + instant redirect (no loading spinner)
 * - Without JS: native form POST + browser redirect
 */
(function () {
  "use strict";

  const form = document.querySelector(".contact-form__card");
  if (!form) return;

  const VAL_TOWN_ENDPOINT = "https://stetson-tts-contact-form.val.run/";

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

    const submitBtn = form.querySelector(".contact-form__submit");
    const originalText = submitBtn.textContent;

    try {
      // Show subtle feedback (optional: add a disabled state to button)
      submitBtn.disabled = true;
      submitBtn.textContent = "Submitting...";

      // Send to Val Town endpoint
      const response = await fetch(VAL_TOWN_ENDPOINT, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });

      if (!response.ok) {
        throw new Error(`Server error: ${response.status}`);
      }

      const result = await response.json();

      if (result.success) {
        // Redirect to thank-you page
        window.location.href = "/thank-you/";
      } else {
        // Handle validation errors from server
        console.error("Form errors:", result.errors);
        submitBtn.disabled = false;
        submitBtn.textContent = originalText;
        // Optionally show error message to user
      }
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
