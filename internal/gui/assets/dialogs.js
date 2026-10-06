// 弹窗组件：通用模态、模型编辑器、MCP 编辑器、批量模型。
import { el, html, icon, SettingRow, SelectControl, EditableSelectControl } from './controls.js'
import { api, messageOf } from './api.js'

const DEFAULT_MODEL_BASE_URL = 'http://127.0.0.1:20128'
const API_OPTIONS = [DEFAULT_MODEL_BASE_URL, `${DEFAULT_MODEL_BASE_URL}/v1`].map(value => ({ value, label: value }))

// 通用模态：焦点圈闭、Escape/Tab 处理、关闭后归还焦点。
export function ModelDialog({ title, subtitle, disabled, returnFocus, onCancel, className = '', closeLabel = '关闭模型弹窗', descriptionId = 'model-modal-source', content }) {
  const backdrop = el('div', 'model-modal-backdrop')
  const dialog = el('section', `model-modal ${className}`)
  dialog.setAttribute('role', 'dialog')
  dialog.setAttribute('aria-modal', 'true')
  dialog.setAttribute('aria-labelledby', 'model-modal-title')
  dialog.setAttribute('aria-describedby', descriptionId)
  const header = el('header', 'model-modal-header')
  const headLeft = el('div')
  headLeft.appendChild(el('h2', '', title))
  headLeft.lastChild.id = 'model-modal-title'
  const subtitleNode = el('p', '', subtitle)
  subtitleNode.id = descriptionId
  headLeft.appendChild(subtitleNode)
  header.appendChild(headLeft)
  const close = el('button', 'icon-button')
  close.type = 'button'
  close.setAttribute('aria-label', closeLabel)
  close.disabled = disabled
  close.innerHTML = icon('x', 18)
  close.addEventListener('click', handleCancel)
  header.appendChild(close)
  dialog.appendChild(header)
  if (content) dialog.appendChild(content)
  backdrop.appendChild(dialog)

  const root = document.getElementById('app-shell')
  const previouslyInert = root ? root.inert : false
  if (root) root.inert = true
  document.getElementById('overlays').appendChild(backdrop)

  const firstFocus = dialog.querySelector('[data-dialog-initial-focus="true"]:not(:disabled)') ||
    dialog.querySelector('input:not(:disabled)') || dialog.querySelector('button:not(:disabled)')
  if (firstFocus) firstFocus.focus()

  let closed = false
  const cleanup = () => {
    if (closed) return
    closed = true
    window.removeEventListener('keydown', onKey, true)
    if (root) root.inert = previouslyInert
    backdrop.remove()
    if (returnFocus && returnFocus.isConnected) returnFocus.focus()
  }
  // 取消路径自动清理 DOM；onSubmit 由调用方显式 cleanup。
  function handleCancel() { cleanup(); onCancel() }
  function onKey(event) {
    if (event.key === 'Escape' && !event.isComposing && !disabled) {
      event.preventDefault()
      handleCancel()
      return
    }
    if (event.key !== 'Tab') return
    const focusable = Array.from(dialog.querySelectorAll('button:not(:disabled), input:not(:disabled), textarea:not(:disabled), summary, [tabindex="0"]'))
      .filter(element => element.getClientRects().length > 0)
    if (!focusable.length) return
    const first = focusable[0]
    const last = focusable[focusable.length - 1]
    if ((event.shiftKey && document.activeElement === first) || (!event.shiftKey && document.activeElement === last)) {
      event.preventDefault()
      ;(event.shiftKey ? last : first).focus()
    }
  }
  window.addEventListener('keydown', onKey, true)
  backdrop.addEventListener('mousedown', event => {
    if (event.target === event.currentTarget && !disabled) handleCancel()
  })
  return { cleanup, dialog, setDisabled(next) { disabled = next } }
}

// 模型编辑器（修改/复制/新增）。
export function ModelEditorDialog({ editor, disabled, onCancel, onSubmit }) {
  const fields = { ...editor.detail.fields }
  const apiKey = { value: editor.detail.apiKey ?? '' }
  const harness = editor.source.harness
  const prefix = `model-editor-${harness}`
  const title = editor.target ? '修改模型' : editor.copyFrom ? '复制模型' : '新增模型'

  const form = el('form', 'model-editor')
  form.setAttribute('aria-label', `${harness} ${title}`)
  const fieldsWrap = el('div', 'model-editor-fields')

  const textInput = (key, label, required = false) => {
    const input = el('input', 'field-input model-text-input')
    input.id = `${prefix}-${key}`
    input.type = 'text'
    input.value = fields[key] ?? ''
    input.disabled = disabled
    input.required = required
    input.autocomplete = 'off'
    input.spellcheck = false
    input.addEventListener('input', () => { fields[key] = input.value })
    return SettingRow(label, input.id, null, input)
  }

  fieldsWrap.appendChild(textInput('model', '模型 ID', true))

  const nameInput = el('input', 'field-input model-text-input')
  nameInput.id = `${prefix}-name`
  nameInput.type = 'text'
  nameInput.value = fields.name
  nameInput.disabled = disabled
  nameInput.autocomplete = 'off'
  nameInput.spellcheck = false
  nameInput.addEventListener('input', () => { fields.name = nameInput.value })
  const nameRefresh = el('button', 'icon-button model-name-refresh')
  nameRefresh.type = 'button'
  nameRefresh.title = '从模型 ID 生成名称'
  nameRefresh.setAttribute('aria-label', '从模型 ID 生成名称')
  nameRefresh.disabled = disabled
  nameRefresh.innerHTML = icon('refreshCw', 15)
  nameRefresh.addEventListener('click', () => {
    const model = (fields.model ?? '').trim()
    const last = model.split('/').pop() || ''
    fields.name = last.replace(/\[1m\]/gi, '').trim()
    nameInput.value = fields.name
  })
  const nameControl = el('div', 'model-name-control')
  nameControl.appendChild(nameInput)
  nameControl.appendChild(nameRefresh)
  fieldsWrap.appendChild(SettingRow('名称', nameInput.id, null, nameControl))

  if (harness === 'claude') fieldsWrap.appendChild(textInput('description', '描述'))

  let providerSelect = null
  if (harness === 'droid') {
    providerSelect = SelectControl({
      id: `${prefix}-provider`, label: '模型 Provider', value: fields.provider ?? 'generic-chat-completion-api', disabled,
      options: [
        { value: 'generic-chat-completion-api', label: 'OpenAI 兼容' },
        { value: 'openai', label: 'OpenAI' },
        { value: 'anthropic', label: 'Anthropic' },
      ],
      onChange: value => { fields.provider = value },
    })
    fieldsWrap.appendChild(SettingRow('Provider', `${prefix}-provider`, null, providerSelect.node))
  }

  if (harness !== 'claude') {
    const baseUrlValue = fields.baseUrl ?? DEFAULT_MODEL_BASE_URL
    fields.baseUrl = baseUrlValue
    const baseUrlSelect = EditableSelectControl({
      id: `${prefix}-baseUrl`, label: '模型 API 地址', value: baseUrlValue, disabled, options: API_OPTIONS,
      onChange: value => { fields.baseUrl = value },
    })
    fieldsWrap.appendChild(SettingRow('API 地址', `${prefix}-baseUrl`, null, baseUrlSelect.node))
  }

  if (harness === 'droid') {
    const apiKeyInput = el('input', 'field-input model-text-input')
    apiKeyInput.id = `${prefix}-apiKey`
    apiKeyInput.type = 'text'
    apiKeyInput.value = apiKey.value
    apiKeyInput.disabled = disabled
    apiKeyInput.autocomplete = 'off'
    apiKeyInput.spellcheck = false
    apiKeyInput.addEventListener('input', () => { apiKey.value = apiKeyInput.value })
    fieldsWrap.appendChild(SettingRow('API Key', apiKeyInput.id, null, apiKeyInput))
  }

  let reasoningRows = harness === 'dsh' && fields.reasoningEfforts && typeof fields.reasoningEfforts === 'object'
    ? Object.entries(fields.reasoningEfforts).map(([key, value]) => ({ key, value: value ?? '' }))
    : []

  const errorNode = el('p', 'field-error model-error')
  errorNode.setAttribute('role', 'alert')

  if (harness === 'dsh') {
    const section = el('section', 'model-reasoning')
    section.setAttribute('aria-label', '思考级别')
    const head = el('div', 'model-reasoning-header')
    head.appendChild(el('span', '', '思考级别'))
    const add = el('button', 'icon-button')
    add.type = 'button'
    add.title = '增加思考级别'
    add.setAttribute('aria-label', '增加思考级别')
    add.disabled = disabled
    add.innerHTML = icon('plus', 14)
    const rowsWrap = el('div', 'model-reasoning-rows')
    const renderRows = () => {
      rowsWrap.textContent = ''
      reasoningRows.forEach((row, index) => {
        const line = el('div', 'model-reasoning-row')
        const keyWrap = el('div', 'model-reasoning-key')
        const keyInput = el('input', 'field-input')
        keyInput.setAttribute('aria-label', `思考级别 key ${index + 1}`)
        keyInput.value = row.key
        keyInput.disabled = disabled
        keyInput.autocomplete = 'off'
        keyInput.spellcheck = false
        keyInput.addEventListener('input', () => { row.key = keyInput.value })
        keyWrap.appendChild(keyInput)
        keyWrap.appendChild(html('span', 'model-reasoning-arrow', '->'))
        const valueWrap = el('div', 'model-reasoning-value')
        const valueInput = el('input', 'field-input')
        valueInput.setAttribute('aria-label', `思考级别 value ${index + 1}`)
        valueInput.value = row.value ?? ''
        valueInput.disabled = disabled
        valueInput.autocomplete = 'off'
        valueInput.spellcheck = false
        valueInput.addEventListener('input', () => { row.value = valueInput.value })
        const remove = el('button', 'icon-button')
        remove.type = 'button'
        remove.setAttribute('aria-label', `删除思考级别 ${index + 1}`)
        remove.disabled = disabled
        remove.innerHTML = icon('trash2', 14)
        remove.addEventListener('click', () => {
          reasoningRows = reasoningRows.filter((_, i) => i !== index)
          renderRows()
        })
        valueWrap.appendChild(valueInput)
        valueWrap.appendChild(remove)
        line.appendChild(keyWrap)
        line.appendChild(valueWrap)
        rowsWrap.appendChild(line)
      })
    }
    add.addEventListener('click', () => { reasoningRows.push({ key: '', value: '' }); renderRows() })
    head.appendChild(add)
    section.appendChild(head)
    renderRows()
    section.appendChild(rowsWrap)
    fieldsWrap.appendChild(section)
  }

  fieldsWrap.appendChild(errorNode)

  const actions = el('div', 'harness-actions model-editor-actions')
  const cancel = el('button', 'button secondary')
  cancel.type = 'button'
  cancel.textContent = '取消'
  cancel.disabled = disabled
  cancel.addEventListener("click", () => onCancel())
  const submit = el('button', 'button primary')
  submit.type = 'submit'
  submit.textContent = '确定'
  submit.disabled = disabled
  actions.appendChild(cancel)
  actions.appendChild(submit)
  form.appendChild(fieldsWrap)
  form.appendChild(actions)

  let submitting = false
  form.addEventListener('submit', event => {
    event.preventDefault()
    if (disabled || submitting) return
    let next = fields
    if (harness === 'dsh') {
      const entries = reasoningRows.filter(item => item.key.trim() || item.value)
      if (entries.some(item => !item.key.trim() || ['__proto__', 'constructor', 'prototype'].includes(item.key.trim()))) {
        errorNode.textContent = '请填写有效的思考级别 key'
        return
      }
      if (new Set(entries.map(item => item.key.trim())).size !== entries.length) {
        errorNode.textContent = '思考级别的 key 不能重复'
        return
      }
      next = { ...fields, reasoningEfforts: entries.length ? Object.fromEntries(entries.map(item => [item.key.trim(), item.value])) : (fields.reasoningEfforts === false ? false : undefined) }
    }
    submitting = true
    errorNode.textContent = ''
    onSubmit(next, harness === 'droid' ? apiKey.value : undefined)
      .catch(error => { errorNode.textContent = messageOf(error) })
      .finally(() => { submitting = false })
  })

  const dialog = ModelDialog({ title, subtitle: harness, disabled, returnFocus: editor.returnFocus, onCancel, content: form })
  return { cleanup: dialog.cleanup }
}

// MCP 编辑器。
export function McpEditorDialog({ editor, disabled, onCancel, onSubmit }) {
  const fields = { ...editor.fields, args: [...editor.fields.args], env: { ...editor.fields.env }, headers: { ...editor.fields.headers }, envHeaders: { ...editor.fields.envHeaders } }
  const harness = editor.source.harness
  const prefix = `mcp-editor-${harness}`
  const title = editor.target ? '修改 MCP' : editor.copyFrom ? '复制 MCP' : '新增 MCP'

  const envPairs = { list: Object.entries(fields.env).map(([key, value]) => ({ key, value })) }
  const headerPairs = { list: Object.entries(fields.headers).map(([key, value]) => ({ key, value })) }
  const envHeaderPairs = { list: Object.entries(fields.envHeaders).map(([key, value]) => ({ key, value })) }

  const form = el('form', 'model-editor')
  form.setAttribute('aria-label', `${harness} ${title}`)
  const fieldsWrap = el('div', 'model-editor-fields')

  const textInput = (key, label, required = false) => {
    const input = el('input', 'field-input model-text-input')
    input.id = `${prefix}-${key}`
    input.type = 'text'
    input.value = fields[key]
    input.disabled = disabled
    input.required = required
    input.autocomplete = 'off'
    input.spellcheck = false
    input.addEventListener('input', () => { fields[key] = input.value })
    return SettingRow(label, input.id, null, input)
  }

  fieldsWrap.appendChild(textInput('name', '名称', true))

  const transportOptions = [
    { value: 'stdio', label: 'STDIO' },
    { value: 'http', label: 'HTTP' },
    ...(harness === 'claude' ? [{ value: 'sse', label: 'SSE' }, { value: 'ws', label: 'WebSocket' }] : []),
  ]
  const transportSelect = SelectControl({
    id: `${prefix}-transport`, label: 'MCP 传输方式', value: fields.transport, disabled, options: transportOptions,
    onChange: value => { fields.transport = value; renderTransportSections() },
  })
  fieldsWrap.appendChild(SettingRow('传输方式', `${prefix}-transport`, null, transportSelect.node))

  const stdioSection = el('div')
  const httpSection = el('div')
  stdioSection.appendChild(textInput('command', '启动命令', true))
  if (harness === 'codex') stdioSection.appendChild(textInput('cwd', '工作目录'))
  stdioSection.appendChild(buildArgsSection())
  stdioSection.appendChild(buildPairsSection('环境变量', envPairs))
  httpSection.appendChild(textInput('url', '地址', true))
  if (harness === 'codex') httpSection.appendChild(textInput('bearerTokenEnvVar', 'Token 环境变量'))
  httpSection.appendChild(buildPairsSection('请求头', headerPairs))
  if (harness === 'codex') httpSection.appendChild(buildPairsSection('请求头环境变量', envHeaderPairs))

  function buildArgsSection() {
    const section = el('section', 'model-reasoning')
    section.setAttribute('aria-label', '启动参数')
    const head = el('div', 'model-reasoning-header')
    head.appendChild(el('span', '', '启动参数'))
    const add = el('button', 'icon-button')
    add.type = 'button'
    add.title = '增加启动参数'
    add.setAttribute('aria-label', '增加启动参数')
    add.disabled = disabled
    add.innerHTML = icon('plus', 14)
    const rows = el('div', 'model-reasoning-rows')
    const renderRows = () => {
      rows.textContent = ''
      fields.args.forEach((arg, index) => {
        const line = el('div', 'mcp-arg-row')
        const input = el('input', 'field-input model-text-input')
        input.setAttribute('aria-label', `启动参数 ${index + 1}`)
        input.value = arg
        input.disabled = disabled
        input.autocomplete = 'off'
        input.spellcheck = false
        input.addEventListener('input', () => { fields.args[index] = input.value })
        const remove = el('button', 'icon-button')
        remove.type = 'button'
        remove.setAttribute('aria-label', `删除启动参数 ${index + 1}`)
        remove.disabled = disabled
        remove.innerHTML = icon('trash2', 14)
        remove.addEventListener('click', () => { fields.args.splice(index, 1); renderRows() })
        line.appendChild(input)
        line.appendChild(remove)
        rows.appendChild(line)
      })
    }
    add.addEventListener('click', () => { fields.args.push(''); renderRows() })
    head.appendChild(add)
    section.appendChild(head)
    renderRows()
    return section
  }

  function buildPairsSection(label, holder) {
    const section = el('section', 'model-reasoning mcp-pairs')
    section.setAttribute('aria-label', label)
    const head = el('div', 'model-reasoning-header')
    head.appendChild(el('span', '', label))
    const add = el('button', 'icon-button')
    add.type = 'button'
    add.title = `增加${label}`
    add.setAttribute('aria-label', `增加${label}`)
    add.disabled = disabled
    add.innerHTML = icon('plus', 14)
    const rows = el('div', 'model-reasoning-rows')
    const renderRows = () => {
      rows.textContent = ''
      holder.list.forEach((pair, index) => {
        const line = el('div', 'model-reasoning-row')
        const keyWrap = el('div', 'model-reasoning-key')
        const keyInput = el('input', 'field-input')
        keyInput.setAttribute('aria-label', `${label}名称 ${index + 1}`)
        keyInput.value = pair.key
        keyInput.disabled = disabled
        keyInput.autocomplete = 'off'
        keyInput.spellcheck = false
        keyInput.addEventListener('input', () => { pair.key = keyInput.value })
        keyWrap.appendChild(keyInput)
        keyWrap.appendChild(html('span', 'model-reasoning-arrow', '->'))
        const valueWrap = el('div', 'model-reasoning-value')
        const valueInput = el('input', 'field-input')
        valueInput.setAttribute('aria-label', `${label}值 ${index + 1}`)
        valueInput.value = pair.value ?? ''
        valueInput.disabled = disabled
        valueInput.autocomplete = 'off'
        valueInput.spellcheck = false
        valueInput.addEventListener('input', () => { pair.value = valueInput.value })
        const remove = el('button', 'icon-button')
        remove.type = 'button'
        remove.setAttribute('aria-label', `删除${label} ${index + 1}`)
        remove.disabled = disabled
        remove.innerHTML = icon('trash2', 14)
        remove.addEventListener('click', () => { holder.list = holder.list.filter((_, i) => i !== index); renderRows() })
        valueWrap.appendChild(valueInput)
        valueWrap.appendChild(remove)
        line.appendChild(keyWrap)
        line.appendChild(valueWrap)
        rows.appendChild(line)
      })
    }
    add.addEventListener('click', () => { holder.list.push({ key: '', value: '' }); renderRows() })
    head.appendChild(add)
    section.appendChild(head)
    renderRows()
    return section
  }

  function renderTransportSections() {
    stdioSection.hidden = fields.transport !== 'stdio'
    httpSection.hidden = fields.transport === 'stdio'
  }
  renderTransportSections()

  const errorNode = el('p', 'field-error model-error')
  errorNode.setAttribute('role', 'alert')
  fieldsWrap.appendChild(stdioSection)
  fieldsWrap.appendChild(httpSection)
  fieldsWrap.appendChild(errorNode)

  const actions = el('div', 'harness-actions model-editor-actions')
  const cancel = el('button', 'button secondary')
  cancel.type = 'button'
  cancel.textContent = '取消'
  cancel.disabled = disabled
  cancel.addEventListener("click", () => onCancel())
  const submit = el('button', 'button primary')
  submit.type = 'submit'
  submit.textContent = '确定'
  submit.disabled = disabled
  actions.appendChild(cancel)
  actions.appendChild(submit)
  form.appendChild(fieldsWrap)
  form.appendChild(actions)

  function pairsMap(pairs, label) {
    const entries = pairs.filter(item => item.key || item.value)
    if (entries.some(item => !item.key.trim())) throw new Error(`请填写${label}的名称`)
    if (new Set(entries.map(item => item.key)).size !== entries.length) throw new Error(`${label}的名称不能重复`)
    return Object.fromEntries(entries.map(item => [item.key, item.value]))
  }

  let submitting = false
  form.addEventListener('submit', event => {
    event.preventDefault()
    if (disabled || submitting) return
    let next
    try {
      next = { ...fields, env: pairsMap(envPairs.list, '环境变量'), headers: pairsMap(headerPairs.list, '请求头'), envHeaders: pairsMap(envHeaderPairs.list, '请求头环境变量') }
    } catch (error) {
      errorNode.textContent = messageOf(error)
      return
    }
    submitting = true
    errorNode.textContent = ''
    onSubmit(next).catch(error => { errorNode.textContent = messageOf(error) }).finally(() => { submitting = false })
  })

  const dialog = ModelDialog({ title, subtitle: harness, disabled, returnFocus: editor.returnFocus, onCancel, closeLabel: '关闭 MCP 弹窗', className: 'mcp-modal', content: form })
  return { cleanup: dialog.cleanup }
}

// 批量模型弹窗。
export function ModelBatchDialog({ action, disabled, returnFocus, onCancel, onSubmit }) {
  const model = { value: '' }
  const originalModel = { value: '' }
  const title = action === 'replace' ? '一键修改模型' : action === 'delete' ? '一键删除模型' : '一键添加模型'
  const form = el('form', 'model-editor')
  form.setAttribute('aria-label', title)
  const fieldsWrap = el('div', 'model-editor-fields')

  const row = (label, id, holder) => {
    const input = el('input', 'field-input model-text-input')
    input.id = id
    input.type = 'text'
    input.required = true
    input.disabled = disabled
    input.autocomplete = 'off'
    input.spellcheck = false
    input.addEventListener('input', () => { holder.value = input.value })
    return SettingRow(label, id, null, input)
  }
  if (action === 'replace') fieldsWrap.appendChild(row('原模型', 'batch-original-model', originalModel))
  fieldsWrap.appendChild(row(action === 'delete' ? '模型 ID' : '新模型', action === 'delete' ? 'batch-delete-model' : 'batch-new-model', model))

  const errorNode = el('p', 'field-error model-error')
  errorNode.setAttribute('role', 'alert')
  fieldsWrap.appendChild(errorNode)

  const actions = el('div', 'harness-actions model-editor-actions')
  const cancel = el('button', 'button secondary')
  cancel.type = 'button'
  cancel.textContent = '取消'
  cancel.disabled = disabled
  cancel.addEventListener("click", () => onCancel())
  const submit = el('button', 'button primary')
  submit.type = 'submit'
  submit.textContent = '确定'
  submit.disabled = disabled
  actions.appendChild(cancel)
  actions.appendChild(submit)
  form.appendChild(fieldsWrap)
  form.appendChild(actions)

  let submitting = false
  form.addEventListener('submit', event => {
    event.preventDefault()
    if (disabled || submitting) return
    submitting = true
    errorNode.textContent = ''
    onSubmit({ action, model: model.value, ...(action === 'replace' ? { originalModel: originalModel.value } : {}) })
      .catch(error => { errorNode.textContent = messageOf(error) })
      .finally(() => { submitting = false })
  })

  const dialog = ModelDialog({ title, subtitle: 'Models', disabled, returnFocus, onCancel, content: form })
  return { cleanup: dialog.cleanup }
}

export { api }
