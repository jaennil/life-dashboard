import { useEffect, useRef, useState } from 'react'
import { Wallet, Dumbbell, TrendingUp, TrendingDown, Zap, Route, Droplets, Wind, MapPin, LocateFixed, Search, X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { api, type DashboardSummary, type Transaction, type WeatherData } from '@/lib/api'

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
    <div className="rounded-xl border bg-card p-5 flex flex-col gap-3">
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
    info: 'border-l-blue-500 bg-blue-500/5',
    warn: 'border-l-amber-500 bg-amber-500/5',
    good: 'border-l-emerald-500 bg-emerald-500/5',
  }
  return (
    <div className={cn('border-l-2 rounded-r-lg px-4 py-3 text-sm text-foreground', colors[type])}>
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
      <div className="bg-card border rounded-xl shadow-xl w-full max-w-sm mx-4 p-4 flex flex-col gap-3" onClick={e => e.stopPropagation()}>
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
      <div className="rounded-xl border bg-card p-5">
        <div className="h-5 w-24 bg-muted rounded animate-pulse mb-4" />
        <div className="h-12 w-32 bg-muted rounded animate-pulse" />
      </div>
    )
  }
  if (!weather) return null

  return (
    <div className="rounded-xl border bg-card p-5 flex flex-col gap-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <button
            onClick={onPickLocation}
            className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors group"
          >
            <MapPin className="w-3 h-3" />
            <span>{weather.city}</span>
          </button>
          <div className="flex items-end gap-2 mt-1">
            <span className="text-5xl font-bold text-foreground">{Math.round(weather.temp)}°</span>
            <span className="text-4xl mb-0.5">{wmoIcon(weather.weather_code)}</span>
          </div>
          <p className="text-sm text-muted-foreground mt-1">{weather.description}</p>
        </div>
        <div className="flex flex-col gap-2 text-xs text-muted-foreground shrink-0 mt-1">
          <span>Ощущается {Math.round(weather.feels_like)}°</span>
          <span className="flex items-center gap-1"><Droplets className="w-3 h-3" />{weather.humidity}%</span>
          <span className="flex items-center gap-1"><Wind className="w-3 h-3" />{Math.round(weather.wind_speed)} км/ч</span>
        </div>
      </div>

      <div className="flex gap-3 overflow-x-auto pb-1 scrollbar-hide">
        {weather.hourly.slice(0, 12).map(h => (
          <div key={h.time} className="flex flex-col items-center gap-1 shrink-0">
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

export function Dashboard() {
  const [summary, setSummary] = useState<DashboardSummary | null>(null)
  const [txs, setTxs] = useState<Transaction[]>([])
  const [weather, setWeather] = useState<WeatherData | null>(null)
  const [loading, setLoading] = useState(true)
  const [weatherLoading, setWeatherLoading] = useState(true)
  const [showPicker, setShowPicker] = useState(false)

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
    Promise.all([api.getDashboardSummary(), api.getRecentTransactions()])
      .then(([s, t]) => { setSummary(s); setTxs(t) })
      .catch(console.error)
      .finally(() => setLoading(false))

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
  }, [])

  const insights: { text: string; type: 'info' | 'warn' | 'good' }[] = []
  if (summary) {
    if (summary.fitness.activities_this_week === 0 && summary.fitness.workouts_this_week === 0) {
      insights.push({ type: 'warn', text: 'На этой неделе нет активностей. Самое время размяться!' })
    }
    if (summary.finance.monthly_spending > summary.finance.monthly_income && summary.finance.monthly_income > 0) {
      insights.push({ type: 'warn', text: `Расходы (${fmt(summary.finance.monthly_spending, 'RUB')}) превышают доходы в этом месяце.` })
    }
    if (summary.fitness.total_distance_km > 0) {
      insights.push({ type: 'good', text: `За эту неделю пройдено ${summary.fitness.total_distance_km.toFixed(1)} км.` })
    }
  }
  if (insights.length === 0) {
    insights.push({ type: 'info', text: 'Подключи больше источников данных чтобы получать персональные инсайты.' })
  }

  return (
    <div className="flex flex-col gap-6">
      {showPicker && <LocationPicker onSelect={handleLocationSelect} onClose={() => setShowPicker(false)} />}
      <div>
        <h1 className="text-2xl font-bold text-foreground">Dashboard</h1>
        <p className="text-sm text-muted-foreground mt-1">
          {new Date().toLocaleDateString('ru-RU', { weekday: 'long', day: 'numeric', month: 'long' })}
        </p>
      </div>

      {/* Weather + Stats */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <div className="lg:col-span-1">
          <WeatherCard weather={weather} loading={weatherLoading} onPickLocation={() => setShowPicker(true)} />
        </div>
        <div className="lg:col-span-2 grid grid-cols-2 gap-4 content-start">
          <StatCard
            title="Баланс"
            value={summary ? fmt(summary.finance.total_balance, 'RUB') : '—'}
            sub="по счетам в балансе"
            icon={Wallet}
            color="bg-blue-500"
            loading={loading}
          />
          <StatCard
            title="Расходы за месяц"
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
              ? `${summary.fitness.total_distance_km.toFixed(1)} км на этой неделе`
              : 'за эту неделю'}
            icon={Route}
            trend={summary && summary.fitness.activities_this_week > 0 ? 'up' : undefined}
            color="bg-orange-500"
            loading={loading}
          />
          <StatCard
            title="Тренировки"
            value={summary ? String(summary.fitness.workouts_this_week) : '—'}
            sub="за эту неделю"
            icon={Dumbbell}
            trend={summary && summary.fitness.workouts_this_week > 0 ? 'up' : undefined}
            color="bg-violet-500"
            loading={loading}
          />
        </div>
      </div>

      {/* AI Insights */}
      <div className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">AI Инсайты</h2>
        <div className="flex flex-col gap-2">
          {insights.map((ins, i) => <InsightCard key={i} {...ins} />)}
        </div>
      </div>

      {/* Recent transactions */}
      <div className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Последние транзакции</h2>
        <div className="rounded-xl border bg-card overflow-hidden">
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
