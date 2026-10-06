// 缓存属于数据资源，切换页面不会丢失；失效后旧请求不能覆盖新数据。
// 移植 inventoryResource.ts + harnessResources.ts。

export function createInventoryResource(empty, load) {
  const resource = {
    snapshot: { data: empty, loaded: false, loading: false, error: '', updatedAt: 0 },
    generation: 0,
    pending: null,
    listeners: new Set(),
  }
  const publish = (next) => {
    resource.snapshot = next
    resource.listeners.forEach(listener => listener(next))
  }
  return {
    snapshot: () => resource.snapshot,
    subscribe(listener) {
      resource.listeners.add(listener)
      listener(resource.snapshot)
      return () => resource.listeners.delete(listener)
    },
    invalidate() {
      resource.generation++
      resource.pending = null
      publish({ ...resource.snapshot, loading: false, updatedAt: 0 })
    },
    update(change) { publish({ ...resource.snapshot, data: change(resource.snapshot.data) }) },
    refresh(force = false, loader) {
      const loadFn = loader || load
      if (resource.pending) return resource.pending
      if (!force && resource.snapshot.loaded && !resource.snapshot.error && Date.now() - resource.snapshot.updatedAt < 30000) {
        return Promise.resolve()
      }
      const currentGeneration = resource.generation
      publish({ ...resource.snapshot, loading: true })
      const request = Promise.resolve().then(() => loadFn(resource.snapshot.data)).then(data => {
        if (resource.generation === currentGeneration) publish({ data, loaded: true, loading: false, error: '', updatedAt: Date.now() })
      }).catch(error => {
        if (resource.generation === currentGeneration) publish({ ...resource.snapshot, loading: false, error: messageOf(error) })
      }).finally(() => { if (resource.pending === request) resource.pending = null })
      resource.pending = request
      return request
    },
  }
}

export function subscribeResource(resource, callback) {
  return resource.subscribe(callback)
}

// 启用时自动加载；回到窗口后检查外部配置变化。
export function useInventoryResource(resource, enabled, blocked) {
  const state = { loading: false }
  if (enabled && !blocked) {
    void resource.refresh()
    state.loading = resource.snapshot.loading || (!resource.snapshot.loaded && !resource.snapshot.error)
  } else if (enabled) {
    state.loading = !resource.snapshot.loaded && !resource.snapshot.error
  }
  return state
}

export { messageOf } from './api.js'

// ---- 数据资源定义 ----

import { api, messageOf } from './api.js'

export const emptyHarnesses = [
  { id: 'claude', name: 'Claude', directory: '.claude' },
  { id: 'agents', name: 'Agents', directory: '.agents' },
  { id: 'droid', name: 'Droid', directory: '.factory' },
  { id: 'codex', name: 'Codex', directory: '.codex' },
].map(harness => ({ id: harness.id, name: harness.name, path: `~/${harness.directory}`, skillsPath: `~/${harness.directory}/skills`, exists: false, skills: [] }))

export const agentsResource = createInventoryResource(
  { agentsSource: { path: '~/.claude/CLAUDE.md', exists: false }, harnesses: emptyHarnesses },
  () => api('harness/inventory?skills=0'),
)
export const skillsResource = createInventoryResource(emptyHarnesses, () => api('harness/skills'))
export const modelsResource = createInventoryResource({ sources: [] }, () => api('models/inventory'))
export const mcpsResource = createInventoryResource({ sources: [] }, () => api('mcps/inventory'))

export function displayHarnessPath(path) {
  if (!path) return path
  const normalized = path.split('\\').join('/')
  if (/(?:^|\/)\.claude\.json$/i.test(normalized)) return '~/.claude.json'
  const match = normalized.match(/(?:^|\/)(\.(?:claude|factory|codex|agents|dsh|pi|config\/opencode)(?:\/.*)?$)/i)
  return match ? `~/${match[1]}` : normalized
}
