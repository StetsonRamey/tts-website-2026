/**
 * Progressive enhancement: real-time client-side form validation
 * with custom error messages and visual feedback.
 * Gracefully degrades if JS is disabled — HTML5 validation still works.
 */
(function () {
  "use strict";

  const form = document.querySelector(".contact-form__card");
  if (!form) return;

  // Custom error messages
  const errorMessages = {
    firstName: {
      valueMissing: "First name is required",
      tooShort: "First name must be at least 2 characters",
    },
    lastName: {
      valueMissing: "Last name is required",
      tooShort: "Last name must be at least 2 characters",
    },
    email: {
      valueMissing: "Email is required",
      typeMismatch: "Please enter a valid email address",
      patternMismatch: "Please enter a valid email address with a domain",
    },
    phone: {
      valueMissing: "Phone number is required",
      tooShort: "Phone number must be 10 digits",
      tooLong: "Phone number is too long",
      patternMismatch: "Phone number must be 10 digits",
    },
    streetAddress: {
      valueMissing: "Street address is required",
      tooShort: "Street address is too short",
    },
    city: {
      valueMissing: "City is required",
      tooShort: "City name is too short",
    },
    state: {
      valueMissing: "State is required",
    },
    zip: {
      valueMissing: "Zip code is required",
      tooShort: "Zip code must be at least 5 digits",
      tooLong: "Zip code is too long",
    },
  };

  /**
   * Get appropriate error message for a field
   */
  function getErrorMessage(field) {
    const validity = field.validity;
    const fieldMessages = errorMessages[field.name];

    if (!fieldMessages) return "This field is invalid";

    if (validity.valueMissing) return fieldMessages.valueMissing;
    if (validity.typeMismatch) return fieldMessages.typeMismatch;
    if (validity.patternMismatch) return fieldMessages.patternMismatch;
    if (validity.tooShort) return fieldMessages.tooShort;
    if (validity.tooLong) return "This field is too long";

    return "This field is invalid";
  }

  /**
   * Mark field as touched (user has interacted with it)
   */
  function markTouched(field) {
    field.parentElement.classList.add("touched");
  }

  /**
   * Validate a single field and update display
   */
  function validateField(field) {
    const errorSpan = field.parentElement.querySelector(".field-error");
    const isValid = field.validity.valid;

    if (isValid) {
      field.removeAttribute("aria-invalid");
      field.classList.remove("is-invalid");
      if (errorSpan) errorSpan.textContent = "";
    } else {
      field.setAttribute("aria-invalid", "true");
      field.classList.add("is-invalid");
      if (errorSpan) {
        errorSpan.textContent = getErrorMessage(field);
      }
    }
  }

  /**
   * Get all form fields to validate
   */
  function getFormFields() {
    return Array.from(form.querySelectorAll("input, select, textarea")).filter(
      (field) => field.hasAttribute("required") || field.hasAttribute("pattern")
    );
  }

  // Validate on input (real-time feedback)
  getFormFields().forEach((field) => {
    // Mark as touched on first blur
    field.addEventListener("blur", () => {
      markTouched(field);
      validateField(field);
    });

    // Real-time validation after touched
    field.addEventListener("input", () => {
      if (field.parentElement.classList.contains("touched")) {
        validateField(field);
      }
    });

    field.addEventListener("change", () => {
      if (field.parentElement.classList.contains("touched")) {
        validateField(field);
      }
    });
  });

  // Prevent submit if invalid
  form.addEventListener("submit", (e) => {
    e.preventDefault();

    // Validate all fields
    const fields = getFormFields();
    let isFormValid = true;

    fields.forEach((field) => {
      validateField(field);
      if (!field.validity.valid) {
        isFormValid = false;
      }
    });

    if (isFormValid) {
      // Form is valid — submit
      // (This will be wired to Val Town in next phase)
      console.log("Form is valid, ready to submit");
      // form.submit(); // Uncomment when Val Town endpoint is ready
    } else {
      // Mark all fields as touched to show validation
      fields.forEach((f) => markTouched(f));
      // Re-validate all to show errors
      fields.forEach((f) => validateField(f));
      // Focus first invalid field
      const firstInvalid = fields.find((f) => !f.validity.valid);
      if (firstInvalid) {
        firstInvalid.focus();
        firstInvalid.scrollIntoView({ behavior: "smooth", block: "center" });
      }
    }
  });
})();
