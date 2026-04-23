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
  nutrition: {
    today_kcal: number
    today_water_ml: number
    avg_calories: number
    days_tracked: number
    target_calories?: number
    target_water_ml?: number
  }
  productivity: {
    active_total: number
    overdue_total: number
    due_today_total: number
    completed_today_total: number
    habits_total: number
    habits_completed_today: number
    habits_pending_today: number
  }
  checkup: AILatestCheckup
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

export interface FinanceObligation {
  key: string
  name: string
  category?: string
  amount: number
  projected_total: number
  next_due_at: string
  cadence_days: number
  cadence_label: string
  occurrences: number
  expected_occurrences: number
  rule_action?: 'ignore' | 'force'
}

export interface FinanceObligationRule {
  key: string
  label: string
  action: 'ignore' | 'force'
  created_at: string
  updated_at: string
}

export interface FinanceObligationsSummary {
  window_days: number
  upcoming_total: number
  count: number
  items: FinanceObligation[]
  rules: FinanceObligationRule[]
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
  avg_water_ml: number
  days_tracked: number
  today_kcal: number
  today_water_ml: number
  targets?: NutritionTargets
}

export interface NutritionTargets {
  source: string
  current_weight_kg?: number
  current_weight_date?: string
  current_weight_comment?: string
  target_weight_kg?: number
  height_cm?: number
  target_calories?: number
  target_protein_g?: number
  target_carbs_g?: number
  target_fat_g?: number
  target_water_ml?: number
  weight_measure?: string
  height_measure?: string
  api_notes?: string[]
  synced_at?: string
  manual?: NutritionManualTargets
}

export interface NutritionManualTargets {
  target_weight_kg?: number
  target_calories?: number
  target_protein_g?: number
  target_carbs_g?: number
  target_fat_g?: number
  target_water_ml?: number
  updated_at?: string
}

export interface NutritionTargetsInput {
  target_weight_kg?: number | null
  target_calories?: number | null
  target_protein_g?: number | null
  target_carbs_g?: number | null
  target_fat_g?: number | null
  target_water_ml?: number | null
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
  water_ml: number
  meals: NutritionMeal[]
}

export interface NutritionWaterState {
  date: string
  water_ml: number
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
  sync_completed_at?: string
  sync_error?: string
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

export interface ProductivityHabitsSummary {
  total: number
  completed_today: number
  pending_today: number
  morning_pending: number
  evening_pending: number
  anytime_pending: number
  completion_rate_7_days: number
}

export interface ProductivityHabit {
  id: string
  name: string
  area_name: string
  routine: 'morning' | 'evening' | 'anytime'
  status: 'completed' | 'skipped' | 'failed' | 'none'
  completed_7_days: number
  current_streak: number
  last_completed_at: string | null
}

export interface ProductivityHabitsResponse {
  date: string
  summary: ProductivityHabitsSummary
  habits: ProductivityHabit[]
}

export interface ProductivityHabitInput {
  name: string
  routine: 'morning' | 'evening' | 'anytime'
  area_name?: string
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
  getProductivityHabits: () => get<ProductivityHabitsResponse>('/productivity/habits'),
  createProductivityHabit: (input: ProductivityHabitInput) =>
    fetch(BASE + '/productivity/habits', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    }).then(async r => {
      if (!r.ok) throw new Error(await r.text())
    }),
  updateProductivityHabit: (id: string, input: ProductivityHabitInput) =>
    fetch(BASE + `/productivity/habits/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    }).then(async r => {
      if (!r.ok) throw new Error(await r.text())
    }),
  deleteProductivityHabit: (id: string) =>
    fetch(BASE + `/productivity/habits/${id}`, {
      method: 'DELETE',
    }).then(async r => {
      if (!r.ok) throw new Error(await r.text())
    }),
  setProductivityHabitStatus: (id: string, status: 'completed' | 'none' | 'skipped' | 'failed', date?: string) =>
    fetch(BASE + `/productivity/habits/${id}/status`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ status, date }),
    }).then(async r => {
      if (!r.ok) throw new Error(await r.text())
    }),
  getSpendingByCategory: (from?: string, to?: string) => {
    const p = new URLSearchParams()
    if (from) p.set('from', from)
    if (to) p.set('to', to)
    return get<CategoryStat[]>('/finance/categories?' + p.toString())
  },
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
  getFinanceObligations: (days = 30) =>
    get<FinanceObligationsSummary>('/finance/obligations?days=' + encodeURIComponent(String(days))),
  saveFinanceObligationRule: (input: { key?: string; label: string; action: 'ignore' | 'force' }) =>
    fetch(BASE + '/finance/obligation-rules', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    }).then(async r => {
      if (!r.ok) throw new Error(await r.text())
    }),
  deleteFinanceObligationRule: (key: string) =>
    fetch(BASE + '/finance/obligation-rules/' + encodeURIComponent(key), {
      method: 'DELETE',
    }).then(async r => {
      if (!r.ok) throw new Error(await r.text())
    }),
  getCategoryList: () => get<string[]>('/finance/category-list'),
  getNutritionSummary: () => get<NutritionSummary>('/nutrition/summary'),
  getNutritionDaily: (days = 14) => get<NutritionDay[]>(`/nutrition/daily?days=${days}`),
  saveNutritionTargets: (input: NutritionTargetsInput) =>
    fetch(BASE + '/nutrition/targets', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    }).then(async r => {
      if (!r.ok) throw new Error(await r.text())
      return r.json() as Promise<NutritionTargets | null>
    }),
  saveNutritionWater: (input: { date?: string; delta_ml?: number; water_ml?: number }) =>
    fetch(BASE + '/nutrition/water', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    }).then(async r => {
      if (!r.ok) throw new Error(await r.text())
      return r.json() as Promise<NutritionWaterState>
    }),
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
      if (!r.ok) throw new Error(await r.text())
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
