(() => {
  const key = '__deckZcodeDefaultFullAccess';
  if (window[key]) {
    window[key].scan();
    return true;
  }

  const MODE = 'yolo';
  const MAX_ATTEMPTS = 3;
  const hideId = 'deck-zcode-full-access-hide';
  const q = selector => document.querySelector(selector);
  const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
  const waitFor = async (find, timeout = 2000) => {
    const end = Date.now() + timeout;
    for (;;) {
      const found = find();
      if (found) return found;
      if (Date.now() > end) return null;
      await sleep(30);
    }
  };

  // 打开 Radix 下拉菜单需要完整的指针事件序列，单独的 click 不会触发。
  const press = el => {
    const base = { bubbles: true, cancelable: true, button: 0, pointerType: 'mouse', isPrimary: true, view: window };
    el.dispatchEvent(new PointerEvent('pointerdown', base));
    el.dispatchEvent(new MouseEvent('mousedown', base));
    el.dispatchEvent(new PointerEvent('pointerup', base));
    el.dispatchEvent(new MouseEvent('mouseup', base));
    el.dispatchEvent(new MouseEvent('click', base));
  };

  const currentMode = () => q('[data-testid="v4-model-config"][data-source="composer"]')?.dataset.mode || '';

  // 切换过程中菜单和风险确认框只会闪现几百毫秒，暂时隐藏以免打断输入。
  const setQuiet = on => {
    let style = document.getElementById(hideId);
    if (!on) {
      style?.remove();
      return;
    }
    if (style) return;
    style = document.createElement('style');
    style.id = hideId;
    style.textContent = '[role="menu"],[role="dialog"],[role="alertdialog"],[data-slot$="overlay"]{opacity:0 !important;transition:none !important;animation:none !important;}';
    (document.head || document.documentElement).appendChild(style);
  };

  const switchToFullAccess = async () => {
    const trigger = q('[data-testid="chat-mode-select-trigger"]');
    if (!trigger) return false;
    press(trigger);
    const item = await waitFor(() => q(`[data-testid="chat-mode-select-item-${MODE}"]`));
    if (!item) return false;
    item.click();
    // 已授权过的项目不会弹确认框，模式会直接生效。
    const risk = await waitFor(() => q('[data-testid="yolo-confirmation-risk"]') || (currentMode() === MODE ? true : null), 1500);
    if (risk && risk !== true) {
      risk.click();
      const confirm = await waitFor(() => {
        const button = q('[data-testid="yolo-confirmation-confirm"]');
        return button && !button.disabled ? button : null;
      });
      if (!confirm) return false;
      confirm.click();
    }
    return !!(await waitFor(() => (currentMode() === MODE ? true : null), 3000));
  };

  // 新会话草稿的容器带有 data-session-id="draft"，发送首条消息后会换成真实会话 ID。
  const draftSelector = '[data-session-id^="draft"]';

  // handled 只绑定当前这一个草稿：用户之后手动改回其他模式时不再覆盖。
  let handledDraft = null;
  let busy = false;
  let attempts = 0;
  let scheduled = false;

  const run = async draft => {
    busy = true;
    setQuiet(true);
    let done = false;
    try {
      done = await switchToFullAccess();
    } catch (error) {
      done = false;
    } finally {
      setQuiet(false);
      busy = false;
    }
    if (done || ++attempts >= MAX_ATTEMPTS) {
      handledDraft = draft;
    } else {
      schedule();
    }
  };

  const scan = () => {
    scheduled = false;
    if (busy) return;
    const draft = q(draftSelector);
    if (!draft) {
      // 离开草稿后重置，下一次新建会话重新生效。
      handledDraft = null;
      attempts = 0;
      return;
    }
    if (handledDraft === draft || !q('[data-testid="v4-composer"]')) return;
    const mode = currentMode();
    if (!mode || !q('[data-testid="chat-mode-select-trigger"]')) return;
    if (mode === MODE) {
      handledDraft = draft;
      return;
    }
    run(draft);
  };

  function schedule() {
    if (scheduled) return;
    scheduled = true;
    setTimeout(scan, 150);
  }

  const observer = new MutationObserver(schedule);
  observer.observe(document.documentElement, { childList: true, subtree: true });
  window[key] = { scan: schedule, observer };
  scan();
  return true;
})()
