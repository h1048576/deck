(() => {
  const key = '__deckZcodeModelLabel';
  if (window[key]) {
    window[key].scan();
    return true;
  }

  let scheduled = false;
  const scan = () => {
    scheduled = false;
    document.querySelectorAll('[data-testid="chat-model-select-trigger"] span[title]').forEach(label => {
      // 仅缩短原生模型标签的文本节点，保留完整 ID、提示和 React 状态。
      const fullName = label.getAttribute('title');
      const shortName = fullName.slice(fullName.lastIndexOf('/') + 1);
      if (!shortName || shortName === fullName) return;
      const walker = document.createTreeWalker(label, NodeFilter.SHOW_TEXT);
      for (let node = walker.nextNode(); node; node = walker.nextNode()) {
        if (node.nodeValue === fullName) node.nodeValue = shortName;
      }
    });
  };
  const schedule = () => {
    if (scheduled) return;
    scheduled = true;
    requestAnimationFrame(scan);
  };
  const observer = new MutationObserver(schedule);
  observer.observe(document.documentElement, {
    childList: true,
    subtree: true,
    characterData: true,
    attributes: true,
    attributeFilter: ['title', 'data-model-current-value'],
  });
  window[key] = { scan: schedule, observer };
  scan();
  return true;
})()
