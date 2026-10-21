(() => {
  const key = '__deckCurrentProject';
  if (window[key]) {
    window[key].scan();
    return;
  }
  const folderIcon = '<svg aria-hidden="true" class="_Icon_uttcz_1 icon-xs shrink-0" focusable="false" height="16" viewBox="0 0 16 16" width="16" xmlns="http://www.w3.org/2000/svg"><path fill-rule="evenodd" clip-rule="evenodd" d="M5.55933 2.14136C6.06479 2.14136 6.55777 2.30207 6.96655 2.59937L7.81812 3.21851C8.04741 3.38523 8.32368 3.47534 8.60718 3.47534H11.9666C13.2874 3.47534 14.3582 4.54606 14.3582 5.86694V11.4666C14.3582 12.7874 13.2874 13.8582 11.9666 13.8582H4.03296C2.71229 13.8579 1.64136 12.7873 1.64136 11.4666V4.53296C1.6416 3.21244 2.71244 2.1416 4.03296 2.14136H5.55933ZM2.69214 7.85913V11.4666C2.69214 12.2074 3.29219 12.8081 4.03296 12.8083H11.9666C12.7075 12.8083 13.3083 12.2075 13.3083 11.4666V7.85913H2.69214ZM4.03296 3.19214C3.29234 3.19238 2.69239 3.79234 2.69214 4.53296V6.80835H13.3083V5.86694C13.3083 5.12596 12.7075 4.52515 11.9666 4.52515H8.60718C8.10172 4.52515 7.60874 4.36541 7.19995 4.06812L6.34839 3.448C6.11917 3.28145 5.84268 3.19214 5.55933 3.19214H4.03296Z" fill="currentColor"></path></svg>';
  const badges = new Map();
  let scheduled = false;
  const style = document.createElement('style');
  style.textContent = `
    .deck-current-project { display:inline-flex; flex:none; align-items:center; gap:4px; padding:0 6px; height:var(--height-token-button-composer, 28px); color:var(--color-token-text-tertiary, #999); font-size:var(--text-sm); white-space:nowrap; vertical-align:middle; }
    .deck-current-project[hidden] { display:none; }
    .deck-current-project svg { width:16px; height:16px; flex-shrink:0; }
  `;
  (document.head || document.documentElement).appendChild(style);

  const getFiber = (element) => {
    let fiber = element[Object.keys(element).find(name => name.startsWith('__reactFiber'))];
    let root = fiber;
    while (root?.return) root = root.return;
    if (root?.stateNode?.current && root.stateNode.current !== root) fiber = fiber?.alternate;
    return fiber;
  };
  const normalize = (path) => {
    if (typeof path !== 'string') return '';
    const normalized = path.replace(/\\/g, '/').replace(/\/+$/, '');
    return /^[a-z]:/i.test(normalized) || normalized.startsWith('//') ? normalized.toLowerCase() : normalized;
  };
  const projectRows = () => [...document.querySelectorAll('[data-app-action-sidebar-project-row]')].map(row => {
    for (let fiber = getFiber(row), depth = 0; fiber && depth < 30; fiber = fiber.return, depth++) {
      const group = fiber.memoizedProps?.project?.group;
      if (group) return { row, group };
    }
    return { row, group: null };
  }).filter(entry => entry.group);

  const context = (button) => {
    for (let fiber = getFiber(button); fiber; fiber = fiber.return) {
      const props = fiber.memoizedProps;
      if (!props || !Object.hasOwn(props, 'conversationId') || !Object.hasOwn(props, 'cwdOverride')) continue;
      // 读取权限组件实际使用的目录参数，不依赖组件名或 Hook 序号。
      const entries = fiber.updateQueue?.memoCache?.data?.flat() || [];
      const resolved = entries.find(value => value && typeof value === 'object' &&
        Object.hasOwn(value, 'cwd') && value.hostId === props.hostId);
      return {
        cwd: props.cwdOverride !== undefined ? props.cwdOverride : resolved?.cwd,
        hostId: props.hostId,
        conversationId: props.conversationId,
      };
    }
    return {};
  };
  const resolve = (button, projects) => {
    const current = context(button);
    const path = normalize(current.cwd);
    const threadKey = current.conversationId ? `${current.hostId || 'local'}:${current.conversationId}` : null;
    let project = threadKey ? projects.find(({ group }) => group.threadKeys?.includes(threadKey)) : null;
    if (!project && path) {
      let longest = -1;
      projects.forEach(entry => {
        const group = entry.group;
        if ((group.hostId || 'local') !== (current.hostId || 'local')) return;
        for (const root of group.rootPaths || [group.path]) {
          const normalized = normalize(root);
          if (normalized && (path === normalized || path.startsWith(`${normalized}/`)) && normalized.length > longest) {
            project = entry;
            longest = normalized.length;
          }
        }
      });
    }
    const name = project?.group.label || (typeof current.cwd === 'string'
      ? current.cwd.replace(/[\\/]+$/, '').split(/[\\/]/).pop() : '');
    const svg = project?.row.querySelector('[data-sidebar-project-drop-zone="project-icon"] svg');
    return { name, path: current.cwd || project?.group.path || '', icon: svg?.outerHTML || folderIcon };
  };
  const scan = () => {
    scheduled = false;
    for (const [button, badge] of badges) {
      if (!button.isConnected) {
        badge.remove();
        badges.delete(button);
      }
    }
    const projects = projectRows();
    document.querySelectorAll('[data-composer-navigation-target="permissions"]').forEach(button => {
      if (!button.closest('[data-composer-footer-responsive]')) return;
      const project = resolve(button, projects);
      let badge = badges.get(button);
      if (!badge) {
        badge = document.createElement('span');
        badge.className = 'deck-current-project';
        badges.set(button, badge);
      }
      let anchor = button;
      while (anchor.parentElement?.classList.contains('contents')) anchor = anchor.parentElement;
      if (anchor.nextElementSibling !== badge) anchor.after(badge);
      badge.hidden = !project.name;
      if (!project.name) return;
      if (badge.dataset.icon !== project.icon) {
        badge.innerHTML = project.icon;
        badge.querySelector('svg')?.setAttribute('aria-hidden', 'true');
        const label = document.createElement('span');
        label.className = 'deck-current-project-name';
        badge.append(label);
        badge.dataset.icon = project.icon;
      }
      const label = badge.querySelector('.deck-current-project-name');
      if (label.textContent !== project.name) label.textContent = project.name;
      const title = `${project.name}\n${project.path}`;
      if (badge.title !== title) badge.title = title;
      const ariaLabel = `当前项目：${project.name}`;
      if (badge.getAttribute('aria-label') !== ariaLabel) badge.setAttribute('aria-label', ariaLabel);
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
    attributeFilter: ['data-app-action-sidebar-project-label', 'data-app-action-sidebar-project-id'],
  });
  const timer = setInterval(() => {
    if (document.visibilityState === 'visible') schedule();
  }, 2000);
  window[key] = { scan: schedule, observer, timer };
  scan();
})();
