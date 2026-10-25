(() => {
  const config = __DECK_STYLE_CONFIG__;
  const key = '__deckApplicationColors';
  window[key]?.dispose();
  const marker = 'data-deck-color';
  const marked = new Set();
  const style = document.createElement('style');
  style.id = 'deck-application-colors';
  const transitionGuard = document.createElement('style');
  transitionGuard.id = 'deck-color-measurement';
  transitionGuard.textContent = ':root, body, body * { transition: none !important; }';
  const backgrounds = {
    codex: ['--color-token-bg-primary', '--color-token-main-surface-primary', '--color-token-side-bar-background', '--color-background-surface', '--app-color-background-surface', '--color-surface-primary', '--color-background-execution-output'],
    dsh: ['--dsw-alias-bg-base', '--dsw-specific-sidebar-fill', '--bt-main', '--bt-sidebar'],
    zcode: ['--color-background', '--color-background-primary', '--color-sidebar'],
    droid: ['--surface-1', '--sidebar-bg', '--core-v2-bg-page', '--core-v2-bg-surface', '--initial-loader-background'],
  };
  const foregrounds = {
    codex: ['--color-token-text-primary', '--color-text-primary', '--color-text', '--color-text-primary-ghost', '--color-text-primary-surface', '--user-message-text-color'],
    dsh: ['--dsw-alias-label-primary', '--bt-ink'],
    zcode: ['--color-foreground', '--color-text-primary'],
    droid: ['--text-default', '--text-heading', '--text-label', '--core-v2-text-primary', '--core-v2-appearance-user-message-text', '--sidebar-active-text', '--sidebar-text-hover'],
  };
  const tokens = [
    ...(backgrounds[config.app] || []).map(name => `${name}:${config.background} !important;`),
    ...(foregrounds[config.app] || []).map(name => `${name}:${config.foreground} !important;`),
  ].join('');
  let timer = null;
  let stopped = false;
  let pending = false;
  const editing = () => {
    let element = document.activeElement;
    while (element?.shadowRoot?.activeElement) element = element.shadowRoot.activeElement;
    return !!element?.closest('textarea, input, [contenteditable="true"], [contenteditable="plaintext-only"]');
  };
  const schedule = () => {
    if (stopped) return;
    pending = true;
    if (!editing() && timer === null) timer = setTimeout(refresh, 300);
  };
  const onFocusOut = () => { if (pending && timer === null) timer = setTimeout(refresh, 0); };
  const ownNode = node => node instanceof Element && (node.id.startsWith('deck-') || node.hasAttribute('data-deck-probe'));
  const styleSignature = value => {
    const style = document.createElement('span').style;
    style.cssText = value || '';
    return [...style].filter(name => /^(--|color|background)/.test(name)).sort()
      .map(name => `${name}:${style.getPropertyValue(name)}!${style.getPropertyPriority(name)}`).join(';');
  };
  const observer = new MutationObserver(records => {
    if (records.some(record => !ownNode(record.target) &&
      !(record.type === 'attributes' && record.attributeName === 'style' &&
        styleSignature(record.oldValue) === styleSignature(record.target.getAttribute('style'))) && (record.type !== 'childList' ||
      [...record.addedNodes, ...record.removedNodes].some(node => node instanceof Element && !ownNode(node))))) schedule();
  });
  function refresh() {
    timer = null;
    if (stopped || !document.body) return;
    if (editing() && style.isConnected) { pending = true; return; }
    pending = false;
    observer.disconnect();
    (document.head || document.documentElement).append(transitionGuard);
    style.disabled = true;
    try {
      const body = getComputedStyle(document.body);
      const root = getComputedStyle(document.documentElement);
      const primaryText = new Set([body.color, root.color]);
      const primaryBackground = new Set([body.backgroundColor, root.backgroundColor]);
      primaryBackground.delete('rgba(0, 0, 0, 0)');
      const matches = new Map();
      // 同色的显式声明也一起处理，语义色、链接和语法高亮保留各自的颜色。
      for (const element of document.querySelectorAll('body, body *')) {
        if (!(element instanceof HTMLElement) || element.closest('.xterm') || ownNode(element)) continue;
        const computed = getComputedStyle(element);
        let value = primaryText.has(computed.color) ? 'f' : '';
        if (primaryBackground.has(computed.backgroundColor) && computed.backgroundImage === 'none') value += 'b';
        if (value) matches.set(element, value);
      }
      for (const element of marked) {
        if (!matches.has(element)) { element.removeAttribute(marker); marked.delete(element); }
      }
      for (const [element, value] of matches) {
        if (element.getAttribute(marker) !== value) element.setAttribute(marker, value);
        marked.add(element);
      }
      style.textContent = `
        :root, body, body *:not(.xterm, .xterm *) { ${tokens} }
        :root, body { background-color:${config.background} !important; color:${config.foreground} !important; }
        [${marker}*="f"] { color:${config.foreground} !important; }
        [${marker}*="b"] { background-color:${config.background} !important; }
      `;
      if (!style.isConnected) (document.head || document.documentElement).append(style);
    } finally {
      style.disabled = false;
      // 先提交最终配色，再恢复原生过渡，避免采样时被颜色动画的中间值干扰。
      getComputedStyle(document.body).color;
      transitionGuard.remove();
      observer.observe(document.documentElement, { childList: true, subtree: true, attributes: true, attributeOldValue: true,
        attributeFilter: ['class', 'style', 'data-theme', 'data-ds-dark-theme'] });
    }
  }
  window[key] = { dispose() {
    stopped = true;
    clearTimeout(timer);
    observer.disconnect();
    document.removeEventListener('DOMContentLoaded', schedule);
    document.removeEventListener('load', schedule, true);
    document.removeEventListener('focusout', onFocusOut, true);
    style.remove();
    transitionGuard.remove();
    for (const element of marked) element.removeAttribute(marker);
    delete window[key];
  } };
  document.addEventListener('DOMContentLoaded', schedule);
  document.addEventListener('load', schedule, true);
  document.addEventListener('focusout', onFocusOut, true);
  refresh();
})();
