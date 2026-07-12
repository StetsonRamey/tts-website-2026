/**
 * Theme toggle — flips data-theme on <html> and persists to localStorage.
 * The no-flash <head> script sets the initial attribute before paint; this
 * just wires the button + keeps the OS preference in sync when no manual
 * choice has been made.
 */
(function () {
  "use strict";

  const root = document.documentElement;
  const btn = document.querySelector(".theme-toggle");
  if (!btn) return;

  const stored = () => localStorage.getItem("theme");
  const isDark = () =>
    root.getAttribute("data-theme") === "dark" ||
    (!stored() && window.matchMedia("(prefers-color-scheme: dark)").matches);

  // Keep aria-pressed in sync with current state.
  const sync = () => btn.setAttribute("aria-pressed", String(isDark()));

  sync();

  btn.addEventListener("click", () => {
    const next = isDark() ? "light" : "dark";
    root.setAttribute("data-theme", next);
    try { localStorage.setItem("theme", next); } catch (e) {}
    sync();
  });

  // If the user hasn't chosen manually, follow OS changes live.
  window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", (e) => {
    if (stored()) return;
    root.setAttribute("data-theme", e.matches ? "dark" : "light");
    sync();
  });
})();
