import { useCallback, useEffect, useState } from 'react'
import { api, type Integration } from '@/lib/api'

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : 'Не удалось загрузить интеграции'
}

export function useIntegrations() {
  const [integrations, setIntegrations] = useState<Integration[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const reload = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const next = await api.getIntegrations()
      setIntegrations(next)
      return next
    } catch (err) {
      setError(errorMessage(err))
      throw err
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void reload().catch(console.error)
  }, [reload])

  return { integrations, loading, error, reload }
}
