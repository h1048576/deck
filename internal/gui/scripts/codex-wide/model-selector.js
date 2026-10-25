(() => {
  const key = '__deckCodexModelSelector';
  const configured = __DECK_MODEL_SELECTOR__;
  if (window[key]?.version === 2) {
    window[key].configure(configured);
    return;
  }
  window[key]?.configure({});
  window[key]?.dispose?.();
  document.getElementById('deck-codex-model-selector-style')?.remove();
  document.getElementById('deck-codex-model-selector-panel')?.remove();
  const nativeSelector = '[data-codex-intelligence-trigger][data-composer-navigation-target="reasoning"]';
  const controls = new Map();
  let entries = [];
  let active = null;
  let scheduled = false;
  let disposed = false;
  const effortNames = { none: '无', minimal: '最低', low: '低', medium: '中', high: '高', xhigh: '极高', max: '最高', ultra: '超高' };
  const style = document.createElement('style');
  style.id = 'deck-codex-model-selector-style';
  style.textContent = `
    .deck-model-selector-trigger { display:inline-flex; align-items:center; gap:4px; flex:0 1 auto; min-width:0; max-width:240px; height:var(--height-token-button-composer,28px); padding:0 6px; border:0; border-radius:6px; background:transparent; color:var(--color-token-text-tertiary,#999); font:inherit; font-size:var(--text-sm); cursor:pointer; white-space:nowrap; -webkit-app-region:no-drag; }
    .deck-model-selector-trigger > span { overflow:hidden; text-overflow:ellipsis; }
    .deck-model-selector-trigger:hover,.deck-model-selector-trigger[aria-expanded="true"] { background:var(--color-token-bg-primary-ghost-hover,#ffffff0d); color:var(--color-token-text-primary,#eee); }
    .deck-model-selector-trigger:focus-visible,.deck-model-selector-item:focus-visible { outline:1px solid currentColor; outline-offset:-2px; }
    .deck-model-selector-trigger:disabled,.deck-model-selector-item:disabled { opacity:.4; cursor:default; }
    .deck-model-selector-trigger svg { width:12px; height:12px; flex:none; }
    .deck-model-selector-panel { position:fixed; inset:auto; margin:0; box-sizing:border-box; min-width:min(260px,calc(100vw - 16px)); max-width:calc(100vw - 16px); max-height:calc(100vh - 16px); overflow-y:auto; overscroll-behavior:contain; padding:5px; border:1px solid var(--color-token-border-default,#ffffff24); border-radius:10px; background:var(--color-token-bg-elevated,var(--color-token-bg-primary,#202020)); color:var(--color-token-text-primary,#eee); box-shadow:0 8px 28px #0004; font:inherit; font-size:var(--text-sm); -webkit-app-region:no-drag; }
    .deck-model-selector-panel:popover-open { display:grid; grid-template-columns:14px minmax(0,max-content) minmax(2em,1fr); column-gap:8px; align-content:start; }
    .deck-model-selector-item { display:grid; grid-column:1 / -1; grid-template-columns:subgrid; align-items:center; column-gap:8px; width:100%; padding:7px 9px; border:0; border-radius:6px; background:transparent; color:inherit; font:inherit; text-align:left; cursor:pointer; }
    .deck-model-selector-item:hover:not(:disabled),.deck-model-selector-item:focus { background:var(--color-token-bg-primary-ghost-hover,#ffffff0d); }
    .deck-model-selector-check { flex:none; width:14px; }
    .deck-model-selector-item-label { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
    .deck-model-selector-item-effort { white-space:nowrap; }
    .deck-model-selector-status { grid-column:1 / -1; padding:5px 9px; color:var(--color-token-text-tertiary,#999); }
  `;
  (document.head || document.documentElement).append(style);
  const panel = document.createElement('div');
  panel.id = 'deck-codex-model-selector-panel';
  panel.className = 'deck-model-selector-panel';
  panel.setAttribute('popover', 'auto');
  panel.setAttribute('role', 'menu');
  panel.setAttribute('aria-label', '选择模型');
  panel.dataset.state = 'closed';
  document.documentElement.append(panel);

  const getFiber = element => {
    let fiber = element[Object.keys(element).find(name => name.startsWith('__reactFiber'))];
    let root = fiber;
    while (root?.return) root = root.return;
    if (root?.stateNode?.current && root.stateNode.current !== root) fiber = fiber?.alternate;
    return fiber;
  };
  const context = native => {
    let picker = null;
    let selection = null;
    // 读取当前提交的 React 属性，每次操作重新获取，避免切换会话后使用旧回调。
    for (let fiber = getFiber(native); fiber; fiber = fiber.return) {
      const props = fiber.memoizedProps;
      if (!selection && typeof props?.onSelectPower === 'function' && Array.isArray(props.powerSelections)) selection = props;
      if (typeof props?.onSelectModel === 'function' && Array.isArray(props.modelOptions)) {
        picker = props;
        break;
      }
    }
    return picker && selection ? { picker, selection } : null;
  };
  const unavailable = (ctx, value) => {
    if (!ctx) return '当前页面暂不支持切换模型';
    const { picker, selection } = ctx;
    if (picker.disabled || picker.modelOptionsDisabled || picker.reasoningEffortDisabled || selection.modelSelectionDisabled || selection.powerSelectionDisabled) return '当前暂不能切换模型';
    if (picker.lockedModelSlug && picker.lockedModelSlug !== value.model) return '当前会话已锁定模型';
    const option = picker.modelOptions.find(option => option.model?.model === value.model || option.model?.id === value.model);
    if (!option) return '当前账号或连接未提供此模型';
    if (option.disabledReason) return typeof option.disabledReason === 'string' ? option.disabledReason : '此模型当前不可用';
    if (!option.model.supportedReasoningEfforts?.some(effort => effort.reasoningEffort === value.reasoningEffort)) return '此模型不支持配置的推理等级';
    return '';
  };
  const selected = (ctx, value) => ctx?.picker.model === value.model && ctx.picker.reasoningEffort === value.reasoningEffort;
  const text = (element, value) => { if (element.textContent !== value) element.textContent = value; };
  const position = () => {
    if (!active || !panel.matches(':popover-open')) return;
    const rect = active.button.getBoundingClientRect();
    if (!active.button.isConnected || !rect.width) { panel.hidePopover(); return; }
    panel.style.maxHeight = `${Math.max(0, rect.top - 16)}px`;
    panel.style.left = `${Math.max(8, Math.min(rect.left, innerWidth - panel.offsetWidth - 8))}px`;
    panel.style.top = `${Math.max(8, rect.top - panel.offsetHeight - 8)}px`;
  };
  const close = (restoreFocus = false) => {
    const button = active?.button;
    if (panel.matches(':popover-open')) panel.hidePopover();
    panel.dataset.state = 'closed';
    button?.setAttribute('aria-expanded', 'false');
    active = null;
    if (restoreFocus) button?.focus({ preventScroll: true });
  };
  const refresh = control => {
    const ctx = context(control.native);
    if (control.pendingSelection && selected(ctx, control.pendingSelection.value)) {
      control.pendingSelection = null;
      clearTimeout(control.confirmTimer);
    }
    const pending = control.pendingSelection?.value;
    const current = entries.find(([, value]) => pending
      ? value.model === pending.model && value.reasoningEffort === pending.reasoningEffort
      : selected(ctx, value));
    const fallback = ctx ? `${ctx.picker.models?.find(model => model.model === ctx.picker.model)?.displayName || ctx.picker.model} ${effortNames[ctx.picker.reasoningEffort] || ctx.picker.reasoningEffort}` : '选择模型';
    const name = current?.[0] || fallback;
    text(control.label, name);
    const title = `选择模型：${name}（Ctrl+M 循环切换）`;
    if (control.button.title !== title) control.button.title = title;
    if (control.button.getAttribute('aria-label') !== title) control.button.setAttribute('aria-label', title);
    const disabled = !ctx || !!ctx.picker.disabled;
    if (control.button.disabled !== disabled) control.button.disabled = disabled;
    if (active === control) {
      for (const row of panel.querySelectorAll('.deck-model-selector-item')) {
        const value = entries[Number(row.dataset.index)]?.[1];
        if (!value) continue;
        const checked = selected(ctx, value);
        if (row.getAttribute('aria-checked') !== String(checked)) row.setAttribute('aria-checked', String(checked));
        text(row.firstElementChild, checked ? '✓' : '');
        const reason = unavailable(ctx, value);
        if (row.disabled !== !!reason) row.disabled = !!reason;
        if (row.title !== reason) row.title = reason;
      }
    }
  };
  const choose = async (control, value, row) => {
    const ctx = context(control.native);
    if (unavailable(ctx, value)) return false;
    const pending = { value, time: Date.now() };
    control.pendingSelection = pending;
    try {
      // 与原生推理等级选项调用同一个入口，同时提交模型与推理等级。
      await ctx.selection.onSelectPower({
        id: `${value.model}:${value.reasoningEffort}`, model: value.model,
        reasoningEffort: value.reasoningEffort, isLocked: false,
      });
      close();
      refresh(control);
      // 原生组件可能稍后才提交；未实际切换时回到真实选项，不保留乐观显示。
      if (control.pendingSelection === pending) {
        clearTimeout(control.confirmTimer);
        control.confirmTimer = setTimeout(() => {
          if (disposed || control.pendingSelection !== pending) return;
          control.pendingSelection = null;
          refresh(control);
        }, 1000);
      }
      schedule();
      control.native.closest('[data-composer-footer-responsive]')?.querySelector('[contenteditable="true"]')?.focus({ preventScroll: true });
      return true;
    } catch (error) {
      if (control.pendingSelection === pending) control.pendingSelection = null;
      console.warn('[Deck] 切换模型失败：', error);
      if (!panel.matches(':popover-open')) open(control);
      row.title = '切换失败，请重试或使用原生模型菜单';
      let status = panel.querySelector('.deck-model-selector-status');
      if (!status) { status = document.createElement('div'); status.className = 'deck-model-selector-status'; status.setAttribute('role', 'status'); panel.append(status); }
      text(status, row.title);
      position();
      return false;
    }
  };
  const open = (control, last = false) => {
    if (active === control && panel.matches(':popover-open')) { close(); return; }
    close();
    active = control;
    panel.replaceChildren(...entries.map(([name, value], index) => {
      const row = document.createElement('button');
      row.type = 'button';
      row.className = 'deck-model-selector-item';
      row.dataset.index = String(index);
      row.setAttribute('role', 'menuitemradio');
      row.tabIndex = -1;
      const check = document.createElement('span');
      check.className = 'deck-model-selector-check';
      check.setAttribute('aria-hidden', 'true');
      const label = document.createElement('span');
      label.className = 'deck-model-selector-item-label';
      const effort = document.createElement('span');
      effort.className = 'deck-model-selector-item-effort';
      // 保留自定义显示名称，只把末尾已有的推理等级移到共享列中。
      const suffix = effortNames[value.reasoningEffort];
      const trimmed = name.trimEnd();
      const split = suffix && trimmed.endsWith(suffix) ? trimmed.slice(0, -suffix.length) : '';
      if (/\s$/.test(split)) {
        label.textContent = split.trimEnd();
        effort.textContent = suffix;
      } else {
        label.textContent = name;
      }
      row.setAttribute('aria-label', name);
      row.append(check, label, effort);
      row.addEventListener('click', () => choose(control, value, row));
      return row;
    }));
    refresh(control);
    panel.showPopover();
    // 原生编辑器通过此标记识别已打开的菜单，暂停拦截其导航键。
    panel.dataset.state = 'open';
    control.button.setAttribute('aria-expanded', 'true');
    position();
    const rows = [...panel.querySelectorAll('button:not(:disabled)')];
    const target = last ? rows.at(-1) : rows.find(row => row.getAttribute('aria-checked') === 'true') || rows[0];
    target?.focus({ preventScroll: true });
  };
  panel.addEventListener('toggle', () => {
    if (!panel.matches(':popover-open')) close();
  });
  const handleMenuKey = event => {
    if (!panel.contains(event.target)) return;
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close(true); return; }
    if (event.key === 'Tab') { event.stopPropagation(); close(true); return; }
    if (event.key === 'Enter' || event.key === ' ') { event.stopPropagation(); return; }
    if (!['ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key)) return;
    event.preventDefault();
    event.stopPropagation();
    const rows = [...panel.querySelectorAll('button:not(:disabled)')];
    if (!rows.length) return;
    const index = rows.indexOf(document.activeElement);
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? rows.length - 1 : (index + (event.key === 'ArrowUp' ? -1 : 1) + rows.length) % rows.length;
    rows[next].focus({ preventScroll: true });
  };
  // Codex 会在捕获阶段处理编辑器导航键，菜单先接管自身的键盘操作。
  window.addEventListener('keydown', handleMenuKey, true);
  const handleShortcut = event => {
    if (!event.ctrlKey || event.altKey || event.metaKey || event.shiftKey || event.isComposing ||
        !(event.code === 'KeyM' || event.key?.toLowerCase() === 'm')) return;
    if (document.querySelector('[role="dialog"][data-state="open"]:not([aria-modal="false"])')) return;
    const footer = event.target instanceof Element ? event.target.closest('[data-composer-footer-responsive]') : null;
    const visible = [...controls.values()].filter(control => control.native.isConnected && control.native.getClientRects().length && !control.button.disabled);
    const control = visible.find(control => control.native.closest('[data-composer-footer-responsive]') === footer) ||
      (active && visible.includes(active) ? active : visible[0]);
    if (!control || !entries.length) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    if (event.repeat) return;
    const ctx = context(control.native);
    // 连续按键可能早于 React 提交，沿用尚未显示的目标，确保每次按键都前进一项。
    const pending = control.pendingSelection;
    const current = pending && Date.now() - pending.time < 1000
      ? entries.findIndex(([, value]) => value.model === pending.value.model && value.reasoningEffort === pending.value.reasoningEffort)
      : entries.findIndex(([, value]) => selected(ctx, value));
    for (let offset = 1; offset <= entries.length; offset++) {
      const [, value] = entries[(current + offset) % entries.length];
      if (!unavailable(ctx, value)) {
        void choose(control, value, control.button);
        return;
      }
    }
  };
  window.addEventListener('keydown', handleShortcut, true);
  const scan = () => {
    scheduled = false;
    if (disposed) return;
    for (const [native, control] of controls) {
      if (!native.isConnected || !entries.length) {
        clearTimeout(control.confirmTimer);
        if (active === control) close();
        control.button.remove();
        controls.delete(native);
      }
    }
    if (!entries.length) return;
    for (const native of document.querySelectorAll(nativeSelector)) {
      const footer = native.closest('[data-composer-footer-responsive]');
      const project = footer?.querySelector('.deck-current-project');
      if (!project) continue;
      let control = controls.get(native);
      if (!control) {
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'deck-model-selector-trigger';
        button.setAttribute('aria-haspopup', 'menu');
        button.setAttribute('aria-expanded', 'false');
        button.setAttribute('aria-controls', panel.id);
        button.setAttribute('aria-keyshortcuts', 'Control+m');
        const label = document.createElement('span');
        button.append(label);
        button.insertAdjacentHTML('beforeend', '<svg aria-hidden="true" viewBox="0 0 16 16" fill="none"><path d="m4 10 4-4 4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>');
        control = { native, button, label };
        controls.set(native, control);
        button.addEventListener('click', () => open(control));
        button.addEventListener('keydown', event => {
          if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return;
          event.preventDefault();
          event.stopPropagation();
          open(control, event.key === 'ArrowUp');
        });
      }
      if (project.nextElementSibling !== control.button) project.after(control.button);
      refresh(control);
    }
    position();
  };
  const schedule = () => {
    if (scheduled) return;
    scheduled = true;
    // 只扫描模型控件，合并同批通知；后台窗口也能及时同步快捷键切换结果。
    queueMicrotask(scan);
  };
  const observer = new MutationObserver(records => {
    const relevant = records.some(record => {
      const target = record.target.nodeType === 1 ? record.target : record.target.parentElement;
      if (target?.closest('.deck-model-selector-trigger,.deck-model-selector-panel')) return false;
      if (target?.closest(nativeSelector)) return true;
      return [...record.addedNodes, ...record.removedNodes].some(node => node.nodeType === 1 &&
        (node.matches(`${nativeSelector},.deck-current-project`) || node.querySelector(`${nativeSelector},.deck-current-project`)));
    });
    if (relevant) schedule();
  });
  observer.observe(document.documentElement, {
    childList: true, subtree: true, characterData: true,
    attributes: true, attributeFilter: ['data-selected-reasoning-effort', 'disabled'],
  });
  window.addEventListener('resize', position, { passive: true });
  document.addEventListener('scroll', position, { capture: true, passive: true });
  const configure = values => {
    close();
    for (const control of controls.values()) {
      clearTimeout(control.confirmTimer);
      control.pendingSelection = null;
    }
    entries = Object.entries(values || {}).filter(([name, value]) => name.trim() && typeof value?.model === 'string' && typeof value?.reasoningEffort === 'string');
    scan();
  };
  window[key] = { version: 2, configure, scan: schedule, dispose() {
    disposed = true;
    observer.disconnect();
    window.removeEventListener('keydown', handleMenuKey, true);
    window.removeEventListener('keydown', handleShortcut, true);
    window.removeEventListener('resize', position);
    document.removeEventListener('scroll', position, true);
    close();
    for (const control of controls.values()) {
      clearTimeout(control.confirmTimer);
      control.button.remove();
    }
    controls.clear();
    panel.remove();
    style.remove();
    delete window[key];
  } };
  configure(configured);
})();
