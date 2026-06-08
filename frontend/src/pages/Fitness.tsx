import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import type { EChartsCoreOption } from 'echarts/core'
import { Dumbbell, Flame, Heart, ChevronDown, ChevronUp, Timer, TrendingUp, Scale, Activity as ActivityIcon, Clock3, Layers3 } from 'lucide-react'
import { EditableWidgetGrid } from '@/components/EditableWidgetGrid'
import { EChart } from '@/components/EChart'
import { InfoTooltip } from '@/components/InfoTooltip'
import { PageSyncButton } from '@/components/PageSyncButton'
import { PageHeader } from '@/components/PageHeader'
import { useFitnessData } from '@/hooks/useFitnessData'
import { useGlobalDateRange } from '@/hooks/useGlobalDateRange'
import { useIntegrations } from '@/hooks/useIntegrations'
import { usePageSync } from '@/hooks/usePageSync'
import { CHART_GRID, CHART_MUTED, CHART_TEXT, CHART_TOOLTIP } from '@/lib/chart-theme'
import { cn, syncCaptionForSources } from '@/lib/utils'
import { type FitnessGoldenCard, type FitnessGoldenMetrics, type Workout } from '@/lib/api'
import { rawDataHref } from '@/lib/raw-data'

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
  const parts = iso.split('-').map(Number)
  if (parts.length === 3 && parts.every(Number.isFinite)) {
    return `${String(parts[2]).padStart(2, '0')}.${String(parts[1]).padStart(2, '0')}`
  }
  const d = new Date(iso)
  return `${String(d.getDate()).padStart(2, '0')}.${String(d.getMonth() + 1).padStart(2, '0')}`
}

function formatDateOnly(date: Date) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

function addDays(iso: string, days: number) {
  const [year, month, day] = iso.split('-').map(Number)
  const date = new Date(year, month - 1, day)
  date.setDate(date.getDate() + days)
  return formatDateOnly(date)
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

function toNumber(value: number | string | null | undefined) {
  if (typeof value === 'number') return value
  if (typeof value === 'string') {
    const parsed = Number(value)
    if (Number.isFinite(parsed)) return parsed
  }
  return 0
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function readString(value: unknown) {
  return typeof value === 'string' ? value : undefined
}

function readScalar(value: unknown) {
  if (typeof value === 'number' || typeof value === 'string' || value == null) return value
  return undefined
}

function toTooltipList(params: unknown) {
  return Array.isArray(params) ? params : [params]
}

function buildStravaGoldenTrendOption(data: FitnessGoldenMetrics['strava']['weekly']): EChartsCoreOption {
  return {
    color: ['#3b82f6', '#f97316', '#10b981'],
    animationDuration: 450,
    legend: {
      top: 0,
      itemWidth: 10,
      itemHeight: 10,
      textStyle: { color: CHART_MUTED, fontSize: 12 },
      data: ['Км', 'Активности', 'Активные дни'],
    },
    grid: { top: 40, right: 12, bottom: 12, left: 12, containLabel: true },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'line' },
      backgroundColor: CHART_TOOLTIP,
      borderColor: CHART_GRID,
      textStyle: { color: CHART_TEXT },
      formatter: (params: unknown) => {
        const points = toTooltipList(params)
          .map(item => {
            if (!isRecord(item)) return null
            return {
              marker: readString(item.marker) ?? '',
              seriesName: readString(item.seriesName) ?? '',
              value: readScalar(item.value),
              axisValue: readString(item.axisValue) ?? '',
            }
          })
          .filter(Boolean)

        const axisValue = points[0]?.axisValue ?? ''
        const lines = points.map(point =>
          `${point!.marker}${point!.seriesName}: ${point!.seriesName === 'Км'
            ? DECIMAL_FORMATTER.format(toNumber(point!.value))
            : toNumber(point!.value)}`
        )

        return [`<div>Неделя с ${fmtWeek(axisValue)}</div>`, ...lines].join('<br/>')
      },
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: data.map(point => point.week),
      axisLabel: { color: CHART_MUTED, formatter: (value: string) => fmtWeek(value) },
      axisLine: { lineStyle: { color: CHART_GRID } },
      axisTick: { show: false },
    },
    yAxis: [
      {
        type: 'value',
        name: 'Км',
        nameTextStyle: { color: CHART_MUTED, fontSize: 11, padding: [0, 0, 0, 8] },
        axisLabel: { color: CHART_MUTED, formatter: (value: number) => DECIMAL_FORMATTER.format(value) },
        splitLine: { lineStyle: { color: CHART_GRID } },
      },
      {
        type: 'value',
        name: 'Сессии',
        nameTextStyle: { color: CHART_MUTED, fontSize: 11, padding: [0, 0, 0, 8] },
        axisLabel: { color: CHART_MUTED },
        splitLine: { show: false },
      },
    ],
    series: [
      {
        name: 'Км',
        type: 'bar',
        yAxisIndex: 0,
        barMaxWidth: 24,
        itemStyle: { borderRadius: [8, 8, 0, 0] },
        data: data.map(point => point.km),
      },
      {
        name: 'Активности',
        type: 'line',
        yAxisIndex: 1,
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 2 },
        areaStyle: { color: 'rgba(249, 115, 22, 0.1)' },
        data: data.map(point => point.activities_count),
      },
      {
        name: 'Активные дни',
        type: 'line',
        yAxisIndex: 1,
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 2, type: 'dashed' },
        data: data.map(point => point.activity_days),
      },
    ],
  }
}

function buildHevyLoadOption(data: FitnessGoldenMetrics['hevy']['weekly']): EChartsCoreOption {
  return {
    color: ['#8b5cf6', '#22c55e'],
    animationDuration: 450,
    legend: {
      top: 0,
      itemWidth: 10,
      itemHeight: 10,
      textStyle: { color: CHART_MUTED, fontSize: 12 },
      data: ['Сеты', 'Тренировки'],
    },
    grid: { top: 40, right: 12, bottom: 12, left: 12, containLabel: true },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'line' },
      backgroundColor: CHART_TOOLTIP,
      borderColor: CHART_GRID,
      textStyle: { color: CHART_TEXT },
      formatter: (params: unknown) => {
        const points = toTooltipList(params)
          .map(item => {
            if (!isRecord(item)) return null
            return {
              marker: readString(item.marker) ?? '',
              seriesName: readString(item.seriesName) ?? '',
              value: readScalar(item.value),
              axisValue: readString(item.axisValue) ?? '',
            }
          })
          .filter(Boolean)

        const axisValue = points[0]?.axisValue ?? ''
        const lines = points.map(point => `${point!.marker}${point!.seriesName}: ${toNumber(point!.value)}`)
        return [`<div>Неделя с ${fmtWeek(axisValue)}</div>`, ...lines].join('<br/>')
      },
    },
    xAxis: {
      type: 'category',
      data: data.map(point => point.week),
      axisLabel: { color: CHART_MUTED, formatter: (value: string) => fmtWeek(value) },
      axisLine: { lineStyle: { color: CHART_GRID } },
      axisTick: { show: false },
    },
    yAxis: [
      {
        type: 'value',
        name: 'Сеты',
        nameTextStyle: { color: CHART_MUTED, fontSize: 11, padding: [0, 0, 0, 8] },
        axisLabel: { color: CHART_MUTED },
        splitLine: { lineStyle: { color: CHART_GRID } },
      },
      {
        type: 'value',
        name: 'Тренировки',
        nameTextStyle: { color: CHART_MUTED, fontSize: 11, padding: [0, 0, 0, 8] },
        axisLabel: { color: CHART_MUTED },
        splitLine: { show: false },
      },
    ],
    series: [
      {
        name: 'Сеты',
        type: 'bar',
        yAxisIndex: 0,
        barMaxWidth: 24,
        itemStyle: { borderRadius: [8, 8, 0, 0] },
        data: data.map(point => point.sets_count),
      },
      {
        name: 'Тренировки',
        type: 'line',
        yAxisIndex: 1,
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 2.5 },
        areaStyle: { color: 'rgba(34, 197, 94, 0.14)' },
        data: data.map(point => point.workouts_count),
      },
    ],
  }
}

function buildHevySplitTimelineOption(data: FitnessGoldenMetrics['hevy']['weekly']): EChartsCoreOption {
  return {
    color: ['#fb7185', '#38bdf8', '#34d399', '#94a3b8'],
    animationDuration: 450,
    legend: {
      top: 0,
      itemWidth: 10,
      itemHeight: 10,
      textStyle: { color: CHART_MUTED, fontSize: 12 },
      data: ['Push', 'Pull', 'Legs', 'Other'],
    },
    grid: { top: 40, right: 12, bottom: 12, left: 12, containLabel: true },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      backgroundColor: CHART_TOOLTIP,
      borderColor: CHART_GRID,
      textStyle: { color: CHART_TEXT },
      formatter: (params: unknown) => {
        const points = toTooltipList(params)
          .map(item => {
            if (!isRecord(item)) return null
            return {
              marker: readString(item.marker) ?? '',
              seriesName: readString(item.seriesName) ?? '',
              value: readScalar(item.value),
              axisValue: readString(item.axisValue) ?? '',
            }
          })
          .filter(Boolean)

        const axisValue = points[0]?.axisValue ?? ''
        const lines = points
          .filter(point => toNumber(point!.value) > 0)
          .map(point => `${point!.marker}${point!.seriesName}: ${toNumber(point!.value)}`)

        return [`<div>Неделя с ${fmtWeek(axisValue)}</div>`, ...lines].join('<br/>')
      },
    },
    xAxis: {
      type: 'category',
      data: data.map(point => point.week),
      axisLabel: { color: CHART_MUTED, formatter: (value: string) => fmtWeek(value) },
      axisLine: { lineStyle: { color: CHART_GRID } },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: CHART_MUTED },
      splitLine: { lineStyle: { color: CHART_GRID } },
    },
    series: [
      {
        name: 'Push',
        type: 'bar',
        stack: 'split',
        emphasis: { focus: 'series' },
        data: data.map(point => point.push_count),
      },
      {
        name: 'Pull',
        type: 'bar',
        stack: 'split',
        emphasis: { focus: 'series' },
        data: data.map(point => point.pull_count),
      },
      {
        name: 'Legs',
        type: 'bar',
        stack: 'split',
        emphasis: { focus: 'series' },
        data: data.map(point => point.legs_count),
      },
      {
        name: 'Other',
        type: 'bar',
        stack: 'split',
        emphasis: { focus: 'series' },
        data: data.map(point => point.other_count),
      },
    ],
  }
}

function buildDonutOption(data: Array<{ name: string; value: number; color: string }>, totalLabel: string, valueSuffix: string): EChartsCoreOption {
  const total = data.reduce((sum, point) => sum + point.value, 0)
  return {
    color: data.map(point => point.color),
    animationDuration: 450,
    tooltip: {
      trigger: 'item',
      backgroundColor: CHART_TOOLTIP,
      borderColor: CHART_GRID,
      textStyle: { color: CHART_TEXT },
      formatter: (param: unknown) => {
        if (!isRecord(param)) return ''
        const name = readString(param.name) ?? ''
        const value = toNumber(readScalar(param.value))
        const percent = toNumber(readScalar(param.percent))
        return `${name}: ${value}${valueSuffix} (${percent.toFixed(0)}%)`
      },
    },
    graphic: [
      {
        type: 'text',
        left: 'center',
        top: '42%',
        style: { text: totalLabel, textAlign: 'center', fill: CHART_MUTED, fontSize: 12 },
      },
      {
        type: 'text',
        left: 'center',
        top: '52%',
        style: { text: `${total}`, textAlign: 'center', fill: CHART_TEXT, fontSize: 16, fontWeight: 700 },
      },
    ],
    series: [
      {
        type: 'pie',
        radius: ['56%', '84%'],
        center: ['50%', '50%'],
        label: { show: false },
        labelLine: { show: false },
        itemStyle: { borderColor: '#162033', borderWidth: 2 },
        emphasis: { scale: true, scaleSize: 5 },
        data: data.map(point => ({ name: point.name, value: point.value })),
      },
    ],
  }
}

const GOLDEN_TONE_STYLES: Record<FitnessGoldenCard['tone'], string> = {
  success: 'border-emerald-500/20 bg-emerald-500/[0.08]',
  warning: 'border-amber-500/20 bg-amber-500/[0.08]',
  danger: 'border-rose-500/20 bg-rose-500/[0.08]',
  muted: 'border-border bg-card/90',
}

const GOLDEN_ICONS: Record<string, typeof TrendingUp> = {
  consistency: ActivityIcon,
  volume: Layers3,
  progression: TrendingUp,
  trend: TrendingUp,
  variety: Scale,
  balance: Scale,
  recency: Clock3,
}

function GoldenMetricCard({
  card,
  loading,
}: {
  card: FitnessGoldenCard
  loading: boolean
}) {
  const Icon = GOLDEN_ICONS[card.key] ?? Dumbbell
  return (
    <div className={cn('rounded-2xl border p-4 shadow-sm flex min-h-[148px] flex-col gap-3', GOLDEN_TONE_STYLES[card.tone])}>
      <div className="flex items-start justify-between gap-3">
        <div className="space-y-1">
          <span className="inline-flex items-center gap-2 text-[10px] uppercase tracking-[0.24em] text-muted-foreground">
            {card.title}
            {!loading ? <InfoTooltip text={card.detail} className="normal-case tracking-normal" /> : null}
          </span>
          {loading ? <div className="h-7 w-24 bg-muted rounded animate-pulse" /> : (
            <div className="text-2xl font-semibold tracking-tight text-foreground">{card.value}</div>
          )}
        </div>
        <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-background/60">
          <Icon className="h-4 w-4 text-foreground/80" />
        </div>
      </div>
      {loading ? <div className="h-4 w-full bg-muted rounded animate-pulse" /> : null}
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
  const globalRange = useGlobalDateRange()
  const navigate = useNavigate()
  const { summary, golden, activities, workouts, loading, reload: reloadFitnessData } = useFitnessData(globalRange.params)
  const [typeFilter, setTypeFilter] = useState('')
  const [sourceTab, setSourceTab] = useState<FitnessSource>('strava')
  const { integrations, reload: reloadIntegrations } = useIntegrations()

  const reloadFitness = useCallback(async () => {
    await Promise.all([reloadFitnessData(), reloadIntegrations()])
  }, [reloadFitnessData, reloadIntegrations])

  const { syncing, syncSources } = usePageSync(reloadFitness)

  useEffect(() => {
    if (!summary) return
    if (summary.activities_total === 0 && summary.workouts_total > 0) {
      const timer = window.setTimeout(() => setSourceTab('hevy'), 0)
      return () => window.clearTimeout(timer)
    }
    return undefined
  }, [summary])

  const stravaTotal = summary?.activities_total ?? activities.length
  const hevyTotal = summary?.workouts_total ?? workouts.length
  const activeIntegration = integrations.find(i => i.name === sourceTab)
  const fitnessSyncCaption = syncCaptionForSources(activeIntegration?.enabled ? [activeIntegration] : [])

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

  const stravaGoldenWeekly = golden?.strava.weekly ?? []
  const hevyGoldenWeekly = golden?.hevy.weekly ?? []

  const cards = sourceTab === 'strava' ? golden?.strava.cards ?? [] : golden?.hevy.cards ?? []

  function openFitnessRaw(source: 'fitness.activities' | 'fitness.workouts', filters: Record<string, string | undefined> = {}) {
    navigate(rawDataHref(source, { ...globalRange.params, ...filters }))
  }

  async function handleSyncFitness() {
    if (!activeIntegration?.enabled) return
    await syncSources(sourceTab)
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-4">
        <PageHeader
          eyebrow="Movement"
          title="Фитнес"
          description="Strava отвечает за активности, Hevy за силовые тренировки. Переключай источник, чтобы смотреть именно тот тип нагрузки, который сейчас анализируешь."
          badges={[
            { label: activeIntegration?.enabled ? `${activeIntegration.display_name} подключён` : 'Источник не подключён', tone: activeIntegration?.enabled ? 'success' : 'warning' },
            { label: sourceTab === 'strava' ? `${stravaTotal} активностей` : `${hevyTotal} тренировок`, tone: 'muted' },
          ]}
          actions={(
            <PageSyncButton
              label={`Синхронизировать ${sourceTab === 'strava' ? 'Strava' : 'Hevy'}`}
              syncCaption={fitnessSyncCaption}
              syncing={syncing}
              disabled={!activeIntegration?.enabled}
              onClick={handleSyncFitness}
            />
          )}
        />

        <div className="flex flex-wrap gap-2">
          {[
            { key: 'strava' as const, label: 'Strava', count: stravaTotal, helper: 'активности' },
            { key: 'hevy' as const, label: 'Hevy', count: hevyTotal, helper: 'тренировки' },
          ].map(tab => (
            <button
              key={tab.key}
              onClick={() => setSourceTab(tab.key)}
              className={cn(
                'rounded-2xl border px-4 py-3 text-left transition-colors min-w-[180px] shadow-sm',
                sourceTab === tab.key ? 'border-primary/30 bg-primary/10' : 'bg-card/90 hover:bg-muted/40',
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

      <EditableWidgetGrid
        storageKey={`fitness_${sourceTab}_widget_layout_v1`}
        widgets={[
          { id: 'golden', label: 'Ключевые метрики', layout: { x: 0, y: 0, w: 12, h: 5 }, bounds: { minW: 4, minH: 4, maxH: 10 } },
          { id: 'source-details', label: sourceTab === 'strava' ? 'Strava widgets' : 'Hevy widgets', layout: { x: 0, y: 5, w: 12, h: 18 }, bounds: { minW: 6, minH: 8, maxH: 40 } },
        ]}
      >
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-5">
        {(loading && cards.length === 0
          ? Array.from({ length: 5 }).map((_, index) => ({ key: `skeleton-${index}`, title: '—', value: '—', detail: '—', tone: 'muted' as const }))
          : cards
        ).map(card => (
          <GoldenMetricCard
            key={card.key}
            card={card}
            loading={loading}
          />
        ))}
      </div>

      {sourceTab === 'strava' ? (
        <>
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2 lg:items-start">
            <div className="rounded-2xl border bg-card/90 p-5 shadow-sm">
              <div className="mb-4">
                <h2 className="inline-flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-foreground">
                  Consistency и объём
                  <InfoTooltip text="Километры, количество сессий и активных дней по неделям. Это быстрее показывает просадку режима, чем голый total." />
                </h2>
              </div>
              {loading ? <div className="h-48 bg-muted rounded animate-pulse" /> : stravaGoldenWeekly.length === 0 ? (
                <p className="text-sm text-muted-foreground text-center py-8">Нет данных</p>
              ) : (
                <EChart
                  option={buildStravaGoldenTrendOption(stravaGoldenWeekly)}
                  height={240}
                  onClick={(params) => {
                    const week = String(params.name ?? '')
                    if (week) openFitnessRaw('fitness.activities', { from: week, to: addDays(week, 6) })
                  }}
                />
              )}
            </div>

            <div className="rounded-2xl border bg-card/90 p-5 shadow-sm">
              <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Типы активностей</h2>
              {loading || activityTypePie.length === 0 ? <div className="h-48 flex items-center justify-center text-sm text-muted-foreground">Нет данных</div> : (
                <div className="flex items-start gap-6">
                  <EChart
                    option={buildDonutOption(activityTypePie, 'Всего', ' активностей')}
                    height={180}
                    width={180}
                    className="shrink-0"
                    onClick={(params) => {
                      const type = String(params.name ?? '')
                      if (type) openFitnessRaw('fitness.activities', { type })
                    }}
                  />
                  <div className="flex max-h-[220px] min-w-0 flex-1 flex-col gap-2 overflow-y-auto py-1 pr-1">
                    {activityTypePie.map(segment => (
                      <button
                        key={segment.name}
                        onClick={() => openFitnessRaw('fitness.activities', { type: segment.name })}
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

          <div className="rounded-2xl border bg-card/90 overflow-hidden shadow-sm">
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
          <div className="grid gap-6 lg:grid-cols-[1.2fr,0.8fr] lg:items-start">
            <div className="rounded-2xl border bg-card/90 p-5 shadow-sm">
              <div className="mb-4">
                <h2 className="inline-flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-foreground">
                  Прогресс по упражнениям
                  <InfoTooltip text="Смотрим последние рабочие сеты по одним и тем же упражнениям и показываем, где вес или повторы реально выросли." />
                </h2>
              </div>
              {loading ? (
                <div className="space-y-3">
                  {Array.from({ length: 3 }).map((_, index) => <div key={index} className="h-16 rounded-xl bg-muted animate-pulse" />)}
                </div>
              ) : (golden?.hevy.progressions.length ?? 0) === 0 ? (
                <div className="rounded-2xl border border-dashed border-border/80 bg-background/30 px-4 py-8 text-sm text-muted-foreground">
                  Пока не хватает повторяющихся рабочих сетов, чтобы честно показать progression.
                </div>
              ) : (
                <div className="space-y-3">
                  {golden?.hevy.progressions.map((progress) => (
                    <div key={`${progress.exercise}-${progress.latest}`} className="rounded-2xl border border-emerald-500/15 bg-emerald-500/[0.06] px-4 py-3">
                      <div className="flex items-start justify-between gap-3">
                        <div>
                          <p className="text-sm font-medium text-foreground">{progress.exercise}</p>
                          <p className="mt-1 text-xs text-muted-foreground">{progress.previous} → {progress.latest}</p>
                        </div>
                        <span className="rounded-full border border-emerald-500/20 bg-emerald-500/10 px-2.5 py-1 text-xs font-medium text-emerald-300">
                          {progress.delta}
                        </span>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div className="rounded-2xl border bg-card/90 p-5 shadow-sm">
              <div className="mb-4">
                <h2 className="inline-flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-foreground">
                  Баланс сплита по неделям
                  <InfoTooltip text="Не просто итоговый push / pull / legs, а как сплит складывался неделя за неделей. Так видно перекос, а не только финальную сумму." />
                </h2>
              </div>
              {loading ? <div className="h-64 bg-muted rounded animate-pulse" /> : hevyGoldenWeekly.length === 0 ? (
                <div className="rounded-2xl border border-dashed border-border/80 bg-background/30 px-4 py-8 text-sm text-muted-foreground">
                  Пока не хватает данных, чтобы показать ритм сплита по неделям.
                </div>
              ) : (
                <div className="space-y-4">
                  <EChart
                    option={buildHevySplitTimelineOption(hevyGoldenWeekly)}
                    height={260}
                    onClick={(params) => {
                      const week = String(params.name ?? '')
                      const split = String(params.seriesName ?? '').toLowerCase()
                      if (week) openFitnessRaw('fitness.workouts', { from: week, to: addDays(week, 6), split: split || undefined })
                    }}
                  />
                  <div className="flex flex-wrap gap-2">
                    {(golden?.hevy.splits ?? []).filter(split => split.count > 0 || split.key !== 'other').map(split => (
                      <div key={split.key} className="rounded-full border border-border/80 bg-background/60 px-3 py-1.5 text-xs text-muted-foreground">
                        <span className="font-medium text-foreground">{split.label}</span> · {split.count}
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>

          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2 lg:items-start">
            <div className="rounded-2xl border bg-card/90 p-5 shadow-sm">
              <div className="mb-4">
                <h2 className="inline-flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-foreground">
                  Нагрузка по неделям
                  <InfoTooltip text="Количество тренировок само по себе слабо говорит о нагрузке. Здесь видно, сколько сетов реально набежало за неделю." />
                </h2>
              </div>
              {loading ? <div className="h-48 bg-muted rounded animate-pulse" /> : hevyGoldenWeekly.length === 0 ? (
                <p className="text-sm text-muted-foreground text-center py-8">Нет данных</p>
              ) : (
                <EChart
                  option={buildHevyLoadOption(hevyGoldenWeekly)}
                  height={240}
                  onClick={(params) => {
                    const week = String(params.name ?? '')
                    if (week) openFitnessRaw('fitness.workouts', { from: week, to: addDays(week, 6) })
                  }}
                />
              )}
            </div>

            <div className="rounded-2xl border bg-card/90 p-5 shadow-sm">
              <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Категории упражнений</h2>
              {loading || workoutCategoryPie.length === 0 ? <div className="h-48 flex items-center justify-center text-sm text-muted-foreground">Нет данных</div> : (
                <div className="flex items-start gap-6">
                  <EChart
                    option={buildDonutOption(workoutCategoryPie, 'Всего', ' упражнений')}
                    height={180}
                    width={180}
                    className="shrink-0"
                    onClick={(params) => {
                      const category = String(params.name ?? '')
                      if (category) openFitnessRaw('fitness.workouts', { category })
                    }}
                  />
                  <div className="flex max-h-[220px] min-w-0 flex-1 flex-col gap-2 overflow-y-auto py-1 pr-1">
                    {workoutCategoryPie.map(segment => (
                      <button key={segment.name} onClick={() => openFitnessRaw('fitness.workouts', { category: segment.name })} className="flex items-center gap-2 rounded px-1 py-0.5 text-left text-xs transition-colors hover:bg-accent/50">
                        <div className="w-2.5 h-2.5 rounded-full" style={{ backgroundColor: segment.color }} />
                        <span className="text-foreground">🏋️ {segment.name}</span>
                        <span className="text-muted-foreground">{segment.value}</span>
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>

          <div className="rounded-2xl border bg-card/90 overflow-hidden shadow-sm">
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
      </EditableWidgetGrid>
    </div>
  )
}
