import { useMutation, useQueryClient } from '@tanstack/react-query'
import { NativeModelOptions } from '@/components/session/NativeModelOptions'
import { useToast } from '@/components/ui/toast'
import { setSessionAgentConfig } from '@/lib/api/sessions'
import type { AgentSessionConfigOption } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

export function SessionModelOptions({ sessionId, options, running }: {
  sessionId: string
  options?: AgentSessionConfigOption[] | null
  running: boolean
}) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const update = useMutation({
    mutationFn: ({ id, value }: { id: string; value: string }) => setSessionAgentConfig(sessionId, id, value),
    onError: (error) => toast(error.message, 'danger'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: keys.sessionOverview(sessionId) })
      queryClient.invalidateQueries({ queryKey: keys.sessionMessages(sessionId) })
    },
  })
  return <NativeModelOptions options={options} running={running} pending={update.isPending} onChange={(id, value) => update.mutate({ id, value })} />
}
