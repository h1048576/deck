(() => {
  if (window.frameElement?.id === 'deck-native-style-sampler') return true;
  const app = __DECK_SIDEBAR_MODE_APP__;
  const mode = __DECK_SIDEBAR_MODE__;
  const key = '__deckSidebarMode';
  if (window[key]?.version === 3 && window[key].app === app && window[key].mode === mode) return true;
  window.__deckAntigravitySidebarMode?.dispose();
  window[key]?.dispose();
  if (mode !== 'compact') {
    delete window[key];
    return true;
  }

  const marker = 'data-deck-sidebar-compact';
  const styleId = 'deck-sidebar-mode';
  const spacingScale = 0.55;
  const rowScale = 0.78;
  const explicitHeightClass = /(?:^|\s)!?h-(?:\d+(?:\.\d+)?|\[[^\]]+\])(?:\s|$)/;
  const roots = new Map();
  const marked = new Set();
  const rules = new Map();

  const round1 = value => Math.round(value * 10) / 10;
  const pxValue = value => (/^-?[\d.]+px$/.test(value) ? Number.parseFloat(value) : NaN);
  const nonEmptyString = value => typeof value === 'string' && value.trim().length > 0;
  const uuidTail = /([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$/i;

  const currentFiberOf = element => {
    const fiberKey = Object.keys(element).find(name => name.startsWith('__reactFiber$'));
    if (!fiberKey) return null;
    let fiber = element[fiberKey];
    let root = fiber;
    while (root?.return) root = root.return;
    if (root?.stateNode?.current && root.stateNode.current !== root) fiber = fiber.alternate ?? fiber;
    return fiber;
  };

  const hasCollectionProp = props =>
    Array.isArray(props.conversations) ||
    Array.isArray(props.cascades) ||
    Array.isArray(props.trajectories) ||
    Array.isArray(props.projects) ||
    Array.isArray(props.workspaces);

  const extractConversationId = props => {
    if (!props || typeof props !== 'object' || hasCollectionProp(props)) return '';
    // 目录/项目折叠项通常带有折叠状态属性，不视为单条会话。
    if (typeof props.isCollapsed === 'boolean' || typeof props.collapsed === 'boolean') return '';
    for (const prop of ['cascadeId', 'conversationId', 'trajectoryId']) {
      if (nonEmptyString(props[prop])) return props[prop];
    }
    for (const prop of ['conversation', 'cascade', 'trajectory', 'summary', 'session', 'item', 'node']) {
      const value = props[prop];
      if (!value || typeof value !== 'object' || Array.isArray(value)) continue;
      for (const idProp of ['cascadeId', 'conversationId', 'trajectoryId']) {
        if (nonEmptyString(value[idProp])) return value[idProp];
      }
      if ((prop === 'conversation' || prop === 'cascade' || prop === 'trajectory') && nonEmptyString(value.id)) {
        return value.id;
      }
      if (prop === 'summary' && nonEmptyString(value.id) &&
          ('stepCount' in value || 'lastModifiedTime' in value || 'summary' in value)) {
        return value.id;
      }
    }
    return '';
  };

  const domConversationId = element => {
    for (const attr of ['data-cascade-id', 'data-conversation-id', 'data-trajectory-id']) {
      const value = element.getAttribute(attr);
      if (nonEmptyString(value)) return value;
    }
    const testId = element.getAttribute('data-testid') || '';
    if (/(?:conversation|cascade|trajectory)/i.test(testId) &&
        !/(?:new[-_]?conversation|list|group|section|header|project|folder|panel|sidebar|split|search|filter|empty)/i.test(testId)) {
      return testId;
    }
    return '';
  };

  const collectIdsFromArray = (list, ids, allowBareString) => {
    if (!Array.isArray(list) || !list.length) return;
    const first = list[0];
    if (typeof first === 'string') {
      if (!allowBareString) return;
      for (const item of list) if (nonEmptyString(item)) ids.add(item);
      return;
    }
    if (!first || typeof first !== 'object') return;
    const idProp = ['cascadeId', 'conversationId', 'trajectoryId'].find(prop => nonEmptyString(first[prop]));
    if (!idProp) return;
    for (const item of list) {
      if (item && typeof item === 'object' && nonEmptyString(item[idProp])) ids.add(item[idProp]);
    }
  };

  const collectKnownConversationIds = panel => {
    const ids = new Set();
    const visited = new Set();
    const inspectFiber = fiber => {
      for (let current = fiber, depth = 0; current && depth < 10 && !visited.has(current); current = current.return, depth++) {
        visited.add(current);
        const props = current.memoizedProps;
        if (props && typeof props === 'object') {
          for (const [name, value] of Object.entries(props)) {
            if (Array.isArray(value)) {
              collectIdsFromArray(value, ids, /^(?:cascade|conversation|trajectory)Ids$/i.test(name));
            }
          }
        }
        let hook = current.memoizedState;
        for (let count = 0; hook && count < 16; count++, hook = hook.next) {
          const state = hook.memoizedState;
          if (Array.isArray(state)) {
            collectIdsFromArray(state, ids, false);
          } else if (Array.isArray(state?.[0])) {
            collectIdsFromArray(state[0], ids, false);
          }
        }
      }
    };
    const rootFiber = currentFiberOf(panel);
    if (rootFiber) inspectFiber(rootFiber);
    for (const node of panel.querySelectorAll('*')) {
      const fiber = currentFiberOf(node);
      if (fiber) inspectFiber(fiber);
    }
    return ids;
  };

  const matchKnownKey = (fiberKey, knownIds) => {
    if (!nonEmptyString(fiberKey) || !knownIds.size) return '';
    if (knownIds.has(fiberKey)) return fiberKey;
    const match = uuidTail.exec(fiberKey);
    if (match && knownIds.has(match[1])) return match[1];
    return '';
  };

  const ownConversationId = (element, knownIds) => {
    const domId = domConversationId(element);
    if (domId) return domId;
    const fiber = currentFiberOf(element);
    if (!fiber) return '';
    const directKey = matchKnownKey(fiber.key, knownIds);
    if (directKey) return directKey;
    // 只检查直接包裹当前 DOM 节点的组件层，遇到父级 HostComponent 即停止，避免误命中目录行。
    for (let current = fiber.return, depth = 0; current && depth < 12; current = current.return, depth++) {
      if (current.tag === 5) break;
      const keyMatch = matchKnownKey(current.key, knownIds);
      if (keyMatch) return keyMatch;
      const propId = extractConversationId(current.memoizedProps);
      if (propId) return propId;
    }
    return '';
  };

  const promoteSingleWrappers = (itemRoots, panel) => {
    const isItemOrSingleWrapper = node =>
      itemRoots.has(node) || (node.childElementCount === 1 && itemRoots.has(node.firstElementChild));
    for (const element of [...itemRoots]) {
      const parent = element.parentElement;
      if (!parent || parent === panel || parent.hasAttribute('data-split-sized') || parent.childElementCount !== 1) continue;
      const grand = parent.parentElement;
      if (!grand || grand === panel || grand.childElementCount < 2) continue;
      let siblingItems = 0;
      for (const child of grand.children) {
        if (isItemOrSingleWrapper(child)) siblingItems++;
      }
      if (siblingItems >= 2) {
        itemRoots.delete(element);
        itemRoots.add(parent);
      }
    }
  };

  const collectListContainers = (itemRoots, panel) => {
    const listContainers = new Set();
    for (const item of itemRoots) {
      const parent = item.parentElement;
      if (!parent || parent === panel || parent.hasAttribute('data-split-sized') || itemRoots.has(parent)) continue;
      if (listContainers.has(parent)) continue;
      let count = 0;
      let nonItemWithControls = false;
      for (const child of parent.children) {
        if (itemRoots.has(child)) {
          count++;
        } else if (child.matches('[aria-expanded]') || child.querySelector('[aria-expanded]') ||
            (child === parent.firstElementChild && (child.matches('button, [role="button"]') || !!child.querySelector('button, [role="button"]')))) {
          nonItemWithControls = true;
        }
      }
      if (count >= 2 && !nonItemWithControls) listContainers.add(parent);
    }
    return listContainers;
  };

  // Antigravity：只识别目录底下的会话条目及其直接列表容器。
  const buildAntigravityScopes = panel => {
    const knownIds = collectKnownConversationIds(panel);
    const directRoots = new Map();
    for (const element of panel.querySelectorAll('*')) {
      if (element.hasAttribute('data-split-sized') || element.closest('[data-split-sash]')) continue;
      const id = ownConversationId(element, knownIds);
      if (id) directRoots.set(element, id);
    }
    const itemRoots = new Set();
    for (const [element, id] of directRoots) {
      let multi = false;
      for (const [other, otherId] of directRoots) {
        if (other !== element && otherId !== id && element.contains(other)) {
          multi = true;
          break;
        }
      }
      if (!multi) itemRoots.add(element);
    }
    for (const element of [...itemRoots]) {
      for (let parent = element.parentElement; parent && parent !== panel; parent = parent.parentElement) {
        if (itemRoots.has(parent)) {
          itemRoots.delete(element);
          break;
        }
      }
    }
    promoteSingleWrappers(itemRoots, panel);
    return { itemRoots, listContainers: collectListContainers(itemRoots, panel) };
  };

  // Droid：左侧栏内仅 [data-session-id] 为会话条目，目录标题不含该属性。
  const buildDroidScopes = panel => {
    const itemRoots = new Set(panel.querySelectorAll('[data-session-id]'));
    for (const element of [...itemRoots]) {
      for (let parent = element.parentElement; parent && parent !== panel; parent = parent.parentElement) {
        if (itemRoots.has(parent)) {
          itemRoots.delete(element);
          break;
        }
      }
    }
    promoteSingleWrappers(itemRoots, panel);
    return { itemRoots, listContainers: collectListContainers(itemRoots, panel) };
  };

  const paseoRowSelector = '[data-testid^="sidebar-workspace-row-"]';
  const paseoListSelector = '[data-testid^="sidebar-workspace-list-"], [data-testid^="sidebar-status-group-rows-"]';
  const paseoProjectSelector = '[data-testid^="sidebar-project-row-"]';

  // Paseo：仅作用于项目下的工作区/会话行，避开项目标题行（sidebar-project-row-*）。
  const classifyPaseo = element => {
    if (element.closest(paseoProjectSelector)) return '';
    if (element.closest(paseoRowSelector)) return 'item';
    if (element.childElementCount === 1 && element.firstElementChild?.matches(paseoRowSelector)) {
      const parent = element.parentElement;
      if (parent && parent.childElementCount >= 2 && !parent.querySelector(paseoProjectSelector)) {
        let siblings = 0;
        for (const child of parent.children) {
          if (child.matches(paseoRowSelector) || (child.childElementCount === 1 && child.firstElementChild?.matches(paseoRowSelector))) {
            siblings++;
          }
        }
        if (siblings >= 2) return 'item';
      }
    }
    if (element.matches(paseoListSelector)) return 'list';
    if (element.childElementCount >= 2 && !element.querySelector(paseoProjectSelector)) {
      let rows = 0;
      for (const child of element.children) {
        if (child.matches(paseoRowSelector) || (child.childElementCount === 1 && child.firstElementChild?.matches(paseoRowSelector))) {
          rows++;
        }
      }
      if (rows >= 2) return 'list';
    }
    return '';
  };

  const ensureStyle = root => {
    if (roots.get(root)?.isConnected) return;
    const style = document.createElement('style');
    style.id = styleId;
    (root === document ? document.head || document.documentElement : root).append(style);
    roots.set(root, style);
  };

  const writeStyles = () => {
    const css = [...rules].map(([declarations, group]) =>
      `[${marker}="${group}"] { ${declarations} }`
    ).join('\n');
    for (const style of roots.values()) {
      if (style.textContent !== css) style.textContent = css;
    }
  };

  const cleanup = () => {
    for (const element of marked) {
      if (!element.isConnected) {
        element.removeAttribute(marker);
        marked.delete(element);
      }
    }
    for (const [root, style] of roots) {
      if (root !== document && !root.host.isConnected) {
        style.remove();
        roots.delete(root);
      }
    }
  };

  const compactListRule = computed => {
    const gap = pxValue(computed.getPropertyValue('row-gap'));
    if (!Number.isFinite(gap) || gap <= 0) return '';
    return `row-gap: ${round1(gap * spacingScale)}px !important;`;
  };

  // 只压缩会话条目及其内部元素的纵向间距与单行高度，避开图标按钮与非布局元素。
  const compactItemRule = (element, computed) => {
    if (computed.display === 'none' || computed.getPropertyValue('-webkit-app-region') === 'drag') return '';
    if (element.matches('svg, svg *, img, canvas, video, hr, .codicon, [class*="codicon-"]')) return '';

    const width = pxValue(computed.width);
    const height = pxValue(computed.height);
    const fontSize = pxValue(computed.fontSize) || 14;
    const minRowHeight = Math.ceil(fontSize * 1.25 + 3);
    const isSquareIcon = Number.isFinite(width) && Number.isFinite(height) &&
      width <= 40 && height <= 40 && Math.abs(width - height) <= 6;
    const declarations = [];

    const scaleSpacing = property => {
      const value = pxValue(computed.getPropertyValue(property));
      if (!Number.isFinite(value) || value <= 0) return;
      declarations.push(`${property}: ${round1(value * spacingScale)}px !important;`);
    };

    if (!isSquareIcon) {
      scaleSpacing('padding-top');
      scaleSpacing('padding-bottom');
    }
    scaleSpacing('margin-top');
    scaleSpacing('margin-bottom');
    scaleSpacing('row-gap');

    const minH = pxValue(computed.minHeight);
    if (!isSquareIcon && Number.isFinite(minH) && minH >= 24 && minH <= 72 && (!Number.isFinite(width) || width > minH * 1.4)) {
      declarations.push(`min-height: ${Math.max(minRowHeight, round1(minH * rowScale))}px !important;`);
    }

    const isDroidSessionRow = app === 'droid' && element.hasAttribute('data-session-id');
    const className = typeof element.className === 'string' ? element.className : '';
    const inlineHeight = element.style.height;
    const hasExplicitHeight = isDroidSessionRow || explicitHeightClass.test(className) ||
      (!!inlineHeight && /^[\d.]+(?:px|rem|em)$/.test(inlineHeight.trim()));
    const isWideRow = Number.isFinite(width) && Number.isFinite(height) &&
      width > height * 1.4 && height >= 24 && height <= 72;
    if (hasExplicitHeight && isWideRow && !/(auto|scroll)/.test(computed.overflowY)) {
      const nextHeight = Math.max(minRowHeight, round1(height * rowScale));
      if (isDroidSessionRow) declarations.push(`--session-row-height: ${nextHeight}px !important;`);
      declarations.push(`height: ${nextHeight}px !important;`);
      const maxH = pxValue(computed.maxHeight);
      if (Number.isFinite(maxH) && maxH >= 24 && maxH <= 72) {
        declarations.push(`max-height: ${Math.max(minRowHeight, round1(maxH * rowScale))}px !important;`);
      }
      const intrinsic = computed.getPropertyValue('contain-intrinsic-block-size');
      const intrinsicMatch = /([\d.]+)px/.exec(intrinsic);
      if (intrinsicMatch) {
        const nextIntrinsic = Math.max(minRowHeight, round1(Number.parseFloat(intrinsicMatch[1]) * rowScale));
        declarations.push(`contain-intrinsic-block-size: auto ${nextIntrinsic}px !important;`);
      }
    }

    return declarations.join(' ');
  };

  const panelSelector = app === 'antigravity'
    ? '[data-testid="sidebar-split"] > :first-child'
    : app === 'droid' ? '[data-testid="window-sidebar"]' : '';

  ensureStyle(document);
  const watcher = window.__deckNativeStyles?.watch({
    relevantProperty: name => /^(padding|margin|gap|row-gap|height|min-height|max-height|display|font-size)/.test(name) ||
      /^--.*(?:spacing|padding|margin|gap|height|font-size|session-row-height)/.test(name),
    cleanup,
    measure(elements, nativeStyle, stats) {
      const scopesByPanel = new Map();
      const getScopes = panel => {
        if (!scopesByPanel.has(panel)) {
          scopesByPanel.set(panel, app === 'droid' ? buildDroidScopes(panel) : buildAntigravityScopes(panel));
        }
        return scopesByPanel.get(panel);
      };
      const inConversationItem = (element, panel, itemRoots) => {
        for (let cur = element; cur && cur !== panel; cur = cur.parentElement) {
          if (itemRoots.has(cur)) return true;
        }
        return false;
      };

      for (const element of elements) {
        let kind = '';
        if (app === 'paseo') {
          kind = classifyPaseo(element);
        } else if (panelSelector) {
          const panel = element.closest(panelSelector);
          if (panel && element !== panel && !element.hasAttribute('data-split-sized') && !element.closest('[data-split-sash]')) {
            const { itemRoots, listContainers } = getScopes(panel);
            if (listContainers.has(element)) kind = 'list';
            else if (inConversationItem(element, panel, itemRoots)) kind = 'item';
          }
        }

        let rule = '';
        if (kind) {
          const computed = nativeStyle(element);
          if (computed) rule = kind === 'list' ? compactListRule(computed) : compactItemRule(element, computed);
        }

        if (rule) {
          ensureStyle(element.getRootNode());
          if (!rules.has(rule)) rules.set(rule, String(rules.size));
          const group = rules.get(rule);
          if (element.getAttribute(marker) !== group) {
            element.setAttribute(marker, group);
            stats.writes++;
          }
          marked.add(element);
        } else if (marked.delete(element)) {
          element.removeAttribute(marker);
          stats.writes++;
        }
      }
      writeStyles();
    },
  });
  writeStyles();

  window[key] = {
    version: 3,
    app,
    mode,
    stats: watcher?.stats,
    dispose() {
      watcher?.dispose();
      for (const style of roots.values()) style.remove();
      for (const element of marked) element.removeAttribute(marker);
      roots.clear();
      marked.clear();
      rules.clear();
      delete window[key];
    },
  };
  return true;
})();
