// Harness 页：AGENTS.md 同步、Skills、Models、MCPs。
import { el, html, icon } from './controls.js'
import { api, messageOf } from './api.js'
import { agentsResource, skillsResource, modelsResource, mcpsResource, displayHarnessPath, emptyHarnesses } from './resources.js'
import { ModelEditorDialog, McpEditorDialog, ModelBatchDialog, ModelDialog, DEFAULT_MODEL_BASE_URL } from './dialogs.js'

const MODEL_HARNESSES = ['claude', 'droid', 'dsh', 'pi', 'opencode']
const MODEL_PATHS = { claude: '~/.claude/settings.json', droid: '~/.factory/settings.json', dsh: '~/.dsh/profiles/{desktop,web}/cordis.patch.yml', pi: '~/.pi/agent/models.json', opencode: '~/.config/opencode/opencode.json' }
const MCP_IDS = ['claude', 'codex']
const MCP_PATHS = { claude: '~/.claude.json', codex: '~/.codex/config.toml' }

export function createHarnessPage({ onBusyChange, onNotice, isDisabled }) {
  const container = el('div', 'settings-page harness-page')

  const state = {
    skillsOpen: true,
    expanded: {},
    modelsSectionOpen: true,
    mcpsSectionOpen: true,
    operation: null,
    result: null,
    locked: false,
    modelsError: '',
    mcpsError: '',
    modelEditor: null,
    mcpEditor: null,
    batch: null,
    modelPreview: null,
    mcpPreview: null,
    agentsPreview: null,
    modelsExpanded: {},
    mcpsExpanded: {},
  }

  let agentsSnapshot = null
  let skillsSnapshot = null
  let modelsSnapshot = null
  let mcpsSnapshot = null

  const hasDialog = () => !!state.agentsPreview || !!state.modelEditor || !!state.mcpEditor || !!state.batch || !!state.modelPreview || !!state.mcpPreview

  async function loadDialog(operation, dialogKey, errorKey, load, show) {
    if (state.locked || isDisabled() || hasDialog()) return
    const returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
    onNotice('', false)
    if (errorKey) state[errorKey] = ''
    state.operation = operation
    render()
    try {
      const data = await load()
      state.operation = null
      render()
      if (container.isConnected && !isDisabled()) show(data, returnFocus)
    } catch (error) {
      state[dialogKey] = null
      if (errorKey) state[errorKey] = messageOf(error)
      onNotice(messageOf(error), true)
    } finally {
      state.operation = null
      render()
    }
  }

  async function commitDialog(action, resource, close, message) {
    onNotice('', false)
    onBusyChange(true)
    try {
      const result = await action()
      resource.invalidate()
      await resource.refresh(true)
      close()
      onNotice(typeof message === 'function' ? message(result) : message, false)
    } finally {
      onBusyChange(false)
      render()
    }
  }

  function showDocumentPreview(key, preview, { title, subtitle, closeLabel, returnFocus }) {
    let dialogRef = null
    const close = () => { dialogRef?.cleanup(); state[key] = null; render() }
    state[key] = preview
    dialogRef = ModelDialog({
      title, subtitle, disabled: false, returnFocus, onCancel: close,
      closeLabel, className: 'model-config-preview',
      content: buildPreviewContent(preview.paths, preview.content, close),
    })
  }

  const subscribe = (resource, setter) => resource.subscribe(snapshot => { setter(snapshot); render() })
  subscribe(agentsResource, next => { agentsSnapshot = next })
  subscribe(skillsResource, next => { skillsSnapshot = next })
  subscribe(modelsResource, next => { modelsSnapshot = next })
  subscribe(mcpsResource, next => { mcpsSnapshot = next })

  function refreshAll(invalidate = true) {
    if (invalidate) {
      agentsResource.invalidate()
      skillsResource.invalidate()
      modelsResource.invalidate()
      mcpsResource.invalidate()
    }
    return Promise.all([agentsResource.refresh(true), skillsResource.refresh(true), modelsResource.refresh(true), mcpsResource.refresh(true)])
  }

  function refreshSkills(invalidate = true) {
    if (invalidate) skillsResource.invalidate()
    return skillsResource.refresh(true)
  }

  function skillActions(id, count, skillId, available = true) {
    if (id !== 'claude' && id !== 'agents' && skillId === undefined) return null
    const scope = `${id}:${skillId ?? 'all'}`
    const target = id === 'claude' ? 'agents' : 'claude'
    const name = skillId ? `技能 ${skillId}` : `${id === 'claude' ? 'Claude' : 'Agents'} 的全部 ${count} 个技能`
    const wrap = el('div', 'harness-actions')
    const locked = state.locked || hasDialog()
    if (id === 'claude' || id === 'agents') {
      const sync = el('button', 'button secondary harness-action')
      sync.innerHTML = `${state.operation === `sync:${scope}` ? icon('refreshCw', 14) + '' : icon('copy', 14)}<span>同步${target}</span>`
      sync.title = `将${name}复制到 ${target}，同名技能会替换并将旧版本移到回收站`
      sync.setAttribute('aria-label', `${name}，同步${target}`)
      sync.disabled = locked || !count || !available
      sync.addEventListener('click', () => {
        void run(`sync:${scope}`, () => api('harness/sync-skills', { source: id, skillId }))
      })
      wrap.appendChild(sync)
    }
    if (skillId !== undefined) {
      const remove = el('button', 'button secondary harness-action harness-delete')
      remove.innerHTML = `${state.operation === `delete:${scope}` ? icon('refreshCw', 14) : icon('trash2', 14)}<span>删除</span>`
      remove.title = `将${name}移到回收站`
      remove.setAttribute('aria-label', `${name}，删除`)
      remove.disabled = locked || !count
      remove.addEventListener('click', () => {
        void run(`delete:${scope}`, () => api('harness/delete-skills', { id, skillId }), { quiet: true })
      })
      wrap.appendChild(remove)
    }
    return wrap
  }

  async function run(key, action, options = {}) {
    if (state.locked || isDisabled() || hasDialog()) return
    onNotice('', false)
    state.operation = key
    state.result = null
    state.locked = true
    onBusyChange(true)
    render()
    try {
      const next = await action()
      state.result = options.quiet && next.success ? null : next
      if (!options.quiet || !next.success) onNotice(next.message, !next.success)
    } catch (error) {
      state.result = { success: false, completed: 0, failed: 1, message: messageOf(error) }
      onNotice(messageOf(error), true)
    } finally {
      if (options.scope === 'all') await refreshAll()
      else { await refreshSkills(); void modelsResource.refresh(true); void mcpsResource.refresh(true) }
      state.operation = null
      state.locked = false
      onBusyChange(false)
      render()
    }
  }

  function sectionHeading(title, expanded, contentId, onToggle, children, disabled) {
    const wrap = el('div', 'harness-section-heading')
    const h2 = el('h2')
    const disclosure = el('button', 'harness-disclosure harness-section-disclosure')
    disclosure.setAttribute('aria-label', `${expanded ? '折叠' : '展开'} ${title} 区域`)
    disclosure.title = `${expanded ? '折叠' : '展开'} ${title}`
    disclosure.setAttribute('aria-expanded', expanded ? 'true' : 'false')
    disclosure.setAttribute('aria-controls', contentId)
    disclosure.disabled = disabled
    disclosure.appendChild(html('span', '', icon('chevronDown', 15, 2, expanded ? 'open' : '')))
    disclosure.appendChild(el('span', '', title))
    disclosure.addEventListener('click', onToggle)
    h2.appendChild(disclosure)
    wrap.appendChild(h2)
    const actions = el('div', 'harness-actions')
    ;(children || []).forEach(child => actions.appendChild(child))
    wrap.appendChild(actions)
    return wrap
  }

  function expandToggle(title, expanded, onToggle, disabled) {
    const action = expanded ? '折叠所有' : '展开所有'
    const button = el('button', 'icon-button')
    button.setAttribute('aria-label', `${action} ${title}`)
    button.title = action
    button.setAttribute('aria-expanded', expanded ? 'true' : 'false')
    button.disabled = disabled
    button.innerHTML = icon(expanded ? 'chevronsDownUp' : 'chevronsUpDown', 15)
    button.addEventListener('click', onToggle)
    return button
  }

  function render() {
    if (!container.isConnected) return
    container.textContent = ''
    const agents = agentsSnapshot ? agentsSnapshot.data.agentsSource : { path: '~/.claude/CLAUDE.md', exists: false }
    const harnesses = skillsSnapshot ? skillsSnapshot.data : emptyHarnesses
    const skillsBusy = skillsSnapshot ? skillsSnapshot.loading : false
    const skillsError = skillsSnapshot ? skillsSnapshot.error : ''
    const agentsLoading = agentsSnapshot ? agentsSnapshot.loading : false
    state.locked = isDisabled() || !!state.operation

    if (agentsSnapshot && agentsSnapshot.error) {
      const banner = el('div', 'error-banner')
      banner.setAttribute('role', 'alert')
      banner.textContent = `读取 Harness 失败：${agentsSnapshot.error}`
      container.appendChild(banner)
    }

    // ---- 指令 (AGENTS.md) ----
    const agentsSection = el('section', 'settings-group')
    agentsSection.setAttribute('aria-label', 'AGENTS.md 管理')
    agentsSection.appendChild(el('h2', '', '指令'))
    const agentsList = el('div', 'settings-list')
    const docRow = el('div', 'setting-row harness-document-row')
    const labelWrap = el('div', 'setting-label')
    labelWrap.appendChild(el('span', 'setting-name', 'AGENTS.md'))
    const pathNode = el('span', 'setting-description harness-path', displayHarnessPath(agents.path))
    pathNode.title = displayHarnessPath(agents.path)
    labelWrap.appendChild(pathNode)
    docRow.appendChild(labelWrap)
    const docActions = el('div', 'harness-actions')
    const previewButton = el('button', 'button secondary')
    previewButton.innerHTML = `${state.operation === 'agents-preview' ? icon('refreshCw', 14) : icon('eye', 14)}<span>预览</span>`
    previewButton.disabled = state.locked || hasDialog() || !agents.exists
    previewButton.addEventListener('click', () => { void previewAgents() })
    const syncButton = el('button', 'button primary')
    syncButton.innerHTML = `${state.operation === 'agents' ? icon('refreshCw', 14) : icon('copy', 14)}<span>同步</span>`
    syncButton.disabled = state.locked || hasDialog() || !agents.exists
    syncButton.addEventListener('click', () => {
      void run('agents', () => api('harness/sync-agents', {}), { scope: 'all' })
    })
    docActions.appendChild(previewButton)
    docActions.appendChild(syncButton)
    docRow.appendChild(docActions)
    agentsList.appendChild(docRow)
    agentsSection.appendChild(agentsList)
    const note = el('p', 'operation-note')
    note.textContent = `以 Claude 的 CLAUDE.md 为源文件，同步为 .factory、.codex、.agents、.dsh 下的 AGENTS.md。${agents.exists ? '' : '源文件未找到。'}`
    agentsSection.appendChild(note)
    container.appendChild(agentsSection)

    // ---- Skills ----
    const allSkillsExpanded = state.skillsOpen && harnesses.every(harness => state.expanded[harness.id])
    container.appendChild(sectionHeading('Skills', state.skillsOpen, 'harness-skills-content',
      () => { state.skillsOpen = !state.skillsOpen; render() },
      [
        expandToggle('Skills', allSkillsExpanded, () => {
          harnesses.forEach(harness => { state.expanded[harness.id] = !allSkillsExpanded })
          if (!allSkillsExpanded) state.skillsOpen = true
          render()
        }),
        (() => {
          const refresh = el('button', 'icon-button')
          refresh.setAttribute('aria-label', '刷新 Skills')
          refresh.title = '刷新'
          refresh.disabled = state.locked
          refresh.innerHTML = icon('refreshCw', 15, 2, skillsBusy ? 'spin' : '')
          refresh.addEventListener('click', () => { void refreshSkills(false) })
          return refresh
        })(),
      ]))

    const skillsContent = el('div')
    skillsContent.id = 'harness-skills-content'
    skillsContent.hidden = !state.skillsOpen
    if (skillsError) {
      const error = el('p', 'field-error', `读取 Skills 失败：${skillsError}`)
      error.setAttribute('role', 'alert')
      skillsContent.appendChild(error)
    }
    harnesses.forEach(harness => {
      const section = el('section', 'settings-group harness-skills-group')
      section.setAttribute('aria-label', `${harness.name} skills`)
      const list = el('div', 'settings-list')
      const head = el('div', 'setting-row harness-skills-header')
      const folderLabel = el('div', 'harness-folder-label')
      const disclosure = el('button', 'harness-disclosure')
      disclosure.setAttribute('aria-label', `${harness.name}，${harness.skills.length} 个技能`)
      disclosure.setAttribute('aria-expanded', state.expanded[harness.id] ? 'true' : 'false')
      disclosure.setAttribute('aria-controls', `skills-${harness.id}`)
      disclosure.appendChild(html('span', '', icon('chevronDown', 16, 2, state.expanded[harness.id] ? 'open' : '')))
      disclosure.appendChild(el('span', '', harness.name))
      disclosure.appendChild(el('span', 'harness-count', skillsBusy ? '…' : String(harness.skills.length)))
      disclosure.addEventListener('click', () => { state.expanded[harness.id] = !state.expanded[harness.id]; render() })
      folderLabel.appendChild(disclosure)
      const skillsPath = el('p', 'harness-path', displayHarnessPath(harness.skillsPath))
      skillsPath.title = displayHarnessPath(harness.skillsPath)
      folderLabel.appendChild(skillsPath)
      head.appendChild(folderLabel)
      const actions = skillActions(harness.id, harness.skills.length)
      if (actions) head.appendChild(actions)
      list.appendChild(head)
      if (state.expanded[harness.id]) {
        const skillList = el('div', 'harness-skill-list')
        skillList.id = `skills-${harness.id}`
        if (harness.skills.length) {
          harness.skills.forEach(skill => {
            const row = el('div', 'setting-row harness-skill-row')
            const name = el('div', 'harness-skill-name')
            name.title = displayHarnessPath(skill.path)
            name.appendChild(html('span', '', icon('fileText', 15)))
            const nameText = el('div')
            nameText.appendChild(el('span', '', skill.name))
            if (skill.id !== skill.name) nameText.appendChild(el('small', '', skill.id))
            if (!skill.available) nameText.appendChild(el('small', 'harness-broken', '链接已失效'))
            name.appendChild(nameText)
            row.appendChild(name)
            const skillActionsNode = skillActions(harness.id, 1, skill.id, skill.available)
            if (skillActionsNode) row.appendChild(skillActionsNode)
            skillList.appendChild(row)
          })
        } else {
          skillList.appendChild(el('p', 'harness-empty', harness.error ? '读取失败，请检查目录权限。' : skillsBusy ? '正在读取技能…' : '暂无技能'))
        }
        list.appendChild(skillList)
      }
      section.appendChild(list)
      if (harness.error) {
        const error = el('p', 'field-error detection-error', harness.error)
        error.setAttribute('role', 'alert')
        section.appendChild(error)
      }
      skillsContent.appendChild(section)
    })
    container.appendChild(skillsContent)

    renderModelsSection(container)
    renderMcpsSection(container)

    if (state.result) {
      const resultLine = html('div', `harness-result ${state.result.success ? 'success' : 'error'}`,
        `${icon(state.result.success ? 'check' : 'circleAlert', 16)}<span></span>`)
      resultLine.setAttribute('role', 'status')
      resultLine.querySelector('span').textContent = state.result.message
      container.appendChild(resultLine)
    }
  }

  async function previewAgents() {
    await loadDialog('agents-preview', 'agentsPreview', null, () => api('harness/agents-preview'), (preview, returnFocus) => {
      showDocumentPreview('agentsPreview', { paths: [preview.path], content: preview.content }, {
        title: 'AGENTS.md 预览', subtitle: displayHarnessPath(preview.path), closeLabel: '关闭预览', returnFocus,
      })
    })
  }

  // ---- Models ----
  function renderModelsSection(root) {
    const wrap = el('div', 'harness-models')
    const inventory = modelsSnapshot ? modelsSnapshot.data : { sources: [] }
    const loading = modelsSnapshot ? modelsSnapshot.loading : false
    const resourceError = modelsSnapshot ? modelsSnapshot.error : ''
    const dialogOpen = hasDialog()
    const locked = state.locked || loading || dialogOpen
    const allExpanded = state.modelsSectionOpen && MODEL_HARNESSES.every(harness => state.modelsExpanded[harness])

    wrap.appendChild(sectionHeading('Models', state.modelsSectionOpen, 'harness-models-content',
      () => { state.modelsSectionOpen = !state.modelsSectionOpen; render() },
      [
        (() => {
          const button = el('button', 'icon-button')
          button.setAttribute('aria-label', '一键修改模型')
          button.title = '一键修改模型'
          button.disabled = locked
          button.innerHTML = icon('pencil', 15)
          button.addEventListener('click', () => openBatch('replace'))
          return button
        })(),
        (() => {
          const button = el('button', 'icon-button')
          button.setAttribute('aria-label', '一键添加模型')
          button.title = '一键添加模型'
          button.disabled = locked
          button.innerHTML = icon('plus', 15)
          button.addEventListener('click', () => openBatch('add'))
          return button
        })(),
        (() => {
          const button = el('button', 'icon-button')
          button.setAttribute('aria-label', '一键删除模型')
          button.title = '一键删除模型'
          button.disabled = locked
          button.innerHTML = icon('trash2', 15)
          button.addEventListener('click', () => openBatch('delete'))
          return button
        })(),
        expandToggle('Models', allExpanded, () => {
          MODEL_HARNESSES.forEach(harness => { state.modelsExpanded[harness] = !allExpanded })
          if (!allExpanded) state.modelsSectionOpen = true
          render()
        }, dialogOpen),
        (() => {
          const refresh = el('button', 'icon-button')
          refresh.setAttribute('aria-label', '刷新 Models')
          refresh.title = '刷新'
          refresh.disabled = locked
          refresh.innerHTML = icon('refreshCw', 15, 2, loading ? 'spin' : '')
          refresh.addEventListener('click', () => { state.modelsError = ''; void modelsResource.refresh(true) })
          return refresh
        })(),
      ]))

    const errorText = state.modelsError || resourceError
    if (errorText) {
      const error = el('p', 'field-error model-error', errorText)
      error.setAttribute('role', 'alert')
      wrap.appendChild(error)
    }

    const content = el('div')
    content.id = 'harness-models-content'
    content.hidden = !state.modelsSectionOpen
    MODEL_HARNESSES.forEach(harness => {
      const sources = inventory.sources.filter(source => source.harness === harness)
      const source = sources.find(item => item.editable) ?? sources[0]
      const pathLabel = harness === 'dsh' ? MODEL_PATHS.dsh : displayHarnessPath(source?.path ?? MODEL_PATHS[harness])
      const isOpen = !!state.modelsExpanded[harness]
      const count = sources.reduce((sum, item) => sum + item.models.length, 0)
      const section = el('section', 'settings-group harness-models-group')
      section.setAttribute('aria-label', `${harness} models`)
      const list = el('div', 'settings-list')
      const head = el('div', 'setting-row harness-skills-header')
      const folderLabel = el('div', 'harness-folder-label')
      const disclosure = el('button', 'harness-disclosure')
      disclosure.setAttribute('aria-label', `${harness}，${count} 个模型`)
      disclosure.setAttribute('aria-expanded', isOpen ? 'true' : 'false')
      disclosure.setAttribute('aria-controls', `models-${harness}`)
      disclosure.appendChild(html('span', '', icon('chevronDown', 16, 2, isOpen ? 'open' : '')))
      disclosure.appendChild(el('span', '', harness))
      disclosure.appendChild(el('span', 'harness-count', loading ? '…' : String(count)))
      disclosure.addEventListener('click', () => { state.modelsExpanded[harness] = !state.modelsExpanded[harness]; render() })
      folderLabel.appendChild(disclosure)
      const pathNode = el('p', 'harness-path', pathLabel)
      pathNode.title = pathLabel
      folderLabel.appendChild(pathNode)
      head.appendChild(folderLabel)
      const actions = el('div', 'harness-actions')
      const view = el('button', 'button secondary harness-action')
      view.innerHTML = `${state.operation === `view:${harness}` ? icon('refreshCw', 14) : icon('eye', 14)}<span>查看</span>`
      view.setAttribute('aria-label', `${harness} 查看模型配置`)
      view.disabled = locked || !source?.editable
      view.addEventListener('click', () => { void viewModels(harness, sources) })
      actions.appendChild(view)
      const add = el('button', 'button secondary harness-action')
      add.innerHTML = `${icon('plus', 14)}<span>新增</span>`
      add.setAttribute('aria-label', `${harness} 新增模型`)
      add.disabled = locked || !source?.editable
      add.addEventListener('click', () => { if (source) void openModelEditor(source) })
      actions.appendChild(add)
      head.appendChild(actions)
      list.appendChild(head)
      if (!isOpen) sources.filter(item => item.error).forEach(item => {
        const error = el('p', 'field-error model-error', `${item.label}：${item.error}`)
        error.setAttribute('role', 'alert')
        list.appendChild(error)
      })
      if (isOpen) {
        const modelList = el('div', 'harness-skill-list')
        modelList.id = `models-${harness}`
        sources.forEach(item => {
          const holder = el('div')
          if (sources.length > 1) holder.appendChild(el('p', 'model-provider-label', item.label))
          if (item.error) {
            const error = el('p', 'field-error model-error', `${item.label}：${item.error}`)
            error.setAttribute('role', 'alert')
            holder.appendChild(error)
          }
          if (item.models.length) holder.appendChild(renderModelList(item, locked || dialogOpen))
          else holder.appendChild(el('p', 'harness-empty', loading ? '正在读取模型…' : '暂无模型'))
          modelList.appendChild(holder)
        })
        if (!sources.length) modelList.appendChild(el('p', 'harness-empty', loading ? '正在读取模型…' : '暂无模型'))
        list.appendChild(modelList)
      }
      section.appendChild(list)
      content.appendChild(section)
    })
    wrap.appendChild(content)
    root.appendChild(wrap)
  }

  function renderModelList(source, disabled) {
    const list = el('div', 'model-list')
    source.models.forEach((item, position) => {
      const row = el('div', 'setting-row harness-skill-row model-row')
      row.dataset.modelIndex = item.index
      const name = el('div', 'harness-skill-name model-name')
      const handle = el('button', 'icon-button model-drag-handle')
      handle.type = 'button'
      handle.setAttribute('aria-label', `${source.harness} ${item.name}，拖动排序`)
      handle.title = '按住拖动排序；Alt + ↑/↓ 移动'
      handle.disabled = disabled || source.models.length < 2
      handle.innerHTML = icon('gripVertical', 14)
      name.appendChild(handle)
      name.appendChild(html('span', '', icon('cpu', 15)))
      const nameText = el('div')
      nameText.appendChild(el('span', '', item.name))
      if (item.model !== item.name) nameText.appendChild(el('small', '', item.model))
      name.appendChild(nameText)
      row.appendChild(name)
      const actions = el('div', 'harness-actions')
      const actionButton = (label, iconName, ariaLabel, onClick, spin) => {
        const button = el('button', 'button secondary harness-action')
        button.innerHTML = `${icon(spin ? 'refreshCw' : iconName, 14)}<span>${label}</span>`
        button.setAttribute('aria-label', ariaLabel)
        button.disabled = disabled
        button.addEventListener('click', onClick)
        return button
      }
      actions.appendChild(actionButton('修改', 'pencil', `${source.harness} ${item.name}，修改`, () => void openModelEditor(source, item), state.operation === `edit:${source.id}:${item.index}`))
      actions.appendChild(actionButton('复制', 'copy', `${source.harness} ${item.name}，复制`, () => void openModelEditor(source, item, true), state.operation === `copy:${source.id}:${item.index}`))
      actions.appendChild(actionButton('删除', 'trash2', `${source.harness} ${item.name}，删除`, () => void removeModel(source, item), state.operation === `delete:${source.id}:${item.index}`))
      row.appendChild(actions)
      attachModelDrag(handle, list, row, source, item, position)
      list.appendChild(row)
    })
    return list
  }

  function attachModelDrag(handle, list, row, source, item, position) {
    let drag = null
    let blockClick = false
    const targetAt = (x, y) => {
      const bounds = list.getBoundingClientRect()
      const viewport = document.getElementById('main-content').getBoundingClientRect()
      if (x < bounds.left || x > bounds.right || y < Math.max(bounds.top, viewport.top) || y > Math.min(bounds.bottom, viewport.bottom)) return null
      const rows = Array.from(list.querySelectorAll('[data-model-index]'))
      const before = rows.find(candidate => { const rect = candidate.getBoundingClientRect(); return y < rect.top + rect.height / 2 })
      const target = before ?? rows[rows.length - 1]
      return target ? { index: Number(target.dataset.modelIndex), after: !before } : null
    }
    handle.addEventListener('pointerdown', event => {
      if (handle.disabled || source.models.length < 2 || event.button !== 0) return
      blockClick = false
      drag = { index: item.index, pointerId: event.pointerId, startX: event.clientX, startY: event.clientY, active: false }
      handle.setPointerCapture(event.pointerId)
    })
    handle.addEventListener('pointermove', event => {
      if (!drag || drag.pointerId !== event.pointerId) return
      if (!drag.active) {
        if (Math.hypot(event.clientX - drag.startX, event.clientY - drag.startY) < 6) return
        drag.active = true
        blockClick = true
        list.classList.add('model-list-dragging')
        row.classList.add('model-drag-source')
      }
      const target = targetAt(event.clientX, event.clientY)
      Array.from(list.querySelectorAll('[data-model-index]')).forEach(node => {
        const isTarget = target && Number(node.dataset.modelIndex) === target.index
        node.classList.toggle('model-drop-after', !!isTarget && target.after)
        node.classList.toggle('model-drop-before', !!isTarget && !target.after)
      })
    })
    const reorder = (index, target) => {
      if (index === target.index) return
      const moved = source.models.find(model => model.index === index)
      const items = source.models.filter(model => model.index !== index)
      const at = items.findIndex(model => model.index === target.index)
      if (!moved || at < 0) return
      items.splice(at + (target.after ? 1 : 0), 0, moved)
      if (items.every((model, i) => model.index === source.models[i].index)) return
      void reorderModels(source, items)
    }
    const cancelDrag = () => {
      const current = drag
      drag = null
      list.classList.remove('model-list-dragging')
      row.classList.remove('model-drag-source')
      Array.from(list.querySelectorAll('[data-model-index]')).forEach(node => node.classList.remove('model-drop-after', 'model-drop-before'))
      if (current && handle.hasPointerCapture(current.pointerId)) handle.releasePointerCapture(current.pointerId)
    }
    const finish = event => {
      if (!drag || drag.pointerId !== event.pointerId) return
      const current = drag
      const target = current.active ? targetAt(event.clientX, event.clientY) : null
      cancelDrag()
      if (target) reorder(current.index, target)
    }
    handle.addEventListener('pointerup', finish)
    handle.addEventListener('pointercancel', cancelDrag)
    handle.addEventListener('lostpointercapture', cancelDrag)
    handle.addEventListener('click', event => { if (blockClick) { event.preventDefault(); event.stopPropagation() } })
    handle.addEventListener('keydown', event => {
      if (event.key === 'Escape' && drag) { event.preventDefault(); cancelDrag(); return }
      if (handle.disabled || !event.altKey || !['ArrowUp', 'ArrowDown'].includes(event.key)) return
      event.preventDefault()
      const target = source.models[position + (event.key === 'ArrowUp' ? -1 : 1)]
      if (target) reorder(item.index, { index: target.index, after: event.key === 'ArrowDown' })
    })
  }

  async function openModelEditor(source, item, copying = false) {
    if (!source.editable) return
    const target = item ? { sourceId: source.id, index: item.index, revision: item.revision } : null
    await loadDialog(`${copying ? 'copy' : item ? 'edit' : 'add'}:${source.id}:${item?.index ?? ''}`, 'modelEditor', 'modelsError', async () => {
      if (target) return api('models/detail', target)
      const detail = { fields: { model: '', name: '', description: '' } }
      if (source.harness !== 'claude') detail.fields.baseUrl = source.baseUrl ?? DEFAULT_MODEL_BASE_URL
      if (source.harness === 'droid') detail.fields.provider = 'generic-chat-completion-api'
      if (source.harness === 'dsh') detail.fields.reasoningEfforts = { xhigh: 'high' }
      return detail
    }, (detail, returnFocus) => {
      state.modelEditor = { source, ...(copying ? { copyFrom: target } : { target }), detail, returnFocus }
      openModelEditorDialog()
    })
  }

  function openModelEditorDialog() {
    const editor = state.modelEditor
    let dialogRef = null
    const close = () => { if (dialogRef) dialogRef.cleanup(); state.modelEditor = null; render() }
    dialogRef = ModelEditorDialog({
      editor,
      disabled: state.locked,
      onCancel: close,
      onSubmit: (fields, apiKey) => commitDialog(
        () => api('models/save', { sourceId: editor.source.id, target: editor.target, copyFrom: editor.copyFrom, fields, apiKey }),
        modelsResource, close, '模型配置已保存',
      ),
    })
  }

  async function removeModel(source, item) {
    const dialogOpen = hasDialog()
    if (state.locked || dialogOpen) return
    state.operation = `delete:${source.id}:${item.index}`
    state.modelsError = ''
    onBusyChange(true)
    render()
    try {
      await api('models/delete', { sourceId: source.id, index: item.index, revision: item.revision })
      modelsResource.invalidate()
      await modelsResource.refresh(true)
      onNotice('模型已删除', false)
    } catch (error) {
      state.modelsError = messageOf(error)
      onNotice(messageOf(error), true)
    } finally {
      state.operation = ''
      onBusyChange(false)
      render()
    }
  }

  async function reorderModels(source, items) {
    const dialogOpen = hasDialog()
    if (state.locked || dialogOpen) return
    state.operation = 'reorder'
    state.modelsError = ''
    onBusyChange(true)
    modelsResource.update(current => ({ sources: current.sources.map(item => item.id === source.id ? { ...item, models: items } : item) }))
    let failure = ''
    try {
      await api('models/reorder', { sourceId: source.id, models: items.map(item => ({ sourceId: source.id, index: item.index, revision: item.revision })) })
    } catch (error) {
      failure = messageOf(error)
      modelsResource.update(current => ({ sources: current.sources.map(item => item.id === source.id ? source : item) }))
    } finally {
      modelsResource.invalidate()
      await modelsResource.refresh(true)
      state.operation = ''
      if (failure) { state.modelsError = failure; onNotice(failure, true) }
      onBusyChange(false)
      render()
    }
  }

  function openBatch(action) {
    const dialogOpen = hasDialog()
    if (state.locked || dialogOpen) return
    state.modelsError = ''
    const returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
    state.batch = { action, returnFocus }
    let dialogRef = null
    const close = () => { if (dialogRef) dialogRef.cleanup(); state.batch = null; render() }
    dialogRef = ModelBatchDialog({
      action,
      disabled: state.locked,
      returnFocus,
      onCancel: close,
      onSubmit: change => commitDialog(
        () => api('models/batch', change), modelsResource, close,
        result => `批量操作完成：修改 ${result.changed} 条模型，跳过 ${result.skipped} 个配置源`,
      ),
    })
  }

  async function viewModels(harness, sources) {
    await loadDialog(`view:${harness}`, 'modelPreview', 'modelsError', async () => {
      const previewSources = harness === 'dsh' ? [sources.find(source => source.editable) ?? sources[0]] : sources
      const documents = await Promise.all(previewSources.map(source => api('models/preview', { sourceId: source.id })))
      return {
        paths: [...new Set(documents.flatMap(item => item.paths))],
        content: documents.map(item => item.content).join('\n\n'),
      }
    }, (preview, returnFocus) => {
      showDocumentPreview('modelPreview', preview, {
        title: '查看模型配置', subtitle: harness, closeLabel: '关闭模型弹窗', returnFocus,
      })
    })
  }

  function buildPreviewContent(paths, content, onClose) {
    const wrap = el('div', 'model-preview-content')
    const pathList = el('div', 'model-preview-paths')
    paths.forEach(path => pathList.appendChild(el('p', '', displayHarnessPath(path))))
    wrap.appendChild(pathList)
    const pre = el('pre')
    pre.setAttribute('role', 'region')
    pre.setAttribute('aria-label', '配置预览内容')
    pre.tabIndex = 0
    pre.textContent = content || '（空文件）'
    wrap.appendChild(pre)
    const actions = el('div', 'harness-actions model-editor-actions')
    const ok = el('button', 'button primary')
    ok.type = 'button'
    ok.textContent = '确定'
    ok.addEventListener('click', onClose)
    actions.appendChild(ok)
    wrap.appendChild(actions)
    return wrap
  }

  // ---- MCPs ----
  function renderMcpsSection(root) {
    const wrap = el('div', 'harness-mcps')
    const inventory = mcpsSnapshot ? mcpsSnapshot.data : { sources: [] }
    const loading = mcpsSnapshot ? mcpsSnapshot.loading : false
    const resourceError = mcpsSnapshot ? mcpsSnapshot.error : ''
    const dialogOpen = hasDialog()
    const locked = state.locked || loading || dialogOpen
    const allExpanded = state.mcpsSectionOpen && MCP_IDS.every(id => state.mcpsExpanded[id])

    wrap.appendChild(sectionHeading('MCPs', state.mcpsSectionOpen, 'harness-mcps-content',
      () => { state.mcpsSectionOpen = !state.mcpsSectionOpen; render() },
      [
        expandToggle('MCPs', allExpanded, () => {
          MCP_IDS.forEach(id => { state.mcpsExpanded[id] = !allExpanded })
          if (!allExpanded) state.mcpsSectionOpen = true
          render()
        }, dialogOpen),
        (() => {
          const refresh = el('button', 'icon-button')
          refresh.setAttribute('aria-label', '刷新 MCPs')
          refresh.title = '刷新'
          refresh.disabled = locked
          refresh.innerHTML = icon('refreshCw', 15, 2, loading ? 'spin' : '')
          refresh.addEventListener('click', () => { void refreshMcp() })
          return refresh
        })(),
      ]))

    const errorText = state.mcpsError || resourceError
    if (errorText) {
      const error = el('p', 'field-error model-error', errorText)
      error.setAttribute('role', 'alert')
      wrap.appendChild(error)
    }

    const content = el('div')
    content.id = 'harness-mcps-content'
    content.hidden = !state.mcpsSectionOpen
    MCP_IDS.forEach(id => {
      const source = inventory.sources.find(item => item.harness === id)
      const servers = source?.servers ?? []
      const path = displayHarnessPath(source?.path ?? MCP_PATHS[id])
      const open = !!state.mcpsExpanded[id]
      const section = el('section', 'settings-group harness-mcps-group')
      section.setAttribute('aria-label', `${id} mcps`)
      const list = el('div', 'settings-list')
      const head = el('div', 'setting-row harness-skills-header')
      const folderLabel = el('div', 'harness-folder-label')
      const disclosure = el('button', 'harness-disclosure')
      disclosure.setAttribute('aria-label', `${id}，${servers.length} 个 MCP`)
      disclosure.setAttribute('aria-expanded', open ? 'true' : 'false')
      disclosure.setAttribute('aria-controls', `mcps-${id}`)
      disclosure.appendChild(html('span', '', icon('chevronDown', 16, 2, open ? 'open' : '')))
      disclosure.appendChild(el('span', '', id))
      disclosure.appendChild(el('span', 'harness-count', loading ? '…' : String(servers.length)))
      disclosure.addEventListener('click', () => { state.mcpsExpanded[id] = !state.mcpsExpanded[id]; render() })
      folderLabel.appendChild(disclosure)
      const pathNode = el('p', 'harness-path', path)
      pathNode.title = path
      folderLabel.appendChild(pathNode)
      head.appendChild(folderLabel)
      const actions = el('div', 'harness-actions')
      const view = el('button', 'button secondary harness-action')
      view.innerHTML = `${state.operation === `view:${id}` ? icon('refreshCw', 14) : icon('eye', 14)}<span>查看</span>`
      view.setAttribute('aria-label', `${id} 查看 MCP 配置`)
      view.disabled = locked || !source?.editable
      view.addEventListener('click', () => { if (source) void viewMcp(source) })
      actions.appendChild(view)
      const add = el('button', 'button secondary harness-action')
      add.innerHTML = `${icon('plus', 14)}<span>新增</span>`
      add.setAttribute('aria-label', `${id} 新增 MCP`)
      add.disabled = locked || !source?.editable
      add.addEventListener('click', () => { if (source) void openMcpEditor(source) })
      actions.appendChild(add)
      const refresh = el('button', 'icon-button')
      refresh.setAttribute('aria-label', `${id} 刷新 MCPs`)
      refresh.title = '刷新'
      refresh.disabled = locked
      refresh.innerHTML = icon('refreshCw', 15)
      refresh.addEventListener('click', () => { void refreshMcp() })
      actions.appendChild(refresh)
      head.appendChild(actions)
      list.appendChild(head)
      if (open) {
        const serverList = el('div', 'harness-skill-list')
        serverList.id = `mcps-${id}`
        if (servers.length) {
          servers.forEach(item => {
            const row = el('div', 'setting-row harness-skill-row')
            const name = el('div', 'harness-skill-name')
            name.appendChild(html('span', '', icon('plug', 15)))
            const nameText = el('div')
            nameText.appendChild(el('span', '', item.name))
            nameText.appendChild(el('small', '', `${item.transport.toUpperCase()}${item.description ? ` · ${item.description}` : ''}`))
            name.appendChild(nameText)
            row.appendChild(name)
            const actions = el('div', 'harness-actions')
            const edit = el('button', 'button secondary harness-action')
            edit.innerHTML = `${icon('pencil', 14)}<span>修改</span>`
            edit.setAttribute('aria-label', `${id} ${item.name} 修改 MCP`)
            edit.disabled = locked || dialogOpen
            edit.addEventListener('click', () => { if (source) void openMcpEditor(source, item) })
            actions.appendChild(edit)
            const copy = el('button', 'button secondary harness-action')
            copy.innerHTML = `${icon('copy', 14)}<span>复制</span>`
            copy.setAttribute('aria-label', `${id} ${item.name} 复制 MCP`)
            copy.disabled = locked || dialogOpen
            copy.addEventListener('click', () => { if (source) void openMcpEditor(source, item, true) })
            actions.appendChild(copy)
            const remove = el('button', 'button secondary harness-action harness-delete')
            remove.innerHTML = `${icon('trash2', 14)}<span>删除</span>`
            remove.setAttribute('aria-label', `${id} ${item.name} 删除 MCP`)
            remove.disabled = locked || dialogOpen
            remove.addEventListener('click', () => { void removeMcp(source, item) })
            actions.appendChild(remove)
            row.appendChild(actions)
            serverList.appendChild(row)
          })
        } else {
          serverList.appendChild(el('p', 'harness-empty', loading ? '正在读取 MCP…' : '暂无 MCP'))
        }
        list.appendChild(serverList)
      }
      section.appendChild(list)
      if (source?.error) {
        const error = el('p', 'field-error model-error', source.error)
        error.setAttribute('role', 'alert')
        section.appendChild(error)
      }
      content.appendChild(section)
    })
    wrap.appendChild(content)
    root.appendChild(wrap)
  }

  async function refreshMcp() {
    state.mcpsError = ''
    mcpsResource.invalidate()
    await mcpsResource.refresh(true)
    render()
  }

  async function openMcpEditor(source, item, copying = false) {
    if (!source.editable) return
    const target = item ? { harness: source.harness, name: item.name, revision: item.revision } : null
    await loadDialog(`${copying ? 'copy' : item ? 'edit' : 'add'}:${source.harness}:${item?.name ?? ''}`, 'mcpEditor', 'mcpsError', async () => {
      if (target) return api('mcps/detail', target)
      return { name: '', transport: 'stdio', command: '', args: [], env: {}, url: '', headers: {}, cwd: '', bearerTokenEnvVar: '', envHeaders: {} }
    }, (fields, returnFocus) => {
      state.mcpEditor = {
        source, returnFocus,
        fields: copying ? { ...fields, name: `${fields.name}-copy` } : fields,
        ...(copying ? { copyFrom: target } : { target }),
      }
      openMcpEditorDialog()
    })
  }

  function openMcpEditorDialog() {
    const editor = state.mcpEditor
    let dialogRef = null
    const close = () => { if (dialogRef) dialogRef.cleanup(); state.mcpEditor = null; render() }
    dialogRef = McpEditorDialog({
      editor,
      disabled: state.locked,
      onCancel: close,
      onSubmit: fields => commitDialog(
        () => api('mcps/save', { harness: editor.source.harness, target: editor.target, copyFrom: editor.copyFrom, fields }),
        mcpsResource, close, 'MCP 配置已保存',
      ),
    })
  }

  async function removeMcp(source, item) {
    const dialogOpen = hasDialog()
    if (state.locked || dialogOpen) return
    state.operation = `delete:${source.harness}:${item.name}`
    state.mcpsError = ''
    onBusyChange(true)
    render()
    try {
      await api('mcps/delete', { harness: source.harness, name: item.name, revision: item.revision })
      await refreshMcp()
      onNotice('MCP 已删除', false)
    } catch (error) {
      state.mcpsError = messageOf(error)
      onNotice(messageOf(error), true)
    } finally {
      state.operation = ''
      onBusyChange(false)
      render()
    }
  }

  async function viewMcp(source) {
    await loadDialog(`view:${source.harness}`, 'mcpPreview', 'mcpsError', () => api('mcps/preview', { harness: source.harness }), (preview, returnFocus) => {
      showDocumentPreview('mcpPreview', { paths: [preview.path], content: preview.content }, {
        title: '查看 MCP 配置', subtitle: source.harness, closeLabel: '关闭 MCP 预览', returnFocus,
      })
    })
  }

  function setInitialCollapse(harnessSettings) {
    state.skillsOpen = !harnessSettings.skillsCollapsed
    state.modelsSectionOpen = !harnessSettings.modelsCollapsed
    state.mcpsSectionOpen = !harnessSettings.mcpsCollapsed
  }

  function setCollapsed(harnessSettings) {
    state.skillsOpen = !harnessSettings.skillsCollapsed
    state.modelsSectionOpen = !harnessSettings.modelsCollapsed
    state.mcpsSectionOpen = !harnessSettings.mcpsCollapsed
    render()
  }

  return { node: container, setCollapsed, setInitialCollapse, refreshAll }
}
