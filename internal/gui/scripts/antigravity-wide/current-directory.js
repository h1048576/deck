(() => {
  const key = '__deckAntigravityCurrentDirectory';
  if (window[key]) {
    window[key].schedule();
    return true;
  }

  const composerSelector = '[data-agent-composer-root]';
  const style = document.createElement('style');
  style.textContent = `
    .deck-antigravity-directory {
      display: inline-flex;
      flex: 0 1 auto;
      align-items: center;
      gap: 4px;
      min-width: 0;
      max-width: min(11rem, 25vw);
      padding-inline: 4px;
      height: 28px;
      color: var(--secondary-foreground, inherit);
      white-space: nowrap;
    }
    .deck-antigravity-directory-icon {
      position: relative;
      flex: none;
      box-sizing: border-box;
      width: 13px;
      height: 9px;
      margin-top: 2px;
      border: 1px solid currentColor;
      border-radius: 2px;
    }
    .deck-antigravity-directory-icon::before {
      position: absolute;
      top: -4px;
      left: -1px;
      width: 5px;
      height: 3px;
      border: 1px solid currentColor;
      border-bottom: 0;
      border-radius: 2px 2px 0 0;
      content: '';
    }
    .deck-antigravity-directory-name { overflow: hidden; text-overflow: ellipsis; }
  `;
  (document.head || document.documentElement).append(style);

  const workspaceUris = element => {
    const fiberKey = Object.keys(element).find(name => name.startsWith('__reactFiber$'));
    let fiber = element[fiberKey];
    let root = fiber;
    while (root?.return) root = root.return;
    if (root?.stateNode?.current && root.stateNode.current !== root) fiber = fiber.alternate ?? fiber;
    // 输入区的原生会话上下文已处理当前项目和会话对应的工作目录。
    for (; fiber; fiber = fiber.return) {
      for (let context = fiber.dependencies?.firstContext; context; context = context.next) {
        const uris = context.memoizedValue?.workspaceInfo?.workspaceUris;
        if (Array.isArray(uris)) return uris;
      }
      const uris = fiber.memoizedProps?.workspaceInfo?.workspaceUris;
      if (Array.isArray(uris)) return uris;
    }
    return [];
  };

  const directory = uri => {
    if (typeof uri !== 'string' || !uri.trim()) return null;
    let path = uri;
    let name = uri;
    try {
      const url = new URL(uri);
      if (url.pathname.startsWith('/')) {
        const decoded = decodeURIComponent(url.pathname);
        name = decoded;
        if (url.protocol === 'file:') {
          if (url.hostname) path = ('//' + url.hostname + decoded).replace(/\//g, '\\');
          else if (/^\/[a-z]:\//i.test(decoded)) path = decoded.slice(1).replace(/\//g, '\\');
          else path = decoded;
        }
      }
    } catch {
      // 原生上下文也可能直接返回路径。仍按目录名和完整路径展示。
    }
    return { name: name.replace(/[\\/]+$/, '').split(/[\\/]/).pop() || path, path };
  };

  const badges = new Map();
  const scan = () => {
    const visible = new Set();
    for (const composer of document.querySelectorAll(composerSelector)) {
      const attach = composer.querySelector('button[aria-label="Add context"]');
      const model = composer.querySelector('[data-testid="model-selector-trigger"]');
      if (!attach || !model || !(attach.compareDocumentPosition(model) & Node.DOCUMENT_POSITION_FOLLOWING)) continue;
      // 将标签插在包含「+」的控件后面，兼容按钮外层有包装节点的布局。
      let anchor = attach;
      while (!anchor.parentElement.contains(model)) anchor = anchor.parentElement;
      const paths = workspaceUris(model).map(directory).filter(Boolean);
      if (!paths.length) continue;
      visible.add(composer);

      let badge = badges.get(composer);
      if (!badge) {
        badge = document.createElement('span');
        badge.className = 'deck-antigravity-directory';
        const icon = document.createElement('span');
        icon.className = 'deck-antigravity-directory-icon';
        icon.setAttribute('aria-hidden', 'true');
        const label = document.createElement('span');
        label.className = 'deck-antigravity-directory-name';
        badge.append(icon, label);
        badges.set(composer, badge);
      }
      const label = paths.length === 1 ? paths[0].name : `${paths[0].name} +${paths.length - 1}`;
      if (badge.lastElementChild.textContent !== label) badge.lastElementChild.textContent = label;
      const title = paths.map(item => item.path).join('\n');
      if (badge.title !== title) badge.title = title;
      if (anchor.nextElementSibling !== badge) anchor.after(badge);
    }
    for (const [composer, badge] of badges) {
      if (visible.has(composer)) continue;
      badge.remove();
      badges.delete(composer);
    }
  };

  let frame = 0;
  const schedule = () => {
    if (frame) return;
    frame = requestAnimationFrame(() => {
      frame = 0;
      scan();
    });
  };
  const observer = new MutationObserver(schedule);
  observer.observe(document.getElementById('root'), { childList: true, subtree: true });
  // 如果目录只在 React 上下文里改变，周期读取可更新同一个输入区上的标签。
  setInterval(() => {
    if (document.visibilityState === 'visible') schedule();
  }, 2000);
  window[key] = { schedule };
  scan();
  return true;
})()
