import { Select } from '@/components/ui/Select'
import { Switch } from '@/components/ui/Switch'
import { fastModeOption } from '@/lib/agentConfig'
import type { AgentSessionConfigOption } from '@/lib/api/types'

export function NativeModelOptions({ options, running, pending, onChange }: {
  options?: AgentSessionConfigOption[] | null
  running: boolean
  pending: boolean
  onChange: (id: string, value: string) => void
}) {
  const fastMode = fastModeOption(options)
  return options?.filter((option) => option.category === 'model_config').map((option) => (
    <div key={option.id} className="px-2.5 py-1">
      <div className="flex min-h-10 items-center justify-between gap-3">
        <span className="text-[13px] text-ink-2">{option === fastMode ? 'Fast Mode' : option.name}</span>
        {option === fastMode ? (
          <Switch
            aria-label="Fast Mode"
            checked={option.current_value === 'on'}
            disabled={pending}
            onChange={(checked) => onChange(option.id, checked ? 'on' : 'off')}
          />
        ) : <Select
          aria-label={option.name}
          value={option.current_value}
          options={option.options.map((value) => ({ value: value.value, label: value.name }))}
          disabled={running || pending}
          onChange={(value) => onChange(option.id, value)}
        />}
      </div>
      {option !== fastMode && option.description ? <p className="max-w-64 text-[11px] text-ink-3">{option.description}</p> : null}
    </div>
  ))
}
