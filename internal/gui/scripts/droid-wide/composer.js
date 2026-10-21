(() => {
  const key = '__deckDroidComposer';
  if (window[key]) {
    window[key].scan();
    return true;
  }
  const folderIcon = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"></path></svg>';
  const style = document.createElement('style');
  style.textContent = `
    [data-testid="chat-composer-actions-scroll"] [data-lesson-anchor="model-selector"] ~ :not(.deck-droid-project) { display:none !important; }
    .deck-droid-project, .deck-droid-usage { display:inline-flex; flex:none; align-items:center; gap:4px; height:28px; padding:0 6px; color:var(--core-v2-text-secondary, #9a9a9a); white-space:nowrap; font-variant-numeric:tabular-nums; }
    .deck-droid-project[hidden], .deck-droid-usage[hidden] { display:none !important; }
    .deck-droid-project svg { width:14px; height:14px; flex-shrink:0; }
    .deck-droid-usage { margin-left:auto; margin-right:4px; }
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

  let usage = { items: [], error: '' };
  const usageText = () => {
    if (usage.error) return { label: '用量 --', title: usage.error };
    if (!usage.items?.length) return null;
    return {
      label: usage.items.map(item => `${item.label} ${item.used}%`).join(' · '),
      title: ['用量', ...usage.items.map(item => `${item.label} 已用 ${item.used}%，重置于 ${item.reset || '-'}`)].join('\n'),
    };
  };
  const setText = (element, value) => {
    if (element.textContent !== value) element.textContent = value;
  };
  const setTitle = (element, value) => {
    if (element.title !== value) element.title = value;
  };

  const widgets = new Map();
  let scheduled = false;
  const scan = () => {
    scheduled = false;
    for (const [wrapper, widget] of widgets) {
      if (!wrapper.isConnected) {
        widget.project.remove();
        widget.usage.remove();
        widgets.delete(wrapper);
      }
    }
    const summary = usageText();
    document.querySelectorAll('[data-testid="chat-composer-wrapper"]').forEach(wrapper => {
      const model = wrapper.querySelector('[data-testid="chat-composer-actions-scroll"] [data-lesson-anchor="model-selector"]');
      const voice = wrapper.querySelector('button[aria-label="Start voice dictation"]');
      let widget = widgets.get(wrapper);
      if (!widget) {
        const project = document.createElement('span');
        project.className = 'deck-droid-project';
        project.innerHTML = `${folderIcon}<span></span>`;
        const usageLabel = document.createElement('span');
        usageLabel.className = 'deck-droid-usage';
        widget = { project, usage: usageLabel };
        widgets.set(wrapper, widget);
      }
      if (model) {
        if (model.nextElementSibling !== widget.project) model.after(widget.project);
        const path = resolveProject(wrapper);
        widget.project.hidden = !path;
        if (path) {
          setText(widget.project.querySelector('span'), basename(path));
          setTitle(widget.project, path);
        }
      } else {
        widget.project.remove();
      }
      // 语音按钮外层是该行的直接子元素，用量放在它前面并用 margin-left:auto 靠右。
      const voiceItem = voice?.closest('[data-testid="chat-composer-actions-scroll"] ~ *');
      if (voiceItem) {
        if (voiceItem.previousElementSibling !== widget.usage) voiceItem.before(widget.usage);
        widget.usage.hidden = !summary;
        if (summary) {
          setText(widget.usage, summary.label);
          setTitle(widget.usage, summary.title);
        }
      } else {
        widget.usage.remove();
      }
    });
  };
  const schedule = () => {
    if (scheduled) return;
    scheduled = true;
    // 窗口在后台时 requestAnimationFrame 会暂停，用量推送需要及时显示。
    setTimeout(scan, 16);
  };
  const observer = new MutationObserver(schedule);
  observer.observe(document.documentElement, { childList: true, subtree: true });
  window[key] = { scan: schedule, observer };
  window.__deckDroidUsage = {
    key: '',
    update(data, version) {
      usage = data || { items: [], error: '' };
      this.key = version;
      schedule();
      return true;
    },
  };
  scan();
  return true;
})()
