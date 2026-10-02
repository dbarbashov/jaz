import type { Bot } from '@/lib/api/types'

// Things a new bot can start on, for someone who does not know yet what to
// ask. They show only until the user says anything; picking one sends it as
// their first message.
const STARTERS = [
  'Every morning, go through my inbox and tell me what needs me',
  'Before each meeting, brief me on who I am meeting',
  'Keep my CRM up to date from my email and chats',
  'Research a topic for me and send me a summary',
  'Remind me about follow-ups I owe people',
]

export function BotWelcome({ bot, onPick }: { bot: Bot; onPick: (text: string) => void }) {
  return (
    <div className="flex flex-col items-start gap-2">
      <div className="max-w-[84%] rounded-card bg-surface px-3.5 py-2.5 text-sm">
        Hi, I&apos;m {bot.name}. What should I take care of? Pick one, or tell me in your own words.
      </div>
      <div className="flex w-full flex-col rounded-card border border-border bg-surface p-1.5">
        {STARTERS.map((text) => (
          <button
            key={text}
            type="button"
            onClick={() => onPick(text)}
            className="cursor-pointer rounded-lg px-2.5 py-2 text-left text-sm text-ink transition-colors duration-150 hover:bg-list-hover"
          >
            {text}
          </button>
        ))}
      </div>
    </div>
  )
}
