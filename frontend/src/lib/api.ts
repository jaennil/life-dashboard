const BASE = '/api/v1'

export interface DashboardSummary {
  finance: {
    total_balance: number
    currency: string
    monthly_spending: number
    monthly_income: number
  }
  fitness: {
    activities_this_week: number
    workouts_this_week: number
    total_distance_km: number
  }
}

export interface Transaction {
  id: string
  occurred_at: string
  amount: number
  currency: string
  comment: string
  payee: string | null
  is_transfer: boolean
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(BASE + path)
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.json() as Promise<T>
}

export const api = {
  getDashboardSummary: () => get<DashboardSummary>('/dashboard/summary'),
  getRecentTransactions: () => get<Transaction[]>('/dashboard/transactions'),
}
