import { useCallback, useEffect, useState } from 'react'
import { api, type FinanceTransaction } from '@/lib/api'

interface FinanceTransactionFilters {
  filter: string
  sort: string
  search: string
  category: string
  from: string
  to?: string
}

const PAGE_SIZE = 30

export function useFinanceTransactions({
  filter,
  sort,
  search,
  category,
  from,
  to,
}: FinanceTransactionFilters) {
  const [txs, setTxs] = useState<FinanceTransaction[]>([])
  const [page, setPage] = useState(1)
  const [hasMore, setHasMore] = useState(true)
  const [loading, setLoading] = useState(false)

  const loadPage = useCallback(async (nextPage: number, replace: boolean) => {
    setLoading(true)
    try {
      const data = await api.getTransactions({
        page: nextPage,
        type: filter,
        sort,
        search,
        category,
        from,
        to,
      })
      setTxs(current => replace ? data : [...current, ...data])
      setHasMore(data.length === PAGE_SIZE)
    } catch {
      // Keep the current transaction list when a paged fetch fails.
    } finally {
      setLoading(false)
    }
  }, [category, filter, from, search, sort, to])

  useEffect(() => {
    setPage(1)
    const timeout = setTimeout(() => {
      void loadPage(1, true)
    }, search ? 300 : 0)

    return () => clearTimeout(timeout)
  }, [category, filter, loadPage, search, sort])

  const loadMore = useCallback(() => {
    const nextPage = page + 1
    setPage(nextPage)
    void loadPage(nextPage, false)
  }, [loadPage, page])

  return {
    txs,
    loading,
    hasMore,
    loadMore,
  }
}
