(() => {
  const key = '__deckWorkbuddyModelFilter';
  const configuredPatterns = __DECK_HIDDEN_MODEL_PATTERNS__;
  if (window[key]?.version === 2) {
    window[key].configure(configuredPatterns);
    return true;
  }
  window[key]?.observer?.disconnect();

  const modelID = item => {
    try {
      const id = JSON.parse(item.getAttribute('data-track-props') || '{}').source;
      if (typeof id === 'string') return id;
    } catch {}
    // 新版组件不再输出埋点属性，从当前行的 React 属性读取模型 ID。
    let fiber = item[Object.keys(item).find(name => name.startsWith('__reactFiber'))];
    let root = fiber;
    while (root?.return) root = root.return;
    if (root?.stateNode?.current && root.stateNode.current !== root) fiber = fiber?.alternate;
    for (let depth = 0; fiber && depth < 8; fiber = fiber.return, depth++) {
      const id = fiber.memoizedProps?.option?.id;
      if (typeof id === 'string') return id;
    }
    return '';
  };

  const marker = 'data-deck-hidden-model';
  let patterns = [];
  let scheduled = false;
  const matches = (...values) => patterns.some(pattern => values.some(value => value && pattern.test(value)));
  const mark = (element, hidden) => {
    if (hidden) {
      if (!element.hasAttribute(marker)) element.setAttribute(marker, '');
    } else {
      element.removeAttribute(marker);
    }
  };
  const scan = () => {
    scheduled = false;
    const hidden = new Set();
    // 模型名称单独读取，避免消耗倍率、标签等附加文字影响精确匹配。
    for (const item of document.querySelectorAll('[data-track-id="agent_model_select"], .cr-model-selector__item')) {
      const name = item.querySelector('[class*="_modelName_"], .cr-model-selector__item-name')?.textContent?.trim() || '';
      if (matches(name, modelID(item))) hidden.add(item);
    }
    // Auto 和 Max 使用同一种开关行，只匹配各自标签，并隐藏整个对应区块。
    for (const section of document.querySelectorAll('.wb-popover-compact [class*="_autoModeSection_"]')) {
      const name = section.querySelector('[class*="_autoModeLabel_"]')?.textContent?.trim() || '';
      if (matches(name)) hidden.add(section);
    }
    for (const item of document.querySelectorAll(`[${marker}]`)) mark(item, hidden.has(item));
    for (const item of hidden) mark(item, true);
  };
  const schedule = () => {
    if (scheduled) return;
    scheduled = true;
    queueMicrotask(scan);
  };
  const configure = values => {
    patterns = [];
    for (const value of Array.isArray(values) ? values : []) {
      if (typeof value !== 'string' || !value.trim()) continue;
      try { patterns.push(new RegExp(value, 'i')); }
      catch (error) { console.warn('[Deck] 已忽略无效的模型隐藏正则表达式：', value, error); }
    }
    scan();
  };
  const install = () => {
    const style = document.getElementById('deck-workbuddy-model-filter') || document.createElement('style');
    style.id = 'deck-workbuddy-model-filter';
    style.textContent = `[${marker}] { display: none !important; }`;
    (document.head || document.documentElement).appendChild(style);
    const observer = new MutationObserver(schedule);
    observer.observe(document.documentElement, {
      childList: true, subtree: true, characterData: true,
      attributes: true, attributeFilter: ['data-track-id', 'data-track-props'],
    });
    window[key] = { version: 2, configure, observer };
    configure(configuredPatterns);
  };
  if (document.documentElement) install();
  else document.addEventListener('DOMContentLoaded', install, { once: true });
  return true;
})()
