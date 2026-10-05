import { Globe } from 'lucide-react'
import { memo, useState } from 'react'

export const Favicon = memo(function Favicon({
  url,
  iconUrl,
  className = 'size-3.5 shrink-0 text-ink-3',
}: {
  url: string
  iconUrl?: string
  className?: string
}) {
  const [failedSources, setFailedSources] = useState<string[]>([])
  let site: URL | undefined
  try {
    const parsed = new URL(url)
    if (parsed.protocol === 'https:' || parsed.protocol === 'http:') {
      site = parsed
    }
  } catch {
    site = undefined
  }
  const source = [
    iconUrl,
    site && `${site.origin}/favicon.ico`,
    site && `${site.origin}/favicon.svg`,
    site && `https://www.google.com/s2/favicons?domain=${encodeURIComponent(site.hostname)}&sz=64`,
  ].find((candidate) => candidate && !failedSources.includes(candidate))
  if (!source) {
    return <Globe size={14} className={className} aria-hidden />
  }
  return (
    <img
      src={source}
      alt=""
      width={14}
      height={14}
      loading="lazy"
      draggable={false}
      onError={() => setFailedSources((previous) => [...previous, source])}
      className={`${className} rounded-sm outline outline-1 outline-black/10 dark:outline-white/10`}
    />
  )
})
