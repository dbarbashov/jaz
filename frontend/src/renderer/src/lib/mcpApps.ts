import type { MCPAppEvent, MCPEntrypoint } from '@/lib/api/types'

export function isAppEntrypoint(app: MCPAppEvent, entrypoints: MCPEntrypoint[]): boolean {
  return entrypoints.some((point) => point.server_id === app.server_id && point.tool === app.tool)
}
