/**
 * Progressive enhancement: auto-format phone numbers as (xxx) xxx-xxxx
 * Gracefully degrades if JS is disabled — user just types unformatted.
 */
(function () {
  "use strict";

  var phoneInput = document.getElementById("phone");
  if (!phoneInput) return;

  // Only apply to tel inputs
  if (phoneInput.type !== "tel") return;

  /**
   * Extract just digits from a string
   */
  function getDigits(value) {
    return value.replace(/\D/g, "");
  }

  /**
   * Format digits as (xxx) xxx-xxxx
   */
  function formatPhone(digits) {
    if (digits.length === 0) return "";
    if (digits.length <= 3) return "(" + digits;
    if (digits.length <= 6) return "(" + digits.slice(0, 3) + ") " + digits.slice(3);
    return "(" + digits.slice(0, 3) + ") " + digits.slice(3, 6) + "-" + digits.slice(6, 10);
  }

  /**
   * Calculate cursor position after formatting
   */
  function getCursorPos(digitsBefore, digitsAfter) {
    // If at end, put cursor at end
    if (digitsAfter.length <= digitsBefore.length) {
      return formatPhone(digitsAfter).length;
    }

    // Figure out where the new digit was inserted
    var newDigitPos = digitsAfter.length - digitsBefore.length;
    var formatted = formatPhone(digitsAfter);
    var digitCount = 0;

    for (var i = 0; i < formatted.length; i++) {
      if (/\d/.test(formatted[i])) {
        digitCount++;
        if (digitCount >= newDigitPos) {
          return i + 1;
        }
      }
    }

    return formatted.length;
  }

  phoneInput.addEventListener("input", function (e) {
    var input = e.target;
    var cursorPos = input.selectionStart;
    var oldValue = input.value;
    var oldDigits = getDigits(oldValue);

    // Get digits before cursor
    var valueBeforeCursor = oldValue.slice(0, cursorPos);
    var digitsBeforeCursor = getDigits(valueBeforeCursor).length;

    // Format the new value
    var newDigits = getDigits(input.value);
    var formatted = formatPhone(newDigits);

    // Calculate new cursor position
    var newDigitsValue = formatted.slice(0, getCursorPos(oldDigits, newDigits));
    var newCursorPos = getDigits(newDigitsValue).length;

    // Find actual position in formatted string
    var digitCount = 0;
    for (var i = 0; i < formatted.length; i++) {
      if (/\d/.test(formatted[i])) {
        digitCount++;
        if (digitCount >= digitsBeforeCursor) {
          newCursorPos = i + 1;
          break;
        }
      }
      // If we've processed all digits, cursor goes to end
      if (i === formatted.length - 1) {
        newCursorPos = formatted.length;
      }
    }

    // Cap at 14 chars: (xxx) xxx-xxxx
    if (newDigits.length > 10) {
      newDigits = newDigits.slice(0, 10);
      formatted = formatPhone(newDigits);
    }

    // Only update if value changed
    if (input.value !== formatted) {
      input.value = formatted;
      // Restore cursor position
      input.setSelectionRange(newCursorPos, newCursorPos);
    }
  });

  // Handle paste: strip non-digits and reformat
  phoneInput.addEventListener("paste", function (e) {
    e.preventDefault();
    var pasted = (e.clipboardData || window.clipboardData).getData("text");
    var digits = getDigits(pasted).slice(0, 10);
    phoneInput.value = formatPhone(digits);
    // Move cursor to end
    phoneInput.setSelectionRange(phoneInput.value.length, phoneInput.value.length);
  });
})();
