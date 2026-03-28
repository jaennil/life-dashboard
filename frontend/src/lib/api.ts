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

export interface MonthStat {
  month: string
  spending: number
  income: number
}

export interface Account {
  id: string
  title: string
  type: string
  currency: string
  balance: number
}

export interface FinanceTransaction {
  id: string
  occurred_at: string
  amount: number
  currency: string
  comment: string
  payee: string | null
}

export interface FitnessSummary {
  activities_this_week: number
  workouts_this_week: number
  distance_km_this_week: number
  activities_total: number
  total_distance_km: number
}

export interface WeekStat {
  week: string
  count: number
  km: number
}

export interface Activity {
  id: string
  type: string
  name: string
  started_at: string
  duration_seconds: number | null
  distance_meters: number | null
  calories: number | null
  avg_heart_rate: number | null
}

export interface WorkoutSet {
  exercise_name: string
  exercise_category: string
  set_index: number
  set_type: string
  weight_kg: number | null
  reps: number | null
}

export interface WorkoutExercise {
  name: string
  category: string
  sets: WorkoutSet[]
}

export interface Workout {
  id: string
  title: string
  started_at: string
  ended_at: string | null
  exercises: WorkoutExercise[]
}

export interface HourlyPoint {
  time: string
  temp: number
  weather_code: number
}

export interface DailyPoint {
  date: string
  temp_max: number
  temp_min: number
  weather_code: number
}

export interface WeatherData {
  city: string
  temp: number
  feels_like: number
  weather_code: number
  description: string
  humidity: number
  wind_speed: number
  hourly: HourlyPoint[]
  daily: DailyPoint[]
}

export interface NutritionSummary {
  avg_calories: number
  avg_protein: number
  avg_carbs: number
  avg_fat: number
  days_tracked: number
  today_kcal: number
}

export interface NutritionMealItem {
  food_name: string
  serving: string
  calories: number
}

export interface NutritionMeal {
  meal_type: string
  items: NutritionMealItem[]
}

export interface NutritionDay {
  date: string
  calories: number
  protein: number
  carbs: number
  fat: number
  fiber: number
  meals: NutritionMeal[]
}

export interface Integration {
  name: string
  display_name: string
  description: string
  configured: boolean
  enabled: boolean
  last_sync_at: string | null
  record_count: number
}

export const api = {
  getDashboardSummary: () => get<DashboardSummary>('/dashboard/summary'),
  getRecentTransactions: () => get<Transaction[]>('/dashboard/transactions'),
  getMonthlyStats: () => get<MonthStat[]>('/finance/monthly'),
  getAccounts: () => get<Account[]>('/finance/accounts'),
  getTransactions: (page = 1, type = '') =>
    get<FinanceTransaction[]>(`/finance/transactions?page=${page}&type=${type}`),
  getFitnessSummary: () => get<FitnessSummary>('/fitness/summary'),
  getFitnessWeekly: () => get<WeekStat[]>('/fitness/weekly'),
  getActivities: () => get<Activity[]>('/fitness/activities'),
  getWorkouts: () => get<Workout[]>('/fitness/workouts'),
  getNutritionSummary: () => get<NutritionSummary>('/nutrition/summary'),
  getNutritionDaily: () => get<NutritionDay[]>('/nutrition/daily'),
  getIntegrations: () => get<Integration[]>('/integrations'),
  toggleIntegration: (name: string, enabled: boolean) =>
    fetch(BASE + `/integrations/${name}/toggle`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled }),
    }).then(r => { if (!r.ok) throw new Error(r.statusText) }),
  syncIntegration: (name: string) =>
    fetch(BASE + `/sync/${name}`, { method: 'POST' })
      .then(r => { if (!r.ok) throw new Error(r.statusText) }),
  getWeather: (lat?: number, lon?: number, city?: string) => {
    const params = new URLSearchParams()
    if (lat != null) params.set('lat', String(lat))
    if (lon != null) params.set('lon', String(lon))
    if (city) params.set('city', city)
    const qs = params.toString()
    return get<WeatherData>('/weather' + (qs ? '?' + qs : ''))
  },
}
