(function () {
  'use strict';

  var overlay, content, scrollSvg, heroHeight;
  var ticking = false;

  function lerp(a, b, t) {
    return a + (b - a) * t;
  }

  function update() {
    var progress = Math.min(window.scrollY / (heroHeight * 0.25), 1);

    var r = Math.round(lerp(255, 45, progress));
    var g = Math.round(lerp(255, 45, progress));
    var b = Math.round(lerp(255, 45, progress));
    var a = lerp(0.6, 0.55, progress);
    overlay.style.background = 'rgba(' + r + ',' + g + ',' + b + ',' + a.toFixed(3) + ')';

    var tc = Math.round(lerp(0, 255, progress));
    var textColor = 'rgb(' + tc + ',' + tc + ',' + tc + ')';
    content.style.color = textColor;
    scrollSvg.style.stroke = textColor;

    ticking = false;
  }

  function onScroll() {
    if (!ticking) {
      requestAnimationFrame(update);
      ticking = true;
    }
  }

  function init() {
    var hero = document.querySelector('.hero');
    if (!hero) return;

    overlay = hero.querySelector('.hero__overlay');
    content = hero.querySelector('.hero__content');
    scrollSvg = hero.querySelector('.hero__scroll svg');
    heroHeight = hero.offsetHeight;

    window.addEventListener('scroll', onScroll, { passive: true });
    window.addEventListener('resize', function () {
      heroHeight = hero.offsetHeight;
    });
  }

  document.addEventListener('DOMContentLoaded', init);
})();
