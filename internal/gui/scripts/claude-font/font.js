// 在 Claude Desktop 内容页的开发者工具 Console 或 Snippet 中执行。
// 更改字体，并去掉行内代码的红色；页面刷新或应用重启后需要重新执行。
(() => {
  const fontFamily = 'Maple Mono NF CN'
  const key = '__deckClaudeFont'
  const styleId = 'deck-claude-font'
  if (location.protocol !== 'app:' || location.hostname !== 'localhost' || !document.body) {
    throw new Error('请在 Claude Desktop 内容页面的开发者工具中执行字体脚本。')
  }

  window[key]?.restore()
  let observer
  let active = true
  let css = ''
  const ensureStyle = () => {
    if (!active) return
    let style = document.getElementById(styleId)
    if (!style) {
      style = document.createElement('style')
      style.id = styleId
      ;(document.head || document.documentElement).appendChild(style)
    }
    if (style.textContent !== css) style.textContent = css
  }
  const status = () => ({
    active,
    fontFamily: getComputedStyle(document.body).fontFamily,
    persistentAfterReload: false,
  })
  const apply = value => {
    if (typeof value !== 'string' || !value.trim() || value.length > 300 || /[;{}<>\r\n]/.test(value)) {
      throw new Error('字体名称无效。')
    }
    const names = value.split(',').map(name => name.trim().replace(/^["']|["']$/g, '')).filter(Boolean)
    if (!names.length) throw new Error('请至少填写一种字体。')
    const family = [...names.map(name => JSON.stringify(name)), 'monospace'].join(', ')
    css = `
      :root, .cds-root {
        --font-ui: ${family} !important;
        --font-user-message: ${family} !important;
        --font-claude-response: ${family} !important;
        --cds-font-sans: ${family} !important;
        --cds-font-mono: ${family} !important;
      }
      body, body :not(svg):not(svg *):not(math):not(math *):not(.katex):not(.katex *):not([data-cds=Icon]) {
        font-family: ${family} !important;
      }
      /* 行内代码默认用红色（--cds-text-danger），改为跟随正文；链接和按钮里的文件路径保留强调色。 */
      code:not(pre code):not(:is(a, button, [role=button]) code) {
        color: inherit !important;
      }
    `
    active = true
    ensureStyle()
    observer?.disconnect()
    observer = new MutationObserver(ensureStyle)
    observer.observe(document.documentElement, { childList: true, subtree: true })
    return status()
  }
  const restore = () => {
    active = false
    observer?.disconnect()
    document.getElementById(styleId)?.remove()
    return status()
  }
  // 可调用 window.__deckClaudeFont.restore() 恢复，或 apply('字体名称') 切换字体。
  window[key] = { apply, restore, status }
  return apply(fontFamily)
})()
