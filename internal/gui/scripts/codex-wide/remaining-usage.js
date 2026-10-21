(() => {
  const key = '__deckRemainingUsage';
  if (window[key]) {
    window[key].scan();
    return;
  }
  const icon = '<svg aria-hidden="true" class="shrink-0 opacity-75 group-focus:opacity-100 group-hover:opacity-100" height="16" viewBox="0 0 16 16" width="16" xmlns="http://www.w3.org/2000/svg"><path d="M8.00122 1.47461C8.75515 1.47463 9.48307 1.59081 10.1672 1.80664C10.4438 1.89388 10.5972 2.18931 10.51 2.46582C10.4228 2.74215 10.1282 2.89554 9.85181 2.80859C9.26859 2.62459 8.64681 2.52541 8.00122 2.52539C4.60867 2.52539 1.85864 5.27542 1.85864 8.66797C1.8587 10.2884 2.48561 11.7614 3.51099 12.8594C3.70857 13.0713 3.69728 13.4037 3.4856 13.6016C3.27377 13.7994 2.94134 13.7878 2.74341 13.5762C1.54365 12.2914 0.807916 10.5649 0.807861 8.66797C0.807861 4.69552 4.02877 1.47461 8.00122 1.47461Z" fill="currentColor"></path> <path d="M14.2219 6.20605C14.499 6.12104 14.7931 6.27662 14.8782 6.55371C15.0836 7.22275 15.1936 7.93312 15.1936 8.66797C15.1936 10.5648 14.4588 12.2914 13.259 13.5762C13.0611 13.7879 12.7287 13.7994 12.5168 13.6016C12.3051 13.4037 12.2938 13.0713 12.4915 12.8594C13.5168 11.7614 14.1437 10.2884 14.1438 8.66797C14.1438 8.03884 14.0493 7.43252 13.8743 6.8623C13.7892 6.58519 13.9449 6.29124 14.2219 6.20605Z" fill="currentColor"></path> <path fill-rule="evenodd" clip-rule="evenodd" d="M11.8665 4.08203C12.0714 3.87724 12.4046 3.87721 12.6096 4.08203C12.8143 4.28693 12.8141 4.61924 12.6096 4.82422L9.77368 7.65918C9.94335 7.95688 10.0413 8.30086 10.0413 8.66797C10.0411 9.79469 9.12794 10.7079 8.00122 10.708C6.8744 10.708 5.96032 9.79476 5.96021 8.66797C5.96021 7.54108 6.87433 6.62695 8.00122 6.62695C8.37948 6.62699 8.73272 6.73163 9.03638 6.91113L11.8665 4.08203ZM8.00122 7.67773C7.45423 7.67773 7.01099 8.12098 7.01099 8.66797C7.0111 9.21486 7.4543 9.6582 8.00122 9.6582C8.54804 9.65809 8.99134 9.21479 8.99146 8.66797C8.99146 8.12105 8.54811 7.67785 8.00122 7.67773Z" fill="currentColor"></path></svg>';
  const controls = new Map();
  let scheduled = false;
  let active = null;
  const style = document.createElement('style');
  style.textContent = `
    .deck-usage-trigger { display:inline-flex; align-items:center; gap:4px; flex-shrink:0; white-space:nowrap; font-variant-numeric:tabular-nums; }
    .deck-usage-trigger svg { width:16px; height:16px; flex-shrink:0; }
    .deck-usage-panel { position:fixed; inset:auto; margin:0; box-sizing:border-box; width:248px; max-width:calc(100vw - 16px); padding:12px; border:1px solid var(--border, #444); border-radius:12px; background:var(--color-token-bg-primary, #181818); color:var(--color-token-text-primary, #eee); box-shadow:0 8px 28px #0004; }
    .deck-usage-heading { display:flex; align-items:center; gap:7px; margin-bottom:10px; }
    .deck-usage-heading svg { width:16px; height:16px; }
    .deck-usage-row { display:flex; justify-content:space-between; gap:12px; padding:4px 0 4px 23px; white-space:nowrap; }
    .deck-usage-value { color:var(--color-token-text-tertiary, #999); font-variant-numeric:tabular-nums; }
  `;
  (document.head || document.documentElement).appendChild(style);
  const panel = document.createElement('div');
  panel.className = 'deck-usage-panel';
  panel.id = 'deck-remaining-usage-panel';
  panel.setAttribute('popover', 'auto');
  panel.setAttribute('role', 'dialog');
  panel.setAttribute('aria-label', '剩余用量');
  document.documentElement.appendChild(panel);

  const text = (element, value) => {
    if (element.textContent !== value) element.textContent = value;
  };
  const findClient = (element) => {
    const fiberKey = Object.keys(element).find(name => name.startsWith('__reactFiber'));
    for (let fiber = element[fiberKey]; fiber; fiber = fiber.return) {
      const props = fiber.memoizedProps;
      const client = props?.client ?? props?.queryClient;
      if (typeof client?.getQueryCache === 'function' &&
          client.getQueryCache().find({ queryKey: ['rate-limit-status'], exact: true })) return client;
    }
    return null;
  };
  const getQuery = (client) => client?.getQueryCache().find({ queryKey: ['rate-limit-status'], exact: true });
  const windows = (client) => {
    const query = getQuery(client);
    const limit = query?.state.status === 'error' ? null : query?.state.data?.rate_limit;
    return [limit?.primary_window, limit?.secondary_window];
  };
  const percent = (window) => Number.isFinite(window?.used_percent)
    ? `${Math.round(Math.max(0, Math.min(100, 100 - window.used_percent)))}%` : '--%';
  const reset = (window) => {
    if (!Number.isFinite(window?.reset_at)) return '--';
    const date = new Date(window.reset_at * 1000);
    return window.limit_window_seconds >= 86400
      ? `${date.getMonth() + 1}月${date.getDate()}日`
      : date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false });
  };
  const label = (window, index) => {
    const seconds = window?.limit_window_seconds;
    if (!Number.isFinite(seconds)) return index === 0 ? '5 小时' : '1 周';
    if (seconds % 604800 === 0) return `${seconds / 604800} 周`;
    if (seconds % 86400 === 0) return `${seconds / 86400} 天`;
    if (seconds % 3600 === 0) return `${seconds / 3600} 小时`;
    return `${Math.round(seconds / 60)} 分钟`;
  };
  panel.innerHTML = `<div class="deck-usage-heading">${icon}<span>剩余用量</span></div>`;
  const rows = [0, 1].map(() => {
    const row = document.createElement('div');
    row.className = 'deck-usage-row';
    const name = document.createElement('span');
    const value = document.createElement('span');
    value.className = 'deck-usage-value';
    row.append(name, value);
    panel.append(row);
    return { name, value };
  });
  const position = () => {
    if (!active) return;
    const rect = active.button.getBoundingClientRect();
    if (!active.button.isConnected || !rect.width) {
      panel.hidePopover();
      return;
    }
    panel.style.left = `${Math.max(8, Math.min(rect.left, innerWidth - panel.offsetWidth - 8))}px`;
    panel.style.top = `${Math.max(8, rect.top - panel.offsetHeight - 8)}px`;
  };
  const render = (control) => {
    const buckets = windows(control.client);
    text(control.value, buckets.map(percent).join('|'));
    control.button.setAttribute('aria-label', `剩余用量：${buckets.map((w, i) => `${label(w, i)} ${percent(w)}`).join('，')}`);
    if (active === control) {
      buckets.forEach((w, i) => {
        text(rows[i].name, label(w, i));
        text(rows[i].value, `${percent(w)} ${reset(w)}`);
      });
      position();
    }
  };
  const refresh = (control) => {
    const query = getQuery(control.client);
    if (query && query.state.fetchStatus !== 'fetching' &&
        Date.now() - query.state.dataUpdatedAt > 60000) {
      query.fetch().catch(() => {});
    }
  };
  panel.addEventListener('toggle', () => {
    if (!panel.matches(':popover-open') && active) {
      active.button.setAttribute('aria-expanded', 'false');
      active = null;
    }
  });
  const scan = () => {
    scheduled = false;
    for (const [model, control] of controls) {
      if (!model.isConnected) {
        control.unsubscribe?.();
        control.button.remove();
        controls.delete(model);
        if (active === control) panel.hidePopover();
      }
    }
    document.querySelectorAll('[data-codex-intelligence-trigger]').forEach(model => {
      let control = controls.get(model);
      if (!control) {
        const footer = model.closest('[data-composer-footer-responsive]');
        if (!footer) return;
        const context = footer.querySelector('[aria-label^="上下文用量"],[aria-label^="Context usage"]');
        let anchor = context || model;
        while (anchor.parentElement !== footer && anchor.parentElement &&
            (anchor.parentElement.classList.contains('contents') ||
             anchor.parentElement.tagName === 'SPAN')) anchor = anchor.parentElement;
        const button = document.createElement('button');
        button.type = 'button';
        button.className = `${model.className} deck-usage-trigger`;
        button.setAttribute('aria-haspopup', 'dialog');
        button.setAttribute('aria-expanded', 'false');
        button.setAttribute('aria-controls', panel.id);
        button.innerHTML = icon;
        const value = document.createElement('span');
        button.append(value);
        anchor.before(button);
        control = { button, value, client: null, unsubscribe: null };
        controls.set(model, control);
        button.addEventListener('click', () => {
          if (active === control && panel.matches(':popover-open')) {
            panel.hidePopover();
            return;
          }
          active?.button.setAttribute('aria-expanded', 'false');
          active = control;
          render(control);
          panel.showPopover();
          button.setAttribute('aria-expanded', 'true');
          position();
          refresh(control);
        });
      }
      const client = findClient(model);
      if (client !== control.client) {
        control.unsubscribe?.();
        control.client = client;
        control.unsubscribe = client?.getQueryCache().subscribe(event => {
          if (event.query?.queryKey.length === 1 && event.query.queryKey[0] === 'rate-limit-status') render(control);
        });
      }
      render(control);
    });
  };
  const schedule = () => {
    if (!scheduled) {
      scheduled = true;
      requestAnimationFrame(scan);
    }
  };
  const observer = new MutationObserver(schedule);
  observer.observe(document.documentElement, { childList: true, subtree: true });
  window.addEventListener('resize', position);
  document.addEventListener('scroll', position, true);
  const timer = setInterval(() => {
    if (document.visibilityState !== 'visible') return;
    scan();
    const refreshed = new Set();
    for (const control of controls.values()) {
      if (!refreshed.has(control.client)) {
        refreshed.add(control.client);
        refresh(control);
      }
    }
  }, 60000);
  window[key] = { scan: schedule, observer, timer };
  scan();
})();
