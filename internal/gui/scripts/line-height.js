(() => {
  if (window.frameElement?.id === 'deck-native-style-sampler') return;
  const key = '__deckCodexLineHeight';
  const percent = __DECK_LINE_HEIGHT_PERCENT__;
  if (window[key]?.version === 2 && window[key].percent === percent) return;
  window[key]?.dispose();
  if (!Number.isSafeInteger(percent) || percent <= 0) return;
  const marker = 'data-deck-line-height';
  const sidebarMarker = 'data-deck-sidebar-spacing';
  const roots = new Map();
  const marked = new Set();
  const sidebarMarked = new Set();
  const heights = new Set();
  const sidebarRules = new Map();
  const normalCache = new Map();
  let disposed = false;
  let caretFrame = null;
  const activeEditor = () => {
    let element = document.activeElement;
    while (element?.shadowRoot?.activeElement) element = element.shadowRoot.activeElement;
    return element?.closest('textarea, input, [contenteditable="true"], [contenteditable="plaintext-only"]');
  };
  // 聚焦时移除输入区的渐隐，保留其他滚动区域的效果。
  const editorCSS = '.vertical-scroll-fade-mask:focus-within { mask-image:none !important; -webkit-mask-image:none !important; }';
  const keepCaretVisible = () => {
    if (disposed || caretFrame !== null) return;
    caretFrame = requestAnimationFrame(() => {
      caretFrame = null;
      const editor = activeEditor();
      if (!editor?.isContentEditable) return;
      const selection = editor.getRootNode().getSelection?.() || window.getSelection();
      if (!selection?.isCollapsed || !selection.rangeCount || !editor.contains(selection.focusNode)) return;
      const range = selection.getRangeAt(0).cloneRange();
      let rect = range.getClientRects()[0];
      // 空段落或元素边界上的折叠选区可能没有矩形，使用光标相邻节点。
      if (!rect?.height && selection.focusNode instanceof Element) {
        const container = selection.focusNode;
        const offset = selection.focusOffset;
        const adjacent = container.childNodes[offset ? offset - 1 : 0];
        if (adjacent?.nodeType === Node.TEXT_NODE && adjacent.length) {
          range.setStart(adjacent, offset ? adjacent.length - 1 : 0);
          range.setEnd(adjacent, offset ? adjacent.length : 1);
          rect = [...range.getClientRects()].at(offset ? -1 : 0);
        } else {
          rect = (adjacent instanceof Element ? adjacent : container).getBoundingClientRect();
        }
      }
      if (!rect || !rect.height) return;
      let top = rect.top, bottom = rect.bottom;
      for (let parent = editor; parent && parent !== document.body; parent = parent.parentElement) {
        if (!/(auto|scroll)/.test(getComputedStyle(parent).overflowY) || parent.scrollHeight <= parent.clientHeight) continue;
        const box = parent.getBoundingClientRect();
        const scale = parent.offsetHeight ? box.height / parent.offsetHeight : 1;
        const start = box.top + parent.clientTop * scale;
        const end = start + parent.clientHeight * scale;
        const delta = bottom > end - 2 ? bottom - end + 2 : top < start + 2 ? top - start - 2 : 0;
        const previous = parent.scrollTop;
        parent.scrollTop += delta / scale;
        const moved = (parent.scrollTop - previous) * scale;
        top -= moved;
        bottom -= moved;
      }
    });
  };

  // 列表行有独立高度，仅调整文字行高不会改变条目密度。
  // 只处理侧栏内容的纵向尺寸，保留面板、滚动容器和导航图标栏的布局。
  const sidebarSpacing = (element, computed) => {
    const panel = element.closest('.app-shell-left-panel');
    if (!panel || element === panel || element.closest('[data-app-navigation-rail]')) return '';
    const declarations = [];
    const scale = property => {
      const value = computed.getPropertyValue(property);
      if (!/^-?[\d.]+px$/.test(value)) return;
      declarations.push(`${property}: ${Number.parseFloat(value) * percent / 100}px !important;`);
    };
    for (const property of ['padding-top', 'padding-bottom', 'margin-top', 'margin-bottom', 'row-gap']) scale(property);
    if (element.matches('.sidebar-item, [class~="group/section-toggle"]')) {
      for (const property of ['height', 'min-height', 'max-height']) scale(property);
      const height = Number.parseFloat(computed.height);
      if (Number.isFinite(height)) {
        declarations.push(`contain-intrinsic-block-size: auto ${height * percent / 100}px !important;`);
      }
    }
    return declarations.join(' ');
  };


  // normal 没有可直接读取的像素值，使用隔离的单行文本测量字体行盒。
  const probeHost = document.createElement('div');
  probeHost.setAttribute('data-deck-probe', '');
  probeHost.style.cssText = 'position:fixed!important;left:-10000px!important;top:0!important;visibility:hidden!important;pointer-events:none!important;';
  const probeRoot = probeHost.attachShadow({ mode: 'closed' });
  const probe = document.createElement('div');
  probe.textContent = 'Mg中文';
  probeRoot.append(probe);
  const normalHeight = (style, cache) => {
    const properties = ['font-family', 'font-size', 'font-weight', 'font-style', 'font-stretch', 'font-feature-settings', 'font-variation-settings', 'writing-mode'];
    const values = properties.map(name => style.getPropertyValue(name));
    const key = values.join('|');
    if (!cache.has(key)) {
      probe.style.cssText = 'all:initial;display:block;position:absolute;white-space:pre;line-height:normal;width:max-content;height:auto;';
      properties.forEach((name, index) => probe.style.setProperty(name, values[index]));
      const rect = probe.getBoundingClientRect();
      cache.set(key, style.writingMode.startsWith('vertical') ? rect.width : rect.height);
    }
    return cache.get(key);
  };


  const ensureStyle = root => {
    if (roots.get(root)?.isConnected) return;
    const style = document.createElement('style');
    style.id = 'deck-line-height';
    (root === document ? document.head || document.documentElement : root).append(style);
    roots.set(root, style);
  };
  const writeStyles = () => {
    const css = editorCSS + '\n' + [...heights].map(height =>
      '[data-deck-line-height="' + height + '"] { line-height:' + Number(height) * percent / 100 + 'px !important; }'
    ).concat([...sidebarRules].map(([declarations, group]) =>
      '[data-deck-sidebar-spacing="' + group + '"] { ' + declarations + ' }'
    )).join('\n');
    for (const style of roots.values()) if (style.textContent !== css) style.textContent = css;
  };
  const cleanup = () => {
    for (const element of marked) if (!element.isConnected) { element.removeAttribute(marker); marked.delete(element); }
    for (const element of sidebarMarked) if (!element.isConnected) { element.removeAttribute(sidebarMarker); sidebarMarked.delete(element); }
    for (const [root, style] of roots) if (root !== document && !root.host.isConnected) { style.remove(); roots.delete(root); }
  };
  ensureStyle(document);
  let watcher = null;
  if (percent !== 100) watcher = window.__deckNativeStyles.watch({
    relevantProperty: name => /^(font|line-height|padding|margin|gap|row-gap|height|min-height|max-height|writing-mode|display)/.test(name) ||
      /^--.*(?:font|line-height|leading|spacing|padding|margin|gap|height|text-(?:xs|sm|base|lg|xl|[2-9]xl|heading))/.test(name),
    cleanup,
    measure(elements, nativeStyle, stats, full) {
      if (full) normalCache.clear();
      // 原生样式在隔离镜像里读取，真实页面的覆盖始终保持启用。
      document.documentElement.append(probeHost);
      const measurements = [];
      try {
        for (const element of elements) {
          const computed = nativeStyle(element);
          if (!computed) continue;
          ensureStyle(element.getRootNode());
          const height = computed.lineHeight === 'normal' ? normalHeight(computed, normalCache) : Number.parseFloat(computed.lineHeight);
          if (!Number.isFinite(height) || height < 0) continue;
          measurements.push([element, String(height), sidebarSpacing(element, computed)]);
        }
      } finally { probeHost.remove(); }
      for (const [element, baseline, spacing] of measurements) {
        heights.add(baseline);
        if (element.getAttribute(marker) !== baseline) { element.setAttribute(marker, baseline); stats.writes++; }
        marked.add(element);
        if (spacing) {
          if (!sidebarRules.has(spacing)) sidebarRules.set(spacing, String(sidebarRules.size));
          const group = sidebarRules.get(spacing);
          if (element.getAttribute(sidebarMarker) !== group) { element.setAttribute(sidebarMarker, group); stats.writes++; }
          sidebarMarked.add(element);
        } else if (sidebarMarked.delete(element)) { element.removeAttribute(sidebarMarker); stats.writes++; }
      }
      writeStyles();
      keepCaretVisible();
    },
  });
  writeStyles();
  window[key] = { version: 2, percent, stats: watcher?.stats, dispose() {
    disposed = true;
    watcher?.dispose();
    cancelAnimationFrame(caretFrame);
    document.removeEventListener('input', keepCaretVisible, true);
    document.removeEventListener('selectionchange', keepCaretVisible);
    for (const style of roots.values()) style.remove();
    for (const element of marked) element.removeAttribute(marker);
    for (const element of sidebarMarked) element.removeAttribute(sidebarMarker);
    probeHost.remove();
    delete window[key];
  } };
  document.addEventListener('input', keepCaretVisible, true);
  document.addEventListener('selectionchange', keepCaretVisible);
})();
