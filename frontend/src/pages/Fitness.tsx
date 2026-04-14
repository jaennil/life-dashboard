import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  XAxis, YAxis, Tooltip, ResponsiveContainer,
  AreaChart, Area, Legend, PieChart, Pie, Cell,
} from 'recharts'
import { Route, Dumbbell, Flame, Heart, ChevronDown, ChevronUp, Timer } from 'lucide-react'
import { PageSyncButton } from '@/components/PageSyncButton'
import { cn } from '@/lib/utils'
import { api, type FitnessSummary, type WeekStat, type Activity, type Workout, type Integration } from '@/lib/api'

const ACTIVITY_ICONS: Record<string, string> = {
  Run: '🏃', Ride: '🚴', Swim: '🏊', Walk: '🚶', WeightTraining: '🏋️',
  Workout: '💪', Hike: '🥾', Yoga: '🧘',
}

const TYPE_COLORS: Record<string, string> = {
  Run: '#f97316', Ride: '#3b82f6', Swim: '#06b6d4', Walk: '#10b981',
  WeightTraining: '#8b5cf6', Workout: '#ec4899', Hike: '#eab308', Yoga: '#14b8a6',
}

const FALLBACK_COLORS = ['#8b5cf6', '#ec4899', '#14b8a6', '#f97316', '#3b82f6', '#10b981', '#eab308', '#f43f5e']
const DECIMAL_FORMATTER = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 1 })

type FitnessSource = 'strava' | 'hevy'

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

function fmtDateTime(iso: string) {
  return new Date(iso).toLocaleString('ru-RU', {
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function fmtWeek(iso: string) {
  const d = new Date(iso)
  return `${d.getDate()}.${String(d.getMonth() + 1).padStart(2, '0')}`
}

function fmtMetric(value: number | null) {
  if (value == null) return null
  return DECIMAL_FORMATTER.format(value)
}

function fmtWorkoutDistance(meters: number | null) {
  if (meters == null) return null
  if (meters >= 1000) return `${DECIMAL_FORMATTER.format(meters / 1000)} км`
  return `${Math.round(meters)} м`
}

function StatCard({
  label,
  value,
  loading,
  icon: Icon,
  color,
}: {
  label: string
  value: string
  loading: boolean
  icon: typeof Route
  color: string
}) {
  return (
    <div className="rounded-xl border bg-card p-4 flex flex-col gap-1">
      <div className="flex items-center justify-between">
        <span className="text-[10px] text-muted-foreground">{label}</span>
        <div className={cn('flex items-center justify-center w-6 h-6 rounded-md', color)}>
          <Icon className="w-3 h-3 text-white" />
        </div>
      </div>
      {loading ? <div className="h-6 w-12 bg-muted rounded animate-pulse" /> : (
        <div className="text-lg font-bold text-foreground">{value}</div>
      )}
    </div>
  )
}

function WorkoutRow({ workout }: { workout: Workout }) {
  const [open, setOpen] = useState(false)
  const dur = workout.ended_at
    ? Math.round((new Date(workout.ended_at).getTime() - new Date(workout.started_at).getTime()) / 60000)
    : null
  const sourceUpdated = workout.source_updated_at ? fmtDateTime(workout.source_updated_at) : null
  const sourceCreated = workout.source_created_at ? fmtDateTime(workout.source_created_at) : null

  const setTypeBadge = (t: string) => {
    if (t === 'warm-up') return <span className="text-xs text-amber-400">Разминка</span>
    if (t === 'drop set') return <span className="text-xs text-blue-400">Drop</span>
    if (t === 'failure') return <span className="text-xs text-rose-400">Отказ</span>
    return null
  }

  return (
    <div>
      <button
        onClick={() => setOpen(o => !o)}
        className="w-full px-5 py-3 flex items-center gap-3 hover:bg-muted/40 transition-colors text-left"
      >
        <div className="w-9 h-9 rounded-xl bg-muted flex items-center justify-center text-base shrink-0">🏋️</div>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium text-foreground">{workout.title || 'Тренировка'}</p>
          <div className="flex items-center gap-2 mt-0.5">
            <span className="text-xs text-muted-foreground">{fmtDate(workout.started_at)}</span>
            {dur && <span className="text-xs text-muted-foreground">{fmtDuration(dur * 60)}</span>}
            {workout.exercises.length > 0 && <span className="text-xs text-muted-foreground">{workout.exercises.length} упр.</span>}
            {sourceUpdated && <span className="text-xs text-muted-foreground">обн. {sourceUpdated}</span>}
          </div>
        </div>
        {workout.exercises.length > 0 && (
          open
            ? <ChevronUp className="w-4 h-4 text-muted-foreground shrink-0" />
            : <ChevronDown className="w-4 h-4 text-muted-foreground shrink-0" />
        )}
      </button>
      {open && workout.exercises.length > 0 && (
        <div className="px-5 pb-3 flex flex-col gap-3">
          {(workout.notes || sourceCreated || sourceUpdated) && (
            <div className="rounded-lg border bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
              {workout.notes && <p className="text-foreground whitespace-pre-wrap">{workout.notes}</p>}
              {sourceCreated && <p>{workout.notes ? 'Создано' : 'В Hevy создано'}: {sourceCreated}</p>}
              {sourceUpdated && <p>{workout.notes || sourceCreated ? 'Обновлено' : 'В Hevy обновлено'}: {sourceUpdated}</p>}
            </div>
          )}
          {workout.exercises.map(ex => (
            <div key={`${ex.index}-${ex.name}`} className="rounded-lg border bg-muted/10 px-3 py-2">
              <p className="text-xs font-semibold text-foreground mb-1">
                {ex.name}
                {ex.category && <span className="font-normal text-muted-foreground ml-1">({ex.category})</span>}
              </p>
              <div className="flex flex-wrap items-center gap-2 mb-2">
                <span className="text-[11px] text-muted-foreground">Блок #{ex.index}</span>
                {ex.template_id && <span className="text-[11px] text-muted-foreground font-mono">tpl: {ex.template_id}</span>}
              </div>
              {ex.notes && <p className="text-xs text-muted-foreground mb-2 whitespace-pre-wrap">{ex.notes}</p>}
              <div className="flex flex-col gap-0.5">
                {ex.sets.map(s => (
                  <div key={`${s.exercise_index}-${s.set_index}`} className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                    <span className="w-8 shrink-0">#{s.set_index}</span>
                    {(s.weight_kg != null || s.reps != null) && (
                      <span className="font-medium text-foreground tabular-nums">
                        {s.weight_kg != null ? `${fmtMetric(s.weight_kg)} кг` : '—'} × {s.reps != null ? `${s.reps}` : '—'}
                      </span>
                    )}
                    {s.distance_meters != null && <span className="text-cyan-400">{fmtWorkoutDistance(s.distance_meters)}</span>}
                    {s.duration_seconds != null && (
                      <span className="text-amber-300 flex items-center gap-0.5">
                        <Timer className="w-3 h-3" />
                        {fmtDuration(s.duration_seconds)}
                      </span>
                    )}
                    {s.rpe != null && <span className="text-violet-400">RPE {fmtMetric(s.rpe)}</span>}
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
  const [sourceTab, setSourceTab] = useState<FitnessSource>('strava')
  const [syncing, setSyncing] = useState(false)
  const [integrations, setIntegrations] = useState<Integration[]>([])

  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const [nextSummary, nextWeekly, nextActivities, nextWorkouts] = await Promise.all([
        api.getFitnessSummary(),
        api.getFitnessWeekly(),
        api.getActivities(),
        api.getWorkouts(),
      ])
      setSummary(nextSummary)
      setWeekly(nextWeekly)
      setActivities(nextActivities)
      setWorkouts(nextWorkouts)
    } catch (error) {
      console.error(error)
    } finally {
      setLoading(false)
    }
  }, [])

  const loadIntegrations = useCallback(async () => {
    try {
      setIntegrations(await api.getIntegrations())
    } catch (error) {
      console.error(error)
    }
  }, [])

  useEffect(() => {
    void loadData()
  }, [loadData])

  useEffect(() => {
    void loadIntegrations()
  }, [loadIntegrations])

  useEffect(() => {
    if (!summary) return
    if (summary.activities_total === 0 && summary.workouts_total > 0) {
      setSourceTab('hevy')
    }
  }, [summary])

  const stravaTotal = summary?.activities_total ?? activities.length
  const hevyTotal = summary?.workouts_total ?? workouts.length
  const activeIntegration = integrations.find(i => i.name === sourceTab)

  const activityTypes = useMemo(() => {
    const types = new Set(activities.map(a => a.type))
    return Array.from(types).sort()
  }, [activities])

  const filteredActivities = typeFilter ? activities.filter(a => a.type === typeFilter) : activities

  const activityTypePie = useMemo(() => {
    const counts: Record<string, number> = {}
    activities.forEach(a => { counts[a.type] = (counts[a.type] || 0) + 1 })
    return Object.entries(counts).map(([type, count]) => ({
      name: type,
      value: count,
      color: TYPE_COLORS[type] || '#94a3b8',
    })).sort((a, b) => b.value - a.value)
  }, [activities])

  const workoutCategoryPie = useMemo(() => {
    const counts: Record<string, number> = {}
    workouts.forEach(workout => {
      workout.exercises.forEach(exercise => {
        const key = exercise.category || exercise.name || 'Другое'
        counts[key] = (counts[key] || 0) + 1
      })
    })

    return Object.entries(counts).map(([name, count], index) => ({
      name,
      value: count,
      color: FALLBACK_COLORS[index % FALLBACK_COLORS.length],
    })).sort((a, b) => b.value - a.value)
  }, [workouts])

  const stravaWeekly = useMemo(() => {
    return weekly.map(point => ({
      week: point.week,
      activities: point.activities_count,
      km: point.km,
    }))
  }, [weekly])

  const hevyWeekly = useMemo(() => {
    return weekly.map(point => ({
      week: point.week,
      workouts: point.workouts_count,
    }))
  }, [weekly])

  const hevyExerciseStats = useMemo(() => {
    const totalExercises = workouts.reduce((sum, workout) => sum + workout.exercises.length, 0)
    return {
      totalExercises,
      avgExercises: workouts.length > 0 ? (totalExercises / workouts.length).toFixed(1) : '—',
    }
  }, [workouts])

  const cards = sourceTab === 'strava'
    ? [
        { label: 'Активн./нед', value: summary ? String(summary.activities_this_week) : '—', icon: Route, color: 'bg-orange-500' },
        { label: 'Км/нед', value: summary ? summary.distance_km_this_week.toFixed(1) : '—', icon: Route, color: 'bg-blue-500' },
        { label: 'Всего активн.', value: summary ? String(summary.activities_total) : '—', icon: Route, color: 'bg-violet-500' },
        { label: 'Всего км', value: summary ? `${Math.round(summary.total_distance_km)}` : '—', icon: Flame, color: 'bg-emerald-500' },
      ]
    : [
        { label: 'Трен./нед', value: summary ? String(summary.workouts_this_week) : '—', icon: Dumbbell, color: 'bg-violet-500' },
        { label: 'Всего трен.', value: summary ? String(summary.workouts_total) : '—', icon: Dumbbell, color: 'bg-cyan-500' },
        { label: 'Упр. в ленте', value: String(hevyExerciseStats.totalExercises), icon: Dumbbell, color: 'bg-amber-500' },
        { label: 'Ср. упр./трен.', value: hevyExerciseStats.avgExercises, icon: Dumbbell, color: 'bg-emerald-500' },
      ]

  async function handleSyncFitness() {
    if (!activeIntegration?.enabled) return
    setSyncing(true)
    try {
      await api.syncIntegration(sourceTab)
      await Promise.all([loadData(), loadIntegrations()])
    } catch (error) {
      console.error(error)
    } finally {
      setSyncing(false)
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
          <div>
            <h1 className="text-2xl font-bold text-foreground">Фитнес</h1>
            <p className="text-sm text-muted-foreground mt-1">
              {new Date().toLocaleDateString('ru-RU', { month: 'long', year: 'numeric' })}
            </p>
          </div>
          <PageSyncButton
            label={`Синхронизировать ${sourceTab === 'strava' ? 'Strava' : 'Hevy'}`}
            syncing={syncing}
            disabled={!activeIntegration?.enabled}
            onClick={handleSyncFitness}
          />
        </div>

        <div className="flex flex-wrap gap-2">
          {[
            { key: 'strava' as const, label: 'Strava', count: stravaTotal, helper: 'активности' },
            { key: 'hevy' as const, label: 'Hevy', count: hevyTotal, helper: 'тренировки' },
          ].map(tab => (
            <button
              key={tab.key}
              onClick={() => setSourceTab(tab.key)}
              className={cn(
                'rounded-xl border px-4 py-3 text-left transition-colors min-w-[180px]',
                sourceTab === tab.key ? 'border-primary bg-primary/10' : 'bg-card hover:bg-muted/40',
              )}
            >
              <div className="flex items-center justify-between gap-4">
                <span className="text-sm font-medium text-foreground">{tab.label}</span>
                <span className="text-xs text-muted-foreground">{tab.count}</span>
              </div>
              <p className="text-xs text-muted-foreground mt-1">{tab.helper}</p>
            </button>
          ))}
        </div>
      </div>

      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        {cards.map(card => (
          <StatCard
            key={card.label}
            label={card.label}
            value={card.value}
            loading={loading}
            icon={card.icon}
            color={card.color}
          />
        ))}
      </div>

      {sourceTab === 'strava' ? (
        <>
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <div className="rounded-xl border bg-card p-5">
              <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Strava по неделям</h2>
              {loading ? <div className="h-48 bg-muted rounded animate-pulse" /> : stravaWeekly.length === 0 ? (
                <p className="text-sm text-muted-foreground text-center py-8">Нет данных</p>
              ) : (
                <ResponsiveContainer width="100%" height={200}>
                  <AreaChart data={stravaWeekly}>
                    <XAxis dataKey="week" tickFormatter={fmtWeek} tick={{ fontSize: 11 }} axisLine={false} tickLine={false} />
                    <YAxis yAxisId="activities" tick={{ fontSize: 11 }} axisLine={false} tickLine={false} width={24} />
                    <YAxis yAxisId="km" orientation="right" tick={{ fontSize: 11 }} axisLine={false} tickLine={false} width={30} />
                    <Tooltip content={({ active, payload, label }: any) => active && payload?.length ? (
                      <div className="rounded-xl border bg-card px-4 py-3 text-sm shadow-lg">
                        <p className="font-medium text-foreground mb-1">Нед. с {fmtWeek(label)}</p>
                        {payload.map((point: any) => (
                          <p key={point.name} style={{ color: point.color }}>
                            {point.name === 'activities' ? 'Активностей' : 'Км'}: {point.name === 'km' ? point.value?.toFixed(1) : point.value}
                          </p>
                        ))}
                      </div>
                    ) : null} />
                    <Legend formatter={value => value === 'activities' ? 'Активности' : 'Км'} wrapperStyle={{ fontSize: 11 }} />
                    <Area yAxisId="activities" type="monotone" dataKey="activities" stroke="#f97316" fill="#f97316" fillOpacity={0.15} strokeWidth={2} />
                    <Area yAxisId="km" type="monotone" dataKey="km" stroke="#3b82f6" fill="#3b82f6" fillOpacity={0.1} strokeWidth={2} />
                  </AreaChart>
                </ResponsiveContainer>
              )}
            </div>

            <div className="rounded-xl border bg-card p-5">
              <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Типы активностей</h2>
              {loading || activityTypePie.length === 0 ? <div className="h-48 flex items-center justify-center text-sm text-muted-foreground">Нет данных</div> : (
                <div className="flex items-center gap-6">
                  <div style={{ width: 180, height: 180, flexShrink: 0 }}>
                    <PieChart width={180} height={180}>
                      <Pie data={activityTypePie} dataKey="value" cx={90} cy={90} innerRadius={50} outerRadius={80} paddingAngle={3}>
                        {activityTypePie.map((segment, index) => <Cell key={index} fill={segment.color} />)}
                      </Pie>
                      <Tooltip formatter={(value: any) => `${value} активностей`} />
                    </PieChart>
                  </div>
                  <div className="flex flex-col gap-2">
                    {activityTypePie.map(segment => (
                      <button
                        key={segment.name}
                        onClick={() => setTypeFilter(typeFilter === segment.name ? '' : segment.name)}
                        className="flex items-center gap-2 text-xs hover:bg-accent/50 rounded px-1 py-0.5 transition-colors"
                      >
                        <div className="w-2.5 h-2.5 rounded-full" style={{ backgroundColor: segment.color }} />
                        <span className={cn('text-foreground', typeFilter === segment.name && 'font-semibold text-primary')}>
                          {activityIcon(segment.name)} {segment.name}
                        </span>
                        <span className="text-muted-foreground">{segment.value}</span>
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>

          <div className="rounded-xl border bg-card overflow-hidden">
            <div className="px-5 py-4 border-b flex items-center justify-between gap-4">
              <div>
                <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Лента Strava</h2>
                <p className="text-xs text-muted-foreground mt-1">Последние активности{stravaTotal > activities.length ? ' (показываем 30 последних)' : ''}</p>
              </div>
              {activityTypes.length > 1 && (
                <div className="flex flex-wrap gap-1 justify-end">
                  <button
                    onClick={() => setTypeFilter('')}
                    className={cn('px-2 py-1 text-xs rounded-lg transition-colors', !typeFilter ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:bg-accent')}
                  >
                    Все
                  </button>
                  {activityTypes.map(type => (
                    <button
                      key={type}
                      onClick={() => setTypeFilter(typeFilter === type ? '' : type)}
                      className={cn('px-2 py-1 text-xs rounded-lg transition-colors', typeFilter === type ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:bg-accent')}
                    >
                      {activityIcon(type)} {type}
                    </button>
                  ))}
                </div>
              )}
            </div>

            <div className="divide-y max-h-[520px] overflow-y-auto">
              {loading ? (
                Array.from({ length: 5 }).map((_, index) => (
                  <div key={index} className="px-5 py-3 flex gap-3">
                    <div className="h-9 w-9 bg-muted rounded-xl animate-pulse shrink-0" />
                    <div className="flex-1 h-4 bg-muted rounded animate-pulse" />
                  </div>
                ))
              ) : filteredActivities.length === 0 ? (
                <div className="px-5 py-8 text-sm text-muted-foreground text-center">Нет данных</div>
              ) : (
                filteredActivities.map(activity => (
                  <div key={activity.id} className="px-5 py-3 flex items-center gap-3">
                    <div className="w-9 h-9 rounded-xl bg-muted flex items-center justify-center text-base shrink-0">{activityIcon(activity.type)}</div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-foreground truncate">{activity.name || activity.type}</p>
                      <div className="flex items-center gap-2 mt-0.5 flex-wrap">
                        <span className="text-xs text-muted-foreground">{fmtDate(activity.started_at)}</span>
                        {fmtDist(activity.distance_meters) && <span className="text-xs text-blue-400">{fmtDist(activity.distance_meters)}</span>}
                        <span className="text-xs text-muted-foreground flex items-center gap-0.5"><Timer className="w-3 h-3" />{fmtDuration(activity.duration_seconds)}</span>
                        {activity.avg_heart_rate && <span className="text-xs text-rose-400 flex items-center gap-0.5"><Heart className="w-3 h-3" />{activity.avg_heart_rate}</span>}
                        {activity.calories && <span className="text-xs text-orange-400 flex items-center gap-0.5"><Flame className="w-3 h-3" />{activity.calories}</span>}
                      </div>
                    </div>
                  </div>
                ))
              )}
            </div>
          </div>
        </>
      ) : (
        <>
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <div className="rounded-xl border bg-card p-5">
              <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Hevy по неделям</h2>
              {loading ? <div className="h-48 bg-muted rounded animate-pulse" /> : hevyWeekly.length === 0 ? (
                <p className="text-sm text-muted-foreground text-center py-8">Нет данных</p>
              ) : (
                <ResponsiveContainer width="100%" height={200}>
                  <AreaChart data={hevyWeekly}>
                    <XAxis dataKey="week" tickFormatter={fmtWeek} tick={{ fontSize: 11 }} axisLine={false} tickLine={false} />
                    <YAxis tick={{ fontSize: 11 }} axisLine={false} tickLine={false} width={24} />
                    <Tooltip content={({ active, payload, label }: any) => active && payload?.length ? (
                      <div className="rounded-xl border bg-card px-4 py-3 text-sm shadow-lg">
                        <p className="font-medium text-foreground mb-1">Нед. с {fmtWeek(label)}</p>
                        <p style={{ color: payload[0].color }}>Тренировок: {payload[0].value}</p>
                      </div>
                    ) : null} />
                    <Area type="monotone" dataKey="workouts" stroke="#8b5cf6" fill="#8b5cf6" fillOpacity={0.18} strokeWidth={2} />
                  </AreaChart>
                </ResponsiveContainer>
              )}
            </div>

            <div className="rounded-xl border bg-card p-5">
              <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Категории упражнений</h2>
              {loading || workoutCategoryPie.length === 0 ? <div className="h-48 flex items-center justify-center text-sm text-muted-foreground">Нет данных</div> : (
                <div className="flex items-center gap-6">
                  <div style={{ width: 180, height: 180, flexShrink: 0 }}>
                    <PieChart width={180} height={180}>
                      <Pie data={workoutCategoryPie} dataKey="value" cx={90} cy={90} innerRadius={50} outerRadius={80} paddingAngle={3}>
                        {workoutCategoryPie.map((segment, index) => <Cell key={index} fill={segment.color} />)}
                      </Pie>
                      <Tooltip formatter={(value: any) => `${value} упражнений`} />
                    </PieChart>
                  </div>
                  <div className="flex flex-col gap-2">
                    {workoutCategoryPie.map(segment => (
                      <div key={segment.name} className="flex items-center gap-2 text-xs px-1 py-0.5">
                        <div className="w-2.5 h-2.5 rounded-full" style={{ backgroundColor: segment.color }} />
                        <span className="text-foreground">🏋️ {segment.name}</span>
                        <span className="text-muted-foreground">{segment.value}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>

          <div className="rounded-xl border bg-card overflow-hidden">
            <div className="px-5 py-4 border-b">
              <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Лента Hevy</h2>
              <p className="text-xs text-muted-foreground mt-1">Последние тренировки{hevyTotal > workouts.length ? ' (показываем 30 последних)' : ''}</p>
            </div>

            <div className="divide-y max-h-[520px] overflow-y-auto">
              {loading ? (
                Array.from({ length: 3 }).map((_, index) => (
                  <div key={index} className="px-5 py-3 flex gap-3">
                    <div className="flex-1 h-4 bg-muted rounded animate-pulse" />
                  </div>
                ))
              ) : workouts.length === 0 ? (
                <div className="px-5 py-8 text-sm text-muted-foreground text-center">Нет данных</div>
              ) : (
                workouts.map(workout => <WorkoutRow key={workout.id} workout={workout} />)
              )}
            </div>
          </div>
        </>
      )}
    </div>
  )
}
