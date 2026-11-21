(() => {
  const key = '__deckDroidComposer';
  if (window[key]?.version === 2) {
    window[key].scan();
    return true;
  }
  window[key]?.observer?.disconnect();
  document.querySelectorAll('.deck-droid-project, .deck-droid-usage').forEach(element => element.remove());
  const folderIcon = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"></path></svg>';
  const style = document.createElement('style');
  style.textContent = `
    .deck-droid-project { display:inline-flex; flex:none; align-items:center; gap:4px; height:28px; padding:0 6px; color:var(--core-v2-text-secondary, #9a9a9a); white-space:nowrap; }
    .deck-droid-project[hidden] { display:none !important; }
    .deck-droid-project svg { width:14px; height:14px; flex-shrink:0; }
    .deck-droid-project { order:1; }
    [data-testid="chat-composer-wrapper"] [data-testid="chat-composer-autonomy-trigger"] { order:2; }
  `;
  (document.head || document.documentElement).appendChild(style);

  const fiberOf = element => {
    const name = element && Object.keys(element).find(item => item.startsWith('__reactFiber'));
    return name ? element[name] : null;
  };
  const basename = path => path.replace(/[\\/]+$/, '').split(/[\\/]/).pop();
  const findProp = (element, read, limit = 160) => {
    for (let fiber = fiberOf(element), depth = 0; fiber && depth < limit; fiber = fiber.return, depth++) {
      const value = read(fiber.memoizedProps);
      if (value) return value;
    }
    return '';
  };
  const resolveProject = wrapper => {
    const input = wrapper.querySelector('[contenteditable="true"][role="textbox"]') || wrapper;
    // 已有会话的根组件以 initialCwd 记录会话目录；新会话页还没有会话，目录来自项目选择器。
    const cwd = findProp(input, props => typeof props?.initialCwd === 'string' && props.initialCwd);
    if (cwd) return cwd;
    const sessionId = findProp(input, props => typeof props?.sessionId === 'string' && props.sessionId);
    if (sessionId && !sessionId.startsWith('new')) {
      const row = document.querySelector(`div[data-session-id="${CSS.escape(sessionId)}"]`);
      const sessionCwd = row && findProp(row, props => typeof props?.session?.cwd === 'string' && props.session.cwd, 40);
      if (sessionCwd) return sessionCwd;
    }
    const trigger = document.querySelector('[data-testid="project-selector-trigger"]');
    return trigger ? findProp(trigger, props => typeof props?.selectedProjectName === 'string' && props.selectedProjectName, 60) : '';
  };

  const setText = (element, value) => {
    if (element.textContent !== value) element.textContent = value;
  };
  const setTitle = (element, value) => {
    if (element.title !== value) element.title = value;
  };

  const projects = new Map();
  let scheduled = false;
  const scan = () => {
    scheduled = false;
    for (const [wrapper, project] of projects) {
      if (!wrapper.isConnected) {
        project.remove();
        projects.delete(wrapper);
      }
    }
    document.querySelectorAll('[data-testid="chat-composer-wrapper"]').forEach(wrapper => {
      const plus = wrapper.querySelector('[data-testid="chat-composer-plus-menu-trigger"]');
      let project = projects.get(wrapper);
      if (!plus) {
        project?.remove();
        projects.delete(wrapper);
        return;
      }
      if (!project) {
        project = document.createElement('span');
        project.className = 'deck-droid-project';
        project.innerHTML = `${folderIcon}<span></span>`;
        projects.set(wrapper, project);
      }
      // 排序由 CSS order 控制，不移动应用自己的节点，避免破坏 React 的 DOM 管理。
      if (plus.nextElementSibling !== project) plus.after(project);
      const path = resolveProject(wrapper);
      project.hidden = !path;
      if (path) {
        setText(project.querySelector('span'), basename(path));
        setTitle(project, path);
      }
    });
  };
  const schedule = () => {
    if (scheduled) return;
    scheduled = true;
    setTimeout(scan, 16);
  };
  const observer = new MutationObserver(schedule);
  observer.observe(document.documentElement, { childList: true, subtree: true });
  window[key] = { version: 2, scan: schedule, observer };
  scan();
  return true;
})()
