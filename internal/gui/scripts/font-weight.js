(() => {
  const key = '__deckFontWeights';
  const normal = __DECK_FONT_WEIGHT__;
  const bold = __DECK_BOLD_FONT_WEIGHT__;
  if (window[key]?.normal === normal && window[key]?.bold === bold) return;
  window[key]?.dispose();
  const marker = 'data-deck-font-weight';
  const roots = new Map();
  const ensureStyle = root => {
    if (roots.get(root)?.isConnected) return;
    const style = document.createElement('style');
    style.id = 'deck-font-weights';
    style.textContent = ['normal', 'bold'].map(kind =>
      `[${marker}="${kind}"][${marker}] { font-weight: ${kind === 'bold' ? bold : normal} !important; }`
    ).join('\n');
    (root === document ? document.head || document.documentElement : root).append(style);
    roots.set(root, style);
  };
  const cleanup = () => {
    for (const [root, style] of roots) {
      if (root !== document && !root.host.isConnected) { style.remove(); roots.delete(root); }
    }
  };
  const watcher = window.__deckNativeStyles.watch({
    relevantProperty: property => /font|weight/.test(property),
    deferWhileEditing: false,
    delay: 50,
    cleanup,
    measure(elements, computedStyle) {
      for (const element of elements) {
        if (element.closest('.xterm')) continue;
        const computed = computedStyle(element);
        if (!computed) continue;
        // 从未覆盖字重的镜像采样，保留标题、CSS 粗体和子元素继承关系。
        const weight = computed.fontWeight;
        const kind = weight === 'bold' || Number.parseFloat(weight) >= 600 ? 'bold' : 'normal';
        ensureStyle(element.getRootNode());
        if (element.getAttribute(marker) !== kind) element.setAttribute(marker, kind);
      }
    },
  });
  window[key] = { normal, bold, dispose() {
    watcher.dispose();
    for (const [root, style] of roots) {
      if (root instanceof Document && root.documentElement.hasAttribute(marker)) root.documentElement.removeAttribute(marker);
      root.querySelectorAll(`[${marker}]`).forEach(element => element.removeAttribute(marker));
      style.remove();
    }
    roots.clear();
  } };
})();
