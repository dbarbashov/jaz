import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ModelSelect } from '@/components/session/ModelSelect'
import { NativeModelOptions } from '@/components/session/NativeModelOptions'
import { useToast } from '@/components/ui/toast'
import { fastModeOption } from '@/lib/agentConfig'
import { setSessionAgentConfig } from '@/lib/api/sessions'
import type { AgentSessionConfigOption } from '@/lib/api/types'
import { keys } from '@/lib/query/keys'

export function useNativeSessionControls(sessionId: string, options: AgentSessionConfigOption[] | null | undefined, running: boolean) {
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
  const change = (id: string, value: string) => update.mutate({ id, value })
  const model = options?.find((option) => option.category === 'model')
  const effort = options?.find((option) => option.category === 'thought_level')
  const fast = fastModeOption(options)
  const fastMode = fast ? {
    checked: fast.current_value === 'on',
    disabled: update.isPending,
    onChange: (checked: boolean) => change(fast.id, checked ? 'on' : 'off'),
  } : undefined
  return {
    menu: <NativeModelOptions options={options} running={running} pending={update.isPending} onChange={change} />,
    picker: model ? <ModelSelect
      value={model.current_value}
      effort={effort?.current_value ?? ''}
      suggestions={model.options.map((value) => ({ value: value.value, label: value.name }))}
      effortOptions={effort?.options.map((value) => ({ value: value.value, label: value.name })) ?? []}
      selectionDisabled={running || update.isPending}
      fastMode={fastMode}
      placement="above"
      onChange={(selection) => {
        if (selection.model !== model.current_value) change(model.id, selection.model)
        else if (effort && selection.effort !== effort.current_value) change(effort.id, selection.effort)
      }}
    /> : null,
  }
}
