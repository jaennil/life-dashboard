import { useEffect, useState, useMemo } from 'react'
import {
  XAxis, YAxis, Tooltip, ResponsiveContainer,
  AreaChart, Area, Legend, PieChart, Pie, Cell,
} from 'recharts'
import { Route, Dumbbell, Flame, Heart, ChevronDown, ChevronUp, Timer } from 'lucide-react'
import { cn } from '@/lib/utils'
import { api, type FitnessSummary, type WeekStat, type Activity, type Workout } from '@/lib/api'

const ACTIVITY_ICONS: Record<string, string> = {
  Run: '🏃', Ride: '🚴', Swim: '🏊', Walk: '🚶', WeightTraining: '🏋️',
  Workout: '💪', Hike: '🥾', Yoga: '🧘',
}

const TYPE_COLORS: Record<string, string> = {
  Run: '#f97316', Ride: '#3b82f6', Swim: '#06b6d4', Walk: '#10b981',
  WeightTraining: '#8b5cf6', Workout: '#ec4899', Hike: '#eab308', Yoga: '#14b8a6',
}

function activityIcon(type: string) { return ACTIVITY_ICONS[type] ?? '⚡' }

function fmtDuration(seconds: number | null) {
  if (!seconds) return '—'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return h > 0 ? `${h}ч ${m}м` : `${m} мин`
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

function WorkoutRow({ workout }: { workout: Workout }) {
  const [open, setOpen] = useState(false)
  const dur = workout.ended_at
    ? Math.round((new Date(workout.ended_at).getTime() - new Date(workout.started_at).getTime()) / 60000)
    : null

  const setTypeBadge = (t: string) => {
    if (t === 'warm-up') return <span className="text-xs text-amber-400">Разминка</span>
    if (t === 'drop set') return <span className="text-xs text-blue-400">Drop</span>
    if (t === 'failure') return <span className="text-xs text-rose-400">Отказ</span>
    return null
  }

  return (
    <div>
      <button onClick={() => setOpen(o => !o)}
        className="w-full px-5 py-3 flex items-center gap-3 hover:bg-muted/40 transition-colors text-left">
        <div className="w-9 h-9 rounded-xl bg-muted flex items-center justify-center text-base shrink-0">🏋️</div>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium text-foreground">{workout.title || 'Тренировка'}</p>
          <div className="flex items-center gap-2 mt-0.5">
            <span className="text-xs text-muted-foreground">{fmtDate(workout.started_at)}</span>
            {dur && <span className="text-xs text-muted-foreground">{fmtDuration(dur * 60)}</span>}
            {workout.exercises.length > 0 && <span className="text-xs text-muted-foreground">{workout.exercises.length} упр.</span>}
          </div>
        </div>
        {workout.exercises.length > 0 && (open ? <ChevronUp className="w-4 h-4 text-muted-foreground shrink-0" /> : <ChevronDown className="w-4 h-4 text-muted-foreground shrink-0" />)}
      </button>
      {open && workout.exercises.length > 0 && (
        <div className="px-5 pb-3 flex flex-col gap-3">
          {workout.exercises.map(ex => (
            <div key={ex.name}>
              <p className="text-xs font-semibold text-foreground mb-1">{ex.name}
                {ex.category && <span className="font-normal text-muted-foreground ml-1">({ex.category})</span>}
              </p>
              <div className="flex flex-col gap-0.5">
                {ex.sets.map(s => (
                  <div key={s.set_index} className="flex items-center gap-2 text-xs text-muted-foreground">
                    <span className="w-8 shrink-0">#{s.set_index}</span>
                    <span className="font-medium text-foreground tabular-nums">
                      {s.weight_kg != null ? `${s.weight_kg} кг` : '—'} × {s.reps != null ? `${s.reps}` : '—'}
                    </span>
                    {setTypeBadge(s.set_type)}
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

export function Fitness() {
  const [summary, setSummary] = useState<FitnessSummary | null>(null)
  const [weekly, setWeekly] = useState<WeekStat[]>([])
  const [activities, setActivities] = useState<Activity[]>([])
  const [workouts, setWorkouts] = useState<Workout[]>([])
  const [loading, setLoading] = useState(true)
  const [typeFilter, setTypeFilter] = useState('')
  const [tab, setTab] = useState<'activities' | 'workouts'>('activities')

  useEffect(() => {
    Promise.all([api.getFitnessSummary(), api.getFitnessWeekly(), api.getActivities(), api.getWorkouts()])
      .then(([s, w, a, wk]) => { setSummary(s); setWeekly(w); setActivities(a); setWorkouts(wk) })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [])

  // Activity types for filter
  const activityTypes = useMemo(() => {
    const types = new Set(activities.map(a => a.type))
    return Array.from(types).sort()
  }, [activities])

  const filteredActivities = typeFilter ? activities.filter(a => a.type === typeFilter) : activities

  // Stats for charts
  const typePie = useMemo(() => {
    const counts: Record<string, number> = {}
    activities.forEach(a => { counts[a.type] = (counts[a.type] || 0) + 1 })
    return Object.entries(counts).map(([type, count]) => ({
      name: type, value: count, color: TYPE_COLORS[type] || '#94a3b8',
    })).sort((a, b) => b.value - a.value)
  }, [activities])

  // Distance by week for area chart
  const distanceByWeek = useMemo(() => {
    return weekly.map(w => ({ ...w, weekLabel: fmtWeek(w.week) }))
  }, [weekly])

  // Totals
  const totalCalories = activities.reduce((s, a) => s + (a.calories || 0), 0)
  // const totalDuration = activities.reduce((s, a) => s + (a.duration_seconds || 0), 0)

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-bold text-foreground">Фитнес</h1>
        <p className="text-sm text-muted-foreground mt-1">
          {new Date().toLocaleDateString('ru-RU', { month: 'long', year: 'numeric' })}
        </p>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
        {[
          { label: 'Активн./нед', value: summary ? String(summary.activities_this_week) : '—', icon: Route, color: 'bg-orange-500' },
          { label: 'Км/нед', value: summary ? `${summary.distance_km_this_week.toFixed(1)}` : '—', icon: Route, color: 'bg-blue-500' },
          { label: 'Тренир./нед', value: summary ? String(summary.workouts_this_week) : '—', icon: Dumbbell, color: 'bg-violet-500' },
          { label: 'Всего км', value: summary ? `${Math.round(summary.total_distance_km)}` : '—', icon: Flame, color: 'bg-emerald-500' },
          { label: 'Всего ккал', value: totalCalories > 0 ? `${Math.round(totalCalories)}` : '—', icon: Flame, color: 'bg-rose-500' },
        ].map(card => (
          <div key={card.label} className="rounded-xl border bg-card p-4 flex flex-col gap-1">
            <div className="flex items-center justify-between">
              <span className="text-[10px] text-muted-foreground">{card.label}</span>
              <div className={cn('flex items-center justify-center w-6 h-6 rounded-md', card.color)}>
                <card.icon className="w-3 h-3 text-white" />
              </div>
            </div>
            {loading ? <div className="h-6 w-12 bg-muted rounded animate-pulse" /> : (
              <div className="text-lg font-bold text-foreground">{card.value}</div>
            )}
          </div>
        ))}
      </div>

      {/* Charts row */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Weekly activity + distance */}
        <div className="rounded-xl border bg-card p-5">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Активность по неделям</h2>
          {loading ? <div className="h-48 bg-muted rounded animate-pulse" /> : weekly.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-8">Нет данных</p>
          ) : (
            <ResponsiveContainer width="100%" height={200}>
              <AreaChart data={distanceByWeek}>
                <XAxis dataKey="week" tickFormatter={fmtWeek} tick={{ fontSize: 11 }} axisLine={false} tickLine={false} />
                <YAxis yAxisId="count" tick={{ fontSize: 11 }} axisLine={false} tickLine={false} width={24} />
                <YAxis yAxisId="km" orientation="right" tick={{ fontSize: 11 }} axisLine={false} tickLine={false} width={30} />
                <Tooltip content={({ active, payload, label }: any) => active && payload?.length ? (
                  <div className="rounded-xl border bg-card px-4 py-3 text-sm shadow-lg">
                    <p className="font-medium text-foreground mb-1">Нед. с {fmtWeek(label)}</p>
                    {payload.map((p: any) => <p key={p.name} style={{ color: p.color }}>{p.name === 'count' ? 'Активностей' : 'Км'}: {p.name === 'km' ? p.value?.toFixed(1) : p.value}</p>)}
                  </div>
                ) : null} />
                <Legend formatter={v => v === 'count' ? 'Активности' : 'Км'} wrapperStyle={{ fontSize: 11 }} />
                <Area yAxisId="count" type="monotone" dataKey="count" stroke="#f97316" fill="#f97316" fillOpacity={0.15} strokeWidth={2} />
                <Area yAxisId="km" type="monotone" dataKey="km" stroke="#3b82f6" fill="#3b82f6" fillOpacity={0.1} strokeWidth={2} />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </div>

        {/* Activity types pie */}
        <div className="rounded-xl border bg-card p-5">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Типы активностей</h2>
          {loading || typePie.length === 0 ? <div className="h-48 flex items-center justify-center text-sm text-muted-foreground">Нет данных</div> : (
            <div className="flex items-center gap-6">
              <div style={{ width: 180, height: 180, flexShrink: 0 }}>
                <PieChart width={180} height={180}>
                  <Pie data={typePie} dataKey="value" cx={90} cy={90} innerRadius={50} outerRadius={80} paddingAngle={3}>
                    {typePie.map((m, i) => <Cell key={i} fill={m.color} />)}
                  </Pie>
                  <Tooltip formatter={(v: any) => `${v} активностей`} />
                </PieChart>
              </div>
              <div className="flex flex-col gap-2">
                {typePie.map(m => (
                  <button key={m.name} onClick={() => setTypeFilter(typeFilter === m.name ? '' : m.name)}
                    className="flex items-center gap-2 text-xs hover:bg-accent/50 rounded px-1 py-0.5 transition-colors">
                    <div className="w-2.5 h-2.5 rounded-full" style={{ backgroundColor: m.color }} />
                    <span className={cn('text-foreground', typeFilter === m.name && 'font-semibold text-primary')}>{activityIcon(m.name)} {m.name}</span>
                    <span className="text-muted-foreground">{m.value}</span>
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Activities + Workouts tabs */}
      <div className="rounded-xl border bg-card overflow-hidden">
        <div className="px-5 py-4 border-b flex items-center justify-between">
          <div className="flex gap-1">
            <button onClick={() => setTab('activities')}
              className={cn('px-3 py-1.5 text-xs rounded-lg transition-colors', tab === 'activities' ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:bg-accent')}>
              Активности ({filteredActivities.length})
            </button>
            <button onClick={() => setTab('workouts')}
              className={cn('px-3 py-1.5 text-xs rounded-lg transition-colors', tab === 'workouts' ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:bg-accent')}>
              Тренировки ({workouts.length})
            </button>
          </div>
          {tab === 'activities' && activityTypes.length > 1 && (
            <div className="flex gap-1">
              <button onClick={() => setTypeFilter('')}
                className={cn('px-2 py-1 text-xs rounded-lg transition-colors', !typeFilter ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:bg-accent')}>
                Все
              </button>
              {activityTypes.map(t => (
                <button key={t} onClick={() => setTypeFilter(typeFilter === t ? '' : t)}
                  className={cn('px-2 py-1 text-xs rounded-lg transition-colors', typeFilter === t ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:bg-accent')}>
                  {activityIcon(t)} {t}
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="divide-y max-h-[520px] overflow-y-auto">
          {tab === 'activities' ? (
            loading ? (
              Array.from({ length: 5 }).map((_, i) => (
                <div key={i} className="px-5 py-3 flex gap-3"><div className="h-9 w-9 bg-muted rounded-xl animate-pulse shrink-0" /><div className="flex-1 h-4 bg-muted rounded animate-pulse" /></div>
              ))
            ) : filteredActivities.length === 0 ? (
              <div className="px-5 py-8 text-sm text-muted-foreground text-center">Нет данных</div>
            ) : (
              filteredActivities.map(a => (
                <div key={a.id} className="px-5 py-3 flex items-center gap-3">
                  <div className="w-9 h-9 rounded-xl bg-muted flex items-center justify-center text-base shrink-0">{activityIcon(a.type)}</div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-foreground truncate">{a.name || a.type}</p>
                    <div className="flex items-center gap-2 mt-0.5 flex-wrap">
                      <span className="text-xs text-muted-foreground">{fmtDate(a.started_at)}</span>
                      {fmtDist(a.distance_meters) && <span className="text-xs text-blue-400">{fmtDist(a.distance_meters)}</span>}
                      <span className="text-xs text-muted-foreground flex items-center gap-0.5"><Timer className="w-3 h-3" />{fmtDuration(a.duration_seconds)}</span>
                      {a.avg_heart_rate && <span className="text-xs text-rose-400 flex items-center gap-0.5"><Heart className="w-3 h-3" />{a.avg_heart_rate}</span>}
                      {a.calories && <span className="text-xs text-orange-400 flex items-center gap-0.5"><Flame className="w-3 h-3" />{a.calories}</span>}
                    </div>
                  </div>
                </div>
              ))
            )
          ) : (
            loading ? (
              Array.from({ length: 3 }).map((_, i) => (
                <div key={i} className="px-5 py-3 flex gap-3"><div className="flex-1 h-4 bg-muted rounded animate-pulse" /></div>
              ))
            ) : workouts.length === 0 ? (
              <div className="px-5 py-8 text-sm text-muted-foreground text-center">Нет данных</div>
            ) : (
              workouts.map(wk => <WorkoutRow key={wk.id} workout={wk} />)
            )
          )}
        </div>
      </div>
    </div>
  )
}
