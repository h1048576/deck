(() => {
  const key = '__deckDshReasoningEffort';
  if (window[key]) {
    window[key].scan();
    return true;
  }

  const directories = new Map();
  let scheduled = false;
  const currentFiber = element => {
    let fiber = element[Object.keys(element).find(name => name.startsWith('__reactFiber'))];
    let root = fiber;
    while (root?.return) root = root.return;
    if (root?.stateNode?.current && root.stateNode.current !== root) fiber = fiber?.alternate;
    return fiber;
  };
  const apply = entry => {
    const { directory, select, locked, available } = entry.props;
    const state = directory.getSnapshot();
    if (!available || locked || state.pending != null || state.status !== 'ready') return;
    const current = state.current;
    if (!current) return;
    const modelKey = JSON.stringify([current.provider, current.model]);
    if (entry.modelKey === modelKey) return;
    const model = state.groups?.find(group => group.id === current.provider)
      ?.models?.find(item => item.id === current.model);
    if (!model) return;

    // 首次接入保留已手动选择的等级；之后每次切换模型都优先使用 xhigh。
    const preserveEffort = entry.modelKey === undefined && current.reasoningEffort != null;
    entry.modelKey = modelKey;
    if (preserveEffort || current.reasoningEffort === 'xhigh' ||
        !model.reasoning?.efforts?.some(effort => effort.id === 'xhigh')) return;

    // 走原生选择接口，使实际会话参数与界面显示同步。
    Promise.resolve().then(() => {
      const latest = directory.getSnapshot();
      if (latest.pending != null || latest.current !== current || entry.props.locked) return;
      return select({ ...current, reasoningEffort: 'xhigh' });
    }).then(result => {
      if (result?.ok === false) console.warn('[Deck] 默认推理等级设置失败', result.error);
    }).catch(error => console.warn('[Deck] 默认推理等级设置失败', error));
  };
  const scan = () => {
    scheduled = false;
    const active = new Set();
    document.querySelectorAll('[data-slot="conversation.input.model"] button').forEach(button => {
      for (let fiber = currentFiber(button); fiber; fiber = fiber.return) {
        const props = fiber.memoizedProps;
        const directory = props?.directory;
        if (typeof directory?.getSnapshot !== 'function' ||
            typeof directory.subscribe !== 'function' || typeof props.select !== 'function') continue;
        active.add(directory);
        let entry = directories.get(directory);
        if (!entry) {
          entry = { props, unsubscribe: directory.subscribe(schedule) };
          directories.set(directory, entry);
        }
        entry.props = props;
        apply(entry);
        break;
      }
    });
    for (const [directory, entry] of directories) {
      if (active.has(directory)) continue;
      entry.unsubscribe();
      directories.delete(directory);
    }
  };
  const schedule = () => {
    if (scheduled) return;
    scheduled = true;
    queueMicrotask(scan);
  };
  const observer = new MutationObserver(schedule);
  observer.observe(document.documentElement, {
    childList: true, subtree: true, attributes: true,
    attributeFilter: ['aria-busy', 'disabled'],
  });
  window[key] = { scan: schedule, observer };
  scan();
  return true;
})()
