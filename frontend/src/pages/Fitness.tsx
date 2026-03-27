import { useEffect, useState } from 'react'
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts'
import { Route, Dumbbell, Flame, Heart } from 'lucide-react'
import { cn } from '@/lib/utils'
import { api, type FitnessSummary, type WeekStat, type Activity, type Workout } from '@/lib/api'

const ACTIVITY_ICONS: Record<string, string> = {
  Run: '🏃',
  Ride: '🚴',
  Swim: '🏊',
  Walk: '🚶',
  WeightTraining: '🏋️',
  Workout: '💪',
  Hike: '🥾',
  Yoga: '🧘',
}

function activityIcon(type: string) {
  return ACTIVITY_ICONS[type] ?? '⚡'
}

function fmtDuration(seconds: number | null) {
  if (!seconds) return '—'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (h > 0) return `${h}ч ${m}м`
  return `${m} мин`
}

function fmtDist(meters: number | null) {
  if (!meters) return null
  return `${(meters / 1000).toFixed(1)} км`
}

function fmtDate(iso: string) {
  return new Date(iso).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })
}

function fmtWeek(iso: string) {
  const d = new Date(iso)
  return `${d.getDate()}.${String(d.getMonth() + 1).padStart(2, '0')}`
}

const CustomTooltip = ({ active, payload, label }: any) => {
  if (!active || !payload?.length) return null
  return (
    <div className="rounded-xl border bg-card px-4 py-3 text-sm shadow-lg">
      <p className="font-medium text-foreground mb-1">Нед. с {fmtWeek(label)}</p>
      <p className="text-orange-500">Активностей: {payload[0]?.value}</p>
      {payload[1]?.value > 0 && <p className="text-blue-400">Км: {payload[1]?.value?.toFixed(1)}</p>}
    </div>
  )
}

export function Fitness() {
  const [summary, setSummary] = useState<FitnessSummary | null>(null)
  const [weekly, setWeekly] = useState<WeekStat[]>([])
  const [activities, setActivities] = useState<Activity[]>([])
  const [workouts, setWorkouts] = useState<Workout[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    Promise.all([
      api.getFitnessSummary(),
      api.getFitnessWeekly(),
      api.getActivities(),
      api.getWorkouts(),
    ])
      .then(([s, w, a, wk]) => { setSummary(s); setWeekly(w); setActivities(a); setWorkouts(wk) })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [])

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-bold text-foreground">Фитнес</h1>
        <p className="text-sm text-muted-foreground mt-1">
          {new Date().toLocaleDateString('ru-RU', { month: 'long', year: 'numeric' })}
        </p>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        {[
          {
            label: 'Активностей за неделю',
            value: summary ? String(summary.activities_this_week) : '—',
            icon: Route, color: 'bg-orange-500',
          },
          {
            label: 'Км за неделю',
            value: summary ? `${summary.distance_km_this_week.toFixed(1)} км` : '—',
            icon: Route, color: 'bg-blue-500',
          },
          {
            label: 'Тренировок за неделю',
            value: summary ? String(summary.workouts_this_week) : '—',
            icon: Dumbbell, color: 'bg-violet-500',
          },
          {
            label: 'Всего км',
            value: summary ? `${Math.round(summary.total_distance_km)} км` : '—',
            icon: Flame, color: 'bg-emerald-500',
          },
        ].map(card => (
          <div key={card.label} className="rounded-xl border bg-card p-4 flex flex-col gap-2">
            <div className="flex items-center justify-between">
              <span className="text-xs text-muted-foreground">{card.label}</span>
              <div className={cn('flex items-center justify-center w-7 h-7 rounded-lg', card.color)}>
                <card.icon className="w-3.5 h-3.5 text-white" />
              </div>
            </div>
            {loading
              ? <div className="h-7 w-16 bg-muted rounded animate-pulse" />
              : <div className="text-xl font-bold text-foreground">{card.value}</div>}
          </div>
        ))}
      </div>

      {/* Weekly chart */}
      <div className="rounded-xl border bg-card p-5">
        <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Активность по неделям</h2>
        {loading ? (
          <div className="h-40 bg-muted rounded animate-pulse" />
        ) : weekly.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center py-8">Нет данных</p>
        ) : (
          <ResponsiveContainer width="100%" height={180}>
            <BarChart data={weekly} barCategoryGap="35%">
              <XAxis dataKey="week" tickFormatter={fmtWeek} tick={{ fontSize: 11 }} axisLine={false} tickLine={false} />
              <YAxis allowDecimals={false} tick={{ fontSize: 11 }} axisLine={false} tickLine={false} width={24} />
              <Tooltip content={<CustomTooltip />} cursor={{ opacity: 0.1 }} />
              <Bar dataKey="count" fill="#f97316" radius={[4, 4, 0, 0]} />
            </BarChart>
          </ResponsiveContainer>
        )}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Activities */}
        <div className="rounded-xl border bg-card overflow-hidden">
          <div className="px-5 py-4 border-b">
            <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Активности</h2>
          </div>
          {loading ? (
            <div className="divide-y">
              {Array.from({ length: 5 }).map((_, i) => (
                <div key={i} className="px-5 py-3 flex gap-3">
                  <div className="h-9 w-9 bg-muted rounded-xl animate-pulse shrink-0" />
                  <div className="flex-1 flex flex-col gap-1.5">
                    <div className="h-4 w-32 bg-muted rounded animate-pulse" />
                    <div className="h-3 w-20 bg-muted rounded animate-pulse" />
                  </div>
                </div>
              ))}
            </div>
          ) : activities.length === 0 ? (
            <div className="px-5 py-8 text-sm text-muted-foreground text-center">
              Нет данных. Подключи Strava в настройках.
            </div>
          ) : (
            <div className="divide-y max-h-[420px] overflow-y-auto">
              {activities.map(a => (
                <div key={a.id} className="px-5 py-3 flex items-center gap-3">
                  <div className="w-9 h-9 rounded-xl bg-muted flex items-center justify-center text-base shrink-0">
                    {activityIcon(a.type)}
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-foreground truncate">{a.name || a.type}</p>
                    <div className="flex items-center gap-2 mt-0.5 flex-wrap">
                      <span className="text-xs text-muted-foreground">{fmtDate(a.started_at)}</span>
                      {fmtDist(a.distance_meters) && (
                        <span className="text-xs text-blue-400">{fmtDist(a.distance_meters)}</span>
                      )}
                      <span className="text-xs text-muted-foreground">{fmtDuration(a.duration_seconds)}</span>
                      {a.avg_heart_rate && (
                        <span className="text-xs text-rose-400 flex items-center gap-0.5">
                          <Heart className="w-3 h-3" />{a.avg_heart_rate}
                        </span>
                      )}
                      {a.calories && (
                        <span className="text-xs text-orange-400 flex items-center gap-0.5">
                          <Flame className="w-3 h-3" />{a.calories}
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Workouts */}
        <div className="rounded-xl border bg-card overflow-hidden">
          <div className="px-5 py-4 border-b">
            <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Тренировки (Hevy)</h2>
          </div>
          {loading ? (
            <div className="divide-y">
              {Array.from({ length: 5 }).map((_, i) => (
                <div key={i} className="px-5 py-3 flex gap-3">
                  <div className="flex-1 h-4 bg-muted rounded animate-pulse" />
                  <div className="h-4 w-16 bg-muted rounded animate-pulse" />
                </div>
              ))}
            </div>
          ) : workouts.length === 0 ? (
            <div className="px-5 py-8 text-sm text-muted-foreground text-center">
              Нет данных. Подключи Hevy в настройках.
            </div>
          ) : (
            <div className="divide-y max-h-[420px] overflow-y-auto">
              {workouts.map(wk => {
                const dur = wk.ended_at
                  ? Math.round((new Date(wk.ended_at).getTime() - new Date(wk.started_at).getTime()) / 60000)
                  : null
                return (
                  <div key={wk.id} className="px-5 py-3 flex items-center gap-3">
                    <div className="w-9 h-9 rounded-xl bg-muted flex items-center justify-center text-base shrink-0">🏋️</div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-foreground truncate">{wk.title || 'Тренировка'}</p>
                      <div className="flex items-center gap-2 mt-0.5">
                        <span className="text-xs text-muted-foreground">{fmtDate(wk.started_at)}</span>
                        {dur && <span className="text-xs text-muted-foreground">{fmtDuration(dur * 60)}</span>}
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
