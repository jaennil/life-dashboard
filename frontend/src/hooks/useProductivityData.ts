import { useCallback, useEffect, useState } from 'react'
import {
  api,
  type DateRangeParams,
  type ProductivityHabitsResponse,
  type ProductivitySummary,
  type ProductivityTask,
} from '@/lib/api'

type TaskFilter = 'all' | 'overdue' | 'today' | 'upcoming' | 'stale'

export function useProductivityData(params: DateRangeParams, targetDate: string | undefined, filter: TaskFilter) {
  const [summary, setSummary] = useState<ProductivitySummary | null>(null)
  const [tasks, setTasks] = useState<ProductivityTask[]>([])
  const [habitsData, setHabitsData] = useState<ProductivityHabitsResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [taskLoading, setTaskLoading] = useState(true)
  const [habitsLoading, setHabitsLoading] = useState(true)

  const reloadSummary = useCallback(async () => {
    try {
      setSummary(await api.getProductivitySummary(params))
    } catch (error) {
      console.error(error)
    }
  }, [params])

  const reloadTasks = useCallback(async (currentFilter: TaskFilter = filter) => {
    setTaskLoading(true)
    try {
      setTasks(await api.getProductivityTasks(currentFilter, params))
    } catch (error) {
      console.error(error)
    } finally {
      setTaskLoading(false)
    }
  }, [filter, params])

  const reloadHabits = useCallback(async () => {
    setHabitsLoading(true)
    try {
      setHabitsData(await api.getProductivityHabits(targetDate))
    } catch (error) {
      console.error(error)
    } finally {
      setHabitsLoading(false)
    }
  }, [targetDate])

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      await Promise.all([reloadSummary(), reloadTasks(filter), reloadHabits()])
    } finally {
      setLoading(false)
    }
  }, [filter, reloadHabits, reloadSummary, reloadTasks])

  useEffect(() => {
    void reload()
  }, [reload])

  return {
    summary,
    tasks,
    habitsData,
    loading,
    taskLoading,
    habitsLoading,
    reload,
    reloadHabits,
  }
}
