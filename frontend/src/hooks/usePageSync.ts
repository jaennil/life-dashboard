import { useCallback, useState } from 'react'
import { api } from '@/lib/api'

export function usePageSync(onSynced?: () => Promise<void> | void) {
  const [syncing, setSyncing] = useState(false)

  const syncSources = useCallback(async (sources: string | string[]) => {
    const names = Array.isArray(sources) ? sources : [sources]
    const enabledNames = names.filter(Boolean)
    if (enabledNames.length === 0) return

    setSyncing(true)
    try {
      for (const name of enabledNames) {
        await api.syncIntegration(name)
      }
      await onSynced?.()
    } finally {
      setSyncing(false)
    }
  }, [onSynced])

  return { syncing, syncSources }
}
