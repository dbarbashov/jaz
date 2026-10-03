import type { MCPAppEvent, MCPEntrypoint } from '@/lib/api/types'

export function isPresentedApp(app: MCPAppEvent, entrypoints: MCPEntrypoint[]): boolean {
  return app.presented === true || entrypoints.some((point) => point.server_id === app.server_id && point.tool === app.tool)
}
