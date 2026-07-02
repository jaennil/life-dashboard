import { useCallback, useEffect, useState } from 'react'
import {
  api,
  type Account,
  type CategoryStat,
  type DailyTotal,
  type FinanceObligationsSummary,
  type MonthStat,
  type TopExpense,
} from '@/lib/api'

export function useFinanceOverview(from: string, to?: string) {
  const [monthly, setMonthly] = useState<MonthStat[]>([])
  const [accounts, setAccounts] = useState<Account[]>([])
  const [categories, setCategories] = useState<CategoryStat[]>([])
  const [incomeCategories, setIncomeCategories] = useState<CategoryStat[]>([])
  const [daily, setDaily] = useState<DailyTotal[]>([])
  const [topExpenses, setTopExpenses] = useState<TopExpense[]>([])
  const [obligations, setObligations] = useState<FinanceObligationsSummary | null>(null)
  const [categoryList, setCategoryList] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const reload = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const [
        nextMonthly,
        nextAccounts,
        nextCategories,
        nextIncomeCategories,
        nextDaily,
        nextTopExpenses,
        nextObligations,
        nextCategoryList,
      ] = await Promise.all([
        api.getMonthlyStats(),
        api.getAccounts(),
        api.getSpendingByCategory(from, to),
        api.getIncomeByCategory(from, to),
        api.getDailyTotals(from, to),
        api.getTopExpenses(from, to),
        api.getFinanceObligations(30),
        api.getCategoryList(),
      ])

      setMonthly(nextMonthly)
      setAccounts(nextAccounts)
      setCategories(nextCategories)
      setIncomeCategories(nextIncomeCategories)
      setDaily(nextDaily)
      setTopExpenses(nextTopExpenses)
      setObligations(nextObligations)
      setCategoryList(nextCategoryList)
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Не удалось загрузить финансовые данные')
    } finally {
      setLoading(false)
    }
  }, [from, to])

  useEffect(() => {
    void reload()
  }, [reload])

  return {
    monthly,
    accounts,
    categories,
    incomeCategories,
    daily,
    topExpenses,
    obligations,
    categoryList,
    loading,
    error,
    reload,
  }
}
