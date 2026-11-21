function installComposerUsage(app, loadUsage) {
  const key = '__deckComposerUsage';
  if (window[key]) {
    window[key].schedule();
    return true;
  }

  const droid = app === 'droid';
  const selector = droid
    ? '[data-testid="chat-composer-wrapper"] [data-lesson-anchor="model-selector"]'
    : '[data-agent-composer-root] [data-testid="model-selector-trigger"]';
  const icon = '<svg aria-hidden="true" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"><path d="M3 13a6.6 6.6 0 1 1 10 0M8 9l3.5-4"/><circle cx="8" cy="9" r="1.1"/></svg>';
  const style = document.createElement('style');
  style.textContent = `
    .deck-composer-usage {
      display:inline-flex; align-items:center; flex:0 0 auto; gap:4px; min-height:24px;
      padding:0 6px; border:0; border-radius:6px; background:transparent; cursor:pointer;
      font:inherit; white-space:nowrap; font-variant-numeric:tabular-nums;
      color:var(--secondary-foreground, var(--core-v2-text-secondary, #999));
    }
    .deck-composer-usage:hover, .deck-composer-usage[aria-expanded="true"] {
      background:var(--secondary, color-mix(in srgb, currentColor 10%, transparent));
    }
    .deck-composer-usage:focus-visible { outline:1px solid currentColor; outline-offset:-1px; }
    .deck-composer-usage svg, .deck-composer-usage-heading svg { width:16px; height:16px; flex:none; }
    .deck-composer-usage-panel {
      position:fixed; inset:auto; margin:0; box-sizing:border-box; width:max-content;
      min-width:min(248px, calc(100vw - 16px)); max-width:calc(100vw - 16px);
      max-height:min(60vh, calc(100vh - 16px)); overflow:auto; padding:12px;
      border:1px solid var(--border, color-mix(in srgb, currentColor 22%, transparent));
      border-radius:12px; background:var(--popover, var(--background, var(--core-v2-bg-surface, Canvas)));
      color:var(--popover-foreground, var(--foreground, var(--core-v2-text-primary, CanvasText)));
      box-shadow:0 8px 28px #0004;
    }
    .deck-composer-usage-heading { display:flex; align-items:center; gap:7px; margin-bottom:8px; }
    .deck-composer-usage-row { display:grid; grid-template-columns:minmax(0, 1fr) auto; gap:12px; padding:4px 0 4px 23px; }
    .deck-composer-usage-value { opacity:.7; white-space:nowrap; font-variant-numeric:tabular-nums; }
  `;
  (document.head || document.documentElement).append(style);
  const panel = document.createElement('div');
  panel.id = 'deck-composer-usage-panel';
  panel.className = 'deck-composer-usage-panel no-focus-agent-input';
  panel.setAttribute('popover', 'auto');
  panel.setAttribute('role', 'dialog');
  panel.setAttribute('aria-label', '剩余用量');
  panel.innerHTML = `<div class="deck-composer-usage-heading">${icon}<span>剩余用量</span></div>`;
  const details = document.createElement('div');
  panel.append(details);
  document.documentElement.append(panel);

  const controls = new Map();
  let active = null;
  let report = null;
  let refreshing = false;
  let requestedAt = 0;
  let frame = 0;
  const remaining = item => Number.isFinite(item?.used) ? Math.round(Math.max(0, Math.min(100, 100 - item.used))) : null;
  const percent = value => value === null ? '--%' : `${value}%`;
  const labels = { '5h': '5 小时', '7d': '1 周', '1m': '月度' };
  const resetLabel = value => {
    if (!value || value === '-' || value === '—') return '—';
    const date = /^(\d{4})[-/](\d{1,2})[-/](\d{1,2})\s+(\d{2}:\d{2})/.exec(value);
    if (!date) return value;
    const [, year, month, day, time] = date;
    const today = new Date();
    return +year === today.getFullYear() && +month === today.getMonth() + 1 && +day === today.getDate()
      ? time : `${+month}月${+day}日 ${time}`;
  };

  const renderButton = ({ button, value }) => {
    const items = report?.items || [];
    // 按配额 ID 匹配 Gemini 公共配额，避免读到单独计量的 Argon 或第三方模型。
    const summaryItems = droid ? items : ['gemini-5h', 'gemini-weekly'].map(id => items.find(item => item.bucketId === id));
    const amounts = summaryItems.map(item => percent(remaining(item)));
    const summary = amounts.join('|') || '--%';
    if (value.textContent !== summary) value.textContent = summary;
    const description = droid
      ? items.map((item, i) => `${labels[item.label] || item.label} ${amounts[i]}`).join('，')
      : `Gemini 5 小时 ${amounts[0]}，每周 ${amounts[1]}`;
    const label = description ? `剩余用量：${description}` : '剩余用量';
    if (button.getAttribute('aria-label') !== label) button.setAttribute('aria-label', label);
    if (!droid) button.title = description || label;
  };

  const position = () => {
    if (!active || !panel.matches(':popover-open')) return;
    const rect = active.button.getBoundingClientRect();
    if (!active.button.isConnected || rect.width === 0) {
      panel.hidePopover();
      return;
    }
    const left = droid ? rect.left : rect.right - panel.offsetWidth;
    panel.style.left = `${Math.max(8, Math.min(left, innerWidth - panel.offsetWidth - 8))}px`;
    const above = rect.top - panel.offsetHeight - 8;
    panel.style.top = `${above >= 8 ? above : Math.max(8, Math.min(rect.bottom + 8, innerHeight - panel.offsetHeight - 8))}px`;
  };

  const renderPanel = () => {
    const items = report?.items || [];
    details.replaceChildren();
    if (!items.length) {
      details.textContent = refreshing || !report ? '正在查询用量…' : report.error || '暂无用量数据';
    }
    for (const item of items) {
      const row = document.createElement('div');
      row.className = 'deck-composer-usage-row';
      const name = document.createElement('span');
      name.textContent = labels[item.label] || item.label;
      const value = document.createElement('span');
      value.className = 'deck-composer-usage-value';
      value.textContent = `${percent(remaining(item))}  ${resetLabel(item.reset)}`;
      value.title = item.reset && item.reset !== '-' ? `重置时间：${item.reset}` : '重置时间未知';
      row.append(name, value);
      details.append(row);
    }
    position();
  };
  const update = data => {
    report = data;
    controls.forEach(renderButton);
    if (active) renderPanel();
  };

  const refresh = async (opened = false) => {
    const interval = opened && report?.error ? 5000 : 60000;
    if (!loadUsage || !controls.size || refreshing || Date.now() - requestedAt < interval) return;
    requestedAt = Date.now();
    refreshing = true;
    if (active) renderPanel();
    let data;
    try {
      data = await loadUsage();
    } catch {
      data = { error: '用量查询失败，请稍后重试' };
    } finally {
      refreshing = false;
    }
    update(data || { items: [] });
  };

  panel.addEventListener('toggle', () => {
    if (active && !panel.matches(':popover-open')) {
      active.button.setAttribute('aria-expanded', 'false');
      active = null;
    }
  });
  const scan = () => {
    const anchors = new Map();
    for (const model of document.querySelectorAll(selector)) {
      const anchor = droid ? model : model.closest('[data-agent-composer-root]')?.querySelector(':scope > .shrink-0');
      if (anchor) anchors.set(model, anchor);
    }
    for (const [model, control] of controls) {
      if (anchors.has(model)) continue;
      control.button.remove();
      controls.delete(model);
      if (active === control && panel.matches(':popover-open')) panel.hidePopover();
    }
    for (const [model, anchor] of anchors) {
      let control = controls.get(model);
      if (!control) {
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'deck-composer-usage no-focus-agent-input';
        button.setAttribute('aria-haspopup', 'dialog');
        button.setAttribute('aria-expanded', 'false');
        button.setAttribute('aria-controls', panel.id);
        button.innerHTML = icon;
        const value = document.createElement('span');
        button.append(value);
        control = { button, value };
        controls.set(model, control);
        renderButton(control);
        button.addEventListener('click', () => {
          if (active === control && panel.matches(':popover-open')) {
            panel.hidePopover();
            return;
          }
          active?.button.setAttribute('aria-expanded', 'false');
          active = control;
          renderPanel();
          if (!panel.matches(':popover-open')) panel.showPopover();
          button.setAttribute('aria-expanded', 'true');
          position();
          void refresh(true);
        });
      }
      if (droid && anchor.previousElementSibling !== control.button) anchor.before(control.button);
      if (!droid && anchor.firstElementChild !== control.button) anchor.prepend(control.button);
    }
    if (document.visibilityState === 'visible') void refresh();
  };
  const schedule = () => {
    if (frame) return;
    frame = requestAnimationFrame(() => {
      frame = 0;
      scan();
    });
  };
  const observer = new MutationObserver(schedule);
  observer.observe(document.documentElement, { childList:true, subtree:true });
  window.addEventListener('resize', position);
  document.addEventListener('scroll', position, true);
  if (loadUsage) {
    document.addEventListener('visibilitychange', schedule);
    setInterval(() => {
      if (document.visibilityState === 'visible') void refresh();
    }, 60000);
  }
  if (droid) {
    window.__deckDroidUsage = {
      key: '',
      update(data, version) {
        this.key = version;
        update(data);
        return true;
      },
    };
  }
  window[key] = { schedule };
  scan();
  return true;
}
