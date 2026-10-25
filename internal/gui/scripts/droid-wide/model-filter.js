(() => {
  const key = '__deckDroidModelFilter';
  const configuredPatterns = __DECK_HIDDEN_MODEL_PATTERNS__;
  if (window[key]) {
    window[key].configure(configuredPatterns);
    return true;
  }
  const wrapper = '[data-testid="model-dropdown-wrapper"]';
  const marker = 'data-deck-droid-hidden-model';
  let patterns = [];
  let scheduled = false;
  const identity = item => {
    // 第一段 span 是模型名，排除倍率、NEW 等标签，保留正则结尾匹配。
    const name = item.querySelector('span')?.textContent.trim() || '';
    let fiber = item[Object.keys(item).find(key => key.startsWith('__reactFiber'))];
    let root = fiber;
    while (root?.return) root = root.return;
    if (root?.stateNode?.current && root.stateNode.current !== root) fiber = fiber?.alternate;
    for (let depth = 0; fiber && depth < 32; fiber = fiber.return, depth++) {
      if (fiber.memoizedProps?.['data-testid'] === 'model-dropdown-wrapper') break;
      if (typeof fiber.key === 'string') return [name, fiber.key];
    }
    return [name];
  };
  const scan = () => {
    scheduled = false;
    for (const container of document.querySelectorAll(wrapper)) {
      for (const item of container.querySelectorAll('[role="menuitem"]')) {
        // Legacy 展开按钮也在此容器中，但没有模型行的布局标记。
        const isModel = !!item.querySelector('[data-flex-row="true"]');
        const hidden = isModel && patterns.some(pattern => identity(item).some(value => pattern.test(value)));
        if (hidden && !item.hasAttribute(marker)) item.setAttribute(marker, '');
        else if (!hidden && item.hasAttribute(marker)) item.removeAttribute(marker);
      }
    }
  };
  const schedule = records => {
    // 只处理模型菜单自身更新或挂载，聊天流式输出和侧栏变化不会触发扫描。
    const relevant = records.some(record => record.target.parentElement?.closest(wrapper) ||
      record.target.matches?.(wrapper) || [...record.addedNodes].some(node =>
        node.nodeType === 1 && (node.matches(wrapper) || node.querySelector(wrapper))));
    if (!relevant || scheduled) return;
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
  const style = document.createElement('style');
  style.id = 'deck-droid-model-filter';
  style.textContent = `[${marker}] { display: none !important; }`;
  (document.head || document.documentElement).appendChild(style);
  const observer = new MutationObserver(schedule);
  observer.observe(document.documentElement, { childList: true, subtree: true, characterData: true });
  window[key] = { configure, observer };
  configure(configuredPatterns);
  return true;
})()
