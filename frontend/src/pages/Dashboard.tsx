import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { Wallet, Dumbbell, TrendingUp, TrendingDown, Zap, Route, Droplets, Wind, MapPin, LocateFixed, Search, X, ListTodo, Bot, UtensilsCrossed, ChevronRight } from 'lucide-react'
import { PageSyncButton } from '@/components/PageSyncButton'
import { PageHeader } from '@/components/PageHeader'
import { useGlobalDateRange } from '@/hooks/useGlobalDateRange'
import { cn, syncCaptionForSources } from '@/lib/utils'
import { api, type DashboardSummary, type Transaction, type WeatherData, type Integration } from '@/lib/api'

const LOC_KEY = 'weather_location'

interface SavedLocation { lat: number; lon: number; city: string }

function loadLocation(): SavedLocation | null {
  try { return JSON.parse(localStorage.getItem(LOC_KEY) || 'null') } catch { return null }
}
function saveLocation(loc: SavedLocation) {
  localStorage.setItem(LOC_KEY, JSON.stringify(loc))
}

function StatCard({
  title,
  value,
  sub,
  icon: Icon,
  trend,
  color,
  loading,
}: {
  title: string
  value: string
  sub: string
  icon: React.ElementType
  trend?: 'up' | 'down'
  color: string
  loading?: boolean
}) {
  return (
    <div className="rounded-2xl border bg-card/90 p-5 shadow-sm flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-muted-foreground">{title}</span>
        <div className={cn('flex items-center justify-center w-8 h-8 rounded-lg', color)}>
          <Icon className="w-4 h-4 text-white" />
        </div>
      </div>
      <div>
        {loading ? (
          <div className="h-8 w-24 bg-muted rounded animate-pulse" />
        ) : (
          <div className="text-2xl font-bold text-foreground">{value}</div>
        )}
        <div className="flex items-center gap-1 mt-1">
          {trend === 'up' && <TrendingUp className="w-3 h-3 text-emerald-500" />}
          {trend === 'down' && <TrendingDown className="w-3 h-3 text-red-500" />}
          <span className="text-xs text-muted-foreground">{sub}</span>
        </div>
      </div>
    </div>
  )
}

function InsightCard({ text, type }: { text: string; type: 'info' | 'warn' | 'good' }) {
  const colors = {
    info: 'border-blue-500/20 bg-blue-500/5',
    warn: 'border-amber-500/20 bg-amber-500/5',
    good: 'border-emerald-500/20 bg-emerald-500/5',
  }
  return (
    <div className={cn('rounded-2xl border border-white/5 px-4 py-3 text-sm text-foreground shadow-sm', colors[type])}>
      <Zap className="inline w-3 h-3 mr-1 opacity-70" />
      {text}
    </div>
  )
}

function wmoIcon(code: number): string {
  if (code === 0) return '☀️'
  if (code === 1) return '🌤️'
  if (code === 2) return '⛅'
  if (code === 3) return '☁️'
  if (code === 45 || code === 48) return '🌫️'
  if (code >= 51 && code <= 55) return '🌦️'
  if (code >= 61 && code <= 65) return '🌧️'
  if (code >= 71 && code <= 77) return '🌨️'
  if (code >= 80 && code <= 82) return '🌧️'
  if (code === 85 || code === 86) return '🌨️'
  if (code >= 95) return '⛈️'
  return '🌡️'
}

function fmtHour(iso: string) {
  return iso.slice(11, 16)
}

function fmtDayShort(iso: string) {
  return new Date(iso).toLocaleDateString('ru-RU', { weekday: 'short', day: 'numeric' })
}

interface GeoResult { name: string; country: string; admin1?: string; latitude: number; longitude: number }

function LocationPicker({ onSelect, onClose }: {
  onSelect: (loc: SavedLocation) => void
  onClose: () => void
}) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<GeoResult[]>([])
  const [searching, setSearching] = useState(false)
  const [geoErr, setGeoErr] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => { inputRef.current?.focus() }, [])

  useEffect(() => {
    if (!query.trim()) { setResults([]); return }
    const t = setTimeout(async () => {
      setSearching(true)
      try {
        const res = await fetch(`https://geocoding-api.open-meteo.com/v1/search?name=${encodeURIComponent(query)}&count=8&language=ru&format=json`)
        const data = await res.json()
        setResults(data.results ?? [])
      } catch { setResults([]) }
      finally { setSearching(false) }
    }, 400)
    return () => clearTimeout(t)
  }, [query])

  function useGeo() {
    setGeoErr('')
    navigator.geolocation.getCurrentPosition(
      pos => {
        const loc = { lat: pos.coords.latitude, lon: pos.coords.longitude, city: 'Моё местоположение' }
        onSelect(loc)
      },
      () => setGeoErr('Геолокация недоступна или запрещена')
    )
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div className="bg-card border rounded-2xl shadow-xl w-full max-w-sm mx-4 p-4 flex flex-col gap-3" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between">
          <span className="text-sm font-semibold text-foreground">Выбор города</span>
          <button onClick={onClose} className="text-muted-foreground hover:text-foreground"><X className="w-4 h-4" /></button>
        </div>

        <button
          onClick={useGeo}
          className="flex items-center gap-2 text-sm text-blue-500 hover:text-blue-400 transition-colors"
        >
          <LocateFixed className="w-4 h-4" /> Определить моё местоположение
        </button>
        {geoErr && <p className="text-xs text-rose-400">{geoErr}</p>}

        <div className="relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
          <input
            ref={inputRef}
            value={query}
            onChange={e => setQuery(e.target.value)}
            placeholder="Поиск города..."
            className="w-full pl-9 pr-3 py-2 text-sm bg-muted rounded-lg border-0 outline-none focus:ring-2 focus:ring-ring text-foreground placeholder:text-muted-foreground"
          />
          {searching && <div className="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />}
        </div>

        {results.length > 0 && (
          <div className="flex flex-col divide-y max-h-56 overflow-y-auto rounded-lg border">
            {results.map((r, i) => (
              <button
                key={i}
                onClick={() => onSelect({ lat: r.latitude, lon: r.longitude, city: r.name })}
                className="px-3 py-2 text-left hover:bg-muted/50 transition-colors"
              >
                <span className="text-sm text-foreground">{r.name}</span>
                <span className="text-xs text-muted-foreground ml-2">{[r.admin1, r.country].filter(Boolean).join(', ')}</span>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

function WeatherCard({ weather, loading, onPickLocation }: {
  weather: WeatherData | null
  loading: boolean
  onPickLocation: () => void
}) {
  if (loading) {
    return (
      <div className="rounded-2xl border bg-card/90 p-5 shadow-sm">
        <div className="h-5 w-24 bg-muted rounded animate-pulse mb-4" />
        <div className="h-12 w-32 bg-muted rounded animate-pulse" />
      </div>
    )
  }
  if (!weather) return null

  return (
    <div className="rounded-2xl border bg-card/90 p-5 shadow-sm flex min-w-0 flex-col gap-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <button
            onClick={onPickLocation}
            className="group flex min-w-0 items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-foreground"
          >
            <MapPin className="h-3 w-3 shrink-0" />
            <span className="truncate">{weather.city}</span>
          </button>
          <div className="mt-1 flex flex-wrap items-end gap-2">
            <span className="text-5xl font-bold text-foreground">{Math.round(weather.temp)}°</span>
            <span className="text-4xl mb-0.5">{wmoIcon(weather.weather_code)}</span>
          </div>
          <p className="mt-1 break-words text-sm text-muted-foreground">{weather.description}</p>
        </div>
        <div className="grid grid-cols-3 gap-2 rounded-xl bg-background/35 p-3 text-xs text-muted-foreground sm:mt-1 sm:min-w-[96px] sm:grid-cols-1 sm:bg-transparent sm:p-0">
          <span className="min-w-0 break-words">Ощущается {Math.round(weather.feels_like)}°</span>
          <span className="flex min-w-0 items-center gap-1"><Droplets className="h-3 w-3 shrink-0" />{weather.humidity}%</span>
          <span className="flex min-w-0 items-center gap-1"><Wind className="h-3 w-3 shrink-0" />{Math.round(weather.wind_speed)} км/ч</span>
        </div>
      </div>

      <div className="grid grid-cols-4 gap-2 rounded-xl border border-white/5 bg-background/20 p-3 sm:grid-cols-6 xl:grid-cols-12">
        {weather.hourly.slice(0, 12).map(h => (
          <div key={h.time} className="flex min-w-0 flex-col items-center gap-1 rounded-lg border border-white/5 bg-background/40 px-2 py-2 text-center">
            <span className="text-xs text-muted-foreground">{fmtHour(h.time)}</span>
            <span className="text-base">{wmoIcon(h.weather_code)}</span>
            <span className="text-xs font-medium text-foreground">{Math.round(h.temp)}°</span>
          </div>
        ))}
      </div>

      <div className="flex flex-col gap-1 border-t pt-3">
        {weather.daily.map(d => (
          <div key={d.date} className="flex items-center gap-3 text-sm">
            <span className="w-24 text-muted-foreground text-xs">{fmtDayShort(d.date)}</span>
            <span className="text-base">{wmoIcon(d.weather_code)}</span>
            <div className="flex-1 flex items-center justify-end gap-3">
              <span className="text-muted-foreground text-xs">{Math.round(d.temp_min)}°</span>
              <span className="font-medium text-foreground text-xs">{Math.round(d.temp_max)}°</span>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

function fmt(amount: number, currency: string) {
  return new Intl.NumberFormat('ru-RU', { style: 'currency', currency, maximumFractionDigits: 0 }).format(amount)
}

function fmtDate(iso: string) {
  return new Date(iso).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })
}

function fmtDateTime(iso?: string) {
  if (!iso) return '—'
  const parsed = new Date(iso)
  if (Number.isNaN(parsed.getTime())) return iso
  return new Intl.DateTimeFormat('ru-RU', {
    day: '2-digit',
    month: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(parsed)
}

function OverviewSectionCard({
  to,
  title,
  summary,
  icon: Icon,
  iconClassName,
  metrics,
  note,
  loading,
}: {
  to: string
  title: string
  summary: string
  icon: React.ElementType
  iconClassName: string
  metrics: Array<{ label: string; value: string }>
  note?: string
  loading?: boolean
}) {
  return (
    <Link
      to={to}
      className="group rounded-2xl border bg-card/90 p-5 shadow-sm transition-all hover:-translate-y-0.5 hover:border-primary/20 hover:bg-card"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <div className={cn('flex h-10 w-10 shrink-0 items-center justify-center rounded-xl', iconClassName)}>
            <Icon className="h-4 w-4 text-white" />
          </div>
          <div className="min-w-0">
            <p className="text-sm font-semibold text-foreground">{title}</p>
            <p className="mt-1 text-xs text-muted-foreground">{summary}</p>
          </div>
        </div>
        <ChevronRight className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5 group-hover:text-foreground" />
      </div>

      {loading ? (
        <div className="mt-4 grid grid-cols-2 gap-2">
          {Array.from({ length: 4 }).map((_, index) => (
            <div key={index} className="rounded-xl border bg-background/40 px-3 py-2">
              <div className="h-3 w-16 rounded bg-muted animate-pulse" />
              <div className="mt-2 h-5 w-20 rounded bg-muted animate-pulse" />
            </div>
          ))}
        </div>
      ) : (
        <div className="mt-4 grid grid-cols-2 gap-2">
          {metrics.map((metric) => (
            <div key={metric.label} className="rounded-xl border bg-background/40 px-3 py-2">
              <p className="text-[10px] uppercase tracking-wide text-muted-foreground/80">{metric.label}</p>
              <p className="mt-1 text-sm font-semibold text-foreground">{metric.value}</p>
            </div>
          ))}
        </div>
      )}

      {note ? (
        <p className="mt-4 text-xs text-muted-foreground">{note}</p>
      ) : null}
    </Link>
  )
}

export function Dashboard() {
  const globalRange = useGlobalDateRange()
  const [summary, setSummary] = useState<DashboardSummary | null>(null)
  const [txs, setTxs] = useState<Transaction[]>([])
  const [weather, setWeather] = useState<WeatherData | null>(null)
  const [loading, setLoading] = useState(true)
  const [weatherLoading, setWeatherLoading] = useState(true)
  const [showPicker, setShowPicker] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [integrations, setIntegrations] = useState<Integration[]>([])

  const loadDashboardData = useCallback(async () => {
    setLoading(true)
    try {
      const [s, t] = await Promise.all([
        api.getDashboardSummary(globalRange.params),
        api.getRecentTransactions(globalRange.params),
      ])
      setSummary(s)
      setTxs(t)
    } catch (error) {
      console.error(error)
    } finally {
      setLoading(false)
    }
  }, [globalRange.params])

  const loadIntegrations = useCallback(async () => {
    try {
      setIntegrations(await api.getIntegrations())
    } catch (error) {
      console.error(error)
    }
  }, [])

  function fetchWeather(loc?: SavedLocation) {
    setWeatherLoading(true)
    api.getWeather(loc?.lat, loc?.lon, loc?.city)
      .then(setWeather)
      .catch(console.error)
      .finally(() => setWeatherLoading(false))
  }

  function handleLocationSelect(loc: SavedLocation) {
    saveLocation(loc)
    setShowPicker(false)
    fetchWeather(loc)
  }

  useEffect(() => {
    void loadDashboardData()
    void loadIntegrations()

    const saved = loadLocation()
    fetchWeather(saved ?? undefined)

    if (!saved && navigator.geolocation) {
      navigator.geolocation.getCurrentPosition(
        pos => {
          const loc = { lat: pos.coords.latitude, lon: pos.coords.longitude, city: 'Моё местоположение' }
          saveLocation(loc)
          fetchWeather(loc)
        },
        () => {},
        { timeout: 5000 }
      )
    }
  }, [loadDashboardData, loadIntegrations])

  const enabledDashboardSources = integrations.filter(i =>
    (i.name === 'zenmoney' || i.name === 'strava' || i.name === 'hevy') && i.enabled
  )
  const dashboardSyncCaption = syncCaptionForSources(enabledDashboardSources)
  const periodText = globalRange.isActive ? 'за выбранный период' : 'за эту неделю'
  const todayText = globalRange.isActive ? 'на конец периода' : 'сегодня'

  async function handleSyncDashboard() {
    if (enabledDashboardSources.length === 0) return
    setSyncing(true)
    try {
      for (const integration of enabledDashboardSources) {
        await api.syncIntegration(integration.name)
      }
      await Promise.all([loadDashboardData(), loadIntegrations()])
    } catch (error) {
      console.error(error)
      throw error
    } finally {
      setSyncing(false)
    }
  }

  const insights: { text: string; type: 'info' | 'warn' | 'good' }[] = []
  if (summary) {
    if (summary.fitness.activities_this_week === 0 && summary.fitness.workouts_this_week === 0) {
      insights.push({ type: 'warn', text: globalRange.isActive ? 'В выбранном периоде нет активностей.' : 'На этой неделе нет активностей. Самое время размяться!' })
    }
    if (summary.finance.monthly_spending > summary.finance.monthly_income && summary.finance.monthly_income > 0) {
      insights.push({ type: 'warn', text: `Расходы (${fmt(summary.finance.monthly_spending, 'RUB')}) превышают доходы ${globalRange.isActive ? 'за выбранный период' : 'в этом месяце'}.` })
    }
    if (summary.fitness.total_distance_km > 0) {
      insights.push({ type: 'good', text: `${globalRange.isActive ? 'За выбранный период' : 'За эту неделю'} пройдено ${summary.fitness.total_distance_km.toFixed(1)} км.` })
    }
    if (summary.productivity.overdue_total > 0) {
      insights.push({ type: 'warn', text: `Есть overdue задачи: ${summary.productivity.overdue_total}. Лучше разобрать их до новых задач.` })
    }
    if (summary.productivity.habits_total > 0 && summary.productivity.habits_pending_today > 0) {
      insights.push({ type: 'info', text: `По рутинам ${todayText} осталось ${summary.productivity.habits_pending_today} незакрытых пунктов.` })
    }
    if (summary.nutrition.days_tracked === 0) {
      insights.push({ type: 'warn', text: 'По питанию за последние дни нет данных. Дневник питания сейчас слепой.' })
    } else if (summary.nutrition.target_calories && summary.nutrition.avg_calories > 0) {
      if (summary.nutrition.avg_calories < summary.nutrition.target_calories * 0.8) {
        insights.push({ type: 'warn', text: `Средние калории ${globalRange.isActive ? 'за период' : 'за 7 дней'} (${Math.round(summary.nutrition.avg_calories)}) заметно ниже цели (${Math.round(summary.nutrition.target_calories)}).` })
      } else if (summary.nutrition.avg_calories > summary.nutrition.target_calories * 1.15) {
        insights.push({ type: 'warn', text: `Средние калории ${globalRange.isActive ? 'за период' : 'за 7 дней'} (${Math.round(summary.nutrition.avg_calories)}) выше цели (${Math.round(summary.nutrition.target_calories)}).` })
      }
    }
    if (summary.nutrition.target_water_ml && summary.nutrition.today_hydration_ml < summary.nutrition.target_water_ml * 0.5) {
      insights.push({ type: 'info', text: `По гидратации сегодня ${Math.round(summary.nutrition.today_hydration_ml)} мл из цели ${Math.round(summary.nutrition.target_water_ml)} мл.` })
    }
    if (!summary.checkup.has_report) {
      insights.push({ type: 'info', text: 'AI-checkup ещё не запускался. Имеет смысл сделать первый обзор по всем сферам.' })
    } else if (summary.checkup.generated_at) {
      const ageDays = (Date.now() - new Date(summary.checkup.generated_at).getTime()) / (1000 * 60 * 60 * 24)
      if (ageDays >= 7) {
        insights.push({ type: 'info', text: `Последний AI-checkup был ${fmtDateTime(summary.checkup.generated_at)}. Пора обновить обзор.` })
      }
    }
  }
  if (insights.length === 0) {
    insights.push({ type: 'info', text: 'Подключи больше источников данных чтобы получать персональные инсайты.' })
  }

  const sectionCards = summary ? [
    {
      to: '/finance',
      title: 'Финансы',
      summary: 'Баланс, cashflow и текущий денежный темп.',
      icon: Wallet,
      iconClassName: 'bg-blue-500',
      metrics: [
        { label: 'Баланс', value: fmt(summary.finance.total_balance, 'RUB') },
        { label: 'Расходы', value: fmt(summary.finance.monthly_spending, 'RUB') },
        { label: 'Доходы', value: fmt(summary.finance.monthly_income, 'RUB') },
        { label: 'Результат', value: fmt(summary.finance.monthly_income - summary.finance.monthly_spending, 'RUB') },
      ],
      note: summary.finance.monthly_income > 0 && summary.finance.monthly_spending > summary.finance.monthly_income
        ? 'В этом месяце траты уже выше доходов.'
        : 'Финансовый поток под контролем.',
    },
    {
      to: '/fitness',
      title: 'Фитнес',
      summary: globalRange.isActive ? 'Активности и силовые тренировки за выбранный период.' : 'Активности и силовые тренировки за текущую неделю.',
      icon: Dumbbell,
      iconClassName: 'bg-violet-500',
      metrics: [
        { label: 'Активности', value: String(summary.fitness.activities_this_week) },
        { label: 'Тренировки', value: String(summary.fitness.workouts_this_week) },
        { label: 'Км', value: `${summary.fitness.total_distance_km.toFixed(1)} км` },
        { label: 'Статус', value: summary.fitness.activities_this_week + summary.fitness.workouts_this_week > 0 ? 'Есть движение' : globalRange.isActive ? 'Пустой период' : 'Пустая неделя' },
      ],
      note: summary.fitness.activities_this_week + summary.fitness.workouts_this_week > 0
        ? 'Можно быстро понять, где есть движение: кардио или силовые.'
        : globalRange.isActive ? 'Период пустой по нагрузке.' : 'Неделя пока пустая по нагрузке.',
    },
    {
      to: '/nutrition',
      title: 'Питание',
      summary: globalRange.isActive ? 'Калории, среднее за период и попадание в цель.' : 'Сегодняшние калории, среднее за неделю и попадание в цель.',
      icon: UtensilsCrossed,
      iconClassName: 'bg-emerald-500',
      metrics: [
        { label: 'Сегодня', value: `${Math.round(summary.nutrition.today_kcal)} ккал` },
        { label: globalRange.isActive ? 'Ср. период' : 'Ср. 7д', value: `${Math.round(summary.nutrition.avg_calories)} ккал` },
        { label: 'Цель', value: summary.nutrition.target_calories ? `${Math.round(summary.nutrition.target_calories)} ккал` : 'не задана' },
        { label: 'Гидратация', value: `${Math.round(summary.nutrition.today_hydration_ml)} мл` },
      ],
      note: summary.nutrition.days_tracked > 0
        ? `Дней с логами: ${summary.nutrition.days_tracked}${globalRange.isActive ? '' : ' из 7'}${summary.nutrition.target_water_ml ? ` · цель гидратации ${Math.round(summary.nutrition.target_water_ml)} мл` : ''}.`
        : 'Пока нет свежих данных по дневнику питания.',
    },
    {
      to: '/productivity',
      title: 'Продуктивность',
      summary: globalRange.isActive ? 'Задачи и ежедневные рутины в выбранном периоде.' : 'Overdue, задачи на сегодня и ежедневные рутины в одном месте.',
      icon: ListTodo,
      iconClassName: 'bg-amber-500',
      metrics: [
        { label: 'Overdue', value: String(summary.productivity.overdue_total) },
        { label: globalRange.isActive ? 'В периоде' : 'Сегодня', value: String(summary.productivity.due_today_total) },
        { label: 'Рутины', value: `${summary.productivity.habits_completed_today}/${summary.productivity.habits_total}` },
        { label: 'Закрыто', value: String(summary.productivity.completed_today_total) },
      ],
      note: summary.productivity.habits_total > 0
        ? `По рутинам осталось ${summary.productivity.habits_pending_today}.`
        : 'Доска показывает и задачи, и повседневные рутины.',
    },
    {
      to: '/ai',
      title: 'AI Checkup',
      summary: 'Последний обзор по всем сферам и точка входа в AI Chat.',
      icon: Bot,
      iconClassName: 'bg-cyan-500',
      metrics: [
        { label: 'Статус', value: summary.checkup.has_report ? 'Есть отчёт' : 'Не запускался' },
        { label: 'Период', value: summary.checkup.period_label || '—' },
        { label: 'Обновлён', value: summary.checkup.generated_at ? fmtDateTime(summary.checkup.generated_at) : '—' },
        { label: 'Действие', value: summary.checkup.has_report ? 'Открыть чат' : 'Запустить checkup' },
      ],
      note: summary.checkup.has_report
        ? 'Быстрый переход к последнему AI-контексту.'
        : 'Сделай первый checkup, чтобы получить сводку по всем разделам.',
    },
  ] : []

  return (
      <div className="flex flex-col gap-6">
      {showPicker && <LocationPicker onSelect={handleLocationSelect} onClose={() => setShowPicker(false)} />}
      <PageHeader
        eyebrow="Overview"
        title="Dashboard"
        description="Быстрый срез по деньгам, активности и текущему состоянию дня. Хорошая стартовая точка перед деталями по отдельным разделам."
        badges={[
          { label: enabledDashboardSources.length > 0 ? `${enabledDashboardSources.length} активных источника` : 'Нет активных источников', tone: enabledDashboardSources.length > 0 ? 'muted' : 'warning' },
          ...(weather?.city ? [{ label: weather.city, tone: 'muted' as const }] : []),
        ]}
        actions={(
          <PageSyncButton
            label="Синхронизировать всё"
            syncCaption={dashboardSyncCaption}
            syncing={syncing}
            disabled={enabledDashboardSources.length === 0}
            onClick={handleSyncDashboard}
          />
        )}
      />

      {/* Weather + Stats */}
      <div className="grid min-w-0 grid-cols-1 gap-4 lg:grid-cols-3">
        <div className="min-w-0 lg:col-span-1">
          <WeatherCard weather={weather} loading={weatherLoading} onPickLocation={() => setShowPicker(true)} />
        </div>
        <div className="grid min-w-0 grid-cols-1 content-start gap-4 sm:grid-cols-2 xl:grid-cols-3 lg:col-span-2">
          <StatCard
            title="Баланс"
            value={summary ? fmt(summary.finance.total_balance, 'RUB') : '—'}
            sub="по счетам в балансе"
            icon={Wallet}
            color="bg-blue-500"
            loading={loading}
          />
          <StatCard
            title={globalRange.isActive ? 'Расходы за период' : 'Расходы за месяц'}
            value={summary ? fmt(summary.finance.monthly_spending, 'RUB') : '—'}
            sub={summary ? `доходы: ${fmt(summary.finance.monthly_income, 'RUB')}` : 'нет данных'}
            icon={TrendingDown}
            color="bg-rose-500"
            loading={loading}
          />
          <StatCard
            title="Активности"
            value={summary ? String(summary.fitness.activities_this_week) : '—'}
            sub={summary && summary.fitness.total_distance_km > 0
              ? `${summary.fitness.total_distance_km.toFixed(1)} км ${periodText}`
              : periodText}
            icon={Route}
            trend={summary && summary.fitness.activities_this_week > 0 ? 'up' : undefined}
            color="bg-orange-500"
            loading={loading}
          />
          <StatCard
            title="Тренировки"
            value={summary ? String(summary.fitness.workouts_this_week) : '—'}
            sub={periodText}
            icon={Dumbbell}
            trend={summary && summary.fitness.workouts_this_week > 0 ? 'up' : undefined}
            color="bg-violet-500"
            loading={loading}
          />
          <StatCard
            title={globalRange.isActive ? 'Питание за период' : 'Питание сегодня'}
            value={summary ? `${Math.round(summary.nutrition.today_kcal)} ккал` : '—'}
            sub={summary?.nutrition.target_calories
              ? `цель: ${Math.round(summary.nutrition.target_calories)} ккал · гидратация ${Math.round(summary?.nutrition.today_hydration_ml ?? 0)} мл`
              : `гидратация: ${Math.round(summary?.nutrition.today_hydration_ml ?? 0)} мл`}
            icon={UtensilsCrossed}
            color="bg-emerald-500"
            loading={loading}
          />
          <StatCard
            title="Overdue"
            value={summary ? String(summary.productivity.overdue_total) : '—'}
            sub={summary ? `${globalRange.isActive ? 'в периоде' : 'сегодня'}: ${summary.productivity.due_today_total}` : 'нет данных'}
            icon={ListTodo}
            color="bg-amber-500"
            loading={loading}
          />
        </div>
      </div>

      {/* Section overview */}
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h2 className="text-sm font-semibold uppercase tracking-wider text-foreground">Срез по разделам</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Самое важное по финансам, фитнесу, питанию, задачам и AI без перехода по вкладкам.
            </p>
          </div>
        </div>
        <div className="grid grid-cols-1 gap-4 xl:grid-cols-2 2xl:grid-cols-3">
          {sectionCards.map(card => (
            <OverviewSectionCard key={card.title} {...card} loading={loading} />
          ))}
        </div>
      </div>

      {/* AI Insights */}
      <div className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Что требует внимания</h2>
        <div className="flex flex-col gap-2">
          {insights.slice(0, 5).map((ins, i) => <InsightCard key={i} {...ins} />)}
        </div>
      </div>

      {/* Recent transactions */}
      <div className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Последние транзакции</h2>
        <div className="rounded-2xl border bg-card/90 overflow-hidden shadow-sm">
          {loading ? (
            <div className="divide-y">
              {Array.from({ length: 5 }).map((_, i) => (
                <div key={i} className="px-5 py-3 flex items-center gap-3">
                  <div className="h-4 w-20 bg-muted rounded animate-pulse" />
                  <div className="flex-1 h-4 bg-muted rounded animate-pulse" />
                  <div className="h-4 w-16 bg-muted rounded animate-pulse" />
                </div>
              ))}
            </div>
          ) : txs.length === 0 ? (
            <div className="px-5 py-4 text-sm text-muted-foreground text-center">Нет транзакций</div>
          ) : (
            <div className="divide-y">
              {txs.map(tx => (
                <div key={tx.id} className="px-5 py-3 flex items-center gap-4">
                  <span className="text-xs text-muted-foreground w-16 shrink-0">{fmtDate(tx.occurred_at)}</span>
                  <span className="flex-1 text-sm text-foreground truncate">
                    {tx.payee || tx.comment || '—'}
                  </span>
                  <span className={cn(
                    'text-sm font-medium tabular-nums',
                    tx.amount > 0 ? 'text-emerald-500' : 'text-rose-500'
                  )}>
                    {tx.amount > 0 ? '+' : ''}{fmt(tx.amount, tx.currency)}
                  </span>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
