import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useToast } from '@/components/ui/toast'
import { setSessionAgentConfig } from '@/lib/api/sessions'
import { keys } from '@/lib/query/keys'

export function useSessionConfig(sessionId: string) {
  const queryClient = useQueryClient()
  const toast = useToast()
  return useMutation({
    mutationFn: ({ id, value }: { id: string; value: string }) => setSessionAgentConfig(sessionId, id, value),
    onError: (error) => toast(error.message, 'danger'),
    onSuccess: () => Promise.all([
      queryClient.invalidateQueries({ queryKey: keys.sessionOverview(sessionId) }),
      queryClient.invalidateQueries({ queryKey: keys.sessionMessages(sessionId) }),
    ]),
  })
}
