(() => {
  const key = '__deckContextMeter';
  if (window[key]?.version === 1) {
    window[key].scan();
    return true;
  }
  window[key]?.dispose?.();
  document.getElementById('deck-context-meter-style')?.remove();
  document.getElementById('deck-context-meter-tip')?.remove();

  const nativeSelector = 'form [data-chat-provider-model-picker]';
  const radius = 7;
  const circumference = 2 * Math.PI * radius;
  const controls = new Map();
  let active = null;
  let scheduled = false;
  let disposed = false;

  const style = document.createElement('style');
  style.id = 'deck-context-meter-style';
  style.textContent = `
    .deck-context-meter { display:inline-flex; align-items:center; justify-content:center; flex:0 0 auto; width:28px; height:28px; padding:0; border:0; border-radius:6px; background:transparent; color:var(--color-token-text-tertiary,#999); cursor:default; -webkit-app-region:no-drag; }
    .deck-context-meter[hidden] { display:none; }
    .deck-context-meter:hover,.deck-context-meter:focus-visible { background:var(--color-token-bg-primary-ghost-hover,#ffffff0d); color:var(--color-token-text-primary,#eee); outline:none; }
    .deck-context-meter svg { width:18px; height:18px; flex:none; transform:rotate(-90deg); }
    .deck-context-meter circle { fill:none; stroke-width:2.2; }
    .deck-context-meter .deck-context-track { stroke:currentColor; opacity:.25; }
    .deck-context-meter .deck-context-value { stroke:currentColor; stroke-linecap:round; transition:stroke-dashoffset .3s; }
    .deck-context-meter[data-level="warn"] .deck-context-value { stroke:#f59e0b; }
    .deck-context-meter[data-level="danger"] .deck-context-value { stroke:#ef4444; }
    .deck-context-tip { position:fixed; inset:auto; margin:0; box-sizing:border-box; width:max-content; max-width:calc(100vw - 16px); padding:8px 12px; border:1px solid var(--color-token-border-default,#ffffff24); border-radius:10px; background:var(--popover,var(--color-token-bg-primary,#202020)); color:var(--color-token-text-primary,#eee); box-shadow:0 8px 28px #0004; font:inherit; font-size:var(--text-sm,12px); line-height:1.6; pointer-events:none; }
    .deck-context-tip-title { font-weight:600; }
    .deck-context-tip-row { display:flex; justify-content:space-between; gap:20px; color:var(--color-token-text-tertiary,#999); }
    .deck-context-tip-row > span:last-child { color:var(--color-token-text-primary,#eee); font-variant-numeric:tabular-nums; }
    .deck-context-tip-sep { height:1px; margin:4px 0; background:var(--color-token-border-default,#ffffff24); }
  `;
  (document.head || document.documentElement).append(style);

  const tip = document.createElement('div');
  tip.id = 'deck-context-meter-tip';
  tip.className = 'deck-context-tip';
  tip.setAttribute('popover', 'manual');
  tip.setAttribute('role', 'tooltip');
  document.documentElement.append(tip);

  // React 为每个组件保留新旧两份 fiber，元素上记录的那份可能落后一次渲染，
  // 因此从当前根节点沿祖先路径向下取已提交的 fiber。
  const currentFibers = element => {
    const start = element[Object.keys(element).find(name => name.startsWith('__reactFiber'))];
    const path = [];
    for (let fiber = start; fiber; fiber = fiber.return) path.unshift(fiber);
    const root = path[0];
    let current = root?.stateNode?.current;
    if (!current || (current !== root && current !== root.alternate)) return path.reverse();
    const chain = [current];
    for (let depth = 1; depth < path.length; depth++) {
      let next = current.child;
      while (next && next !== path[depth] && next !== path[depth].alternate) next = next.sibling;
      if (!next) break;
      chain.push(next);
      current = next;
    }
    return chain.reverse();
  };
  const usageOf = native => {
    for (const fiber of currentFibers(native)) {
      const props = fiber.memoizedProps;
      if (props && typeof props === 'object' && 'contextWindowLiveUsage' in props) return props.contextWindowLiveUsage || null;
    }
    return null;
  };

  const number = value => typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : null;
  const compact = value => {
    if (value >= 1e6) return `${(value / 1e6).toFixed(1).replace(/\.0$/, '')}M`;
    if (value >= 1e3) return `${(value / 1e3).toFixed(1).replace(/\.0$/, '')}k`;
    return String(value);
  };
  const percent = ratio => {
    const value = ratio * 100;
    return value > 0 && value < 1 ? '<1%' : `${value < 10 ? value.toFixed(1).replace(/\.0$/, '') : Math.round(value)}%`;
  };
  const describe = usage => {
    const used = number(usage?.usedTokens);
    if (used === null) return null;
    const max = number(usage.maxTokens) || 0;
    const ratio = max ? Math.min(used / max, 1) : 0;
    const rows = [];
    if (max) rows.push(['剩余', `${compact(Math.max(max - used, 0))} tokens`]);
    const input = number(usage.inputTokens);
    if (input !== null) rows.push(['输入', number(usage.cachedInputTokens) ? `${input.toLocaleString()}（缓存 ${usage.cachedInputTokens.toLocaleString()}）` : input.toLocaleString()]);
    const output = number(usage.outputTokens);
    if (output !== null) rows.push(['输出', output.toLocaleString()]);
    const reasoning = number(usage.reasoningOutputTokens);
    if (reasoning) rows.push(['推理', reasoning.toLocaleString()]);
    const time = usage.updatedAt ? new Date(usage.updatedAt) : null;
    if (time && !Number.isNaN(time.getTime())) rows.push(['更新于', time.toLocaleTimeString('zh-CN', { hour12: false })]);
    const summary = max ? `已用 ${percent(ratio)}（${compact(used)} / ${compact(max)} tokens）` : `已用 ${compact(used)} tokens`;
    return { ratio, summary, rows, level: ratio >= 0.95 ? 'danger' : ratio >= 0.8 ? 'warn' : 'normal',
      signature: JSON.stringify([used, max, rows]) };
  };

  const fillTip = info => {
    const title = document.createElement('div');
    title.className = 'deck-context-tip-title';
    title.textContent = '上下文窗口';
    const summary = document.createElement('div');
    summary.textContent = info.summary;
    const separator = document.createElement('div');
    separator.className = 'deck-context-tip-sep';
    const rows = info.rows.map(([name, value]) => {
      const row = document.createElement('div');
      row.className = 'deck-context-tip-row';
      const left = document.createElement('span');
      const right = document.createElement('span');
      left.textContent = name;
      right.textContent = value;
      row.append(left, right);
      return row;
    });
    tip.replaceChildren(title, summary, ...(rows.length ? [separator, ...rows] : []));
  };
  const placeTip = () => {
    if (!active || !tip.matches(':popover-open')) return;
    const rect = active.button.getBoundingClientRect();
    if (!active.button.isConnected || !rect.width) { hide(); return; }
    const left = Math.max(8, Math.min(rect.right - tip.offsetWidth, innerWidth - tip.offsetWidth - 8));
    tip.style.left = `${left}px`;
    tip.style.top = `${Math.max(8, rect.top - tip.offsetHeight - 8)}px`;
  };
  const hide = () => {
    if (tip.matches(':popover-open')) tip.hidePopover();
    active = null;
  };
  const show = control => {
    if (!control.info) return;
    active = control;
    fillTip(control.info);
    if (!tip.matches(':popover-open')) tip.showPopover();
    placeTip();
  };

  const refresh = control => {
    const info = describe(usageOf(control.native));
    if (!info) {
      control.info = null;
      control.button.hidden = true;
      if (active === control) hide();
      return;
    }
    control.button.hidden = false;
    if (control.info?.signature === info.signature) return;
    control.info = info;
    control.value.setAttribute('stroke-dashoffset', String(circumference * (1 - info.ratio)));
    control.button.dataset.level = info.level;
    control.button.setAttribute('aria-label', `上下文窗口：${info.summary}`);
    if (active === control) { fillTip(info); placeTip(); }
  };

  const scan = () => {
    scheduled = false;
    if (disposed) return;
    for (const [native, control] of controls) {
      if (!native.isConnected || !native.getClientRects().length) {
        if (active === control) hide();
        control.button.remove();
        controls.delete(native);
      }
    }
    for (const native of document.querySelectorAll(nativeSelector)) {
      if (!native.getClientRects().length) continue;
      let control = controls.get(native);
      if (!control) {
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'deck-context-meter';
        button.hidden = true;
        button.innerHTML = `<svg viewBox="0 0 20 20" aria-hidden="true"><circle class="deck-context-track" cx="10" cy="10" r="${radius}"/><circle class="deck-context-value" cx="10" cy="10" r="${radius}" stroke-dasharray="${circumference}" stroke-dashoffset="${circumference}"/></svg>`;
        control = { native, button, value: button.querySelector('.deck-context-value'), info: null };
        controls.set(native, control);
        const current = control;
        button.addEventListener('mouseenter', () => show(current));
        button.addEventListener('focus', () => show(current));
        button.addEventListener('mouseleave', hide);
        button.addEventListener('blur', hide);
      }
      if (native.previousElementSibling !== control.button) native.before(control.button);
      refresh(control);
    }
  };
  const schedule = () => {
    if (scheduled) return;
    scheduled = true;
    requestAnimationFrame(scan);
  };

  const observer = new MutationObserver(records => {
    const relevant = records.some(record => {
      const target = record.target.nodeType === 1 ? record.target : record.target.parentElement;
      if (target?.closest('.deck-context-meter,.deck-context-tip')) return false;
      return [...record.addedNodes, ...record.removedNodes].some(node => node.nodeType === 1 &&
        (node.matches('[data-chat-provider-model-picker]') || node.querySelector('[data-chat-provider-model-picker]')));
    });
    if (relevant) schedule();
  });
  observer.observe(document.documentElement, { childList: true, subtree: true });
  // 用量由 React 状态更新，不一定引起 DOM 变化，需要定时读取。
  const timer = setInterval(() => {
    if (document.visibilityState === 'visible') schedule();
  }, 1000);
  window.addEventListener('resize', placeTip, { passive: true });

  window[key] = { version: 1, scan: schedule, dispose() {
    disposed = true;
    observer.disconnect();
    clearInterval(timer);
    window.removeEventListener('resize', placeTip);
    hide();
    for (const control of controls.values()) control.button.remove();
    controls.clear();
    tip.remove();
    style.remove();
    delete window[key];
  } };
  scan();
  return true;
})()
