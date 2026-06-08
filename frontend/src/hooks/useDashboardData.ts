import { useCallback, useEffect, useState } from 'react'
import { api, type DashboardSummary, type DateRangeParams, type Transaction } from '@/lib/api'

export function useDashboardData(params: DateRangeParams) {
  const [summary, setSummary] = useState<DashboardSummary | null>(null)
  const [txs, setTxs] = useState<Transaction[]>([])
  const [loading, setLoading] = useState(true)

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      const [nextSummary, nextTransactions] = await Promise.all([
        api.getDashboardSummary(params),
        api.getRecentTransactions(params),
      ])
      setSummary(nextSummary)
      setTxs(nextTransactions)
    } catch (error) {
      console.error(error)
    } finally {
      setLoading(false)
    }
  }, [params])

  useEffect(() => {
    void reload()
  }, [reload])

  return { summary, txs, loading, reload }
}
