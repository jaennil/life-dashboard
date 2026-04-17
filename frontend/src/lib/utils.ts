import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function formatLastSyncAt(iso: string | null | undefined) {
  if (!iso) return 'никогда'

  const date = new Date(iso)
  if (!Number.isFinite(date.getTime())) return 'никогда'

  const now = new Date()
  const options: Intl.DateTimeFormatOptions = {
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  }
  if (date.getFullYear() !== now.getFullYear()) {
    options.year = 'numeric'
  }

  return date.toLocaleString('ru-RU', options).replace(',', '')
}

export function oldestLastSyncAt(items: Array<{ last_sync_at: string | null }>) {
  if (items.length === 0) return undefined

  let oldest = Number.POSITIVE_INFINITY
  for (const item of items) {
    if (!item.last_sync_at) return null

    const ts = Date.parse(item.last_sync_at)
    if (!Number.isFinite(ts)) return null
    oldest = Math.min(oldest, ts)
  }

  return Number.isFinite(oldest) ? new Date(oldest).toISOString() : null
}

export function syncCaptionForSources(items: Array<{ last_sync_at: string | null }>) {
  const oldest = oldestLastSyncAt(items)
  if (oldest === undefined) return undefined

  const prefix = items.length > 1 ? 'Старейшая синхр.' : 'Синхр.'
  return `${prefix}: ${formatLastSyncAt(oldest)}`
}
