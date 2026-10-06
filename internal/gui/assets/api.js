// 与 Go 后端通信的 JSON API 客户端（magpie 风格：同一个 handler 的 /api/*）。
export async function api(path, body) {
  const isGet = body === undefined
  const res = await fetch('/api/' + path, {
    method: isGet ? 'GET' : 'POST',
    headers: isGet ? undefined : { 'Content-Type': 'application/json' },
    body: isGet ? undefined : JSON.stringify(body),
  })
  if (res.status === 204) return null
  const text = await res.text()
  let data
  try { data = text ? JSON.parse(text) : null } catch {
    throw new Error(text.trim().slice(0, 200) || `${res.status} ${res.statusText}`)
  }
  if (!res.ok) {
    throw new Error((data && data.error) || `${res.status} ${res.statusText}`)
  }
  return data
}

// 统一错误消息提取。
export function messageOf(error) {
  return error instanceof Error ? error.message : String(error)
}
