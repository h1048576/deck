// 共享控件：DOM 工具、下拉、开关、尺寸输入。
import { ICONS } from './icons.js'

export function icon(name, size = 16, strokeWidth = 2, className = '') {
  const inner = ICONS[name] || ''
  return `<svg class="${className}" width="${size}" height="${size}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="${strokeWidth}" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${inner}</svg>`
}

export function el(tag, className, text) {
  const node = document.createElement(tag)
  if (className) node.className = className
  if (text !== undefined) node.textContent = text
  return node
}

export function html(tag, className, markup) {
  const node = document.createElement(tag)
  if (className) node.className = className
  node.innerHTML = markup
  return node
}

// ---- 下拉菜单（SelectControl / EditableSelectControl / FontControl）----

function useDropdown(options, value, disabled, onChange) {
  const state = { open: false, active: -1, menu: null, anchor: null, options, value, disabled, onChange }
  const enabled = () => options.map((option, index) => option.disabled ? -1 : index).filter(index => index !== -1)

  function show(last = false) {
    if (state.disabled) return
    const selected = options.findIndex(option => option.value === value && !option.disabled)
    const list = enabled()
    state.active = selected >= 0 ? selected : (last ? list[list.length - 1] : list[0]) ?? -1
    open()
  }

  function choose(index) {
    const option = options[index]
    if (state.disabled || !option || option.disabled) return
    close()
    onChange(option.value)
    const focusable = state.anchor && state.anchor.querySelector('input, button')
    if (focusable) focusable.focus()
  }

  function onKeyDown(event, editable = false) {
    if (state.disabled || event.isComposing) return
    if (event.key === 'Escape' && state.open) { event.preventDefault(); event.stopPropagation(); close(); return }
    if (event.key === 'Tab') { close(); return }
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      if (!state.open) { show(event.key === 'ArrowUp'); return }
      const list = enabled()
      const current = list.indexOf(state.active)
      const next = (current + (event.key === 'ArrowDown' ? 1 : -1) + list.length) % list.length
      state.active = list[next] ?? -1
      updateActive()
    } else if (state.open && (event.key === 'Home' || event.key === 'End')) {
      event.preventDefault()
      const list = enabled()
      state.active = (event.key === 'Home' ? list[0] : list[list.length - 1]) ?? -1
      updateActive()
    } else if (event.key === 'Enter' || (!editable && event.key === ' ')) {
      if (state.open) { event.preventDefault(); choose(state.active) }
      else if (!editable) { event.preventDefault(); show() }
    }
  }

  function place() {
    if (!state.menu || !state.anchor) return
    const bounds = state.anchor.getBoundingClientRect()
    if (bounds.bottom < 8 || bounds.top > window.innerHeight - 8) { close(); return }
    const margin = 8
    const gap = 4
    const below = window.innerHeight - bounds.bottom - gap - margin
    const above = bounds.top - gap - margin
    const desired = Math.min(400, options.length * 42 + 14)
    const upwards = below < desired && above > below
    const maxHeight = Math.max(0, Math.min(400, upwards ? above : below))
    const width = Math.min(bounds.width, window.innerWidth - margin * 2)
    const left = Math.max(margin, Math.min(bounds.left, window.innerWidth - width - margin))
    state.menu.style.left = left + 'px'
    state.menu.style.width = width + 'px'
    state.menu.style.maxHeight = maxHeight + 'px'
    if (upwards) {
      state.menu.style.top = ''
      state.menu.style.bottom = (window.innerHeight - bounds.top + gap) + 'px'
    } else {
      state.menu.style.bottom = ''
      state.menu.style.top = (bounds.bottom + gap) + 'px'
    }
    const compact = width < 120
    state.menu.classList.toggle('dropdown-menu-compact', compact)
  }

  function open() {
    if (state.open) return
    state.open = true
    const menu = el('div', 'dropdown-menu')
    menu.id = state.id + '-options'
    menu.setAttribute('role', 'listbox')
    menu.setAttribute('aria-label', state.label)
    menu.style.visibility = 'hidden'
    document.body.appendChild(menu)
    state.menu = menu
    renderOptions()
    requestAnimationFrame(() => { place(); if (state.menu) state.menu.style.visibility = '' })
    const dismiss = (event) => {
      const target = event.target
      if (target instanceof Node && !state.anchor.contains(target) && !state.menu.contains(target)) close()
    }
    document.addEventListener('pointerdown', dismiss)
    document.addEventListener('focusin', dismiss)
    window.addEventListener('resize', place)
    window.addEventListener('scroll', place, true)
    const observer = new ResizeObserver(place)
    observer.observe(state.anchor)
    state.cleanup = () => {
      document.removeEventListener('pointerdown', dismiss)
      document.removeEventListener('focusin', dismiss)
      window.removeEventListener('resize', place)
      window.removeEventListener('scroll', place, true)
      observer.disconnect()
    }
  }

  function renderOptions() {
    if (!state.menu) return
    state.menu.textContent = ''
    options.forEach((option, index) => {
      const item = el('div', `dropdown-option ${index === state.active ? 'active' : ''} ${option.disabled ? 'disabled' : ''}`)
      item.id = `${state.id}-option-${index}`
      item.setAttribute('role', 'option')
      item.setAttribute('aria-selected', option.value === value ? 'true' : 'false')
      if (option.disabled) item.setAttribute('aria-disabled', 'true')
      const label = el('span', '', option.label)
      item.appendChild(label)
      if (option.value === value) item.appendChild(html('span', '', icon('check', 16)))
      item.addEventListener('pointermove', () => { if (!option.disabled) { state.active = index; updateActive() } })
      item.addEventListener('pointerdown', event => event.preventDefault())
      item.addEventListener('click', () => choose(index))
      state.menu.appendChild(item)
    })
    updateActive()
  }

  function updateActive() {
    if (!state.menu) return
    Array.from(state.menu.children).forEach((child, index) => {
      child.classList.toggle('active', index === state.active)
      if (index === state.active) child.scrollIntoView({ block: 'nearest' })
    })
  }

  function close() {
    if (!state.open) return
    state.open = false
    if (state.cleanup) { state.cleanup(); state.cleanup = null }
    if (state.menu) { state.menu.remove(); state.menu = null }
  }

  function setDisabled(next) { state.disabled = next; if (next) close() }
  return { state, show, close, onKeyDown, choose, setDisabled }
}

export function SelectControl({ id, label, value, disabled = false, onChange, options }) {
  const dropdown = useDropdown(options, value, disabled, onChange)
  dropdown.state.id = id
  dropdown.state.label = label
  const anchor = el('div', 'select-control')
  dropdown.state.anchor = anchor
  const trigger = el('button', 'dropdown-trigger')
  trigger.id = id
  trigger.type = 'button'
  trigger.setAttribute('role', 'combobox')
  trigger.setAttribute('aria-label', label)
  trigger.setAttribute('aria-haspopup', 'listbox')
  trigger.setAttribute('aria-expanded', 'false')
  trigger.disabled = disabled
  const span = el('span', '', options.find(option => option.value === value)?.label ?? value)
  trigger.appendChild(span)
  trigger.appendChild(html('span', 'dropdown-chevron', icon('chevronDown', 15)))
  trigger.addEventListener('click', () => { if (dropdown.state.open) dropdown.close(); else dropdown.show() })
  trigger.addEventListener('keydown', event => dropdown.onKeyDown(event))
  anchor.appendChild(trigger)
  const update = (nextValue, nextDisabled) => {
    dropdown.state.value = nextValue
    span.textContent = options.find(option => option.value === nextValue)?.label ?? nextValue
    if (nextDisabled !== undefined) {
      trigger.disabled = nextDisabled
      dropdown.setDisabled(nextDisabled)
    }
  }
  return { node: anchor, update }
}

export function EditableSelectControl({ id, label, value, disabled = false, options, onChange }) {
  const dropdown = useDropdown(options, value, disabled, onChange)
  dropdown.state.id = id
  dropdown.state.label = label
  const anchor = el('div', 'editable-select-control')
  dropdown.state.anchor = anchor
  const input = el('input', 'field-input')
  input.id = id
  input.type = 'text'
  input.setAttribute('role', 'combobox')
  input.value = value
  input.setAttribute('aria-label', label)
  input.setAttribute('aria-haspopup', 'listbox')
  input.autocomplete = 'off'
  input.spellcheck = false
  input.disabled = disabled
  input.addEventListener('input', () => onChange(input.value))
  input.addEventListener('keydown', event => dropdown.onKeyDown(event, true))
  const button = el('button', 'font-dropdown-button')
  button.type = 'button'
  button.setAttribute('aria-label', `选择${label}`)
  button.setAttribute('aria-haspopup', 'listbox')
  button.innerHTML = icon('chevronDown', 15)
  button.disabled = disabled
  button.addEventListener('click', () => {
    if (dropdown.state.open) dropdown.close(); else dropdown.show()
    const focusInput = anchor.querySelector('input')
    if (focusInput) focusInput.focus()
  })
  button.addEventListener('keydown', event => dropdown.onKeyDown(event))
  anchor.appendChild(input)
  anchor.appendChild(button)
  const update = (nextValue, nextDisabled) => {
    dropdown.state.value = nextValue
    input.value = nextValue
    if (nextDisabled !== undefined) {
      input.disabled = nextDisabled
      button.disabled = nextDisabled
      dropdown.setDisabled(nextDisabled)
    }
  }
  return { node: anchor, update }
}

const FONT_OPTIONS = ['Cascadia Mono, LXGW WenKai Mono', 'LXGW WenKai Mono', 'Cascadia Mono', 'Microsoft YaHei', 'Segoe UI', 'PingFang SC', 'Noto Sans CJK SC', 'Arial'].map(font => ({ value: font, label: font }))
const SYSTEM_FONT_OPTIONS = [{ value: '系统默认', label: '系统默认' }, ...FONT_OPTIONS]

export function FontControl({ id, value, disabled, error, onChange, systemOption = false, label }) {
  const options = systemOption ? SYSTEM_FONT_OPTIONS : FONT_OPTIONS
  const resolvedLabel = label ?? (systemOption ? '应用字体' : '字体')
  const dropdown = useDropdown(options, value, disabled, onChange)
  dropdown.state.id = id
  dropdown.state.label = resolvedLabel
  const anchor = el('div', 'font-control')
  dropdown.state.anchor = anchor
  const input = el('input', 'field-input')
  input.id = id
  input.type = 'text'
  input.setAttribute('role', 'combobox')
  input.value = value
  input.setAttribute('aria-label', resolvedLabel)
  input.setAttribute('aria-haspopup', 'listbox')
  input.autocomplete = 'off'
  input.spellcheck = false
  input.disabled = disabled
  if (error) { input.setAttribute('aria-invalid', 'true'); input.setAttribute('aria-describedby', `${id}-error`) }
  input.addEventListener('input', () => onChange(input.value))
  input.addEventListener('keydown', event => dropdown.onKeyDown(event, true))
  const button = el('button', 'font-dropdown-button')
  button.type = 'button'
  button.setAttribute('aria-label', `选择${resolvedLabel}`)
  button.setAttribute('aria-haspopup', 'listbox')
  button.innerHTML = icon('chevronDown', 15)
  button.disabled = disabled
  button.addEventListener('click', () => {
    if (dropdown.state.open) dropdown.close(); else dropdown.show()
    if (anchor.querySelector('input')) anchor.querySelector('input').focus()
  })
  button.addEventListener('keydown', event => dropdown.onKeyDown(event))
  anchor.appendChild(input)
  anchor.appendChild(button)
  const update = (nextValue, nextDisabled) => {
    dropdown.state.value = nextValue
    input.value = nextValue
    if (nextDisabled !== undefined) {
      input.disabled = nextDisabled
      button.disabled = nextDisabled
      dropdown.setDisabled(nextDisabled)
    }
  }
  return { node: anchor, update }
}

// ---- 简单控件 ----

export function SettingRow(label, htmlFor, error, ...children) {
  const row = el('div', `setting-row ${error ? 'has-error' : ''}`)
  const labelNode = el('label', 'setting-label', label)
  if (htmlFor) labelNode.htmlFor = htmlFor
  row.appendChild(labelNode)
  const value = el('div', 'setting-value')
  children.forEach(child => value.appendChild(child))
  if (error) {
    const errorNode = el('p', 'field-error', error)
    if (htmlFor) errorNode.id = `${htmlFor}-error`
    value.appendChild(errorNode)
  }
  row.appendChild(value)
  return row
}

export function Switch(label, checked, disabled, onChange) {
  const button = el('button', `switch ${checked ? 'on' : ''}`)
  button.type = 'button'
  button.setAttribute('role', 'switch')
  button.setAttribute('aria-label', label)
  button.setAttribute('aria-checked', checked ? 'true' : 'false')
  button.disabled = disabled
  button.appendChild(el('span'))
  button.addEventListener('click', () => onChange(!checked))
  return button
}

export function PixelControl({ id, min, max, step = 1, value, placeholder, disabled, invalid, describedBy, onInput }) {
  const wrap = el('div', 'dimension-control')
  const input = el('input', 'field-input dimension-number')
  input.id = id
  input.type = 'number'
  input.min = String(min)
  if (max !== undefined) input.max = String(max)
  input.step = String(step)
  input.value = value
  if (placeholder) input.placeholder = placeholder
  input.disabled = disabled
  if (invalid) input.setAttribute('aria-invalid', 'true')
  if (describedBy) input.setAttribute('aria-describedby', describedBy)
  input.addEventListener('input', () => onInput(input.value))
  wrap.appendChild(input)
  const unit = el('span', 'dimension-unit', 'px')
  if (disabled) unit.setAttribute('aria-disabled', 'true')
  wrap.appendChild(unit)
  return { node: wrap, input }
}

const SIZE_UNITS = ['%', 'vw', 'rem', 'px']

export function DimensionControl({ id, label, value, fallback, disabled, error, onChange }) {
  const match = /^([0-9]*(?:\.[0-9]*)?)(%|vw|rem|px)$/.exec(value)
  const fallbackMatch = /^([0-9]+)(%|vw|rem|px)$/.exec(fallback)
  const number = match ? match[1] : ''
  const unit = match ? match[2] : fallbackMatch[2]
  const legacy = !match && !!value
  const numberInput = el('input', 'field-input dimension-number')
  numberInput.id = id
  numberInput.type = 'number'
  numberInput.min = '0'
  numberInput.max = '9999'
  numberInput.step = '5'
  numberInput.value = number
  numberInput.disabled = disabled
  numberInput.setAttribute('aria-label', `${label}数值`)
  if (error) numberInput.setAttribute('aria-invalid', 'true')
  if (error) numberInput.setAttribute('aria-describedby', `${id}-error`)
  if (legacy) numberInput.placeholder = value
  const wrap = el('div', 'dimension-control')
  wrap.appendChild(numberInput)
  const options = [
    ...(legacy ? [{ value: 'legacy', label: value, disabled: true }] : []),
    ...SIZE_UNITS.map(sizeUnit => ({ value: sizeUnit, label: sizeUnit }))
  ]
  const select = SelectControl({ id: `${id}-unit`, label: `${label}单位`, value: legacy ? 'legacy' : unit, disabled, options, onChange: next => {
    onChange(`${number || fallbackMatch[1]}${next}`)
  } })
  wrap.appendChild(select.node)
  numberInput.addEventListener('input', () => onChange(`${numberInput.value}${unit}`))
  return SettingRow(label, id, error, wrap)
}
