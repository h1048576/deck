(() => {
  const key = '__deckCodexContentWidthGuard';
  window[key]?.dispose();
  const selector = '[data-app-shell-main-content-layout], [data-app-shell-focus-area="main"], [data-app-shell-focus-area="right-panel"]';
  const property = '--deck-codex-pane-width';
  const requestedProperty = '--deck-codex-requested-width-px';
  const panes = new Set();
  let frame = 0;

  const effectiveZoom = (element) => {
    if (element.currentCSSZoom > 0) return element.currentCSSZoom;
    let zoom = 1;
    for (let current = element; current; current = current.parentElement) {
      zoom *= Number.parseFloat(getComputedStyle(current).zoom) || 1;
    }
    return zoom;
  };

  const update = (pane) => {
    const style = getComputedStyle(pane);
    const padding = (Number.parseFloat(style.paddingLeft) || 0) + (Number.parseFloat(style.paddingRight) || 0);
    const width = Math.max(0, pane.clientWidth - padding);
    const value = `${width}px`;
    if (pane.style.getPropertyValue(property) !== value) pane.style.setProperty(property, value);
    const requested = style.getPropertyValue('--deck-codex-requested-width').trim();
    const match = requested.match(/^([0-9]+(?:\.[0-9]+)?)(px|rem|em|vw|vh|%)$/);
    if (match) {
      // Codex 在应用容器上使用 CSS zoom，视口单位需先换算为容器中的像素。
      const zoom = effectiveZoom(pane);
      const scales = {
        px: 1,
        rem: Number.parseFloat(getComputedStyle(document.documentElement).fontSize),
        em: Number.parseFloat(style.fontSize),
        vw: window.innerWidth / (100 * zoom),
        vh: window.innerHeight / (100 * zoom),
        '%': width / 100,
      };
      const pixels = `${Number(match[1]) * scales[match[2]]}px`;
      if (pane.style.getPropertyValue(requestedProperty) !== pixels) pane.style.setProperty(requestedProperty, pixels);
    }
  };
  const resizeObserver = new ResizeObserver((entries) => {
    entries.forEach(({ target }) => update(target));
  });
  const scan = () => {
    frame = 0;
    const current = new Set(document.querySelectorAll(selector));
    for (const pane of panes) {
      if (!current.has(pane)) {
        resizeObserver.unobserve(pane);
        pane.style.removeProperty(property);
        pane.style.removeProperty(requestedProperty);
        panes.delete(pane);
      }
    }
    for (const pane of current) {
      if (!panes.has(pane)) {
        panes.add(pane);
        resizeObserver.observe(pane);
      }
      update(pane);
    }
  };
  const schedule = () => {
    if (!frame) frame = requestAnimationFrame(scan);
  };
  const relevant = (node) => node instanceof Element && (node.matches(selector) || !!node.querySelector(selector));
  const observer = new MutationObserver((records) => {
    if (records.some((record) => record.type === 'attributes'
      ? relevant(record.target)
      : [...record.addedNodes, ...record.removedNodes].some(relevant))) schedule();
  });
  observer.observe(document.documentElement, {
    childList: true,
    subtree: true,
    attributes: true,
    attributeFilter: ['class', 'style', 'data-app-shell-focus-area', 'data-app-shell-main-content-layout'],
  });
  window.addEventListener('resize', schedule);
  window[key] = {
    dispose: () => {
      observer.disconnect();
      resizeObserver.disconnect();
      cancelAnimationFrame(frame);
      window.removeEventListener('resize', schedule);
      panes.forEach((pane) => {
        pane.style.removeProperty(property);
        pane.style.removeProperty(requestedProperty);
      });
    },
  };
  scan();
  return true;
})();
