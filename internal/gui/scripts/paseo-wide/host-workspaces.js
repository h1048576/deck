(() => {
  const key = '__deckPaseoHostWorkspaces';
  if (window[key]) {
    window[key].scan();
    return true;
  }
  const selector = '[data-testid^="sidebar-host-row-"]';
  const prefix = 'sidebar-host-row-';
  let store;
  let loading;
  let lastAttempt = 0;
  let scheduled = false;
  const controls = new Map();
  const style = document.createElement('style');
  style.textContent = `
    .deck-host-workspace-radio { appearance:none; box-sizing:border-box; width:14px; height:14px; min-width:14px; padding:0; margin:0 8px 0 0; border:1px solid currentColor; border-radius:50%; color:inherit; background:transparent; cursor:pointer; flex-shrink:0; align-self:center; }
    .deck-host-workspace-radio:checked { background:currentColor; box-shadow:inset 0 0 0 3px var(--deck-host-radio-background,#171717); }
    .deck-host-workspace-radio:focus-visible { outline:2px solid currentColor; outline-offset:3px; }
    .deck-host-workspace-radio:disabled { opacity:.4; cursor:wait; }
  `;
  (document.head || document.documentElement).append(style);

  // 从当前安装版本的 Metro bundle 定位导出，避免依赖随版本变化的模块编号。
  const resolveStore = async () => {
    if (store) return store;
    if (loading) return loading;
    lastAttempt = Date.now();
    loading = (async () => {
      if (typeof window.__r !== 'function') return;
      for (const script of document.querySelectorAll('script[src]')) {
        const url = new URL(script.src, location.href);
        if (url.origin !== location.origin || !url.pathname.endsWith('.js')) continue;
        const cacheKey = `deck-paseo-sidebar-module:${url.href}`;
        const connect = id => {
          const candidate = window.__r(Number(id))?.useSidebarViewStore;
          const state = candidate?.getState?.();
          if (!Array.isArray(state?.hostFilters) || typeof candidate.setState !== 'function' ||
              typeof candidate.subscribe !== 'function') return;
          store = candidate;
          window[key].unsubscribe = store.subscribe(schedule);
          return store;
        };
        // bundle URL 包含版本哈希，重载时可安全复用模块定位结果。
        try {
          const cached = sessionStorage.getItem(cacheKey);
          if (cached && connect(cached)) return store;
        } catch { /* 缓存不可用时继续从 bundle 定位。 */ }
        const response = await fetch(url.href);
        if (!response.ok) continue;
        const source = await response.text();
        const exportAt = source.indexOf('"useSidebarViewStore",{');
        if (exportAt < 0) continue;
        const end = source.indexOf('\n__d(', exportAt);
        const moduleSource = source.slice(exportAt, end < 0 ? undefined : end);
        const match = moduleSource.match(/\},(\d+),\[[\d,]*\]\);\s*$/);
        if (!match) continue;
        if (!connect(match[1])) continue;
        try { sessionStorage.setItem(cacheKey, match[1]); } catch { /* 缓存为可选优化。 */ }
        return store;
      }
    })().catch(error => {
      console.warn('[Deck] 无法连接 Paseo 工作区筛选状态', error);
    }).finally(() => {
      loading = null;
    });
    return loading;
  };

  const scan = () => {
    scheduled = false;
    for (const [row, radio] of controls) {
      if (!row.isConnected) {
        radio.remove();
        controls.delete(row);
      }
    }
    const rows = document.querySelectorAll(selector);
    if (!rows.length) return;
    if (!store && !loading && Date.now() - lastAttempt > 5000) resolveStore().then(schedule);
    const selected = store?.getState().hostFilters || [];
    for (const row of rows) {
      const hostId = row.getAttribute('data-testid').slice(prefix.length);
      if (!hostId) continue;
      let radio = controls.get(row);
      if (!radio) {
        radio = document.createElement('input');
        radio.type = 'radio';
        radio.name = 'deck-paseo-workspace-host';
        radio.className = 'deck-host-workspace-radio';
        // 阻止原生菜单行的点击处理打开主机设置。
        for (const type of ['pointerdown', 'pointerup', 'keydown', 'keyup']) {
          radio.addEventListener(type, event => event.stopPropagation());
        }
        radio.addEventListener('click', event => event.stopPropagation());
        radio.addEventListener('change', () => {
          if (!store || !radio.checked) return;
          const currentId = row.getAttribute('data-testid').slice(prefix.length);
          // 一次更新原生持久化状态，同时清除可能遮住目标主机工作区的项目筛选。
          store.setState({ hostFilters: [currentId], projectFilters: [], labelFilter: { labels: [] } });
          schedule();
        });
        controls.set(row, radio);
      }
      if (row.firstChild !== radio) row.prepend(radio);
      radio.disabled = !store;
      radio.checked = selected.length === 1 && selected[0] === hostId;
      const label = `显示 ${row.textContent.trim()} 的工作区`;
      if (radio.getAttribute('aria-label') !== label) radio.setAttribute('aria-label', label);
      radio.title = store ? label : '正在连接工作区筛选';
    }
  };
  const schedule = () => {
    if (scheduled) return;
    scheduled = true;
    // 在菜单插入 DOM 的同一轮微任务中补齐控件，避免再等待一帧。
    queueMicrotask(scan);
  };
  const selectRow = event => {
    if (!(event.target instanceof Element)) return;
    const row = event.target.closest(selector);
    if (!row) return;
    // 原生齿轮的无障碍名称为 Open <host> settings，保留该入口。
    const settings = event.target.closest('[aria-label^="Open "][aria-label$=" settings"]');
    if (settings && row.contains(settings)) return;
    if (event.type === 'keydown' && !['Enter', ' '].includes(event.key)) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    if (!store) return;
    const hostId = row.getAttribute('data-testid').slice(prefix.length);
    if (!hostId) return;
    store.setState({ hostFilters: [hostId], projectFilters: [], labelFilter: { labels: [] } });
    // 等原生 radio 的取消默认动作完成后，再同步选中状态。
    schedule();
  };
  // 在 React 的菜单行处理之前接管点击，名称、地址和单选按钮行为一致。
  document.addEventListener('click', selectRow, true);
  document.addEventListener('keydown', selectRow, true);
  const observer = new MutationObserver(schedule);
  observer.observe(document.documentElement, { childList: true, subtree: true, characterData: true });
  window[key] = { scan: schedule, observer };
  resolveStore().then(schedule);
  scan();
  return true;
})()
