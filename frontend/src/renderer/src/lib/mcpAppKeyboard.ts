// Sandboxed app key events stay in the iframe; forward only the host's tab shortcuts.
export const mcpAppKeyboard = `<script>
window.addEventListener('keydown', (event) => {
  if (!event.metaKey || event.defaultPrevented || event.altKey || event.ctrlKey || event.shiftKey || event.repeat) return
  if (!/^[1-9]$/.test(event.key) || document.querySelector('[role="dialog"][aria-modal="true"], dialog[open]')) return
  event.preventDefault()
  window.parent.postMessage({ type: 'jaz:navigation-shortcut', key: event.key }, '*')
})
</script>`
