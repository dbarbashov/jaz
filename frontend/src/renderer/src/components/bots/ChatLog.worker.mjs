import { mock } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

mock.module('@/lib/theme', () => ({ useTheme: () => ({ resolved: 'light' }) }))
mock.module('@/components/ui/Modal', () => ({ Modal: () => null }))
mock.module('@/lib/appearance', () => ({
  useEffectsEnabled: () => false,
  useInlineDiffs: () => false,
  useInlineShellCommands: () => false,
}))
mock.module('@/lib/clientRuntime', () => ({
  DEFAULT_API_BASE_URL: 'http://127.0.0.1:5299',
  clientRuntime: { platform: 'browser', defaultApiBaseUrl: () => 'http://127.0.0.1:5299' },
}))
globalThis.localStorage = { getItem: () => null }

const { ChatLog } = await import('./ChatLog')
const { botChat } = await import('@/lib/bots')
const client = new QueryClient()
const self = { id: 'bot-1', name: 'Bot' }
const attachments = [
  { type: 'attachment', id: 'text-1', name: 'pasted-text-1.txt', mime_type: 'text/plain' },
  { type: 'attachment', id: 'image-1', name: 'screenshot.png', mime_type: 'image/png' },
]
const html = ['', 'Read these'].map((text) => {
  const messages = [{
    seq: 1,
    role: 'user',
    content: text,
    created_at: '2026-10-06T09:00:00Z',
    blocks: [{ type: 'text', text }, ...attachments],
  }]
  const { entries } = botChat(messages, [], self, false, [])
  return renderToStaticMarkup(createElement(QueryClientProvider, { client },
    createElement(ChatLog, { entries, bots: [], named: false, working: [] }),
  ))
})
client.clear()
globalThis.postMessage(html)
