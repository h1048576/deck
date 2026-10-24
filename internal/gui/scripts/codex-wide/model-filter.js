(() => {
  const key = '__deckCodexModelFilter';
  const configuredPatterns = __DECK_HIDDEN_MODEL_PATTERNS__;
  if (window[key]) {
    window[key].configure(configuredPatterns);
    return true;
  }

  const marker = 'data-deck-codex-hidden-model';
  let patterns = [];
  let scheduled = false;
  const identity = item => {
    let name = item.querySelector('[data-menu-row-content]')?.textContent?.trim() || item.textContent.trim();
    let fiber = item[Object.keys(item).find(key => key.startsWith('__reactFiber'))];
    let root = fiber;
    while (root?.return) root = root.return;
    if (root?.stateNode?.current && root.stateNode.current !== root) fiber = fiber?.alternate;
    // Radix 的 textValue 是不含描述的名称，模型行组件的 key 是实际模型 ID。
    for (let depth = 0; fiber && depth < 32; fiber = fiber.return, depth++) {
      const props = fiber.memoizedProps;
      if (props?.['data-model-picker-view']) break;
      if (typeof props?.textValue === 'string') name = props.textValue;
      if (typeof fiber.key === 'string' && props && Object.hasOwn(props, 'data-model-selected')) {
        return [name, fiber.key];
      }
    }
    return [name];
  };
  const scan = () => {
    scheduled = false;
    const hidden = new Set();
    // 限定模型选择视图，避免影响推理等级、权限等其他单选菜单。
    for (const item of document.querySelectorAll('[data-model-picker-view] [role="menuitemradio"]')) {
      if (patterns.length && patterns.some(pattern => identity(item).some(value => pattern.test(value)))) hidden.add(item);
    }
    for (const item of document.querySelectorAll(`[${marker}]`)) {
      if (!hidden.has(item)) item.removeAttribute(marker);
    }
    for (const item of hidden) {
      if (!item.hasAttribute(marker)) item.setAttribute(marker, '');
    }
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
  const schedule = () => {
    if (scheduled) return;
    scheduled = true;
    queueMicrotask(scan);
  };
  const install = () => {
    const style = document.createElement('style');
    style.id = 'deck-codex-model-filter';
    style.textContent = `[${marker}] { display: none !important; }`;
    (document.head || document.documentElement).appendChild(style);
    const observer = new MutationObserver(schedule);
    observer.observe(document.documentElement, {
      childList: true, subtree: true, characterData: true,
      attributes: true, attributeFilter: ['role', 'data-model-picker-view'],
    });
    window[key] = { configure, observer };
    configure(configuredPatterns);
  };
  if (document.documentElement) install();
  else document.addEventListener('DOMContentLoaded', install, { once: true });
  return true;
})();
