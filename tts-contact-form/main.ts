/**
 * Tis The Season KC — Contact Form Endpoint
 * HTTP endpoint for handling form submissions with validation
 */

interface FormData {
  firstName: string;
  lastName: string;
  email: string;
  phone: string;
  streetAddress: string;
  city: string;
  state: string;
  zip: string;
  message?: string;
}

/**
 * Validate form data
 */
function validateFormData(data: Partial<FormData>): { valid: boolean; errors: string[] } {
  const errors: string[] = [];

  if (!data.firstName || data.firstName.trim().length < 2) {
    errors.push("First name must be at least 2 characters");
  }

  if (!data.lastName || data.lastName.trim().length < 2) {
    errors.push("Last name must be at least 2 characters");
  }

  if (!data.email || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(data.email)) {
    errors.push("Invalid email address");
  }

  // Extract digits from phone (handles various formats)
  const phoneDigits = data.phone?.replace(/\D/g, "") || "";
  if (!phoneDigits || phoneDigits.length !== 10) {
    errors.push("Phone number must be exactly 10 digits");
  }

  if (!data.streetAddress || data.streetAddress.trim().length < 5) {
    errors.push("Street address is required and must be at least 5 characters");
  }

  if (!data.city || data.city.trim().length < 2) {
    errors.push("City is required");
  }

  if (!data.state || !["MO", "KS"].includes(data.state)) {
    errors.push("State is required (MO or KS)");
  }

  // Extract digits from zip (handles formats like 12345 or 12345-6789)
  const zipDigits = data.zip?.replace(/\D/g, "") || "";
  if (!zipDigits || (zipDigits.length !== 5 && zipDigits.length !== 9)) {
    errors.push("Zip code must be 5 or 9 digits");
  }

  return {
    valid: errors.length === 0,
    errors,
  };
}

/**
 * Common headers for JSON responses with CORS
 */
const corsHeaders = {
  "Access-Control-Allow-Origin": "*", // Allow all origins for now (can restrict to tistheseasonkc.com later)
  "Access-Control-Allow-Methods": "POST, OPTIONS",
  "Access-Control-Allow-Headers": "Content-Type",
};

/**
 * Generate HTML error page with form pre-filled for no-JS users
 */
function generateErrorHTML(data: Partial<FormData>, errors: string[]): string {
  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Form Errors — Tis The Season Holiday Lighting</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      font-family: system-ui, -apple-system, sans-serif;
      background: #f5f5f0;
      padding: 2rem;
    }
    .container {
      max-width: 50rem;
      margin: 0 auto;
      background: white;
      padding: 2rem;
      border-radius: 0.5rem;
      box-shadow: 0 1px 3px rgba(0,0,0,0.08);
    }
    .error-banner {
      background: rgba(197, 48, 48, 0.1);
      border-left: 4px solid #c53030;
      padding: 1rem;
      margin-bottom: 2rem;
      border-radius: 0.25rem;
    }
    .error-banner h2 {
      color: #c53030;
      font-size: 1.1rem;
      margin-bottom: 0.5rem;
    }
    .error-banner ul {
      list-style: none;
      margin-left: 1rem;
    }
    .error-banner li:before {
      content: "✗ ";
      color: #c53030;
      font-weight: bold;
      margin-right: 0.5rem;
    }
    form {
      display: flex;
      flex-direction: column;
      gap: 1rem;
    }
    label {
      font-weight: 600;
      font-size: 0.9rem;
    }
    input, select, textarea {
      padding: 0.625rem 0.75rem;
      border: 1px solid #e0e0d8;
      border-radius: 0.25rem;
      font-size: 1rem;
      font-family: inherit;
    }
    input:focus, select:focus, textarea:focus {
      outline: 2px solid #69C4BB;
      outline-offset: 1px;
      border-color: #69C4BB;
    }
    textarea {
      resize: vertical;
      min-height: 6rem;
    }
    .form-row {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 1rem;
    }
    .form-row.address {
      grid-template-columns: 2fr 1fr 1fr;
    }
    button {
      background: #69C4BB;
      color: white;
      border: none;
      padding: 0.875rem 1.5rem;
      border-radius: 0.25rem;
      font-weight: 600;
      cursor: pointer;
      font-size: 1rem;
    }
    button:hover {
      background: #52a89e;
    }
    .back-link {
      display: block;
      margin-top: 1rem;
      color: #69C4BB;
      text-decoration: none;
    }
    .back-link:hover {
      text-decoration: underline;
    }
  </style>
</head>
<body>
  <div class="container">
    <div class="error-banner">
      <h2>Please Fix These Errors</h2>
      <ul>
        ${errors.map((err) => `<li>${escapeHtml(err)}</li>`).join("")}
      </ul>
    </div>

    <form method="POST" action="https://stetson-tts-contact-form.web.val.run/">
      <div class="form-row">
        <div>
          <label for="first-name">First Name</label>
          <input type="text" id="first-name" name="firstName" required value="${escapeHtml(data.firstName || "")}" />
        </div>
        <div>
          <label for="last-name">Last Name</label>
          <input type="text" id="last-name" name="lastName" required value="${escapeHtml(data.lastName || "")}" />
        </div>
      </div>

      <div class="form-row">
        <div>
          <label for="email">Email</label>
          <input type="email" id="email" name="email" required value="${escapeHtml(data.email || "")}" />
        </div>
        <div>
          <label for="phone">Phone Number</label>
          <input type="tel" id="phone" name="phone" required value="${escapeHtml(data.phone || "")}" />
        </div>
      </div>

      <div>
        <label for="street-address">Street Address</label>
        <input type="text" id="street-address" name="streetAddress" required value="${escapeHtml(data.streetAddress || "")}" />
      </div>

      <div class="form-row address">
        <div>
          <label for="city">City</label>
          <input type="text" id="city" name="city" required value="${escapeHtml(data.city || "")}" />
        </div>
        <div>
          <label for="state">State</label>
          <select id="state" name="state" required>
            <option value="">Select state...</option>
            <option value="MO" ${data.state === "MO" ? "selected" : ""}>MO</option>
            <option value="KS" ${data.state === "KS" ? "selected" : ""}>KS</option>
          </select>
        </div>
        <div>
          <label for="zip">Zip Code</label>
          <input type="text" id="zip" name="zip" required value="${escapeHtml(data.zip || "")}" />
        </div>
      </div>

      <div>
        <label for="message">Message</label>
        <textarea id="message" name="message">${escapeHtml(data.message || "")}</textarea>
      </div>

      <button type="submit">Submit Request</button>
      <a href="https://tistheseasonkc.com/contact/" class="back-link">← Back to Contact Form</a>
    </form>
  </div>
</body>
</html>`;
}

/**
 * Escape HTML special characters
 */
function escapeHtml(text: string): string {
  const map: Record<string, string> = {
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': "&quot;",
    "'": "&#039;",
  };
  return text.replace(/[&<>"']/g, (ch) => map[ch]);
}

const jsonHeaders = {
  "Content-Type": "application/json",
  ...corsHeaders,
};

/**
 * HTTP endpoint for form submissions
 */
export default async function (req: Request): Promise<Response> {
  // Handle CORS preflight
  if (req.method === "OPTIONS") {
    return new Response(null, {
      status: 204,
      headers: corsHeaders,
    });
  }

  // Only accept POST
  if (req.method !== "POST") {
    return new Response(
      JSON.stringify({ success: false, error: "Method not allowed" }),
      { status: 405, headers: jsonHeaders }
    );
  }

  // Parse form data (handle both JSON and form-urlencoded)
  let formData: Partial<FormData>;
  const contentType = req.headers.get("content-type") || "";

  if (contentType.includes("application/json")) {
    formData = await req.json() as Partial<FormData>;
  } else {
    // Parse form-urlencoded (no-JS fallback)
    const text = await req.text();
    const params = new URLSearchParams(text);
    // Trim whitespace from all values
    formData = Object.fromEntries(
      Array.from(params.entries()).map(([k, v]) => [k, v.trim()])
    ) as Partial<FormData>;
  }

  // Validate
  const { valid, errors } = validateFormData(formData);

  // Determine if this is a form submission (no-JS) or fetch (JS)
  const isFormSubmission = contentType.includes("application/x-www-form-urlencoded");

  if (!valid) {
    if (isFormSubmission) {
      // No-JS: return HTML with form + errors
      return new Response(
        generateErrorHTML(formData, errors),
        { status: 400, headers: { "Content-Type": "text/html; charset=utf-8" } }
      );
    } else {
      // JS fetch: return JSON
      return new Response(
        JSON.stringify({ success: false, errors }),
        { status: 400, headers: jsonHeaders }
      );
    }
  }

  // TODO: Store in database or send email
  console.log("Form submission:", formData);

  if (isFormSubmission) {
    // No-JS: redirect to thank-you page
    return new Response(null, {
      status: 303,
      headers: {
        Location: "https://tistheseasonkc.com/thank-you/",
        ...corsHeaders,
      },
    });
  } else {
    // JS fetch: return JSON
    return new Response(
      JSON.stringify({ success: true, message: "Form submitted successfully" }),
      { status: 200, headers: jsonHeaders }
    );
  }
}
