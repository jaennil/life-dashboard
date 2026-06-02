import { useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import type { DateRangeParams } from '@/lib/api'

export function useGlobalDateRange() {
  const [searchParams] = useSearchParams()
  const from = searchParams.get('from') ?? ''
  const to = searchParams.get('to') ?? ''

  return useMemo(() => {
    const params: DateRangeParams = {}
    if (from) params.from = from
    if (to) params.to = to

    return {
      from,
      to,
      params,
      isActive: Boolean(from || to),
      targetDate: to || from || undefined,
      label: from || to ? `${from || 'начало'} — ${to || 'сегодня'}` : undefined,
    }
  }, [from, to])
}
