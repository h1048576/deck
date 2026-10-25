(() => {
  // 所有受管应用共用原生行高采样；保留旧状态键以便清理已注入版本。
  const key = '__deckCodexLineHeight';
  const percent = __DECK_LINE_HEIGHT_PERCENT__;
  window[key]?.dispose();
  if (!Number.isSafeInteger(percent) || percent <= 0) return;

  const marker = 'data-deck-line-height';
  const sidebarMarker = 'data-deck-sidebar-spacing';
  const roots = new Map();
  const marked = new Set();
  const sidebarMarked = new Set();
  let timer = null;
  let disposed = false;
  let pending = false;
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
  const schedule = () => {
    if (disposed || percent === 100) return;
    pending = true;
    // 编辑期间保持现有覆盖，新段落继承当前行高；失焦后统一补采样。
    if (!activeEditor() && timer === null) timer = setTimeout(refresh, 300);
  };
  const ownNode = node => node instanceof Element && (node.id.startsWith('deck-') || node.hasAttribute('data-deck-probe'));
  const styleSignature = value => {
    const style = document.createElement('span').style;
    style.cssText = value || '';
    return [...style].filter(name => /^(--|font|line-height|padding|margin|gap|row-gap|height|min-height|max-height|writing-mode|display)/.test(name))
      .sort().map(name => `${name}:${style.getPropertyValue(name)}!${style.getPropertyPriority(name)}`).join(';');
  };
  const observer = new MutationObserver(records => {
    if (records.some(record => !ownNode(record.target) &&
      !(record.type === 'attributes' && record.attributeName.startsWith('data-deck-')) &&
      !(record.type === 'attributes' && record.attributeName === 'style' &&
        styleSignature(record.oldValue) === styleSignature(record.target.getAttribute('style'))) &&
      (record.type !== 'childList' || [...record.addedNodes, ...record.removedNodes].some(node => node instanceof Element && !ownNode(node))))) schedule();
  });
  const onFocusOut = () => { if (pending && timer === null) timer = setTimeout(refresh, 0); };

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

  function refresh() {
    timer = null;
    if (disposed || !document.documentElement) return;
    if (activeEditor() && roots.size) { pending = true; return; }
    pending = false;
    observer.disconnect();
    // 同一批读取前关闭所有覆盖，防止继承和多次调整产生累计缩放。
    for (const style of roots.values()) style.disabled = true;
    try {
      document.documentElement.append(probeHost);
      const measurements = [];
      const heights = new Set();
      const sidebarMeasurements = new Map();
      const sidebarRules = new Map();
      const normalCache = new Map();
      const activeRoots = new Set();
      const visit = root => {
        activeRoots.add(root);
        if (!roots.has(root) || !roots.get(root).isConnected) {
          const style = document.createElement('style');
          style.id = 'deck-line-height';
          style.disabled = true;
          (root === document ? document.head || document.documentElement : root).append(style);
          roots.set(root, style);
        }
        for (const element of root.querySelectorAll('*')) {
          if (element === probeHost || /^(STYLE|SCRIPT|LINK|META|HEAD|TITLE)$/.test(element.tagName)) continue;
          if (element.shadowRoot) visit(element.shadowRoot);
          if (!(element instanceof HTMLElement)) continue;
          const computed = getComputedStyle(element);
          const spacing = sidebarSpacing(element, computed);
          if (spacing) {
            if (!sidebarRules.has(spacing)) sidebarRules.set(spacing, sidebarRules.size);
            sidebarMeasurements.set(element, String(sidebarRules.get(spacing)));
          }
          const height = computed.lineHeight === 'normal'
            ? normalHeight(computed, normalCache) : Number.parseFloat(computed.lineHeight);
          if (!Number.isFinite(height) || height < 0) continue;
          const baseline = String(height);
          measurements.push([element, baseline]);
          heights.add(baseline);
        }
      };
      if (percent !== 100) visit(document);
      else {
        activeRoots.add(document);
        if (!roots.has(document)) {
          const style = document.createElement('style');
          style.id = 'deck-line-height';
          (document.head || document.documentElement).append(style);
          roots.set(document, style);
        }
      }
      // 先完成读取，再统一写入，所有子元素均使用自己的原生基准。
      for (const element of marked) {
        if (!element.isConnected) {
          element.removeAttribute(marker);
          marked.delete(element);
        }
      }
      for (const [element, baseline] of measurements) {
        if (element.getAttribute(marker) !== baseline) element.setAttribute(marker, baseline);
        marked.add(element);
      }
      for (const element of sidebarMarked) {
        if (!sidebarMeasurements.has(element)) {
          element.removeAttribute(sidebarMarker);
          sidebarMarked.delete(element);
        }
      }
      for (const [element, group] of sidebarMeasurements) {
        if (element.getAttribute(sidebarMarker) !== group) element.setAttribute(sidebarMarker, group);
        sidebarMarked.add(element);
      }
      const css = [...heights].map(height =>
        `[${marker}="${height}"] { line-height: ${Number(height) * percent / 100}px !important; }`
      ).concat([...sidebarRules].map(([declarations, group]) =>
        `[${sidebarMarker}="${group}"] { ${declarations} }`
      )).join('\n');
      for (const [root, style] of roots) {
        if (!activeRoots.has(root)) {
          style.remove();
          roots.delete(root);
        } else {
          const next = editorCSS + '\n' + css;
          if (style.textContent !== next) style.textContent = next;
        }
      }
    } finally {
      probeHost.remove();
      for (const [root, style] of roots) {
        style.disabled = false;
        if (percent !== 100) observer.observe(root, { attributes: true, attributeOldValue: true, attributeFilter: ['class', 'style', 'data-theme', 'data-ds-dark-theme'], childList: true, subtree: true });
      }
      keepCaretVisible();
    }
  }

  const dispose = () => {
    disposed = true;
    clearTimeout(timer);
    cancelAnimationFrame(caretFrame);
    observer.disconnect();
    document.removeEventListener('focusout', onFocusOut, true);
    document.removeEventListener('input', keepCaretVisible, true);
    document.removeEventListener('selectionchange', keepCaretVisible);
    window.removeEventListener('resize', schedule);
    document.removeEventListener('load', schedule, true);
    document.fonts?.removeEventListener('loadingdone', schedule);
    for (const style of roots.values()) style.remove();
    for (const element of marked) element.removeAttribute(marker);
    for (const element of sidebarMarked) element.removeAttribute(sidebarMarker);
    probeHost.remove();
    delete window[key];
  };
  window[key] = { dispose, percent };
  document.addEventListener('focusout', onFocusOut, true);
  document.addEventListener('input', keepCaretVisible, true);
  document.addEventListener('selectionchange', keepCaretVisible);
  window.addEventListener('resize', schedule);
  document.addEventListener('load', schedule, true);
  document.fonts?.addEventListener('loadingdone', schedule);
  refresh();
})();
