import { useQuery } from '@tanstack/react-query'
import { Link, linkOptions, useRouterState } from '@tanstack/react-router'
import { LayoutDashboard, MessageSquare, Settings } from 'lucide-react'
import { useState } from 'react'
import { AppIcon } from '@/components/apps/AppIcon'
import { BotsGlyph } from '@/components/bots/BotAvatar'
import { botsQuery } from '@/lib/api/bots'
import { entrypointKey, mcpEntrypointsQuery } from '@/lib/api/mcp'

export const RAIL_WIDTH = 48

// The sections beside Chat. Bots keeps the thread panel for its own list;
// the rest take the whole content card.
export const SECTIONS = [
  { to: '/bots', label: 'Bots', Icon: BotsGlyph },
  { to: '/boards', label: 'Boards', Icon: LayoutDashboard },
] as const

export type RailTab = 'chat' | 'settings' | (typeof SECTIONS)[number]['to'] | `/apps/${string}/${string}`

// Settings rides in the URL search over any page, so it outranks the path.
export function railTab(pathname: string, settingsOpen: boolean): RailTab {
  if (settingsOpen) return 'settings'
  const app = /^\/apps\/([^/]+\/[^/]+)/.exec(pathname)?.[1]
  if (app) return `/apps/${app}` as RailTab
  // Loops open from the Bots panel, so they keep it.
  const section = pathname.startsWith('/loops') ? '/bots' : pathname
  return SECTIONS.find(({ to }) => section.startsWith(to))?.to ?? 'chat'
}

// Everything beside Chat takes the whole content card: the built-in sections,
// then each sidebar app a connected MCP server declares.
export function useRailSections() {
  const apps = (useQuery(mcpEntrypointsQuery).data ?? []).filter((entry) => entry.type === 'global')
  return [
    ...SECTIONS.map(({ to, label, Icon }) => ({ path: to, label, icon: <Icon aria-hidden />, link: linkOptions({ to }) })),
    ...apps.map((app) => ({
      path: `/apps/${entrypointKey(app)}`,
      label: app.title,
      icon: <AppIcon app={app} />,
      link: linkOptions({ to: '/apps/$serverId/$tool', params: { serverId: app.server_id, tool: app.tool } }),
    })),
  ]
}

const TAB_CLASS =
  'group relative grid size-9 place-items-center rounded-[10px] transition-[background-color,color,transform] duration-150 active:scale-[0.96] [&_svg]:size-[18px] [&_svg]:stroke-[1.75]'

const tabClass = (active: boolean) =>
  `${TAB_CLASS} ${active ? 'bg-list-active text-ink' : 'text-ink-2 hover:bg-list-hover hover:text-ink'}`

// The rail is icon-only, so each tab names itself beside the icon on hover.
function TabLabel({ children }: { children: string }) {
  return (
    <span
      aria-hidden
      className="pointer-events-none absolute left-full top-1/2 z-tooltip ml-2 -translate-y-1/2 whitespace-nowrap rounded-lg bg-surface-2 px-2 py-1 text-[12px] font-medium text-ink opacity-0 shadow-raised ring-1 ring-border/70 transition-opacity duration-100 group-hover:opacity-100 group-hover:delay-150 group-focus-visible:opacity-100"
    >
      {children}
    </span>
  )
}

export function NavRail({ tab, onOpenSettings }: { tab: RailTab; onOpenSettings: () => void }) {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const sections = useRailSections()
  const botsUnread = useQuery({
    ...botsQuery,
    refetchInterval: 15_000,
    select: (bots) => bots.some((bot) => bot.unread),
  }).data

  // The Chat tab returns to the thread the user left, the way switching apps
  // does; a fresh /new clears it so the tab lands back on the composer.
  const [lastSession, setLastSession] = useState<string>()
  const session = /^\/sessions\/([^/]+)/.exec(pathname)?.[1]
  const current = session ?? (pathname === '/new' ? undefined : lastSession)
  if (current !== lastSession) setLastSession(current)

  return (
    <nav aria-label="Sections" style={{ width: RAIL_WIDTH }} className="flex shrink-0 flex-col items-center gap-2 pb-2 pt-[5px] max-sm:hidden">
      <Link
        to={lastSession ? '/sessions/$sessionId' : '/new'}
        params={lastSession ? { sessionId: lastSession } : {}}
        aria-label="Chat"
        className={tabClass(tab === 'chat')}
      >
        <MessageSquare aria-hidden />
        <TabLabel>Chat</TabLabel>
      </Link>
      {sections.map((section) => (
        <Link
          key={section.path}
          {...section.link}
          aria-label={section.path === '/bots' && botsUnread ? 'Bots (unread messages)' : section.label}
          className={tabClass(tab === section.path)}
        >
          {section.icon}
          {section.path === '/bots' && botsUnread && (
            <span aria-hidden className="absolute right-1 top-1 size-1.5 rounded-full bg-primary" />
          )}
          <TabLabel>{section.label}</TabLabel>
        </Link>
      ))}
      <button
        type="button"
        onClick={onOpenSettings}
        aria-label="Settings"
        className={`${tabClass(tab === 'settings')} mt-auto`}
      >
        <Settings aria-hidden />
        <TabLabel>Settings</TabLabel>
      </button>
    </nav>
  )
}
