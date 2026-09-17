const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

const attributionScript = fs.readFileSync(path.join(__dirname, "attribution.js"), "utf8");
const formSubmitScript = fs.readFileSync(path.join(__dirname, "form-submit.js"), "utf8");

function storage(mode, values) {
  const data = new Map(Object.entries(values || {}));
  return {
    getItem(key) {
      if (mode === "denied" || mode === "read-failure") throw new Error("storage unavailable");
      return data.get(key) || null;
    },
    setItem(key, value) {
      if (mode === "denied" || mode === "write-failure") throw new Error("storage unavailable");
      if (mode !== "non-persisting") data.set(key, value);
    },
    data
  };
}

function loadAttribution(options) {
  const hidden = { value: "" };
  const listeners = [];
  const form = {
    addEventListener(type, listener, capture) { listeners.push({ type, listener, capture }); },
    querySelectorAll() { return []; },
    querySelector() { return null; },
    closest() { return null; }
  };
  const document = {
    cookie: options.cookie || "",
    querySelector(selector) { return selector === ".contact-form__card" ? form : null; },
    querySelectorAll(selector) {
      if (selector === "input[name='_attribution']") return [hidden];
      if (selector === "form.contact-form__card") return [form];
      return [];
    }
  };
  const context = vm.createContext({
    Date,
    JSON,
    Object,
    URLSearchParams,
    console: { error() {} },
    document,
    sessionStorage: options.storage,
    window: { location: { search: options.search || "", pathname: options.pathname || "/free-estimate/" } }
  });
  vm.runInContext(attributionScript, context, { filename: "attribution.js" });
  return { context, form, hidden, listeners };
}

const campaign = "?utm_source=facebook&utm_medium=paid_social&utm_campaign=fall&fbclid=" + "f".repeat(214);

test("current-page attribution survives unavailable and non-persisting session storage", () => {
  for (const mode of ["normal", "denied", "read-failure", "write-failure", "non-persisting"]) {
    const page = loadAttribution({ search: campaign, storage: storage(mode) });
    const snapshot = page.context.window.ttsAttribution.snapshot();
    assert.equal(snapshot.utm_source, "facebook", mode);
    assert.equal(snapshot.utm_medium, "paid_social", mode);
    assert.equal(snapshot.utm_campaign, "fall", mode);
    assert.equal(snapshot.fbclid, "f".repeat(214), mode);
    assert.equal(JSON.parse(page.hidden.value).fbclid, "f".repeat(214), mode);
  }
});

test("working session storage carries attribution to an untagged contact page", () => {
  const shared = storage("normal");
  loadAttribution({ search: campaign, storage: shared });
  const contact = loadAttribution({ pathname: "/contact/", storage: shared });
  const snapshot = contact.context.window.ttsAttribution.snapshot();
  assert.equal(snapshot.utm_source, "facebook");
  assert.equal(snapshot.fbclid, "f".repeat(214));
  assert.equal(snapshot.page, "/contact/");
});

test("a new campaign replaces old Meta attribution instead of merging it", () => {
  const shared = storage("normal", {
    tts_ad_attribution: JSON.stringify({ fbclid: "old-meta-click", fbc: "fb.1.1.old", utm_source: "facebook" })
  });
  const page = loadAttribution({ search: "?gclid=" + "g".repeat(214) + "&utm_source=google", storage: shared });
  const snapshot = page.context.window.ttsAttribution.snapshot();
  assert.deepEqual(JSON.parse(JSON.stringify(snapshot)), {
    gclid: "g".repeat(214),
    utm_source: "google",
    landing_page: "/free-estimate/",
    page: "/free-estimate/"
  });
});

test("complete Meta cookies and click IDs reach the enhanced form fetch payload", async () => {
  const fbc = "fb.1.1700000000000." + "c".repeat(214);
  const fbp = "fb.1.1700000000000." + "p".repeat(214);
  const page = loadAttribution({
    search: "?fbclid=" + "f".repeat(214),
    cookie: "_fbc=" + encodeURIComponent(fbc) + "; _fbp=" + encodeURIComponent(fbp),
    storage: storage("write-failure")
  });
  let payload;
  page.form.querySelector = (selector) => selector === ".contact-form__submit" ? { disabled: false, textContent: "Send" } : null;
  page.context.FormData = class FormData {
    *[Symbol.iterator]() { yield ["_attribution", page.hidden.value]; }
  };
  page.context.fetch = async (_url, options) => {
    payload = JSON.parse(options.body);
    return { ok: false, json: async () => ({}) };
  };
  vm.runInContext(formSubmitScript, page.context, { filename: "form-submit.js" });
  const submit = page.listeners.find((entry) => entry.type === "submit" && !entry.capture).listener;
  await submit({ preventDefault() {} });

  const attribution = JSON.parse(payload._attribution);
  assert.equal(attribution.fbclid, "f".repeat(214));
  assert.equal(attribution.fbc, fbc);
  assert.equal(attribution.fbp, fbp);
});
