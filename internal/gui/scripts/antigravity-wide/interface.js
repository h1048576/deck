(() => {
  const id = 'antigravity-wide-ui-override';
  const font = __DECK_ANTIGRAVITY_FONT__;
  const size = __DECK_ANTIGRAVITY_FONT_SIZE__;
  const width = '__DECK_ANTIGRAVITY_WIDTH__';
  const sidebar = '__DECK_ANTIGRAVITY_SIDEBAR__';
  const openSplit = '[data-testid="sidebar-split"]:has(> [data-split-sash] [data-testid="sidebar-sash"])';
  const css = `
    :root {
      --vscode-font-family: ${font} !important;
      --editor-font-family: ${font} !important;
      --font-sans: ${font} !important;
      --font-mono: ${font} !important;
      --default-font-family: ${font} !important;
      --default-mono-font-family: ${font} !important;
      --app-font-scale: ${size / 14} !important;
      --font-size: ${size}px !important;
      --vscode-font-size: ${size}px !important;
      --vscode-editor-font-size: ${size}px !important;
      --editor-font-size: ${size}px !important;
      --max-conversation-width: ${width} !important;
    }
    :is(#antigravity-wide-ui-override#antigravity-wide-ui-override, :root, :root *:not(.xterm, .xterm *, .katex *)),
    :is(#antigravity-wide-ui-override#antigravity-wide-ui-override, :root *:not(.xterm, .xterm *, .katex *))::placeholder {
      font-size: ${size}px !important;
    }
    :is(#antigravity-wide-ui-override#antigravity-wide-ui-override, :root, :root *:not(.xterm, .xterm *, .codicon, [class*="codicon-"], .katex, .katex *)),
    :is(#antigravity-wide-ui-override#antigravity-wide-ui-override, :root *:not(.xterm, .xterm *, .codicon, [class*="codicon-"], .katex, .katex *))::placeholder {
      font-family: ${font} !important;
    }
    /* 去掉原生 max(30vw, ...) 的下限，仍受各会话列可用空间限制。 */
    #root [style*="max-width:"][style*="--max-conversation-width"] {
      max-width: min(100%, ${width}) !important;
    }
    ${openSplit} > :first-child {
      width: min(${sidebar}, 60%) !important;
    }
    ${openSplit} > :first-child > [data-split-sized] {
      width: 100% !important;
    }
    :root:has(${openSplit}) :is(.agy-layout-vars, .agy-portaled-layout-vars) {
      --sidebar-width: min(${sidebar}, 60vw) !important;
    }
    [data-testid="terminal-active"] > [data-deck-terminal-remeasure] {
      width: calc(100% - 1px) !important;
    }
    [data-deck-antigravity-hidden-model] { display: none !important; }
  `;
  let style = document.getElementById(id);
  if (!style) {
    style = document.createElement('style');
    style.id = id;
    (document.head || document.documentElement).append(style);
  }
  if (style.textContent !== css) style.textContent = css;

  const key = '__deckAntigravityModelFilter';
  window[key]?.disconnect();
  const marker = 'data-deck-antigravity-hidden-model';
  for (const row of document.querySelectorAll(`[${marker}]`)) row.removeAttribute(marker);
  const patterns = [];
  for (const pattern of __DECK_HIDDEN_MODEL_PATTERNS__ || []) {
    try { patterns.push(new RegExp(pattern, 'i')); }
    catch { /* 配置中的无效正则不影响其他规则。 */ }
  }
  if (!patterns.length) {
    delete window[key];
    return true;
  }

  const selector = '[data-testid="model-selector-panel"] :is([data-model-label], [data-model-base])';
  const modelIdentity = label => {
    const name = label.dataset.modelLabel || label.dataset.modelBase;
    const fiberKey = Object.keys(label).find(item => item.startsWith('__reactFiber$'));
    for (let fiber = label[fiberKey]; fiber; fiber = fiber.return) {
      const model = fiber.memoizedProps?.model;
      if (model?.label === name && ['string', 'number'].includes(typeof model.value)) {
        return [name, String(model.value)];
      }
    }
    return [name];
  };
  const scan = () => {
    const hidden = new Set();
    for (const label of document.querySelectorAll(selector)) {
      const row = label.closest('button, [role^="menuitem"], [role="option"]') || label;
      if (patterns.some(pattern => modelIdentity(label).some(value => pattern.test(value)))) hidden.add(row);
    }
    for (const row of document.querySelectorAll(`[${marker}]`)) {
      if (!hidden.has(row)) row.removeAttribute(marker);
    }
    for (const row of hidden) row.setAttribute(marker, '');
  };
  const observer = new MutationObserver(records => {
    const includesMenu = node => node instanceof Element &&
      (node.matches('[data-testid="model-selector-panel"]') || node.querySelector('[data-testid="model-selector-panel"]'));
    if (records.some(record => record.target.closest?.('[data-testid="model-selector-panel"]') ||
        [...record.addedNodes].some(includesMenu))) scan();
  });
  observer.observe(document.documentElement, {
    childList: true, subtree: true, attributes: true, attributeFilter: ['data-model-label', 'data-model-base'],
  });
  window[key] = observer;
  scan();
  return true;
})()
