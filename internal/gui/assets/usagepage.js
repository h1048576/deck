import { el, html, icon } from './controls.js'
import { api, messageOf } from './api.js'

export function createUsagePage() {
  const refreshInterval = 5 * 60 * 1000
  const node = el('div', 'settings-page usage-page')
  const actions = el('div', 'page-header-actions')
  const refreshAllButton = el('button', 'button secondary start-button')
  refreshAllButton.type = 'button'
  refreshAllButton.setAttribute('aria-label', '刷新全部用量')
  refreshAllButton.addEventListener('click', refreshAll)
  actions.appendChild(refreshAllButton)
  let lastFullRefreshAt = 0
  let refreshTimer = null
  const sources = [
    { id: 'codex', name: 'Codex' },
    { id: 'droid', name: 'Droid' },
    { id: 'workbuddy', name: 'WorkBuddy' },
  ].map(source => ({ ...source, data: null, loading: false, error: '' }))

  function metric(list, label, value, detail, percent) {
    const row = el('div', 'setting-row')
    const title = el('div', 'setting-label')
    title.appendChild(el('span', '', label))
    if (detail) title.appendChild(el('small', 'usage-detail', detail))
    const content = el('div', 'usage-value')
    content.appendChild(el('span', '', value))
    if (Number.isFinite(percent)) {
      const progress = el('progress', 'usage-progress')
      progress.max = 100
      progress.value = Math.max(0, Math.min(100, percent))
      progress.setAttribute('aria-label', `${label}：${value}`)
      content.appendChild(progress)
    }
    row.append(title, content)
    list.appendChild(row)
  }

  function render() {
    const loading = sources.some(source => source.loading)
    refreshAllButton.disabled = loading
    refreshAllButton.innerHTML = `${icon('refreshCw', 15, 2, loading ? 'spin' : '')}<span>${loading ? '刷新中…' : '刷新全部'}</span>`
    node.replaceChildren()
    for (const source of sources) {
      const section = el('section', 'settings-group')
      section.setAttribute('aria-label', `${source.name} 用量`)
      section.setAttribute('aria-busy', String(source.loading))
      const header = el('div', 'usage-heading')
      header.appendChild(el('h2', '', source.name))
      const refresh = html('button', 'button secondary harness-action', `${icon('refreshCw', 14, 2, source.loading ? 'spin' : '')}<span>${source.loading ? '查询中…' : '刷新'}</span>`)
      refresh.disabled = source.loading
      refresh.setAttribute('aria-label', `刷新 ${source.name} 用量`)
      refresh.addEventListener('click', () => { void load(source) })
      header.appendChild(refresh)
      section.appendChild(header)
      const list = el('div', 'settings-list')
      if (source.error) {
        const error = el('p', 'field-error harness-empty', source.error)
        error.setAttribute('role', 'alert')
        list.appendChild(error)
      } else if (!source.data) {
        list.appendChild(el('p', 'harness-empty', source.loading ? '正在查询用量…' : '暂无用量数据，可手动刷新'))
      } else if (source.id === 'workbuddy') {
        const { today, used, limit } = source.data
        metric(list, '今日', today && Number.isFinite(Number(today)) ? `${today} 已用` : today || '—')
        const percent = Number(limit) > 0 && used !== '' ? Number(used) / Number(limit) * 100 : undefined
        metric(list, '月度', `${used} / ${limit}`, '', percent)
      } else {
        const labels = { '5h': '5 小时', '7d': '7 天', '1m': '月度' }
        for (const item of source.data.items || []) {
          metric(list, labels[item.label] || item.label, item.used == null ? '—' : `${item.used}% 已用`, `重置时间：${item.reset || '—'}`, item.used)
        }
        if (!source.data.items?.length) list.appendChild(el('p', 'harness-empty', '暂无用量数据'))
      }
      section.appendChild(list)
      node.appendChild(section)
    }
  }

  async function load(source) {
    if (source.loading) return
    source.loading = true
    render()
    try {
      const data = await api(`usage/${source.id}`)
      source.data = data
      source.error = data.error || ''
    } catch (error) {
      source.error = messageOf(error)
    } finally {
      source.loading = false
      render()
    }
  }

  function refreshAll() {
    lastFullRefreshAt = Date.now()
    scheduleRefresh()
    for (const source of sources) void load(source)
  }

  function canAutoRefresh(date) {
    const day = date.getDay()
    return day !== 0 && day !== 6 && date.getHours() >= 8 && date.getHours() < 22
  }

  function scheduleRefresh() {
    clearTimeout(refreshTimer)
    const now = new Date()
    const next = new Date(Math.max(now.getTime(), lastFullRefreshAt + refreshInterval))
    if (!canAutoRefresh(next)) {
      if (next.getHours() >= 22) next.setDate(next.getDate() + 1)
      next.setHours(8, 0, 0, 0)
      while (next.getDay() === 0 || next.getDay() === 6) next.setDate(next.getDate() + 1)
    }
    refreshTimer = setTimeout(refreshStale, Math.max(0, next.getTime() - now.getTime()))
  }

  function refreshStale() {
    const now = new Date()
    if (canAutoRefresh(now) && (!lastFullRefreshAt || now.getTime() - lastFullRefreshAt >= refreshInterval)) {
      refreshAll()
    } else {
      scheduleRefresh()
    }
  }

  render()
  return { node, actions, refreshStale }
}
