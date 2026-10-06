import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { createRoot } from 'react-dom/client'
import { BrowserWorkspace } from '@/components/browser/BrowserWorkspace'
import { SidePanelStateProvider } from '@/components/session/SidePanelState'
import { ThreadView } from '@/components/session/ThreadView'
import { ToastProvider } from '@/components/ui/toast'
import type { AgentSessionConfigOption, Session } from '@/lib/api/types'
import { TitlebarProvider } from '@/lib/titlebar'
import { VoiceProvider } from '@/lib/voice/VoiceProvider'

export async function exerciseSessionConfig(): Promise<void> {
  const session: Session = {
    id: 'config-smoke', slug: 'config-smoke', title: 'Composer settings', runtime: 'acp', status: 'running',
    created_at: '', updated_at: '', last_attention_at: '', runtime_ref: { type: 'acp', agent: 'codex' },
  }
  const fast: AgentSessionConfigOption = {
    id: 'fast-mode', name: 'Fast mode', category: 'model_config', current_value: 'off',
    options: [{ value: 'off', name: 'Off' }, { value: 'on', name: 'On' }],
  }
  const save = Promise.withResolvers<void>()
  const refresh = Promise.withResolvers<void>()
  let saving = false
  let refreshing = false
  const originalFetch = window.fetch
  window.fetch = async (input, init) => {
    const url = String(input)
    if (url.includes('/v1/sessions/config-smoke/agent/config')) {
      const body = JSON.parse(String(init?.body))
      if (body.id !== fast.id || body.value !== 'on') {
        throw new Error('Unexpected setting change: ' + JSON.stringify(body))
      }
      saving = true
      await save.promise
      fast.current_value = body.value
      return new Response(null, { status: 204 })
    }
    if (url.includes('/v1/sessions/config-smoke/messages')) {
      return Response.json({ session, messages: [], events: [], latest_event_seq: 0 })
    }
    if (url.includes('/v1/sessions/config-smoke/seen')) {
      return Response.json(session)
    }
    if (url.includes('/v1/sessions/config-smoke/overview')) {
      if (saving) {
        refreshing = true
        await refresh.promise
      }
      return Response.json({ agent_events: [{ session_id: session.id, seq: 1, at: new Date().toISOString(), type: 'agent_session', agent_session: { config_options: [fast] } }] })
    }
    if (url.includes('/v1/sessions/config-smoke/repo')) {
      return Response.json({ git: false })
    }
    return originalFetch(input, init)
  }
  const element = document.createElement('div')
  element.style.cssText = 'position:fixed;inset:0;background:var(--color-bg);z-index:1'
  document.body.append(element)
  const root = createRoot(element)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const route = createRootRoute({ component: () => (
    <TitlebarProvider><BrowserWorkspace><SidePanelStateProvider>
      <div className="h-full"><ThreadView sessionId={session.id} /></div>
    </SidePanelStateProvider></BrowserWorkspace></TitlebarProvider>
  ) })
  const router = createRouter({ routeTree: route, history: createMemoryHistory({ initialEntries: ['/'] }) })
  const until = async (check: () => boolean) => {
    const deadline = Date.now() + 5000
    while (!check()) {
      if (Date.now() > deadline) {
        throw new Error('Session config: ' + check.toString())
      }
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
  const toggle = () => document.querySelector<HTMLButtonElement>('button[aria-label="Fast Mode"]')
  const open = async () => {
    element.querySelector<HTMLButtonElement>('button[aria-label="Composer options"]')!.click()
    await until(() => Boolean(toggle()))
  }
  try {
    root.render(<QueryClientProvider client={client}><ToastProvider><VoiceProvider>
      <RouterProvider router={router} />
    </VoiceProvider></ToastProvider></QueryClientProvider>)
    await until(() => Boolean(element.querySelector('button[aria-label="Composer options"]')))
    if (element.querySelector('button[aria-label^="Model:"]')) {
      throw new Error('Existing chat shows a model picker')
    }
    await open()
    toggle()!.click()
    await until(() => saving && Boolean(toggle()?.disabled))
    await window.smoke.key('Escape')
    await until(() => !toggle())
    await open()
    if (!toggle()!.disabled) {
      throw new Error('Closing the menu lost the pending setting change')
    }
    save.resolve()
    await until(() => refreshing)
    await new Promise((resolve) => setTimeout(resolve, 50))
    if (!toggle()!.disabled) {
      throw new Error('Setting unlocked before refreshed native state arrived')
    }
    refresh.resolve()
    await until(() => toggle()?.getAttribute('aria-checked') === 'true' && !toggle()?.disabled)
  } finally {
    save.resolve()
    refresh.resolve()
    root.unmount()
    client.clear()
    element.remove()
    window.fetch = originalFetch
  }
}
