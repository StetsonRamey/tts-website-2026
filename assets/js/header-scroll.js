/**
 * Header scroll state — toggles a class on <html> so CSS can switch the
 * site header from a solid background (at the top of the page) to the
 * fade-to-transparent gradient used once the header is pinned/scrolled.
 * This prevents the fade zone from revealing a contrasting hero
 * background color when the page is scrolled all the way to the top.
 */
(function () {
  var header = document.querySelector('.site-header');
  if (!header) return;

  var ticking = false;

  function update() {
    ticking = false;
    var scrolled = window.scrollY > 1;
    header.classList.toggle('is-scrolled', scrolled);
  }

  function onScroll() {
    if (ticking) return;
    ticking = true;
    requestAnimationFrame(update);
  }

  window.addEventListener('scroll', onScroll, { passive: true });
  update();
})();
