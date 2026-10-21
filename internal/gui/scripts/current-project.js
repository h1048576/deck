(() => {
  const app = '__DECK_PROJECT_APP__';
  const key = `__deckCurrentProject_${app}`;
  if (window[key]) {
    window[key].scan();
    return true;
  }
  const badges = new Map();
  let scheduled = false;
  const style = document.createElement('style');
  style.textContent = `
    .deck-project-label { display:inline-flex; flex:none; align-items:center; gap:4px; padding:0 6px; color:inherit; opacity:.65; white-space:nowrap; vertical-align:middle; }
    .deck-project-label[hidden] { display:none; }
    .deck-project-label svg { width:16px; height:16px; flex-shrink:0; }
  `;
  // 与各应用侧栏中工作区展开时的图标保持一致。
  const icons = {
    dsh: '<svg viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true"><path d="M2.55912 7.93683C2.67584 7.49906 3.0723 7.19446 3.52536 7.19446H13.6491C14.3061 7.19446 14.7846 7.81725 14.6153 8.45209L13.4411 12.856C13.3244 13.2938 12.9279 13.5984 12.4748 13.5984H2.35113C1.69411 13.5984 1.21562 12.9756 1.38489 12.3407L2.55912 7.93683Z" fill="currentColor" opacity="0.16"></path><path d="M13.6491 6.69446C14.6346 6.69453 15.3522 7.62895 15.0983 8.58118L13.9245 12.9845C13.7494 13.6412 13.1539 14.0988 12.4743 14.0988H2.35126C1.36574 14.0988 0.648153 13.1643 0.902044 12.212L2.07587 7.80774C2.25102 7.15128 2.84567 6.69455 3.52509 6.69446H13.6491ZM3.52509 7.69446C3.29865 7.69455 3.10004 7.84674 3.04169 8.06555L1.86786 12.4698C1.78345 12.7872 2.02285 13.0988 2.35126 13.0988H12.4743C12.7007 13.0988 12.8992 12.9463 12.9577 12.7277L14.1325 8.32336C14.2171 8.00598 13.9776 7.69453 13.6491 7.69446H3.52509Z" fill="currentColor"></path><path d="M4.7666 1.90137C5.13227 1.90144 5.48571 2.03525 5.75977 2.27734L7.27246 3.61328C7.36379 3.69382 7.48174 3.73828 7.60352 3.73828H12.3994C13.2276 3.73841 13.8993 4.41005 13.8994 5.23828V6.7168C13.8183 6.70327 13.735 6.69436 13.6494 6.69434H12.8994V5.23828C12.8993 4.96233 12.6754 4.73841 12.3994 4.73828H7.60352C7.23781 4.73828 6.88446 4.60438 6.61035 4.3623L5.09766 3.02637C5.00636 2.94576 4.88838 2.90144 4.7666 2.90137H2.0498C1.77366 2.90137 1.5498 3.12523 1.5498 3.40137V9.78223L0.902344 12.2119C0.648452 13.1642 1.36604 14.0986 2.35156 14.0986H2.0498C1.2214 14.0986 0.549838 13.427 0.549805 12.5986V3.40137C0.549805 2.57294 1.22138 1.90137 2.0498 1.90137H4.7666Z" fill="currentColor"></path></svg>',
    zcode: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m6 14 1.5-2.9A2 2 0 0 1 9.24 10H20a2 2 0 0 1 1.94 2.5l-1.54 6a2 2 0 0 1-1.95 1.5H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h3.9a2 2 0 0 1 1.69.9l.81 1.2a2 2 0 0 0 1.67.9H18a2 2 0 0 1 2 2v2"></path></svg>',
  };
  (document.head || document.documentElement).append(style);
  const fiberOf = element => {
    let fiber = element[Object.keys(element).find(key => key.startsWith('__reactFiber'))];
    let root = fiber;
    while (root?.return) root = root.return;
    if (root?.stateNode?.current && root.stateNode.current !== root) fiber = fiber?.alternate;
    return fiber;
  };
  const basename = path => typeof path === 'string' ? path.replace(/[\\/]+$/, '').split(/[\\/]/).pop() : '';
  const resolveDsh = input => {
    for (let fiber = fiberOf(input); fiber; fiber = fiber.return) {
      const props = fiber.memoizedProps;
      if (!props?.useWorkspaces || !props?.selectWorkspace) continue;
      let cwd = '';
      let workspace;
      // 原生会话组件订阅的工作区快照包含项目名称、路径和所属会话。
      for (let hook = fiber.memoizedState; hook; hook = hook.next) {
        const value = hook.memoizedState;
        if (typeof value === 'string' && /^(?:[a-z]:[\\/]|\/|\\\\)/i.test(value)) cwd = value;
        if (Array.isArray(value?.items)) {
          workspace = value.items.find(item => item.sessionIds?.includes(props.sessionId));
        }
      }
      const path = cwd || workspace?.path || '';
      return { name: workspace?.title || basename(path), path };
    }
    return {};
  };
  const resolveZcode = input => {
    for (let fiber = fiberOf(input); fiber; fiber = fiber.return) {
      const props = fiber.memoizedProps;
      if (typeof props?.workspacePath === 'string' && props.workspacePath) {
        return { name: basename(props.workspacePath), path: props.workspacePath };
      }
    }
    return {};
  };
  const scan = () => {
    scheduled = false;
    for (const [button, badge] of badges) {
      if (!button.isConnected) {
        badge.remove();
        badges.delete(button);
      }
    }
    const selector = app === 'dsh' ? '[data-composer-input]' : '[data-testid="v4-composer-input"]';
    document.querySelectorAll(selector).forEach(input => {
      const container = app === 'dsh' ? input.closest('[data-composer-seat]') : input.closest('form');
      const button = app === 'dsh'
        ? container?.querySelector('button[aria-label^="访问模式"],button[aria-label^="Access mode"]')
        : container?.querySelector('[data-testid="chat-mode-select-trigger"]');
      if (!button) return;
      const project = app === 'dsh' ? resolveDsh(input) : resolveZcode(input);
      let badge = badges.get(button);
      if (!badge) {
        badge = document.createElement('span');
        badge.className = 'deck-project-label';
        badge.innerHTML = `${icons[app] || icons.zcode}<span></span>`;
        badges.set(button, badge);
      }
      const anchor = app === 'dsh' ? button.parentElement : button;
      if (anchor.nextElementSibling !== badge) anchor.after(badge);
      badge.hidden = !project.name;
      if (!project.name) return;
      const label = badge.querySelector('span');
      if (label.textContent !== project.name) label.textContent = project.name;
      const title = `${project.name}\n${project.path}`;
      if (badge.title !== title) badge.title = title;
      badge.setAttribute('aria-label', `当前项目：${project.name}`);
    });
  };
  const schedule = () => {
    if (scheduled) return;
    scheduled = true;
    requestAnimationFrame(scan);
  };
  const observer = new MutationObserver(schedule);
  observer.observe(document.documentElement, { childList: true, subtree: true, characterData: true });
  const timer = setInterval(() => {
    if (document.visibilityState === 'visible') schedule();
  }, 2000);
  window[key] = { scan: schedule, observer, timer };
  scan();
  return true;
})()
