(() => {
  if (window.__deckT3ThreadDetails) return true;
  const toggleSelector = '[data-panel-layout-controls] button[aria-label="Toggle thread details panel"]';
  const panelSelector = '[data-thread-details-panel="inline"]';
  const handled = new Set();
  let clickedRoute;

  // 用户先打开弹窗或内联面板时，保留这次主动操作。
  document.addEventListener('click', event => {
    if (event.isTrusted && event.target instanceof Element && event.target.closest(toggleSelector)) {
      clickedRoute = location.href;
    }
  }, true);

  const collapseDefault = () => {
    const panel = document.querySelector(panelSelector);
    if (!panel) return;
    const thread = panel.getAttribute('data-workspace-focus-key') || location.href;
    if (handled.has(thread)) return;
    const toggle = document.querySelector(toggleSelector);
    if (!toggle || toggle.disabled || toggle.getAttribute('aria-pressed') !== 'true') return;
    handled.add(thread);
    if (clickedRoute !== location.href) toggle.click();
  };

  const observer = new MutationObserver(collapseDefault);
  observer.observe(document.documentElement, {
    childList: true, subtree: true, attributes: true,
    attributeFilter: ['aria-pressed', 'data-workspace-focus-key'],
  });
  window.__deckT3ThreadDetails = observer;
  collapseDefault();
  return true;
})()
