import { type ReactNode, useState } from 'react'
import { motion } from 'motion/react'
import { DitherTerrain, DitherWordmark } from '@/components/launch/DitherArt'
import { ComposerCard } from '@/components/session/Composer'
import { FileDropScope } from '@/components/ui/FileDrop'
import { useAppearance } from '@/lib/appearance'
import { DEFAULT_HOME_WORDMARK, effectiveHomeWordmark, isHomeLogoUrl } from '@/lib/homeWordmark'
import type { SendMessageHandler } from '@/lib/sendMessage'

const HOME_WIDTH = 640

function HomeLogo({ value, invertInLightMode, invertInDarkMode }: {
  value: string
  invertInLightMode: boolean
  invertInDarkMode: boolean
}) {
  const url = isHomeLogoUrl(value) ? value : null
  const [failed, setFailed] = useState(false)
  return (
    <div className="flex h-48 items-center justify-center">
      {url && !failed ? (
        <img
          src={url}
          alt="Home logo"
          referrerPolicy="no-referrer"
          onError={() => setFailed(true)}
          className={`block max-h-full max-w-full object-contain ${invertInLightMode ? 'invert' : ''} ${invertInDarkMode ? 'dark:invert' : 'dark:invert-0'}`}
        />
      ) : (
        <DitherWordmark text={url ? DEFAULT_HOME_WORDMARK : value} maxWidth={HOME_WIDTH} />
      )}
    </div>
  )
}

export function NewSessionHome({
  creating,
  disabled = false,
  goalAvailable = false,
  leftSlot,
  optionsSlot,
  draftStorageKey,
  fileRoot,
  onSend,
  onVoice,
}: {
  creating: boolean
  disabled?: boolean
  goalAvailable?: boolean
  leftSlot: ReactNode
  optionsSlot?: ReactNode
  draftStorageKey?: string
  /** directory the composer's @-mention file picker indexes ('' = workspace root) */
  fileRoot?: string
  onSend: SendMessageHandler
  onVoice?: () => void
}) {
  const { settings } = useAppearance()
  const wordmark = effectiveHomeWordmark(settings.homeWordmark)

  return (
    <FileDropScope className="relative flex h-full flex-col overflow-hidden">
      <motion.div
        className="flex flex-1 items-center justify-center px-10 py-8 max-sm:px-4"
        initial={{ opacity: 0, y: 14, scale: 0.985 }}
        animate={{ opacity: 1, y: 0, scale: 1 }}
        transition={{ type: 'spring', stiffness: 320, damping: 28 }}
      >
        <div className="flex w-full flex-col gap-8" style={{ maxWidth: HOME_WIDTH }}>
          <HomeLogo
            key={wordmark}
            value={wordmark}
            invertInLightMode={settings.invertHomeLogoInLightMode}
            invertInDarkMode={settings.invertHomeLogoInDarkMode}
          />
          <ComposerCard
            streaming={creating}
            autoFocus
            placeholder="Ask anything, or hand your assistant a task…"
            planAvailable
            goalControlVisible
            goalAvailable={goalAvailable}
            disabled={creating || disabled}
            leftSlot={leftSlot}
            optionsSlot={optionsSlot}
            draftStorageKey={draftStorageKey}
            clearTiming="never"
            fileRoot={fileRoot}
            onSend={onSend}
            onVoice={onVoice}
          />
        </div>
      </motion.div>
      {/* in flow, so the hero centers in whatever the brandscape leaves; a short
          window shrinks the sky, never the composer */}
      <DitherTerrain sky className="flex min-h-0 shrink flex-col justify-end" />
    </FileDropScope>
  )
}
