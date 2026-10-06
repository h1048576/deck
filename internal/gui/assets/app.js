// wide-pure 前端入口：外壳、页面切换、批量操作、设置持久化。
import { el, html, icon, SettingRow, Switch, PixelControl, DimensionControl, SelectControl, FontControl } from './controls.js'
import { api, messageOf } from './api.js'
import { agentsResource, skillsResource, modelsResource, mcpsResource, displayHarnessPath } from './resources.js'
import { createHarnessPage } from './harnesspage.js'
import { ModelDialog } from './dialogs.js'

// ---- 常量 ----
const FEATURES = [
  { id: 'codex', name: 'Codex', iconSrc: 'icons/codex.png' },
  { id: 'droid', name: 'Droid', iconSrc: 'icons/droid.svg' },
  { id: 'paseo', name: 'Paseo', iconSrc: 'icons/paseo.png' },
  { id: 'qoder', name: 'Qoder', iconSrc: 'icons/qoder.png' },
  { id: 'workbuddy', name: 'WorkBuddy', iconSrc: 'icons/workbuddy.png' },
  { id: 'dsh', name: 'DSH', iconSrc: 'icons/dsh.svg' },
  { id: 'zcode', name: 'ZCode', iconSrc: 'icons/zcode.png' },
]
const DEFAULT_MENU_ORDER = ['codex', 'droid', 'zcode', 'workbuddy', 'dsh', 'qoder', 'paseo']
const APPLICATIONS = {
  codex: { name: 'Codex', sidebarWidth: true, maxWidth: false, chatHeight: false, fontFamily: true, fontSize: true, merge: false, diff: false, changes: false, summary: true, terminal: true },
  droid: { name: 'Droid', sidebarWidth: true, maxWidth: true, chatHeight: true, fontFamily: true, fontSize: true, merge: false, diff: false, changes: false, summary: false, terminal: true },
  zcode: { name: 'ZCode', sidebarWidth: true, maxWidth: false, chatHeight: false, fontFamily: true, fontSize: true, merge: false, diff: false, changes: true, summary: false, terminal: true },
  workbuddy: { name: 'WorkBuddy', sidebarWidth: true, maxWidth: true, chatHeight: false, fontFamily: true, fontSize: true, merge: false, diff: false, changes: false, summary: false, terminal: false },
  dsh: { name: 'DSH', sidebarWidth: true, maxWidth: true, chatHeight: true, fontFamily: true, fontSize: true, merge: false, diff: false, changes: false, summary: false, terminal: true },
  qoder: { name: 'Qoder', sidebarWidth: true, maxWidth: true, chatHeight: false, fontFamily: true, fontSize: true, merge: false, diff: false, changes: false, summary: false, terminal: true },
  paseo: { name: 'Paseo', sidebarWidth: true, maxWidth: false, chatHeight: false, fontFamily: false, fontSize: false, merge: true, diff: true, changes: false, summary: false, terminal: false },
}
const PORTS = { codex: 9331, droid: 9332, zcode: 9333, workbuddy: 9334, dsh: 9335, qoder: 9336, paseo: 9337 }
const DEFAULT_DROID = {
  width: '70vw', sidebarWidth: '15vw', maxWidth: '90rem', chatHeight: '80px',
  fontFamily: 'Cascadia Mono, LXGW WenKai Mono', fontSize: 17, fontWeight: 300, terminalFollow: true,
  hideLocalMerge: false, hideGitDiff: false, hideChanges: false, preventSummary: false, port: 9332, executablePath: '',
}
const DEFAULT_APPLICATIONS = {
  codex: { ...DEFAULT_DROID, fontWeight: 100, preventSummary: true, port: PORTS.codex },
  droid: { ...DEFAULT_DROID },
  zcode: { ...DEFAULT_DROID, fontSize: 18, hideChanges: true, port: PORTS.zcode },
  workbuddy: { ...DEFAULT_DROID, fontWeight: 200, port: PORTS.workbuddy },
  dsh: { ...DEFAULT_DROID, port: PORTS.dsh },
  qoder: { ...DEFAULT_DROID, port: PORTS.qoder },
  paseo: { ...DEFAULT_DROID, hideLocalMerge: true, hideGitDiff: true, port: PORTS.paseo },
}
const DEFAULT_APPEARANCE = { fontFamily: 'Cascadia Mono, LXGW WenKai Mono', fontSize: 17, mode: 'compact' }
const DEFAULT_HARNESS_SETTINGS = { skillsCollapsed: true, modelsCollapsed: true, mcpsCollapsed: true }
const SYSTEM_FONT = '"Segoe UI", "Microsoft YaHei", -apple-system, BlinkMacSystemFont, sans-serif'
const BATCH_ACTION_LABELS = { start: '启动', restart: '重启', exit: '退出' }
const BATCH_STAGE_LABELS = { queued: '等待处理…', detecting: '检测安装…', checking: '检查运行状态…', stopping: '正在退出…', starting: '正在启动…', waiting: '等待窗口就绪…', applying: '正在应用界面设置…', maximizing: '正在最大化窗口…' }
const PLATFORM_NAMES = { windows: 'Windows', darwin: 'macOS', linux: 'Linux' }

// ---- 状态 ----
const state = {
  ready: false,
  desktop: false,
  version: '0.1.0',
  platform: 'windows',
  active: 'start',
  collapsed: false,
  menuOrder: [...DEFAULT_MENU_ORDER],
  applications: JSON.parse(JSON.stringify(DEFAULT_APPLICATIONS)),
  appearance: { ...DEFAULT_APPEARANCE },
  harnessSettings: { ...DEFAULT_HARNESS_SETTINGS },
  theme: 'system',
  startupMode: 'default',
  openAtLogin: false,
  startupAvailable: false,
  systemDark: matchMedia('(prefers-color-scheme: dark)').matches,
  windowMaximized: false,
  busy: null, // { id, action, scope? }
  batchProgress: null,
  batchRunning: false,
  batchError: '',
  selectedApplications: [],
  advanced: false,
  notice: null,
  bootError: '',
  configWarning: '',
  saveErrors: {},
  detectionCache: new Map(),
  detectionRequests: new Map(),
  restoreConfirmation: null,
  config: {},
}

const featureOf = id => FEATURES.find(item => item.id === id)

// ---- 校验（与 shared/types.ts 一致）----
const sizePattern = '(?:0|[1-9][0-9]{0,3})(?:\\.[0-9]+)?'
const widthPattern = new RegExp(`^(?:auto|fit-content|${sizePattern}(?:px|rem|em|vw|vh|%))$`)
const maxPattern = new RegExp(`^(?:none|${sizePattern}(?:px|rem|em|vw|vh|%))$`)
const sidebarPattern = new RegExp(`^${sizePattern}(?:px|rem|em|vw|vh|%)$`)
const heightPattern = new RegExp(`^(?:auto|${sizePattern}(?:px|rem|em|vh|%))$`)

function settingsErrors(value) {
  const errors = {}
  if (!widthPattern.test(value.width)) errors.width = '请输入有效宽度，例如 70vw、80% 或 1200px'
  if (!sidebarPattern.test(value.sidebarWidth)) errors.sidebarWidth = '请输入有效侧栏宽度，例如 15vw、15% 或 280px'
  if (!maxPattern.test(value.maxWidth)) errors.maxWidth = '请输入有效最大宽度，例如 90rem 或 1200px'
  if (!heightPattern.test(value.chatHeight)) errors.chatHeight = '请输入有效高度，例如 80px'
  if (!Number.isInteger(value.fontSize) || value.fontSize < 8 || value.fontSize > 72) errors.fontSize = '字号须为 8–72 的整数'
  if (!Number.isInteger(value.fontWeight) || value.fontWeight < 100 || value.fontWeight > 1000) errors.fontWeight = '字重须为 100–1000 的整数'
  if (!value.fontFamily.trim() || value.fontFamily.length > 300 || /[;{}<>\r\n]/.test(value.fontFamily)) errors.fontFamily = '请填写字体名称，不能包含 CSS 控制字符'
  if (!Number.isInteger(value.port) || value.port < 1024 || value.port > 65535) errors.port = '端口须为 1024–65535 的整数'
  if (typeof value.executablePath !== 'string' || /[\r\n\0]/.test(value.executablePath)) errors.executablePath = '应用路径无效'
  return errors
}

function appearanceErrors(value) {
  const errors = {}
  if (typeof value.fontFamily !== 'string' || !value.fontFamily.trim() || value.fontFamily.length > 300 || /[;{}<>\r\n]/.test(value.fontFamily)) errors.fontFamily = '请输入有效的字体名称'
  if (!Number.isInteger(value.fontSize) || value.fontSize < 10 || value.fontSize > 24) errors.fontSize = '字号须为 10–24 的整数'
  if (value.mode !== 'normal' && value.mode !== 'compact') errors.mode = '界面模式无效'
  return errors
}

function normalizeMenuOrder(input) {
  let saved = Array.isArray(input) ? input.filter(id => DEFAULT_MENU_ORDER.includes(id)) : []
  if (saved.length && !saved.includes('dsh')) {
    const workbuddy = saved.indexOf('workbuddy')
    const qoder = saved.indexOf('qoder')
    saved.splice(workbuddy >= 0 ? workbuddy + 1 : qoder >= 0 ? qoder : saved.length, 0, 'dsh')
  }
  return [...new Set([...saved, ...DEFAULT_MENU_ORDER])]
}

// ---- 应用图标 ----
function appIconNode(id, size = 20) {
  const feature = featureOf(id)
  const img = el('img', `app-icon app-icon-${id}`)
  img.src = feature.iconSrc
  img.width = size
  img.height = size
  img.alt = ''
  img.setAttribute('aria-hidden', 'true')
  img.draggable = false
  return img
}

// ---- 外壳渲染 ----
const shell = {
  appShell: document.getElementById('app-shell'),
  brandBar: document.getElementById('brand-bar'),
  menuGlobal: document.getElementById('menu-global'),
  menuApps: document.getElementById('menu-apps'),
  announcement: document.getElementById('menu-announcement'),
  sidebarBottom: document.getElementById('sidebar-bottom'),
  titlebar: document.getElementById('titlebar'),
  pageContainer: document.getElementById('page-container'),
  overlays: document.getElementById('overlays'),
}

let harnessPage = null

function renderBrandBar() {
  const bar = shell.brandBar
  bar.textContent = ''
  bar.appendChild(html('span', '', icon('maximize2', 22)))
  if (!state.collapsed) bar.appendChild(el('span', '', 'wide-pure'))
  const toggle = el('button', 'icon-button collapse-toggle')
  toggle.title = state.collapsed ? '展开菜单' : '收起菜单'
  toggle.setAttribute('aria-label', state.collapsed ? '展开菜单' : '收起菜单')
  toggle.setAttribute('aria-expanded', state.collapsed ? 'false' : 'true')
  toggle.innerHTML = icon('panelLeft', 18, 1.6)
  toggle.addEventListener('click', () => { state.collapsed = !state.collapsed; renderShell() })
  bar.appendChild(toggle)
}

function globalMenuItem(id, label, iconName) {
  const selected = state.active === id
  const item = el('button', `feature-item ${selected ? 'selected' : ''}`)
  item.setAttribute('aria-label', label)
  if (selected) item.setAttribute('aria-current', 'page')
  item.title = label
  item.innerHTML = icon(iconName, 20, 1.6)
  if (!state.collapsed) item.appendChild(el('span', '', label))
  item.addEventListener('click', () => setActive(id))
  return item
}

function renderMenus() {
  shell.menuGlobal.textContent = ''
  shell.menuGlobal.appendChild(globalMenuItem('start', '开始', 'play'))
  shell.menuGlobal.appendChild(globalMenuItem('harness', 'Harness', 'slidersHorizontal'))

  shell.menuApps.textContent = ''
  shell.menuApps.classList.toggle('menu-dragging', !!menuDrag.dragging)
  state.menuOrder.forEach(id => {
    const feature = featureOf(id)
    const selected = state.active === id
    const item = el('button', `feature-item draggable-menu-item ${selected ? 'selected' : ''} ${menuDrag.dragging === id ? 'drag-source' : ''} ${menuDrag.dropTarget?.id === id ? (menuDrag.dropTarget.after ? 'drop-after' : 'drop-before') : ''}`)
    item.setAttribute('aria-label', feature.name)
    if (selected) item.setAttribute('aria-current', 'page')
    item.dataset.featureId = id
    item.title = `${feature.name} · Ctrl+${state.menuOrder.indexOf(id) + 1} · 拖动调整顺序`
    const iconWrap = el('div', 'menu-icon')
    iconWrap.appendChild(appIconNode(id, 20))
    iconWrap.appendChild(html('span', '', icon('gripVertical', 18, 2, 'menu-grip')))
    item.appendChild(iconWrap)
    if (!state.collapsed) item.appendChild(el('span', '', feature.name))
    item.addEventListener('pointerdown', event => startMenuDrag(event, id))
    item.addEventListener('pointermove', moveMenuDrag)
    item.addEventListener('pointerup', dropMenuDrag)
    item.addEventListener('pointercancel', endMenuDrag)
    item.addEventListener('lostpointercapture', endMenuDrag)
    item.addEventListener('click', () => { if (!menuDrag.blockClick) setActive(id) })
    item.addEventListener('keydown', event => {
      menuDrag.blockClick = false
      if (!event.altKey || !['ArrowUp', 'ArrowDown'].includes(event.key) || !state.ready || state.bootError) return
      event.preventDefault()
      const direction = event.key === 'ArrowDown' ? 1 : -1
      const index = state.menuOrder.indexOf(id)
      const target = featureOf(state.menuOrder[index + direction])
      if (target) reorderMenu(id, target.id, direction === 1)
    })
    shell.menuApps.appendChild(item)
  })

  shell.sidebarBottom.textContent = ''
  const settingsItem = el('button', `feature-item own-settings ${state.active === 'settings' ? 'selected' : ''}`)
  if (state.active === 'settings') settingsItem.setAttribute('aria-current', 'page')
  settingsItem.title = '设置 · Ctrl+,'
  settingsItem.innerHTML = icon('settings2', 18, 1.6)
  if (!state.collapsed) settingsItem.appendChild(el('span', '', '设置'))
  settingsItem.addEventListener('click', () => setActive('settings'))
  shell.sidebarBottom.appendChild(settingsItem)
  if (!state.collapsed) {
    const badge = el('div', 'local-badge')
    badge.appendChild(html('span', '', icon('monitor', 14)))
    badge.appendChild(el('span', '', PLATFORM_NAMES[state.platform] || state.platform))
    badge.appendChild(el('small', '', `v${state.version}`))
    shell.sidebarBottom.appendChild(badge)
  }
}

function windowMaximizeIcon(maximized) {
  return maximized
    ? '<svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1" shape-rendering="crispEdges" aria-hidden="true"><path d="M3.5 3.5V1.5h7v7h-2M1.5 3.5h7v7h-7z"></path></svg>'
    : '<svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1" shape-rendering="crispEdges" aria-hidden="true"><rect x="1.5" y="1.5" width="9" height="9"></rect></svg>'
}

function renderTitlebar() {
  const bar = shell.titlebar
  bar.textContent = ''
  const breadcrumb = el('div', 'breadcrumb')
  breadcrumb.appendChild(el('span', '', 'wide-pure'))
  breadcrumb.appendChild(html('span', '', icon('chevronRight', 13)))
  breadcrumb.appendChild(el('span', '', pageTitle()))
  bar.appendChild(breadcrumb)
  const right = el('div', 'titlebar-right')
  if (state.platform !== 'darwin') {
    const controls = el('div', 'window-controls')
    const minimize = el('button')
    minimize.setAttribute('aria-label', '最小化')
    minimize.innerHTML = icon('minus', 15)
    minimize.addEventListener('click', () => { void windowAction('minimize') })
    const maximize = el('button')
    maximize.setAttribute('aria-label', state.windowMaximized ? '还原' : '最大化')
    maximize.title = state.windowMaximized ? '还原' : '最大化'
    maximize.innerHTML = windowMaximizeIcon(state.windowMaximized)
    maximize.addEventListener('click', () => { void windowAction('maximize') })
    const close = el('button', 'window-close')
    close.setAttribute('aria-label', '关闭窗口')
    close.innerHTML = icon('x', 16)
    close.addEventListener('click', () => { void windowAction('close') })
    controls.appendChild(minimize)
    controls.appendChild(maximize)
    controls.appendChild(close)
    right.appendChild(controls)
  }
  bar.appendChild(right)
}

function pageTitle() {
  if (state.active === 'settings') return '设置'
  if (state.active === 'start') return '开始'
  if (state.active === 'harness') return 'Harness'
  return featureOf(state.active)?.name ?? ''
}

function pageIconName() {
  if (state.active === 'settings') return 'settings2'
  if (state.active === 'start') return 'play'
  if (state.active === 'harness') return 'slidersHorizontal'
  return null
}

function renderShell() {
  shell.appShell.classList.toggle('sidebar-collapsed', state.collapsed)
  renderBrandBar()
  renderMenus()
  renderTitlebar()
  renderPageHeader()
}

function renderPageHeader() {
  // 页头位于 page-container 顶部，随页面一起渲染。
  renderPage()
}

// ---- 页面渲染 ----
const pages = { start: el('div', 'settings-page global-page'), app: el('div'), harness: null, settings: el('div', 'settings-page app-settings-page') }

function renderPage() {
  const container = shell.pageContainer
  if (!state.ready) {
    container.textContent = ''
    return
  }
  container.textContent = ''

  if (state.bootError) {
    const banner = el('div', 'error-banner', `初始化失败：${state.bootError}`)
    container.appendChild(banner)
  }
  if (state.configWarning) container.appendChild(el('div', 'info-banner', state.configWarning))
  const saveErrorList = Object.values(state.saveErrors)
  if (saveErrorList.length) {
    const banner = el('div', 'error-banner', `自动保存失败：${saveErrorList.join('；')}`)
    banner.setAttribute('role', 'alert')
    container.appendChild(banner)
  }

  const header = el('div', 'page-header')
  const titleGroup = el('div', 'page-title-group')
  const iconName = pageIconName()
  if (iconName) titleGroup.appendChild(html('span', '', icon(iconName, 26, 1.6)))
  else titleGroup.appendChild(appIconNode(state.active, 26))
  titleGroup.appendChild(el('h1', '', pageTitle()))
  header.appendChild(titleGroup)
  if (!['settings', 'start', 'harness'].includes(state.active)) {
    const actions = el('div', 'page-header-actions')
    const id = state.active
    const errors = settingsErrors(state.applications[id])
    const detection = state.detectionCache.get(detectionKey(id))
    const missing = detection?.checked && !detection.installation
    const invalid = Object.keys(errors).length > 0
    const disabled = !state.desktop || !!state.busy || !state.ready || !!state.bootError
    const running = state.busy?.id === id
    const start = el('button', 'button primary start-button')
    start.innerHTML = `${running && running.action === 'apply' ? icon('refreshCw', 15, 2, 'spin') : icon('play', 15, 2, 'fill-current')}<span>${running && running.action === 'apply' ? '启动中…' : '启动'}</span>`
    start.disabled = disabled || invalid || missing
    start.addEventListener('click', () => { void run(id, 'apply') })
    const quit = el('button', 'button secondary start-button')
    quit.innerHTML = `${running && running.action === 'exit' ? icon('refreshCw', 15, 2, 'spin') : icon('logOut', 15)}<span>${running && running.action === 'exit' ? '退出中…' : '退出'}</span>`
    quit.title = `完整退出 ${pageTitle()}`
    quit.disabled = disabled || !!errors.executablePath || missing
    quit.addEventListener('click', () => { void run(id, 'exit') })
    actions.appendChild(start)
    actions.appendChild(quit)
    header.appendChild(actions)
  }
  container.appendChild(header)

  if (state.active === 'start') container.appendChild(renderStartPage())
  else if (state.active === 'harness') {
    if (!harnessPage) {
      harnessPage = createHarnessPage({
        onBusyChange: value => { state.busy = value ? { id: 'harness', action: 'manage' } : null; renderShell() },
        onNotice: (text, error) => { state.notice = { text, error }; renderToast() },
        isDisabled: () => !!state.busy || !state.ready || !!state.bootError,
      })
      harnessPage.setInitialCollapse(state.harnessSettings)
    }
    container.appendChild(harnessPage.node)
    // 订阅回调在节点挂载前触发过 render（isConnected=false 被跳过），挂载后主动刷新绘制。
    harnessPage.refreshAll(false)
  } else if (state.active === 'settings') container.appendChild(renderSettingsPage())
  else container.appendChild(renderApplicationPage(state.active))

  if (state.restoreConfirmation) renderRestoreConfirmation()
}

// ---- 开始页 ----
function renderStartPage() {
  const page = el('div', 'settings-page global-page')
  const section = el('section', 'settings-group')
  section.setAttribute('aria-label', '全局应用操作')
  section.appendChild(el('h2', '', '应用'))
  const list = el('div', 'settings-list')
  const disabled = !state.desktop || !!state.busy || !state.ready || !!state.bootError

  ;(['start', 'restart', 'exit']).forEach(action => {
    const label = BATCH_ACTION_LABELS[action]
    const actionIcon = action === 'start' ? 'play' : action === 'restart' ? 'refreshCw' : 'logOut'
    const running = state.busy?.id === 'all' && state.busy.action === action
    const runningAll = running && state.busy.scope === 'all'
    const runningSelected = running && state.busy.scope === 'selected'
    const row = el('div', 'setting-row')
    row.appendChild(el('label', 'setting-label', label))
    const buttons = el('div', 'global-action-buttons')
    const selectedButton = el('button', 'button primary global-action-button')
    selectedButton.innerHTML = `${runningSelected ? icon('refreshCw', 15, 2, 'spin') : icon(actionIcon, 15)}<span>${runningSelected ? `${label}中…` : '选中'}</span>`
    selectedButton.setAttribute('aria-label', `${label}选中的 harness`)
    selectedButton.disabled = disabled || !state.selectedApplications.length
    selectedButton.addEventListener('click', () => { void runAll(action, [...state.selectedApplications]) })
    const allButton = el('button', 'button primary global-action-button')
    allButton.innerHTML = `${runningAll ? icon('refreshCw', 15, 2, 'spin') : icon(actionIcon, 15)}<span>${runningAll ? `${label}中…` : '所有'}</span>`
    allButton.setAttribute('aria-label', `${label}所有 harness`)
    allButton.disabled = disabled
    allButton.addEventListener('click', () => { void runAll(action) })
    buttons.appendChild(selectedButton)
    buttons.appendChild(allButton)
    row.appendChild(buttons)
    list.appendChild(row)
  })

  const selectionRow = el('div', 'setting-row global-selection-row')
  selectionRow.setAttribute('role', 'group')
  selectionRow.setAttribute('aria-label', '选择要启动、重启或退出的 harness')
  const options = el('div', 'global-application-options')
  state.menuOrder.forEach(id => {
    const feature = featureOf(id)
    const label = el('label', 'global-application-option')
    const checkbox = el('input')
    checkbox.type = 'checkbox'
    checkbox.checked = state.selectedApplications.includes(id)
    checkbox.disabled = disabled
    checkbox.addEventListener('change', () => {
      if (checkbox.checked) state.selectedApplications = [...state.selectedApplications, id]
      else state.selectedApplications = state.selectedApplications.filter(item => item !== id)
      renderPage()
    })
    label.appendChild(checkbox)
    label.appendChild(appIconNode(id, 16))
    label.appendChild(el('span', '', feature.name))
    options.appendChild(label)
  })
  selectionRow.appendChild(options)
  list.appendChild(selectionRow)
  section.appendChild(list)
  page.appendChild(section)

  if (state.batchError) {
    const banner = el('div', 'error-banner', state.batchError)
    banner.setAttribute('role', 'alert')
    page.appendChild(banner)
  }

  if (state.batchProgress) {
    const resultSection = el('section', 'settings-group')
    resultSection.setAttribute('aria-label', '全局操作结果')
    resultSection.appendChild(el('h2', '', state.busy?.id === 'all' ? '进行中' : '处理结果'))
    const progress = state.batchProgress
    if (state.busy?.id === 'all') {
      const progressWrap = el('div', 'batch-progress')
      progressWrap.setAttribute('role', 'status')
      progressWrap.setAttribute('aria-live', 'polite')
      const line = el('div')
      line.appendChild(html('span', '', icon('refreshCw', 15, 2, 'spin')))
      const text = el('span', '', progress.currentIds.length
        ? `正在${BATCH_ACTION_LABELS[progress.action]} ${progress.currentIds.map(id => APPLICATIONS[id].name).join('、')}…`
        : progress.completed === progress.total ? '正在完成…' : '准备中…')
      line.appendChild(text)
      line.appendChild(el('span', 'batch-count', `${progress.completed} / ${progress.total}`))
      progressWrap.appendChild(line)
      const bar = el('progress')
      bar.value = progress.completed
      bar.max = progress.total
      bar.setAttribute('aria-label', '全局操作进度')
      progressWrap.appendChild(bar)
      resultSection.appendChild(progressWrap)
    }
    const results = el('div', 'settings-list batch-results')
    progress.applicationIds.forEach(id => {
      const result = progress.results.find(item => item.id === id)
      const running = progress.currentIds.includes(id)
      const status = result?.status ?? (running ? 'running' : 'queued')
      const row = el('div', `batch-result setting-row ${status}`)
      const name = el('div', 'batch-app-name')
      name.appendChild(appIconNode(id, 20))
      name.appendChild(el('span', '', APPLICATIONS[id].name))
      row.appendChild(name)
      const message = el('span', 'batch-result-message')
      if (running) message.appendChild(html('span', '', icon('refreshCw', 13, 2, 'spin')))
      message.appendChild(document.createTextNode(result?.message ?? BATCH_STAGE_LABELS[progress.stages[id] ?? 'queued']))
      row.appendChild(message)
      results.appendChild(row)
    })
    resultSection.appendChild(results)
    page.appendChild(resultSection)
  }
  return page
}

// ---- 应用设置页 ----
const detectionKey = id => JSON.stringify([id, state.applications[id].executablePath])

function detectionFresh(result) {
  if (!result?.checked) return false
  const age = Date.now() - result.checkedAt
  if (result.error) return age < 5000
  return age < (result.installation ? 60000 : 15000)
}

function requestDetection(id, force = true, pathOverride) {
  if (!state.desktop) return Promise.resolve()
  const settings = { ...state.applications[id], ...(pathOverride !== undefined ? { executablePath: pathOverride } : {}) }
  const key = JSON.stringify([id, settings.executablePath])
  const existing = state.detectionRequests.get(key)
  if (existing) return existing
  if (!force && detectionFresh(state.detectionCache.get(key))) return Promise.resolve()
  state.detectionCache.set(key, { installation: state.detectionCache.get(key)?.installation ?? null, error: '', checking: true, checked: false, checkedAt: 0 })
  const isActive = state.active === id
  if (isActive) renderPage()
  const request = api('detect', { featureId: id, path: settings.executablePath, force }).then(installation => {
    state.detectionCache.set(key, { installation, error: '', checking: false, checked: true, checkedAt: Date.now() })
  }).catch(error => {
    state.detectionCache.set(key, { installation: null, error: messageOf(error), checking: false, checked: true, checkedAt: Date.now() })
  }).finally(() => { state.detectionRequests.delete(key); if (state.active === id) renderPage() })
  state.detectionRequests.set(key, request)
  return request
}

function renderApplicationPage(id) {
  const page = el('div', `settings-page application-settings-${id}`)
  const capability = APPLICATIONS[id]
  const settings = state.applications[id]
  const defaults = DEFAULT_APPLICATIONS[id]
  const errors = settingsErrors(settings)
  const disabled = !!state.busy || !state.ready || !!state.bootError
  const detection = state.detectionCache.get(detectionKey(id))
  const installation = detection?.installation ?? null
  const detecting = detection?.checking ?? false
  const detectionError = detection?.error ?? ''
  const missing = detection?.checked && !installation

  const inputProps = key => ({ disabled, invalid: !!errors[key], describedBy: errors[key] ? `${key}-error` : undefined })

  const update = (key, value) => {
    const candidate = { ...state.applications[id], [key]: value }
    state.applications[id] = candidate
    if (Object.keys(settingsErrors(candidate)).length === 0) {
      persist(id, () => api('save', { featureId: id, settings: candidate }))
    }
  }

  // ---- 布局 ----
  const layoutSection = el('section', 'settings-group')
  layoutSection.setAttribute('aria-label', '内容布局')
  layoutSection.appendChild(el('h2', '', '布局'))
  const layoutList = el('div', 'settings-list')
  if (capability.sidebarWidth) {
    layoutList.appendChild(DimensionControl({ id: 'sidebarWidth', label: '左侧栏宽度', value: settings.sidebarWidth, fallback: defaults.sidebarWidth, disabled, error: errors.sidebarWidth, onChange: value => update('sidebarWidth', value) }))
  }
  layoutList.appendChild(DimensionControl({ id: 'width', label: '内容区宽度', value: settings.width, fallback: defaults.width, disabled, error: errors.width, onChange: value => update('width', value) }))
  if (capability.maxWidth) {
    layoutList.appendChild(DimensionControl({ id: 'maxWidth', label: '最大宽度', value: settings.maxWidth, fallback: defaults.maxWidth, disabled, error: errors.maxWidth, onChange: value => update('maxWidth', value) }))
  }
  if (capability.chatHeight) {
    const chat = PixelControl({ id: 'chatHeight', min: 0, max: 9999, step: 5, value: settings.chatHeight.endsWith('px') ? settings.chatHeight.slice(0, -2) : '', placeholder: !settings.chatHeight.endsWith('px') ? settings.chatHeight : undefined, ...inputProps('chatHeight'), onInput: value => update('chatHeight', `${value}px`) })
    layoutList.appendChild(SettingRow('输入框高度', 'chatHeight', errors.chatHeight, chat.node))
  }
  layoutSection.appendChild(layoutList)
  page.appendChild(layoutSection)

  // ---- 字体 ----
  const fontSection = el('section', 'settings-group')
  fontSection.setAttribute('aria-label', `${pageTitle()} 字体`)
  fontSection.appendChild(el('h2', '', '字体'))
  const fontList = el('div', 'settings-list')
  if (capability.fontFamily) {
    const font = FontControl({ id: 'fontFamily', label: `${pageTitle()} 字体`, value: settings.fontFamily, disabled, error: errors.fontFamily, onChange: value => update('fontFamily', value) })
    fontList.appendChild(SettingRow('字体', 'fontFamily', errors.fontFamily, font.node))
  }
  if (capability.fontSize) {
    const fontSize = PixelControl({ id: 'fontSize', min: 8, max: 72, value: settings.fontSize, ...inputProps('fontSize'), onInput: value => update('fontSize', Number(value)) })
    fontList.appendChild(SettingRow('字号', 'fontSize', errors.fontSize, fontSize.node))
  }
  const weight = el('input', 'field-input number-input')
  weight.id = 'fontWeight'
  weight.type = 'number'
  weight.min = '100'
  weight.max = '1000'
  weight.step = '100'
  weight.value = settings.fontWeight
  weight.disabled = disabled
  if (errors.fontWeight) weight.setAttribute('aria-invalid', 'true')
  if (errors.fontWeight) weight.setAttribute('aria-describedby', 'fontWeight-error')
  weight.addEventListener('input', () => update('fontWeight', Number(weight.value)))
  fontList.appendChild(SettingRow('字重', 'fontWeight', errors.fontWeight, weight))
  if (capability.terminal) {
    fontList.appendChild(SettingRow('终端跟随', null, null, Switch('终端跟随', settings.terminalFollow, disabled, value => update('terminalFollow', value))))
  }
  fontSection.appendChild(fontList)
  page.appendChild(fontSection)

  // ---- 界面 ----
  if (capability.merge || capability.diff || capability.changes || capability.summary) {
    const uiSection = el('section', 'settings-group')
    uiSection.setAttribute('aria-label', `${pageTitle()} 界面`)
    uiSection.appendChild(el('h2', '', '界面'))
    const uiList = el('div', 'settings-list')
    if (capability.merge) uiList.appendChild(SettingRow('隐藏本地 Merge', null, null, Switch('隐藏本地 Merge', settings.hideLocalMerge, disabled, value => update('hideLocalMerge', value))))
    if (capability.diff) uiList.appendChild(SettingRow('隐藏 Git Diff 统计', null, null, Switch('隐藏 Git Diff 统计', settings.hideGitDiff, disabled, value => update('hideGitDiff', value))))
    if (capability.changes) uiList.appendChild(SettingRow('隐藏右上角更改控件', null, null, Switch('隐藏右上角更改控件', settings.hideChanges, disabled, value => update('hideChanges', value))))
    if (capability.summary) uiList.appendChild(SettingRow('阻止摘要面板自动弹出', null, null, Switch('阻止摘要面板自动弹出', settings.preventSummary, disabled, value => update('preventSummary', value))))
    uiSection.appendChild(uiList)
    page.appendChild(uiSection)
  }

  // ---- 应用 ----
  const appSection = el('section', 'settings-group')
  appSection.setAttribute('aria-label', `${pageTitle()} 应用连接`)
  appSection.appendChild(el('h2', '', '应用'))
  const appList = el('div', 'settings-list')
  const installValue = el('div', `installation-value ${installation ? 'connected' : ''}`)
  const statusText = installation
    ? `${installation.name} ${installation.version}`
    : detecting ? '检测中…' : state.desktop ? `未找到 ${id === 'droid' ? 'Droid / Factory' : pageTitle()}` : '桌面连接未启用'
  installValue.appendChild(el('span', '', statusText))
  const redetect = el('button', 'icon-button')
  redetect.title = '重新检测'
  redetect.setAttribute('aria-label', '重新检测')
  redetect.disabled = !state.desktop || detecting || !!state.busy
  redetect.innerHTML = icon('refreshCw', 16, 2, detecting ? 'spin' : '')
  redetect.addEventListener('click', () => { void requestDetection(id, true) })
  installValue.appendChild(redetect)
  const choose = el('button', 'icon-button')
  choose.title = '选择应用'
  choose.setAttribute('aria-label', '选择应用')
  choose.disabled = !state.desktop || !!state.busy
  choose.innerHTML = icon('folderOpen', 17)
  choose.addEventListener('click', () => { void chooseExecutable(id) })
  installValue.appendChild(choose)
  appList.appendChild(SettingRow('安装状态', null, null, installValue))

  const disclosureRow = el('div', 'setting-row')
  disclosureRow.appendChild(el('label', 'setting-label', '高级设置'))
  const disclosureButton = el('button', `disclosure-button ${state.advanced ? 'open' : ''}`)
  disclosureButton.setAttribute('aria-label', '高级设置')
  disclosureButton.setAttribute('aria-expanded', state.advanced ? 'true' : 'false')
  disclosureButton.setAttribute('aria-controls', 'advanced-content')
  disclosureButton.innerHTML = icon('chevronDown', 17)
  disclosureButton.addEventListener('click', () => { state.advanced = !state.advanced; renderPage() })
  disclosureRow.appendChild(disclosureButton)
  appList.appendChild(disclosureRow)

  if (state.advanced) {
    const advancedWrap = el('div')
    advancedWrap.id = 'advanced-content'
    const pathInput = el('input', 'field-input path-input')
    pathInput.id = 'executablePath'
    pathInput.placeholder = '自动检测'
    pathInput.value = settings.executablePath
    pathInput.disabled = disabled
    pathInput.title = installation?.path ?? ''
    if (errors.executablePath) pathInput.setAttribute('aria-invalid', 'true')
    if (errors.executablePath) pathInput.setAttribute('aria-describedby', 'executablePath-error')
    let debounce = null
    pathInput.addEventListener('input', () => {
      const value = pathInput.value
      update('executablePath', value)
      clearTimeout(debounce)
      debounce = setTimeout(() => { if (!settingsErrors(state.applications[id]).executablePath) void requestDetection(id, false, value) }, 400)
    })
    advancedWrap.appendChild(SettingRow('安装路径', 'executablePath', errors.executablePath, pathInput))
    const portInput = el('input', 'field-input number-input')
    portInput.id = 'port'
    portInput.type = 'number'
    portInput.min = '1024'
    portInput.max = '65535'
    portInput.value = settings.port
    portInput.disabled = disabled
    if (errors.port) portInput.setAttribute('aria-invalid', 'true')
    if (errors.port) portInput.setAttribute('aria-describedby', 'port-error')
    portInput.addEventListener('input', () => update('port', Number(portInput.value)))
    advancedWrap.appendChild(SettingRow('调试端口', 'port', errors.port, portInput))
    appList.appendChild(advancedWrap)
  }
  appSection.appendChild(appList)
  if (detectionError) {
    const error = el('p', 'field-error detection-error', detectionError)
    error.setAttribute('role', 'alert')
    appSection.appendChild(error)
  }
  page.appendChild(appSection)

  // ---- 操作 ----
  const actions = el('div', 'settings-actions')
  const resetPresets = el('button', 'text-button')
  resetPresets.innerHTML = `${icon('rotateCcw', 15)}<span>恢复预设值</span>`
  resetPresets.disabled = disabled
  resetPresets.addEventListener('click', () => askRestore('presets'))
  actions.appendChild(resetPresets)
  const resetNormal = el('button', 'text-button')
  const resetting = state.busy?.id === id && state.busy.action === 'normal'
  resetNormal.innerHTML = resetting ? `${icon('refreshCw', 15, 2, 'spin')}<span>恢复中…</span>` : `${icon('rotateCcw', 15)}<span>恢复默认界面</span>`
  resetNormal.disabled = !state.desktop || disabled || Object.keys(errors).length > 0 || missing
  resetNormal.addEventListener('click', () => askRestore('normal'))
  actions.appendChild(resetNormal)
  page.appendChild(actions)

  // 页面展示缓存，过期数据在进入页面时后台校验。
  if (!detectionFresh(detection) && !errors.executablePath) {
    setTimeout(() => { void requestDetection(id, false) }, settings.executablePath ? 400 : 0)
  }
  return page
}

// ---- 设置页 ----
function renderSettingsPage() {
  const page = pages.settings
  page.textContent = ''
  const disabled = !!state.busy || !state.ready || !!state.bootError
  const appearance = state.appearance
  const appErrors = appearanceErrors(appearance)

  function updateAppearance(key, value) {
    const candidate = { ...state.appearance, [key]: value }
    state.appearance = candidate
    applyAppearance()
    if (Object.keys(appearanceErrors(candidate)).length === 0) {
      persist('appearance', () => api('appearance', candidate))
    }
  }

  const appearanceSection = el('section', 'settings-group')
  appearanceSection.setAttribute('aria-label', '应用外观')
  appearanceSection.appendChild(el('h2', '', '外观'))
  const appearanceList = el('div', 'settings-list')

  const themeSelect = SelectControl({
    id: 'app-theme', label: '应用主题', value: state.theme, disabled: !state.ready || !!state.bootError,
    options: [{ value: 'system', label: '跟随系统' }, { value: 'light', label: '浅色' }, { value: 'dark', label: '深色' }],
    onChange: value => changeTheme(value),
  })
  appearanceList.appendChild(SettingRow('主题', 'app-theme', null, themeSelect.node))

  const font = FontControl({ id: 'app-fontFamily', value: appearance.fontFamily, systemOption: true, disabled: !state.ready || !!state.bootError, error: appErrors.fontFamily, onChange: value => updateAppearance('fontFamily', value) })
  appearanceList.appendChild(SettingRow('字体', 'app-fontFamily', appErrors.fontFamily, font.node))

  const fontSize = PixelControl({ id: 'app-fontSize', min: 10, max: 24, value: appearance.fontSize, disabled: !state.ready || !!state.bootError, invalid: !!appErrors.fontSize, describedBy: appErrors.fontSize ? 'app-fontSize-error' : undefined, onInput: value => updateAppearance('fontSize', Number(value)) })
  appearanceList.appendChild(SettingRow('字号', 'app-fontSize', appErrors.fontSize, fontSize.node))

  const modeSelect = SelectControl({
    id: 'app-mode', label: '应用模式', value: appearance.mode, disabled: !state.ready || !!state.bootError,
    options: [{ value: 'normal', label: '正常' }, { value: 'compact', label: '紧凑' }],
    onChange: value => updateAppearance('mode', value),
  })
  appearanceList.appendChild(SettingRow('模式', 'app-mode', appErrors.mode, modeSelect.node))
  appearanceSection.appendChild(appearanceList)
  page.appendChild(appearanceSection)

  const harnessSection = el('section', 'settings-group')
  harnessSection.setAttribute('aria-label', 'Harness 设置')
  harnessSection.appendChild(el('h2', '', 'Harness'))
  const harnessList = el('div', 'settings-list')
  harnessList.appendChild(SettingRow('Skills 折叠', null, null, Switch('Skills 折叠', state.harnessSettings.skillsCollapsed, disabled, value => changeHarnessSetting('skillsCollapsed', value))))
  harnessList.appendChild(SettingRow('Models 折叠', null, null, Switch('Models 折叠', state.harnessSettings.modelsCollapsed, disabled, value => changeHarnessSetting('modelsCollapsed', value))))
  harnessList.appendChild(SettingRow('MCPs 折叠', null, null, Switch('MCPs 折叠', state.harnessSettings.mcpsCollapsed, disabled, value => changeHarnessSetting('mcpsCollapsed', value))))
  harnessSection.appendChild(harnessList)
  page.appendChild(harnessSection)

  const startupSection = el('section', 'settings-group')
  startupSection.setAttribute('aria-label', '应用启动')
  startupSection.appendChild(el('h2', '', '启动'))
  const startupList = el('div', 'settings-list')
  startupList.appendChild(SettingRow('开机启动', null, null, Switch('开机启动', state.openAtLogin, disabled || !state.desktop || !state.startupAvailable, value => changeOpenAtLogin(value))))
  const startupMode = SelectControl({
    id: 'app-startupMode', label: '应用启动方式', value: state.startupMode, disabled: !state.ready || !!state.bootError,
    options: [{ value: 'default', label: '默认' }, { value: 'maximized', label: '最大化' }],
    onChange: value => { state.startupMode = value; persist('startupMode', () => api('startup-mode', { mode: value })) },
  })
  startupList.appendChild(SettingRow('启动方式', 'app-startupMode', null, startupMode.node))
  startupSection.appendChild(startupList)
  page.appendChild(startupSection)

  const actions = el('div', 'settings-actions')
  const reset = el('button', 'text-button')
  reset.innerHTML = `${icon('rotateCcw', 15)}<span>恢复预设值</span>`
  reset.disabled = disabled
  reset.addEventListener('click', () => askRestore('presets'))
  actions.appendChild(reset)
  page.appendChild(actions)
  return page
}

// ---- 持久化 ----
const pending = new Set()
const saveRevisions = {}
const saveErrorsRef = { current: {} }

function persist(domain, task) {
  saveRevisions[domain] = (saveRevisions[domain] ?? 0) + 1
  const revision = saveRevisions[domain]
  const promise = task().then(() => {
    if (revision !== saveRevisions[domain]) return
    const next = { ...saveErrorsRef.current }
    delete next[domain]
    saveErrorsRef.current = next
    state.saveErrors = next
    state.configWarning = ''
    renderPage()
  }).catch(error => {
    if (revision !== saveRevisions[domain]) return
    saveErrorsRef.current = { ...saveErrorsRef.current, [domain]: messageOf(error) }
    state.saveErrors = saveErrorsRef.current
    renderPage()
  }).finally(() => { pending.delete(promise) })
  pending.add(promise)
}

function applyAppearance() {
  const value = state.appearance
  document.documentElement.style.fontFamily = value.fontFamily === '系统默认' ? SYSTEM_FONT : `${value.fontFamily}, ${SYSTEM_FONT}`
  document.documentElement.style.fontSize = `${value.fontSize}px`
  document.documentElement.dataset.mode = value.mode
}

function changeTheme(next) {
  state.theme = next
  applyTheme()
  persist('theme', () => api('theme', { theme: next }))
}

function applyTheme() {
  const dark = state.theme === 'dark' || (state.theme === 'system' && state.systemDark)
  document.documentElement.dataset.theme = dark ? 'dark' : 'light'
}

function changeHarnessSetting(key, value) {
  const next = { ...state.harnessSettings, [key]: value }
  state.harnessSettings = next
  harnessPage?.setCollapsed(next)
  persist('harness', () => api('harness-settings', next))
}

function changeOpenAtLogin(next) {
  persist('openAtLogin', async () => {
    const enabled = await api('open-at-login', { enabled: next })
    state.openAtLogin = enabled
    renderPage()
  })
}

function reorderMenu(source, target, after) {
  const current = state.menuOrder
  if (source === target) return
  const next = current.filter(id => id !== source)
  const targetIndex = next.indexOf(target)
  if (targetIndex === -1) return
  next.splice(targetIndex + (after ? 1 : 0), 0, source)
  if (next.every((id, index) => id === current[index])) return
  state.menuOrder = next
  shell.announcement.textContent = `${featureOf(source).name} 已移至第 ${next.indexOf(source) + 1} 位`
  persist('menuOrder', () => api('menu-order', { order: next }))
  renderShell()
}

// ---- 菜单拖拽 ----
const menuDrag = { dragging: null, dropTarget: null, drag: null, blockClick: false, preview: null }

function menuTargetAt(x, y) {
  const menu = shell.menuApps
  const bounds = menu.getBoundingClientRect()
  if (x < bounds.left || x > bounds.right || y < bounds.top || y > bounds.bottom) return null
  const items = Array.from(menu.querySelectorAll('[data-feature-id]'))
  const before = items.find(item => { const rect = item.getBoundingClientRect(); return y < rect.top + rect.height / 2 })
  const target = before ?? items[items.length - 1]
  return target ? { id: target.dataset.featureId, after: !before } : null
}

function startMenuDrag(event, id) {
  menuDrag.blockClick = false
  if (event.button !== 0 || !state.ready || state.bootError) return
  const rect = event.currentTarget.getBoundingClientRect()
  menuDrag.drag = { id, pointerId: event.pointerId, startX: event.clientX, startY: event.clientY, left: rect.left, top: rect.top, width: rect.width, active: false }
  event.currentTarget.setPointerCapture(event.pointerId)
}

function moveMenuDrag(event) {
  const drag = menuDrag.drag
  if (!drag || drag.pointerId !== event.pointerId) return
  if (!drag.active) {
    if (Math.hypot(event.clientX - drag.startX, event.clientY - drag.startY) < 6) return
    drag.active = true
    menuDrag.blockClick = true
    menuDrag.dragging = drag.id
  }
  const preview = shell.overlays.querySelector('.menu-drag-preview')
  const node = preview ?? el('div', 'feature-item menu-drag-preview')
  if (!preview) {
    node.setAttribute('aria-hidden', 'true')
    shell.overlays.appendChild(node)
  }
  node.style.left = (drag.left + event.clientX - drag.startX) + 'px'
  node.style.top = (drag.top + event.clientY - drag.startY) + 'px'
  node.style.width = drag.width + 'px'
  node.textContent = ''
  node.appendChild(appIconNode(drag.id, 20))
  if (!state.collapsed) node.appendChild(el('span', '', featureOf(drag.id).name))
  const target = menuTargetAt(event.clientX, event.clientY)
  menuDrag.dropTarget = target?.id === drag.id ? null : target
  renderMenuDragState()
}

function dropMenuDrag(event) {
  const drag = menuDrag.drag
  if (!drag || drag.pointerId !== event.pointerId) return
  const target = drag.active ? menuTargetAt(event.clientX, event.clientY) : null
  endMenuDrag()
  if (target) reorderMenu(drag.id, target.id, target.after)
}

function renderMenuDragState() {
  // 保留按钮节点，避免丢失指针捕获和松开鼠标后的 click 事件。
  shell.menuApps.classList.toggle('menu-dragging', !!menuDrag.dragging)
  shell.menuApps.querySelectorAll('[data-feature-id]').forEach(item => {
    const id = item.dataset.featureId
    const target = menuDrag.dropTarget?.id === id
    item.classList.toggle('drag-source', menuDrag.dragging === id)
    item.classList.toggle('drop-before', target && !menuDrag.dropTarget.after)
    item.classList.toggle('drop-after', target && menuDrag.dropTarget.after)
  })
}

function endMenuDrag() {
  const drag = menuDrag.drag
  if (!drag) return
  menuDrag.drag = null
  menuDrag.dragging = null
  menuDrag.dropTarget = null
  const source = shell.menuApps.querySelector(`[data-feature-id="${drag.id}"]`)
  if (source?.hasPointerCapture(drag.pointerId)) source.releasePointerCapture(drag.pointerId)
  const preview = shell.overlays.querySelector('.menu-drag-preview')
  if (preview) preview.remove()
  renderMenuDragState()
}

// ---- 窗口与操作 ----
async function windowAction(action) {
  if (action === 'close') {
    await Promise.all([...pending])
    if (Object.keys(saveErrorsRef.current).length) {
      state.notice = { text: '自动保存失败，请修正设置后再关闭窗口。', error: true }
      renderToast()
      return
    }
  }
  void api('window', { action })
}

async function chooseExecutable(id) {
  try {
    const path = await api('choose', { featureId: id })
    if (path) {
      state.applications[id] = { ...state.applications[id], executablePath: path }
      persist(id, () => api('save', { featureId: id, settings: state.applications[id] }))
      renderPage()
    }
  } catch (error) {
    state.notice = { text: messageOf(error), error: true }
    renderToast()
  }
}

async function run(id, action) {
  const errors = settingsErrors(state.applications[id])
  const detection = state.detectionCache.get(detectionKey(id))
  const missing = detection?.checked && !detection.installation
  if (!state.desktop || !!state.busy || !state.ready || !!state.bootError) return
  if (action === 'exit' ? !!errors.executablePath : Object.keys(errors).length) return
  state.busy = { id, action }
  state.notice = null
  renderShell(); renderPage(); renderToast()
  try {
    await Promise.all([...pending])
    const value = state.applications[id]
    const result = action === 'exit'
      ? await api('quit', { featureId: id, path: value.executablePath })
      : await api('run', { featureId: id, action, settings: value })
    state.notice = { text: result.message, error: !result.success }
    if (result.success && action !== 'exit') {
      const next = { ...saveErrorsRef.current }
      delete next[id]
      saveErrorsRef.current = next
      state.saveErrors = next
    }
    if (action !== 'exit') void requestDetection(id, true)
  } catch (error) {
    state.notice = { text: messageOf(error), error: true }
  } finally {
    state.busy = null
    renderShell(); renderPage(); renderToast()
  }
}

async function runAll(action, ids) {
  if (!state.desktop || !!state.busy || !state.ready || !!state.bootError || state.batchRunning) return
  const order = state.menuOrder.filter(id => !ids || ids.includes(id))
  if (!order.length) return
  state.batchRunning = true
  state.busy = { id: 'all', action, scope: ids ? 'selected' : 'all' }
  state.notice = null
  state.batchError = ''
  state.batchProgress = { action, applicationIds: order, stages: Object.fromEntries(order.map(id => [id, 'queued'])), currentIds: [], completed: 0, total: order.length, results: [] }
  renderShell(); renderPage(); renderToast()
  try {
    await Promise.all([...pending])
    if (action !== 'exit' && Object.keys(saveErrorsRef.current).length) throw new Error('自动保存失败，请修正设置后再执行全局操作。')
    const result = await api('run-all', { action, ids: ids ? order : undefined })
    state.batchProgress = { action, applicationIds: order, stages: {}, currentIds: [], completed: result.results.length, total: order.length, results: result.results }
  } catch (error) {
    state.batchProgress = null
    state.batchError = messageOf(error)
  } finally {
    state.batchRunning = false
    state.busy = null
    renderShell(); renderPage(); renderToast()
  }
}

function setActive(id) {
  state.active = id
  state.advanced = false
  renderShell()
}

// ---- 恢复确认 ----
function askRestore(action) {
  const disabled = !!state.busy || !state.ready || !!state.bootError
  const isApp = !['settings', 'start', 'harness'].includes(state.active)
  if (disabled || state.restoreConfirmation || (!isApp && state.active !== 'settings')) return
  if (action === 'normal') {
    const id = state.active
    const errors = settingsErrors(state.applications[id])
    const detection = state.detectionCache.get(detectionKey(id))
    if (!isApp || !state.desktop || Object.keys(errors).length || (detection?.checked && !detection.installation)) return
  }
  state.restoreConfirmation = { action, target: state.active === 'settings' ? 'settings' : state.active }
  renderRestoreConfirmation()
}

function renderRestoreConfirmation() {
  if (!state.restoreConfirmation) return
  if (shell.overlays.querySelector('.restore-confirmation-dialog')) return
  const request = state.restoreConfirmation
  const disabled = !!state.busy || !state.ready || !!state.bootError
  const targetName = request.target === 'settings' ? 'wide-pure' : APPLICATIONS[request.target].name
  const message = request.action === 'normal'
    ? `将恢复 ${targetName} 的默认界面并重新启动应用，请先保存当前工作。是否继续？`
    : request.target === 'settings'
      ? '将把 wide-pure 的主题、字体、字号、模式、Harness 折叠、开机启动和启动方式恢复为预设值。是否继续？'
      : `将把 ${targetName} 的设置恢复为预设值。是否继续？`
  const content = el('div')
  const text = el('p', 'restore-confirmation-message', message)
  text.id = 'restore-confirmation-message'
  content.appendChild(text)
  const actions = el('div', 'harness-actions model-editor-actions')
  const cancel = el('button', 'button secondary')
  cancel.type = 'button'
  cancel.textContent = '取消'
  cancel.dataset.dialogInitialFocus = 'true'
  cancel.disabled = disabled
  const ok = el('button', 'button primary')
  ok.type = 'button'
  ok.textContent = '确定'
  ok.disabled = disabled
  actions.appendChild(cancel)
  actions.appendChild(ok)
  content.appendChild(actions)
  const dialog = ModelDialog({
    title: request.action === 'normal' ? '恢复默认界面' : '恢复预设值',
    subtitle: request.target === 'settings' ? 'wide-pure 设置' : APPLICATIONS[request.target].name,
    disabled,
    returnFocus: document.activeElement instanceof HTMLElement ? document.activeElement : null,
    onCancel: () => { state.restoreConfirmation = null },
    closeLabel: '关闭确认弹窗',
    descriptionId: 'restore-confirmation-message',
    className: 'restore-confirmation-dialog',
    content,
  })
  ok.addEventListener('click', () => {
    if (state.busy) return
    dialog.cleanup()
    confirmRestore()
  })
}

function confirmRestore() {
  const request = state.restoreConfirmation
  if (!request || !!state.busy) return
  state.restoreConfirmation = null
  if (request.target !== state.active) return
  if (request.action === 'normal') { void run(request.target, 'normal'); return }
  if (request.target === 'settings') {
    state.appearance = { ...DEFAULT_APPEARANCE }
    applyAppearance()
    persist('appearance', () => api('appearance', state.appearance))
    changeTheme('system')
    state.startupMode = 'default'
    persist('startupMode', () => api('startup-mode', { mode: 'default' }))
    state.harnessSettings = { ...DEFAULT_HARNESS_SETTINGS }
    harnessPage?.setCollapsed(state.harnessSettings)
    persist('harness', () => api('harness-settings', state.harnessSettings))
    if (state.startupAvailable && state.openAtLogin) changeOpenAtLogin(false)
    renderPage()
  } else {
    const id = request.target
    state.applications[id] = { ...DEFAULT_APPLICATIONS[id] }
    persist(id, () => api('save', { featureId: id, settings: state.applications[id] }))
    renderPage()
  }
}

// ---- 提示 ----
function renderToast() {
  const existing = shell.overlays.querySelector('.toast')
  if (existing) existing.remove()
  if (!state.notice) return
  const toast = html('div', `toast ${state.notice.error ? 'error' : ''}`,
    `${icon(state.notice.error ? 'circleAlert' : 'check', 18)}<span></span><button class="icon-button" aria-label="关闭提示">${icon('x', 15)}</button>`)
  toast.setAttribute('role', state.notice.error ? 'alert' : 'status')
  toast.querySelector('span').textContent = state.notice.text
  toast.querySelector('button').addEventListener('click', () => { state.notice = null; renderToast() })
  shell.overlays.appendChild(toast)
  if (!state.notice.error) {
    setTimeout(() => { if (toast.isConnected) { state.notice = null; renderToast() } }, 3500)
  }
}

// ---- 启动 ----
async function bootstrap() {
  try {
    const data = await api('bootstrap')
    state.desktop = data.desktop
    state.platform = data.platform
    state.version = data.version
    state.applications = { ...DEFAULT_APPLICATIONS, ...data.preferences.applications }
    state.appearance = data.preferences.appearance
    state.harnessSettings = data.preferences.harness
    state.theme = data.preferences.theme
    state.startupMode = data.preferences.startupMode
    state.menuOrder = normalizeMenuOrder(data.preferences.menuOrder)
    state.openAtLogin = data.openAtLogin
    state.startupAvailable = data.startupAvailable
    state.windowMaximized = data.windowMaximized
    state.configWarning = data.configWarning || ''
    state.ready = true
  } catch (error) {
    state.bootError = messageOf(error)
    state.ready = true
  }
  applyTheme()
  applyAppearance()
  if (harnessPage && state.harnessSettings) harnessPage.setInitialCollapse(state.harnessSettings)
  renderShell()
  renderPage()
  // 首屏绘制后预检各应用安装。
  setTimeout(() => {
    state.menuOrder.filter(id => id !== state.active).forEach(id => {
      if (!settingsErrors(state.applications[id]).executablePath) void requestDetection(id, false)
    })
  }, 800)
}

// 键盘快捷键。
window.addEventListener('keydown', event => {
  if (shell.appShell.inert) return
  if (event.key === 'Escape' && menuDrag.drag) { event.preventDefault(); endMenuDrag(); return }
  if (!(event.ctrlKey || event.metaKey)) return
  if (/^[1-7]$/.test(event.key)) {
    event.preventDefault()
    setActive(state.menuOrder[Number(event.key) - 1])
  }
  if (event.key === ',') {
    event.preventDefault()
    setActive('settings')
  }
})

// 系统主题变化。
matchMedia('(prefers-color-scheme: dark)').addEventListener('change', event => {
  state.systemDark = event.matches
  applyTheme()
})

// 回到窗口时刷新缓存与检测。
window.addEventListener('focus', () => {
  if (Date.now() - (modelsResource.snapshot().updatedAt) >= 2000) void modelsResource.refresh(true)
  if (Date.now() - (mcpsResource.snapshot().updatedAt) >= 2000) void mcpsResource.refresh(true)
  if (Date.now() - (skillsResource.snapshot().updatedAt) >= 2000) void skillsResource.refresh(true)
  if (Date.now() - (agentsResource.snapshot().updatedAt) >= 2000) void agentsResource.refresh(true)
  if (!state.busy && !document.hidden && ['settings', 'start', 'harness'].indexOf(state.active) === -1) {
    void requestDetection(state.active, false)
  }
})

// 窗口最大化状态轮询。
setInterval(async () => {
  if (!state.desktop) return
  try {
    const data = await api('window/state')
    if (data.maximized !== state.windowMaximized) {
      state.windowMaximized = data.maximized
      renderTitlebar()
    }
  } catch { /* 服务未就绪时忽略 */ }
}, 500)

// 通知轮询（后台连接错误等）。
let noticeCursor = 0
setInterval(async () => {
  if (!state.desktop) return
  try {
    const data = await api(`notices?since=${noticeCursor}`)
    noticeCursor = data.cursor
    data.notices.forEach(notice => {
      state.notice = { text: notice.message, error: !notice.success }
      renderToast()
    })
  } catch { /* 忽略 */ }
}, 1000)

// 批量进度轮询。
setInterval(async () => {
  if (!state.desktop || !state.batchRunning) return
  try {
    const data = await api('batch/progress')
    if (data.progress && data.active) {
      state.batchProgress = data.progress
      renderPage()
    }
  } catch { /* 忽略 */ }
}, 400)

// 拖拽运行时（frameless 窗口拖动）。
if (state.desktop) {
  import('/wails/runtime.js').catch(() => null)
}

void bootstrap()
