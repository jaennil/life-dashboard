import { useCallback, useEffect, useState } from 'react'
import {
  api,
  type Activity,
  type DateRangeParams,
  type FitnessGoldenMetrics,
  type FitnessSummary,
  type Workout,
} from '@/lib/api'

export function useFitnessData(params: DateRangeParams) {
  const [summary, setSummary] = useState<FitnessSummary | null>(null)
  const [golden, setGolden] = useState<FitnessGoldenMetrics | null>(null)
  const [activities, setActivities] = useState<Activity[]>([])
  const [workouts, setWorkouts] = useState<Workout[]>([])
  const [loading, setLoading] = useState(true)

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      const [nextSummary, nextGolden, nextActivities, nextWorkouts] = await Promise.all([
        api.getFitnessSummary(params),
        api.getFitnessGolden(params),
        api.getActivities(params),
        api.getWorkouts(params),
      ])
      setSummary(nextSummary)
      setGolden(nextGolden)
      setActivities(nextActivities)
      setWorkouts(nextWorkouts)
    } catch (error) {
      console.error(error)
    } finally {
      setLoading(false)
    }
  }, [params])

  useEffect(() => {
    void reload()
  }, [reload])

  return { summary, golden, activities, workouts, loading, reload }
}
