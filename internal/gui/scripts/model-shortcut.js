(() => {
  const app = '__DECK_PROJECT_APP__';
  // 旧版本曾对 T3Code 注入原生模型名称缩短脚本，升级时停止该观察器。
  if (app === 't3code' && window.__deckZcodeModelLabel) {
    window.__deckZcodeModelLabel.observer?.disconnect();
    delete window.__deckZcodeModelLabel;
  }
  const key = `__deckConfiguredModelSelector_${app}`;
  const old = window[`__deckModelShortcut_${app}`];
  if (old) { window.removeEventListener('keydown', old.handle, true); delete window[`__deckModelShortcut_${app}`]; }
  const configured = __DECK_MODEL_SELECTOR__;
  if (window[key]?.version === 21) {
    window[key].configure(configured);
    return true;
  }
  window[key]?.configure({});
  window[key]?.dispose?.();
  document.getElementById('deck-codex-model-selector-style')?.remove();
  document.getElementById('deck-codex-model-selector-panel')?.remove();
  // 新版 Codex 已移除 intelligence-trigger，保留新旧版本共有的导航标记。
  const nativeSelector = app === 't3code' ? '[data-chat-provider-model-picker]' : '[data-testid="combined-model-selector"]';
  const controls = new Map();
  let entries = [];
  let active = null;
  let scheduled = false;
  let disposed = false;
  const style = document.createElement('style');
  style.id = 'deck-codex-model-selector-style';
  style.textContent = `
    .deck-model-selector-trigger { display:inline-flex; align-items:center; gap:4px; flex:0 0 auto; min-width:0; max-width:calc(100vw - 32px); height:var(--height-token-button-composer,28px); padding:0 6px; border:0; border-radius:6px; background:transparent; color:var(--color-token-text-tertiary,#999); font:inherit; font-size:var(--text-sm); cursor:pointer; white-space:nowrap; -webkit-app-region:no-drag; }
    .deck-model-selector-trigger > span { overflow:hidden; text-overflow:ellipsis; }
    .deck-model-selector-trigger:hover,.deck-model-selector-trigger[aria-expanded="true"] { background:var(--color-token-bg-primary-ghost-hover,#ffffff0d); color:var(--color-token-text-primary,#eee); }
    .deck-model-selector-trigger:focus-visible,.deck-model-selector-item:focus-visible { outline:1px solid currentColor; outline-offset:-2px; }
    .deck-model-selector-trigger:disabled,.deck-model-selector-item:disabled { opacity:.4; cursor:default; }
    .deck-model-selector-trigger svg { width:12px; height:12px; flex:none; }
    .deck-model-selector-panel { position:fixed; inset:auto; margin:0; box-sizing:border-box; min-width:min(260px,calc(100vw - 16px)); max-width:calc(100vw - 16px); max-height:calc(100vh - 16px); overflow-y:auto; overscroll-behavior:contain; padding:5px; border:1px solid var(--color-token-border-default,#ffffff24); border-radius:10px; background:var(--popover,var(--color-token-bg-primary,#202020)); color:var(--color-token-text-primary,#eee); box-shadow:0 8px 28px #0004; font:inherit; font-size:var(--text-sm); -webkit-app-region:no-drag; }
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

  // React 为每个组件保留新旧两份 fiber，元素上记录的那份可能落后一次渲染。
  // 因此从当前根节点沿祖先路径向下取已提交的 fiber，避免读到过期的选中状态。
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
  const context = native => {
    const chain = currentFibers(native);
    for (let index = 0; index < chain.length; index++) {
      const p = chain[index].memoizedProps;
      if (app === 't3code' && typeof p?.onProviderModelSelect === 'function' && p.instanceEntries) {
        let target = p.effortColumn?.props?.traits?.threadRef;
        for (let at = index; !target && at < chain.length; at++) {
          const props = chain[at].memoizedProps;
          target = props?.composerDraftTarget || props?.draftId || props?.routeThreadRef;
        }
        return { ...p, composerTarget: target };
      }
      if (app === 'paseo' && Array.isArray(p?.providers) && typeof p.onSelect === 'function' && 'selectedModel' in p) {
        let thinking = null;
        for (let at = index + 1; at < chain.length; at++) {
          const props = chain[at].memoizedProps;
          if (!thinking && props && 'selectedThinkingOptionId' in props) thinking = props;
          if (props && Array.isArray(props.modelSelectorProviders) && 'selectedModelId' in props && 'onSelectModel' in props) {
            return { ...p, selectedThinkingOptionId: props.selectedThinkingOptionId,
              onSelectThinkingOption: props.onSelectThinkingOption || thinking?.handleThinkingSelect,
              onSelectModel: props.onSelectModel, onSelectProviderAndModel: props.onSelectProviderAndModel,
              canSwitchProvider: typeof props.onSelectProviderAndModel === 'function' };
          }
        }
        return { ...p, selectedThinkingOptionId: thinking?.selectedThinkingOptionId,
          onSelectThinkingOption: thinking?.handleThinkingSelect, canSwitchProvider: false };
      }
    }
    return null;
  };
  let storePromise;
  const modelStore = () => storePromise ||= (async () => {
    const url = [...document.querySelectorAll('link[rel="modulepreload"]')].map(el => el.href)
      .find(url => /\/utils-[^/]+\.js(?:\?.*)?$/.test(url));
    if (!url) throw new Error('无法加载模型选项，请刷新应用后重试');
    const module = await import(url);
    const store = Object.values(module).find(value => typeof value?.getState === 'function' &&
      typeof value.getState()?.setProviderModelOptions === 'function');
    if (!store) throw new Error('当前应用版本不支持快捷推理等级');
    return store;
  })().catch(error => { storePromise = null; throw error; });
  const reasoningDescriptor = (p, value) => p?.instanceEntries?.find(e => e.instanceId === value.provider)
    ?.snapshot?.models?.find(m => m.slug === value.model)?.capabilities?.optionDescriptors
    ?.find(d => d.role === 'reasoning' || ['reasoningEffort', 'reasoning_effort', 'effort'].includes(d.id));
  const currentEffort = (p, value) => {
    const descriptor = reasoningDescriptor(p, value);
    const options = p?.selectedModelSelection?.options || p?.effortColumn?.props?.traits?.modelOptions || [];
    return options.find(o => o.id === descriptor?.id)?.value ?? descriptor?.currentValue ?? descriptor?.options?.find(o => o.isDefault)?.id;
  };
  const unavailable = (p, value) => {
    if (!p) return '当前页面暂不支持切换模型';
    if (p.disabled || p.isLoading || p.multipleModelSelections?.length) return '当前暂不能切换模型';
    if (app === 't3code') {
      const entry = p.instanceEntries.find(e => e.instanceId === value.provider);
      if (!entry?.isAvailable) return '供应商当前不可用';
      if (p.lockedProvider && p.lockedProvider !== entry.driverKind) return '当前会话已锁定供应商';
      if (p.lockedContinuationGroupKey && p.lockedContinuationGroupKey !== entry.continuationGroupKey) return '当前会话不支持此供应商';
      if (!p.modelOptionsByInstance.get(value.provider)?.some(m => m.slug === value.model)) return '当前未提供此模型';
      if (value.reasoningEffort && !reasoningDescriptor(p, value)?.options?.some(o => o.id === value.reasoningEffort)) return '此模型不支持配置的推理等级';
      return p.getModelDisabledReason?.(value.provider, value.model) || '';
    }
    if (!p.canSwitchProvider && value.provider !== p.selectedProvider) return '已有会话只能切换当前 provider 的模型';
    return '';
  };
  const selected = (p, value) => app === 't3code'
    ? p?.selectedModelSelection?.instanceId === value.provider && p.selectedModelSelection.model === value.model && (!value.reasoningEffort || currentEffort(p, value) === value.reasoningEffort)
    : p?.selectedProvider === value.provider && p.selectedModel === value.model && (!value.reasoningEffort || p.selectedThinkingOptionId === value.reasoningEffort);
  const matchesModel = (p, value) => app === 't3code'
    ? p?.selectedModelSelection?.instanceId === value.provider && p.selectedModelSelection.model === value.model
    : p?.selectedProvider === value.provider && p.selectedModel === value.model;
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
      ? value.provider === pending.provider && value.model === pending.model && value.reasoningEffort === pending.reasoningEffort
      : selected(ctx, value));
    const modelMatch = !pending && !current ? entries.find(([, value]) => matchesModel(ctx, value)) : null;
    const fallback = app === 't3code' ? ctx?.selectedModelSelection?.model || '选择模型' : ctx?.selectedModel || '选择模型';
    const actualEffort = app === 't3code'
      ? ctx?.selectedModelSelection && currentEffort(ctx, { provider: ctx.selectedModelSelection.instanceId, model: ctx.selectedModelSelection.model })
      : ctx?.selectedThinkingOptionId;
    const effort = pending ? pending.reasoningEffort : actualEffort;
    const name = [current?.[0] || modelMatch?.[0] || fallback, typeof effort === 'string' ? effort.toLowerCase() : ''].filter(Boolean).join(' ');
    text(control.label, name);
    const shortcut = 'Ctrl+M / Alt+M';
    const title = `选择模型：${name}（${shortcut} 循环切换）`;
    if (control.button.title !== title) control.button.title = title;
    if (control.button.getAttribute('aria-label') !== title) control.button.setAttribute('aria-label', title);
    const disabled = !ctx || !!ctx.disabled;
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
  const choose = async (control, value, row, quiet = false) => {
    const ctx = context(control.native);
    if (unavailable(ctx, value)) return false;
    const threadRefAtStart = app === 't3code' ? ctx.composerTarget : null;
    const pending = { value, time: Date.now(), request: ++control.requestId };
    control.pendingSelection = pending;
    try {
      // 与原生推理等级选项调用同一个入口，同时提交模型与推理等级。
      if (app === 't3code') {
        const store = value.reasoningEffort ? await modelStore() : null;
        if (control.requestId !== pending.request) return false;
        const latest = context(control.native);
        if (disposed || !control.native.isConnected) return false;
        if (unavailable(latest, value)) throw new Error('当前会话无法切换到该模型');
        const threadRef = latest.composerTarget;
        if (value.reasoningEffort && !threadRef) throw new Error('无法确定当前会话');
        if (JSON.stringify(threadRefAtStart) !== JSON.stringify(threadRef)) return null;
        const entry = latest.instanceEntries.find(e => e.instanceId === value.provider);
        const descriptor = reasoningDescriptor(latest, value);
        const state = store?.getState();
        const selection = latest.selectedModelSelection;
        const existing = selection?.instanceId === value.provider && selection.model === value.model
          ? selection.options || latest.effortColumn?.props?.traits?.modelOptions || []
          : state?.stickyOptionsByModelByProvider[value.provider]?.[value.model] || [];
        latest.onProviderModelSelect(value.provider, value.model);
        if (state) {
          const draft = state.getComposerDraft(threadRef);
          const serverSelection = latest.activeThreadModelSelection;
          const alreadyCurrent = matchesModel(latest, value) ||
            (serverSelection?.instanceId === value.provider && serverSelection.model === value.model);
          if ((draft?.activeProvider !== value.provider || draft.modelSelectionByProvider?.[value.provider]?.model !== value.model) && !alreadyCurrent) {
            throw new Error('T3Code 未接受此次模型切换');
          }
          state.setProviderModelOptions(threadRef, entry.driverKind,
            [...existing.filter(o => o.id !== descriptor.id), { id: descriptor.id, value: value.reasoningEffort }],
            { instanceId: value.provider, model: value.model });
        }
      }
      else {
        // 同一输入框的切换串行执行，后续按键保留最新目标，避免异步请求乱序覆盖。
        const previous = control.paseoQueue || Promise.resolve();
        const operation = previous.catch(() => {}).then(async () => {
          if (disposed || !control.native.isConnected || control.requestId !== pending.request) return;
          const start = context(control.native);
          if (unavailable(start, value)) throw new Error('当前会话无法切换到该模型');
          if (start.canSwitchProvider) await start.onSelectProviderAndModel(value.provider, value.model);
          else if (typeof start.onSelectModel === 'function') await start.onSelectModel(value.model);
          else await start.onSelect(value.provider, value.model);
          // 原生状态更新可能晚于回调，等待目标模型提交后再设置其思考等级。
          let latest;
          for (let attempt = 0; attempt < 160; attempt++) {
            if (disposed || !control.native.isConnected) return;
            latest = context(control.native);
            if (latest?.selectedProvider === value.provider && latest.selectedModel === value.model) break;
            await new Promise(resolve => setTimeout(resolve, 50));
          }
          if (control.requestId !== pending.request) return;
          if (latest?.selectedProvider !== value.provider || latest.selectedModel !== value.model) throw new Error('Paseo 未应用此供应商或模型');
          if (value.reasoningEffort && latest.selectedThinkingOptionId !== value.reasoningEffort) {
            if (typeof latest.onSelectThinkingOption !== 'function') throw new Error('当前模型无法设置 reasoning');
            await latest.onSelectThinkingOption(value.reasoningEffort);
          }
        });
        control.paseoQueue = operation;
        close();
        refresh(control);
        await operation;
        if (control.requestId !== pending.request || disposed || !control.native.isConnected) return false;
      }
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
      control.native.closest('form')?.querySelector('[contenteditable="true"]')?.focus({ preventScroll: true });
      return true;
    } catch (error) {
      if (control.requestId !== pending.request) return false;
      if (control.pendingSelection === pending) control.pendingSelection = null;
      console.warn('[Deck] 切换模型失败：', error);
      if (quiet) return false;
      if (!panel.matches(':popover-open')) open(control);
      row.title = error instanceof Error ? error.message : '切换失败，请重试或使用原生模型菜单';
      let status = panel.querySelector('.deck-model-selector-status');
      if (!status) { status = document.createElement('div'); status.className = 'deck-model-selector-status'; status.setAttribute('role', 'status'); panel.append(status); }
      text(status, row.title);
      position();
      return false;
    }
  };
  const chooseNextAvailable = async (control, start, count) => {
    const cycle = control.cycleId = (control.cycleId || 0) + 1;
    for (let offset = 0; offset < Math.min(count, entries.length); offset++) {
      if (disposed || !control.native.isConnected || control.cycleId !== cycle) return;
      const [, value] = entries[(start + offset + entries.length) % entries.length];
      if (unavailable(context(control.native), value)) continue;
      const result = await choose(control, value, control.button, true);
      if (result === true || result === null || control.cycleId !== cycle) return;
    }
    if (control.cycleId !== cycle) return;
    control.pendingSelection = null;
    refresh(control);
    if (!panel.matches(':popover-open')) open(control);
    const status = document.createElement('div');
    status.className = 'deck-model-selector-status';
    status.setAttribute('role', 'status');
    status.textContent = '向后尝试的模型均不可用，请稍后重试';
    panel.querySelector('.deck-model-selector-status')?.remove();
    panel.append(status);
    position();
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
      label.textContent = name;
      effort.textContent = value.reasoningEffort || '';
      row.setAttribute('aria-label', [label.textContent, effort.textContent].filter(Boolean).join(' '));
      row.append(check, label, effort);
      row.addEventListener('click', () => app === 't3code' ? chooseNextAvailable(control, index, 6) : choose(control, value, row));
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
    const validModifier = (event.ctrlKey && !event.altKey) || (event.altKey && !event.ctrlKey);
    if (!validModifier || event.metaKey || event.shiftKey || event.isComposing ||
        !(event.code === 'KeyM' || event.key?.toLowerCase() === 'm')) return;
    if (document.querySelector('[role="dialog"][data-state="open"]:not([aria-modal="false"])')) return;
    const footer = event.target instanceof Element ? event.target.closest('form') : null;
    const visible = [...controls.values()].filter(control => control.native.isConnected && control.native.getClientRects().length && !control.button.disabled);
    const control = visible.find(control => control.native.closest('form') === footer) ||
      (active && visible.includes(active) ? active : visible[0]);
    if (!control || !entries.length) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    if (event.repeat) return;
    const ctx = context(control.native);
    // 连续按键可能早于 React 提交，沿用尚未显示的目标，确保每次按键都前进一项。
    const pending = control.pendingSelection;
    let current = pending
      ? entries.findIndex(([, value]) => value.provider === pending.value.provider && value.model === pending.value.model && value.reasoningEffort === pending.value.reasoningEffort)
      : entries.findIndex(([, value]) => selected(ctx, value));
    if (current < 0 && !pending) current = entries.findIndex(([, value]) => matchesModel(ctx, value));
    if (app === 't3code') { void chooseNextAvailable(control, current + 1, 5); return; }
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
      if (!native.isConnected || !native.getClientRects().length || !entries.length) {
        clearTimeout(control.confirmTimer);
        if (active === control) close();
        control.button.remove();
        controls.delete(native);
      }
    }
    if (!entries.length) return;
    for (const native of document.querySelectorAll(nativeSelector)) {
      // 未显示页面保留的输入区不应生成控件，否则会挂到当前页面的目录标签后。
      if (!native.getClientRects().length) continue;
      const project = app === 't3code' ? native.closest('form')?.querySelector('.deck-project-label') : (() => { for (let row = native.parentElement; row; row = row.parentElement) { const label = [...row.querySelectorAll('.deck-project-label')].find(item => item.getClientRects().length); if (label) return label; } return null; })();
      if (!project) continue;
      let control = controls.get(native);
      if (!control) {
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'deck-model-selector-trigger';
        button.setAttribute('aria-haspopup', 'menu');
        button.setAttribute('aria-expanded', 'false');
        button.setAttribute('aria-controls', panel.id);
        button.setAttribute('aria-keyshortcuts', 'Control+m Alt+m');
        const label = document.createElement('span');
        button.append(label);
        button.insertAdjacentHTML('beforeend', '<svg aria-hidden="true" viewBox="0 0 16 16" fill="none"><path d="m4 10 4-4 4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>');
        control = { native, button, label, requestId: 0 };
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
      // Paseo 没有 Codex 的颜色变量，沿用目录标签已匹配的原生文字样式。
      if (app === 'paseo') {
        const look = getComputedStyle(project);
        for (const name of ['color', 'fontSize', 'fontFamily']) if (control.button.style[name] !== look[name]) control.button.style[name] = look[name];
      }
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
        (node.matches(`${nativeSelector},.deck-project-label,[data-testid="mode-control"]`) || node.querySelector(`${nativeSelector},.deck-project-label,[data-testid="mode-control"]`)));
    });
    if (relevant) schedule();
  });
  observer.observe(document.documentElement, {
    childList: true, subtree: true, characterData: true,
    attributes: true, attributeFilter: ['data-selected-reasoning-effort', 'disabled'],
  });
  window.addEventListener('resize', position, { passive: true });
  document.addEventListener('scroll', position, { capture: true, passive: true });
  let configuredSignature = null;
  const configure = values => {
    const normalized = (Array.isArray(values) ? values : Object.entries(values || {})).filter(([name, value]) => name.trim() && typeof value?.model === 'string' && typeof value?.provider === 'string');
    const signature = JSON.stringify(normalized);
    if (signature === configuredSignature) { schedule(); return; }
    configuredSignature = signature;
    close();
    for (const control of controls.values()) {
      clearTimeout(control.confirmTimer);
      control.requestId++;
      control.cycleId = (control.cycleId || 0) + 1;
      control.pendingSelection = null;
    }
    entries = normalized;
    scan();
  };
  window[key] = { version: 21, configure, scan: schedule, dispose() {
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
  return true;
})()
