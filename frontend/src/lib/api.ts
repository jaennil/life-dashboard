const BASE = '/api/v1'

export interface User {
  id: string
  username: string
  totp_enabled: boolean
}

export interface LoginResult {
  needs_totp?: boolean
  user?: User
}

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

export interface DailyTotal {
  date: string
  spending: number
  income: number
}

export interface TopExpense {
  payee: string
  amount: number
  count: number
}

export interface Account {
  id: string
  title: string
  type: string
  currency: string
  balance: number
  in_balance: boolean
  company_id: number | null
  company_title: string | null
}

export interface FinanceTransaction {
  id: string
  occurred_at: string
  amount: number
  currency: string
  comment: string
  payee: string | null
  category: string | null
}

export interface FitnessSummary {
  activities_this_week: number
  workouts_this_week: number
  distance_km_this_week: number
  activities_total: number
  workouts_total: number
  total_distance_km: number
}

export interface WeekStat {
  week: string
  activities_count: number
  workouts_count: number
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
  exercise_index: number
  set_index: number
  set_type: string
  weight_kg: number | null
  reps: number | null
  distance_meters: number | null
  duration_seconds: number | null
  rpe: number | null
}

export interface WorkoutExercise {
  index: number
  name: string
  category: string
  notes: string
  template_id: string
  sets: WorkoutSet[]
}

export interface Workout {
  id: string
  source: string
  title: string
  notes: string
  started_at: string
  ended_at: string | null
  source_created_at: string | null
  source_updated_at: string | null
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

export interface CategoryStat {
  category: string
  amount: number
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
  macros?: Record<string, number>
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
  oauth_configured: boolean
  enabled: boolean
  has_credentials: boolean
  last_sync_at: string | null
  record_count: number
}

export interface ConnectionResult {
  status: string
  source?: string
  sync_started_at?: string
}

export interface AIHistoryMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  created_at: string
}

export interface AILatestCheckup {
  has_report: boolean
  period?: string
  period_label?: string
  generated_at?: string
}

export interface ProductivityDayBucket {
  date: string
  count: number
}

export interface ProductivitySummary {
  active_total: number
  overdue_total: number
  due_today_total: number
  due_next_7_days_total: number
  recurring_total: number
  stale_total: number
  completed_today_total: number
  completed_7_days_total: number
  upcoming_load: ProductivityDayBucket[]
}

export interface ProductivityTask {
  id: string
  external_id: string
  content: string
  description: string
  project_name: string
  section_name: string
  priority: number
  is_recurring: boolean
  added_at: string | null
  due_at: string | null
  due_date: string | null
  last_completed_at: string | null
  is_overdue: boolean
  due_bucket: 'overdue' | 'today' | 'upcoming' | 'later' | 'no_due'
}

export interface HealthAPIKeyInfo {
  api_key: string
  webhook_url: string
  last_sync_at: string | null
  metric_count: number
  sleep_count: number
}

export const api = {
  getDashboardSummary: () => get<DashboardSummary>('/dashboard/summary'),
  getRecentTransactions: () => get<Transaction[]>('/dashboard/transactions'),
  getMonthlyStats: () => get<MonthStat[]>('/finance/monthly'),
  getAccounts: () => get<Account[]>('/finance/accounts'),
  getTransactions: (params: { page?: number; type?: string; category?: string; search?: string; from?: string; to?: string; sort?: string } = {}) => {
    const p = new URLSearchParams()
    if (params.page) p.set('page', String(params.page))
    if (params.type) p.set('type', params.type)
    if (params.category) p.set('category', params.category)
    if (params.search) p.set('search', params.search)
    if (params.from) p.set('from', params.from)
    if (params.to) p.set('to', params.to)
    if (params.sort) p.set('sort', params.sort)
    return get<FinanceTransaction[]>('/finance/transactions?' + p.toString())
  },
  getFitnessSummary: () => get<FitnessSummary>('/fitness/summary'),
  getFitnessWeekly: () => get<WeekStat[]>('/fitness/weekly'),
  getActivities: () => get<Activity[]>('/fitness/activities'),
  getWorkouts: () => get<Workout[]>('/fitness/workouts'),
  getProductivitySummary: () => get<ProductivitySummary>('/productivity/summary'),
  getProductivityTasks: (filter: 'all' | 'overdue' | 'today' | 'upcoming' | 'stale' = 'all') =>
    get<ProductivityTask[]>('/productivity/tasks?filter=' + encodeURIComponent(filter)),
  getSpendingByCategory: (from?: string) => get<CategoryStat[]>('/finance/categories' + (from ? '?from=' + from : '')),
  getDailyTotals: (from?: string, to?: string) => {
    const p = new URLSearchParams()
    if (from) p.set('from', from)
    if (to) p.set('to', to)
    return get<DailyTotal[]>('/finance/daily?' + p.toString())
  },
  getTopExpenses: (from?: string, to?: string) => {
    const p = new URLSearchParams()
    if (from) p.set('from', from)
    if (to) p.set('to', to)
    return get<TopExpense[]>('/finance/top-expenses?' + p.toString())
  },
  getCategoryList: () => get<string[]>('/finance/category-list'),
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
  getAIHistory: () => get<AIHistoryMessage[]>('/ai/history'),
  getLatestAICheckup: () => get<AILatestCheckup>('/ai/checkup/latest'),
  clearAIHistory: () =>
    fetch(BASE + '/ai/history', { method: 'DELETE' }).then(async r => {
      if (!r.ok) throw new Error(await r.text())
    }),
  saveToken: (name: string, token: string, extra?: Record<string, string>) =>
    fetch(BASE + `/integrations/${name}/token`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token, ...extra }),
    }).then(async r => {
      if (!r.ok) throw new Error(r.statusText)
      return r.json() as Promise<ConnectionResult>
    }),
  me: () => get<User>('/auth/me'),
  register: (username: string, password: string) =>
    fetch(BASE + '/auth/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    }).then(async r => {
      if (!r.ok) throw new Error(await r.text())
      return r.json() as Promise<User>
    }),
  login: (username: string, password: string, totp_code?: string) =>
    fetch(BASE + '/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password, totp_code: totp_code ?? '' }),
    }).then(async r => {
      if (!r.ok) throw new Error(await r.text())
      return r.json() as Promise<LoginResult>
    }),
  logout: () =>
    fetch(BASE + '/auth/logout', { method: 'POST' }).then(r => {
      if (!r.ok) throw new Error(r.statusText)
    }),
  totpSetup: () => get<{ secret: string; qr: string }>('/auth/totp/setup'),
  totpEnable: (code: string) =>
    fetch(BASE + '/auth/totp/enable', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ code }),
    }).then(async r => { if (!r.ok) throw new Error(await r.text()) }),
  totpDisable: (code: string) =>
    fetch(BASE + '/auth/totp/disable', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ code }),
    }).then(async r => { if (!r.ok) throw new Error(await r.text()) }),
  getAPIKey: () => get<HealthAPIKeyInfo>('/health/apikey'),
  generateAPIKey: () =>
    fetch(BASE + '/health/apikey', { method: 'POST' })
      .then(async r => {
        if (!r.ok) throw new Error(r.statusText)
        return r.json() as Promise<HealthAPIKeyInfo>
      }),
  getWeather: (lat?: number, lon?: number, city?: string) => {
    const params = new URLSearchParams()
    if (lat != null) params.set('lat', String(lat))
    if (lon != null) params.set('lon', String(lon))
    if (city) params.set('city', city)
    const qs = params.toString()
    return get<WeatherData>('/weather' + (qs ? '?' + qs : ''))
  },
}
