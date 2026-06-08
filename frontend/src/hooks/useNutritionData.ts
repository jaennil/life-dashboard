import { useCallback, useEffect, useState } from 'react'
import {
  api,
  type DateRangeParams,
  type NutritionDay,
  type NutritionGoldenMetrics,
  type NutritionSummary,
} from '@/lib/api'

export function useNutritionData(params: DateRangeParams, period: number) {
  const [summary, setSummary] = useState<NutritionSummary | null>(null)
  const [golden, setGolden] = useState<NutritionGoldenMetrics | null>(null)
  const [daily, setDaily] = useState<NutritionDay[]>([])
  const [loading, setLoading] = useState(true)

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      const [nextSummary, nextGolden, nextDaily] = await Promise.all([
        api.getNutritionSummary(params),
        api.getNutritionGolden(period, params),
        api.getNutritionDaily(period, params),
      ])
      setSummary(nextSummary)
      setGolden(nextGolden)
      setDaily(nextDaily)
    } catch (error) {
      console.error(error)
    } finally {
      setLoading(false)
    }
  }, [params, period])

  useEffect(() => {
    void reload()
  }, [reload])

  return { summary, golden, daily, loading, reload }
}
