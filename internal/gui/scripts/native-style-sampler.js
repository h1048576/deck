(() => {
  const key = '__deckNativeStyles';
  if (window[key]?.version === 1) return;
  window[key]?.dispose();
  let frame = null;
  let nativeDocument = null;
  let copies = new WeakMap();
  let cssDirty = true;
  let clients = 0;
  const ownNode = node => node instanceof Element &&
    (node.id.startsWith('deck-') || node.hasAttribute('data-deck-probe'));
  const parentOf = element => element.parentElement || element.getRootNode().host;
  const attributes = (source, target) => {
    for (const attribute of [...target.attributes]) target.removeAttribute(attribute.name);
    for (const attribute of source.attributes) {
      if (!attribute.name.startsWith('data-deck-') && !/^on/i.test(attribute.name)) {
        target.setAttribute(attribute.name, attribute.value);
      }
    }
  };
  const cssText = sheets => [...sheets].filter(sheet => !ownNode(sheet.ownerNode)).map(sheet => {
    try { return [...sheet.cssRules].map(rule => rule.cssText).join('\n'); } catch { return ''; }
  }).join('\n');
  const clone = (node, inShadow = false) => {
    if (!(node instanceof Element)) {
      const copy = nativeDocument.importNode(node, false);
      copies.set(node, copy);
      return copy;
    }
    if (ownNode(node) || /^(SCRIPT|LINK|META|BASE)$/.test(node.tagName) || (!inShadow && node.tagName === 'STYLE')) return null;
    const copy = nativeDocument.importNode(node, false);
    copies.set(node, copy);
    attributes(node, copy);
    // 镜像只参与样式计算，不加载页面、媒体或执行应用代码。
    if (/^(IFRAME|OBJECT|EMBED|IMG|VIDEO|AUDIO|SOURCE)$/.test(node.tagName)) {
      for (const name of ['src', 'srcdoc', 'srcset', 'data', 'poster', 'autoplay']) copy.removeAttribute(name);
      return copy;
    }
    for (const child of node.childNodes) {
      const childCopy = clone(child, inShadow);
      if (childCopy) copy.append(childCopy);
    }
    if (node.shadowRoot) {
      const root = copy.shadowRoot || copy.attachShadow({ mode: 'open' });
      copies.set(node.shadowRoot, root);
      for (const child of node.shadowRoot.childNodes) {
        const childCopy = clone(child, true);
        if (childCopy) root.append(childCopy);
      }
      const style = nativeDocument.createElement('style');
      style.textContent = cssText(node.shadowRoot.adoptedStyleSheets || []);
      root.append(style);
    }
    return copy;
  };
  const prepare = (scopes, full) => {
    if (!frame?.isConnected) {
      frame = document.createElement('iframe');
      frame.id = 'deck-native-style-sampler';
      frame.setAttribute('data-deck-probe', '');
      frame.setAttribute('aria-hidden', 'true');
      frame.tabIndex = -1;
      frame.setAttribute('sandbox', 'allow-same-origin');
      frame.style.cssText = 'position:fixed!important;left:-100000px!important;top:0!important;visibility:hidden!important;pointer-events:none!important;border:0!important;contain:strict!important;';
      document.documentElement.append(frame);
      nativeDocument = frame.contentDocument;
      nativeDocument.open();
      nativeDocument.write('<!doctype html><html><head><meta http-equiv="Content-Security-Policy" content="default-src \'none\'; style-src \'unsafe-inline\'; font-src data:; img-src data:"></head><body></body></html>');
      nativeDocument.close();
      copies = new WeakMap();
      cssDirty = true;
      full = true;
    }
    frame.style.width = `${window.innerWidth}px`;
    frame.style.height = `${window.innerHeight}px`;
    attributes(document.documentElement, nativeDocument.documentElement);
    copies.set(document.documentElement, nativeDocument.documentElement);
    // 复用原页面已加载的字体，镜像无需重新请求应用资源。
    for (const font of document.fonts || []) if (font.status === 'loaded') nativeDocument.fonts.add(font);
    if (cssDirty) {
      nativeDocument.head.querySelectorAll('style').forEach(style => style.remove());
      const style = nativeDocument.createElement('style');
      style.textContent = cssText([...document.styleSheets, ...(document.adoptedStyleSheets || [])]) +
        '\n*, *::before, *::after { animation:none!important; transition:none!important; }';
      nativeDocument.head.append(style);
      cssDirty = false;
    }
    if (full || !copies.has(document.body)) {
      const body = clone(document.body);
      nativeDocument.body.replaceWith(body);
      return;
    }
    attributes(document.body, nativeDocument.body);
    for (const scope of scopes) {
      if (!scope.isConnected || scope === document.documentElement) continue;
      let branch = scope;
      while (parentOf(branch) && !copies.has(parentOf(branch))) branch = parentOf(branch);
      const parent = copies.get(branch.parentNode);
      if (!parent) continue;
      // 同步继承链上的类名和变量，镜像中的旧行高覆盖不会参与继承。
      for (let ancestor = parentOf(branch); ancestor && copies.has(ancestor); ancestor = parentOf(ancestor)) {
        attributes(ancestor, copies.get(ancestor));
      }
      const previous = copies.get(branch);
      const next = clone(branch, branch.getRootNode() instanceof ShadowRoot);
      if (!next) continue;
      if (previous?.parentNode === parent) previous.replaceWith(next);
      else {
        let sibling = branch.nextSibling;
        while (sibling && copies.get(sibling)?.parentNode !== parent) sibling = sibling.nextSibling;
        parent.insertBefore(next, sibling ? copies.get(sibling) : null);
      }
    }
  };
  const editing = () => {
    let element = document.activeElement;
    while (element?.shadowRoot?.activeElement) element = element.shadowRoot.activeElement;
    return !!element?.closest('textarea, input, [contenteditable="true"], [contenteditable="plaintext-only"]');
  };
  const elementsIn = (scope, result = []) => {
    if (scope instanceof HTMLElement && !ownNode(scope)) result.push(scope);
    for (const element of scope.querySelectorAll('*')) {
      if (ownNode(element)) continue;
      if (element instanceof HTMLElement && !/^(STYLE|SCRIPT|LINK|META|HEAD|TITLE|BASE)$/.test(element.tagName)) result.push(element);
      if (element.shadowRoot) elementsIn(element.shadowRoot, result);
    }
    return result;
  };
  const watch = ({ measure, relevantProperty, cleanup }) => {
    clients++;
    let stopped = false, timer = null, full = true, initialized = false;
    let firstQueued = 0;
    const dirty = new Set();
    const observedRoots = new Set([document.documentElement]);
    const stats = { passes: 0, fullPasses: 0, elements: 0, writes: 0, lastDurationMs: 0 };
    const style = document.createElement('span').style;
    const signature = value => {
      style.cssText = value || '';
      return [...style].filter(relevantProperty).sort()
        .map(name => `${name}:${style.getPropertyValue(name)}!${style.getPropertyPriority(name)}`).join(';');
    };
    const queue = (scope = document.documentElement, invalidate = false) => {
      if (stopped) return;
      if (invalidate) cssDirty = true;
      if (scope === document.documentElement || scope === document.body) full = true;
      else if (scope instanceof Element && !ownNode(scope)) dirty.add(scope);
      if (!firstQueued) firstQueued = performance.now();
      if (editing() && initialized) return;
      clearTimeout(timer);
      timer = setTimeout(refresh, Math.min(300, Math.max(0, 1500 - (performance.now() - firstQueued))));
    };
    const observer = new MutationObserver(records => {
      for (const record of records) {
        if (ownNode(record.target)) continue;
        if (record.target instanceof Element && record.target.matches('style, link[rel="stylesheet"]')) {
          queue(document.documentElement, true);
          continue;
        }
        if (record.type === 'attributes') {
          if (record.oldValue === record.target.getAttribute(record.attributeName)) continue;
          if (record.attributeName === 'style' && signature(record.oldValue) === signature(record.target.getAttribute('style'))) continue;
          queue(record.target);
        } else {
          const added = [...record.addedNodes].filter(node => node instanceof Element && !ownNode(node));
          const removed = [...record.removedNodes].filter(node => node instanceof Element && !ownNode(node));
          // 删除节点只清理缓存；不因一个图标/流式占位符被替换而重采样整段历史。
          for (const node of removed) copies.get(node)?.remove();
          if (removed.length) cleanup?.();
          for (const node of added) queue(node);
          if (added.some(node => node.matches('style, link[rel="stylesheet"]'))) queue(document.documentElement, true);
        }
      }
    });
    const observe = root => observer.observe(root, { childList: true, subtree: true, attributes: true, attributeOldValue: true,
      attributeFilter: ['class', 'style', 'data-theme', 'data-ds-dark-theme'] });
    function refresh() {
      timer = null;
      if (stopped || !document.body || (!full && !dirty.size)) return;
      if (editing() && initialized) return;
      const start = performance.now();
      const scopes = full ? [document.documentElement] : [...dirty].filter(element => {
        if (!element.isConnected) return false;
        for (let parent = parentOf(element); parent; parent = parentOf(parent)) if (dirty.has(parent)) return false;
        return true;
      });
      const wasFull = full;
      full = false;
      dirty.clear();
      firstQueued = 0;
      if (!scopes.length) return;
      observer.disconnect();
      try {
        prepare(scopes, wasFull);
        const elements = scopes.flatMap(scope => elementsIn(scope));
        for (const element of elements) if (element.shadowRoot) observedRoots.add(element.shadowRoot);
        measure(elements, element => {
          const copy = copies.get(element);
          return copy ? nativeDocument.defaultView.getComputedStyle(copy) : null;
        }, stats, wasFull);
        cleanup?.();
        stats.passes++;
        if (wasFull) stats.fullPasses++;
        stats.elements += elements.length;
        initialized = true;
      } finally {
        for (const root of observedRoots) {
          if (root instanceof ShadowRoot && !root.host.isConnected) observedRoots.delete(root);
          else observe(root);
        }
        stats.lastDurationMs = Math.round((performance.now() - start) * 100) / 100;
      }
    }
    const onFocusOut = () => { if (full || dirty.size) { clearTimeout(timer); timer = setTimeout(refresh, 0); } };
    const invalidate = () => queue(document.documentElement, true);
    const onLoad = event => { if (event.target instanceof Element && event.target.matches('link[rel="stylesheet"], style')) invalidate(); };
    window.addEventListener('resize', invalidate);
    document.fonts?.addEventListener('loadingdone', invalidate);
    document.addEventListener('load', onLoad, true);
    document.addEventListener('focusout', onFocusOut, true);
    refresh();
    return { stats, dispose() {
      stopped = true;
      clearTimeout(timer);
      observer.disconnect();
      window.removeEventListener('resize', invalidate);
      document.fonts?.removeEventListener('loadingdone', invalidate);
      document.removeEventListener('load', onLoad, true);
      document.removeEventListener('focusout', onFocusOut, true);
      if (--clients === 0) { frame?.remove(); frame = null; nativeDocument = null; copies = new WeakMap(); }
    } };
  };
  window[key] = { version: 1, watch, dispose() { frame?.remove(); } };
})();
