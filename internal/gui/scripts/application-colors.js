(() => {
  if (window.frameElement?.id === 'deck-native-style-sampler') return;
  const config = __DECK_STYLE_CONFIG__;
  const key = '__deckApplicationColors';
  const signature = JSON.stringify(config);
  if (window[key]?.version === 2 && window[key].signature === signature) return;
  window[key]?.dispose();
  const marker = 'data-deck-color';
  const marked = new Set();
  const style = document.createElement('style');
  style.id = 'deck-application-colors';
  const backgrounds = {
    codex: ['--color-token-bg-primary', '--color-token-main-surface-primary', '--color-token-side-bar-background', '--color-background-surface', '--app-color-background-surface', '--color-surface-primary', '--color-background-execution-output'],
    dsh: ['--dsw-alias-bg-base', '--dsw-specific-sidebar-fill', '--bt-main', '--bt-sidebar'],
    zcode: ['--color-background', '--color-background-primary', '--color-sidebar'],
    droid: ['--surface-1', '--sidebar-bg', '--core-v2-bg-page', '--core-v2-bg-surface', '--initial-loader-background'],
  };
  // WorkBuddy 用 CSS Modules 硬编码颜色，没有变量可覆盖，侧栏和内容区背景按选择器直接指定。
  const backgroundSelectors = {
    workbuddy: [
      // 左侧边栏
      '.conversation-list', '.conversation-section-label', '.sidebar-next', '.sidebar-next-main-header',
      // 右侧输出内容区（会话视图和 Teams 网格视图）
      '.conversation-shell', '.teams-grid-scroll-content',
    ],
  };
  const foregrounds = {
    codex: ['--color-token-text-primary', '--color-text-primary', '--color-text', '--color-text-primary-ghost', '--color-text-primary-surface', '--user-message-text-color'],
    dsh: ['--dsw-alias-label-primary', '--bt-ink'],
    zcode: ['--color-foreground', '--color-text-primary'],
    droid: ['--text-default', '--text-heading', '--text-label', '--core-v2-text-primary', '--core-v2-appearance-user-message-text', '--sidebar-active-text', '--sidebar-text-hover'],
  };
  const tokens = [
    ...(backgrounds[config.app] || []).map(name => `${name}:${config.background} !important;`),
    ...(foregrounds[config.app] || []).map(name => `${name}:${config.foreground} !important;`),
  ].join('');
  const selectorTokens = (backgroundSelectors[config.app] || [])
    .map(selector => `${selector}{background-color:${config.background} !important;}`).join('');

  style.textContent = `
    :root, body, body *:not(.xterm, .xterm *) { ${tokens} }
    :root, body { background-color:${config.background} !important; color:${config.foreground} !important; }
    [${marker}*="f"] { color:${config.foreground} !important; }
    [${marker}*="b"] { background-color:${config.background} !important; }
    ${selectorTokens}
  `;
  (document.head || document.documentElement).append(style);
  const watcher = window.__deckNativeStyles.watch({
    relevantProperty: name => /^(color|background)/.test(name) ||
      /^--.*(?:color|background|foreground|surface|text|ink|label|fill|bg-)/.test(name),
    cleanup() {
      for (const element of marked) if (!element.isConnected) { element.removeAttribute(marker); marked.delete(element); }
    },
    measure(elements, nativeStyle, stats) {
      const body = nativeStyle(document.body);
      const root = nativeStyle(document.documentElement);
      if (!body || !root) return;
      const primaryText = new Set([body.color, root.color]);
      const primaryBackground = new Set([body.backgroundColor, root.backgroundColor]);
      primaryBackground.delete('rgba(0, 0, 0, 0)');
      const matches = [];
      // 保留语义色；只采样变化的子树，不撤掉真实页面的配色。
      for (const element of elements) {
        if (element.closest('.xterm')) continue;
        const computed = nativeStyle(element);
        if (!computed) continue;
        let value = primaryText.has(computed.color) ? 'f' : '';
        if (primaryBackground.has(computed.backgroundColor) && computed.backgroundImage === 'none') value += 'b';
        matches.push([element, value]);
      }
      for (const [element, value] of matches) {
        if (value) {
          if (element.getAttribute(marker) !== value) { element.setAttribute(marker, value); stats.writes++; }
          marked.add(element);
        } else if (marked.delete(element)) { element.removeAttribute(marker); stats.writes++; }
      }
    },
  });
  window[key] = { version: 2, signature, stats: watcher.stats, dispose() {
    watcher.dispose();
    style.remove();
    for (const element of marked) element.removeAttribute(marker);
    delete window[key];
  } };
})();
