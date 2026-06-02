import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import type { EChartsCoreOption } from 'echarts/core'
import { Activity, Droplets, Flame, GlassWater, RotateCcw, Target, UtensilsCrossed } from 'lucide-react'
import { EChart } from '@/components/EChart'
import { ExpandablePanel } from '@/components/ExpandablePanel'
import { PageSyncButton } from '@/components/PageSyncButton'
import { PageHeader } from '@/components/PageHeader'
import { useGlobalDateRange } from '@/hooks/useGlobalDateRange'
import { cn, syncCaptionForSources } from '@/lib/utils'
import { api, type HydrationBeverageType, type HydrationMode, type Integration, type NutritionDay, type NutritionGoldenCard, type NutritionGoldenMetrics, type NutritionSummary, type NutritionTargetsInput } from '@/lib/api'
import { rawDataHref } from '@/lib/raw-data'

const MEAL_LABELS: Record<string, string> = {
  breakfast: 'Завтрак', lunch: 'Обед', dinner: 'Ужин', snacks: 'Перекус', other: 'Прочее',
}
const MEAL_KEYS_BY_LABEL = Object.fromEntries(Object.entries(MEAL_LABELS).map(([key, label]) => [label, key]))

const MEAL_COLORS: Record<string, string> = {
  breakfast: '#f97316',
  lunch: '#10b981',
  dinner: '#3b82f6',
  snacks: '#8b5cf6',
  other: '#f43f5e',
}

const MEAL_ORDER = ['breakfast', 'lunch', 'dinner', 'snacks', 'other'] as const

const MACRO_COLORS = { protein: '#3b82f6', fat: '#f97316', carbs: '#10b981', fiber: '#8b5cf6' }
const HYDRATION_MODE_LABELS: Record<HydrationMode, string> = {
  strict: 'Строго',
  flexible: 'Гибко',
}
const HYDRATION_MODE_NOTES: Record<HydrationMode, string> = {
  strict: 'В цель воды идёт только чистая вода.',
  flexible: 'В цель идут вода, чай и кофе. Энергетики и молочные напитки считаются отдельно.',
}
const HYDRATION_MODE_ACCENT: Record<HydrationMode, string> = {
  strict: 'border-cyan-500/20 bg-cyan-500/10 text-cyan-200',
  flexible: 'border-emerald-500/20 bg-emerald-500/10 text-emerald-200',
}
const HYDRATION_BEVERAGES: Array<{ type: HydrationBeverageType; label: string; short: string; emoji: string; amount: number; color: string }> = [
  { type: 'tea', label: 'Чай', short: 'Чай', emoji: '🍵', amount: 250, color: '#10b981' },
  { type: 'coffee', label: 'Кофе', short: 'Кофе', emoji: '☕', amount: 250, color: '#c084fc' },
  { type: 'energy', label: 'Энергетик', short: 'Энерджи', emoji: '⚡', amount: 250, color: '#f59e0b' },
  { type: 'milkshake', label: 'Коктейль', short: 'Коктейль', emoji: '🥤', amount: 330, color: '#fb7185' },
  { type: 'other', label: 'Прочее', short: 'Прочее', emoji: '🧃', amount: 250, color: '#94a3b8' },
]
const CHART_TEXT = '#e5eefc'
const CHART_MUTED = 'rgba(148, 163, 184, 0.85)'
const CHART_GRID = 'rgba(148, 163, 184, 0.12)'
const CHART_TOOLTIP = 'rgba(15, 23, 42, 0.96)'

function fmtDate(iso: string) {
  return new Date(iso).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })
}

function fmtShort(iso: string) {
  const d = new Date(iso)
  return `${d.getDate()}.${String(d.getMonth() + 1).padStart(2, '0')}`
}

const PERIODS = [
  { label: '7д', days: 7 },
  { label: '14д', days: 14 },
  { label: '30д', days: 30 },
  { label: '90д', days: 90 },
]

function fmtWeight(value?: number) {
  return typeof value === 'number' ? `${value.toFixed(1)} кг` : '—'
}

function fmtTargetDelta(current?: number, target?: number) {
  if (typeof current !== 'number' || typeof target !== 'number') return '—'
  const delta = current - target
  if (Math.abs(delta) < 0.05) return 'цель достигнута'
  return delta > 0 ? `сбросить ${delta.toFixed(1)} кг` : `набрать ${Math.abs(delta).toFixed(1)} кг`
}

function fmtSyncTime(value?: string) {
  if (!value) return '—'
  return new Date(value).toLocaleString('ru-RU', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })
}

function fmtOptionalNumber(value?: number, unit = '') {
  return typeof value === 'number' ? `${value.toFixed(0)}${unit}` : '—'
}

function fmtWaterMl(value?: number) {
  return typeof value === 'number' ? `${Math.round(value)} мл` : '—'
}

function hydrationBeverageLabel(type: HydrationBeverageType) {
  return HYDRATION_BEVERAGES.find(item => item.type === type)?.label ?? type
}

function hydrationBeverageEmoji(type: HydrationBeverageType) {
  return HYDRATION_BEVERAGES.find(item => item.type === type)?.emoji ?? '🧃'
}

function hydrationModeLabel(mode: HydrationMode | undefined) {
  return HYDRATION_MODE_LABELS[mode ?? 'strict']
}

function numberInputValue(value?: number) {
  return typeof value === 'number' ? String(value) : ''
}

function parseRequiredDecimalOrNull(label: string, value: string) {
  const trimmed = value.trim().replace(',', '.')
  if (!trimmed) return null
  const parsed = Number(trimmed)
  if (!Number.isFinite(parsed)) {
    throw new Error(`Некорректное значение для поля «${label}»`)
  }
  return parsed
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

function buildCaloriesOption(data: NutritionDay[], calorieTarget?: number): EChartsCoreOption {
  return {
    color: ['#f97316'],
    animationDuration: 450,
    grid: { top: 24, right: 12, bottom: 12, left: 12, containLabel: true },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      backgroundColor: CHART_TOOLTIP,
      borderColor: CHART_GRID,
      textStyle: { color: CHART_TEXT },
      formatter: (params: unknown) => {
        const point = toTooltipList(params)[0]
        if (!isRecord(point)) return ''
        return [
          `<div>${fmtDate(readString(point.axisValue) ?? '')}</div>`,
          `${readString(point.marker) ?? ''}${toNumber(readScalar(point.value)).toFixed(0)} ккал`,
        ].join('<br/>')
      },
    },
    xAxis: {
      type: 'category',
      data: data.map(point => point.date),
      axisLabel: { color: CHART_MUTED, formatter: (value: string) => fmtShort(value) },
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
        type: 'bar',
        barMaxWidth: 26,
        itemStyle: {
          borderRadius: [6, 6, 0, 0],
          color: (params: { dataIndex: number }) => {
            const item = data[params.dataIndex]
            return typeof calorieTarget === 'number' && item.calories > calorieTarget ? '#f87171' : '#f97316'
          },
        },
        markLine: typeof calorieTarget === 'number' ? {
          symbol: 'none',
          lineStyle: { color: '#fbbf24', type: 'dashed', width: 1.5 },
          label: { color: '#fbbf24', formatter: `Цель ${calorieTarget.toFixed(0)} ккал` },
          data: [{ yAxis: calorieTarget }],
        } : undefined,
        data: data.map(point => point.calories),
      },
    ],
  }
}

function buildMacrosTrendOption(data: NutritionDay[]): EChartsCoreOption {
  return {
    color: [MACRO_COLORS.protein, MACRO_COLORS.fat, MACRO_COLORS.carbs],
    animationDuration: 450,
    legend: {
      top: 0,
      itemWidth: 10,
      itemHeight: 10,
      textStyle: { color: CHART_MUTED, fontSize: 12 },
      data: ['Белки', 'Жиры', 'Углеводы'],
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

        const lines = points.map(point => `${point!.marker}${point!.seriesName}: ${toNumber(point!.value).toFixed(0)} г`)
        return [`<div>${fmtDate(points[0]?.axisValue ?? '')}</div>`, ...lines].join('<br/>')
      },
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: data.map(point => point.date),
      axisLabel: { color: CHART_MUTED, formatter: (value: string) => fmtShort(value) },
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
        name: 'Белки',
        type: 'line',
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 2 },
        areaStyle: { color: 'rgba(59, 130, 246, 0.12)' },
        data: data.map(point => point.protein),
      },
      {
        name: 'Жиры',
        type: 'line',
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 2 },
        areaStyle: { color: 'rgba(249, 115, 22, 0.10)' },
        data: data.map(point => point.fat),
      },
      {
        name: 'Углеводы',
        type: 'line',
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 2 },
        areaStyle: { color: 'rgba(16, 185, 129, 0.10)' },
        data: data.map(point => point.carbs),
      },
    ],
  }
}

function buildHydrationOption(data: NutritionDay[], waterTarget?: number): EChartsCoreOption {
  const hasCountedDrinks = data.some(point => point.counted_drinks_ml > 0)
  const hasOtherDrinks = data.some(point => point.other_drinks_ml > 0)
  const series: Array<Record<string, unknown>> = [
    {
      name: 'Вода',
      type: 'bar',
      stack: 'hydration',
      barMaxWidth: 22,
      itemStyle: { borderRadius: [4, 4, 0, 0], color: '#38bdf8' },
      data: data.map(point => point.water_ml),
    },
  ]
  if (hasCountedDrinks) {
    series.push({
      name: 'Чай / кофе',
      type: 'bar',
      stack: 'hydration',
      barMaxWidth: 22,
      itemStyle: { borderRadius: [4, 4, 0, 0], color: '#10b981' },
      data: data.map(point => point.counted_drinks_ml),
    })
  }
  if (hasOtherDrinks) {
    series.push({
      name: 'Прочие напитки',
      type: 'bar',
      stack: 'other',
      barMaxWidth: 22,
      itemStyle: { borderRadius: [4, 4, 0, 0], color: '#f59e0b' },
      data: data.map(point => point.other_drinks_ml),
    })
  }
  return {
    color: ['#38bdf8', '#10b981', '#f59e0b'],
    animationDuration: 450,
    legend: {
      top: 0,
      itemWidth: 10,
      itemHeight: 10,
      textStyle: { color: CHART_MUTED, fontSize: 12 },
      data: [...series.map(item => String(item.name)), 'Гидратация'],
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
              value: toNumber(readScalar(item.value)),
              axisValue: readString(item.axisValue) ?? '',
            }
          })
          .filter(Boolean)
        const hydrationPoint = points.find(point => point!.seriesName === 'Гидратация')
        const lines = points
          .filter(point => point!.seriesName !== 'Гидратация' && point!.value > 0)
          .map(point => `${point!.marker}${point!.seriesName}: ${point!.value.toFixed(0)} мл`)
        return [
          `<div>${fmtDate(points[0]?.axisValue ?? '')}</div>`,
          ...lines,
          `<div style="margin-top:6px;font-weight:600">Итого в цель: ${hydrationPoint ? hydrationPoint.value.toFixed(0) : '0'} мл</div>`,
        ].join('<br/>')
      },
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: data.map(point => point.date),
      axisLabel: { color: CHART_MUTED, formatter: (value: string) => fmtShort(value) },
      axisLine: { lineStyle: { color: CHART_GRID } },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: CHART_MUTED, formatter: (value: number) => `${value.toFixed(0)} мл` },
      splitLine: { lineStyle: { color: CHART_GRID } },
    },
    series: [
      ...series,
      {
        name: 'Гидратация',
        type: 'line',
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 2.5, color: '#86efac' },
        data: data.map(point => point.hydration_ml),
        markLine: typeof waterTarget === 'number' ? {
          symbol: 'none',
          lineStyle: { color: '#22c55e', type: 'dashed', width: 1.5 },
          label: { color: '#86efac', formatter: `Цель ${Math.round(waterTarget)} мл` },
          data: [{ yAxis: waterTarget }],
        } : undefined,
      },
    ],
  }
}

function buildNutritionDonutOption(data: Array<{ name: string; value: number; color: string }>, centerLabel: string, suffix: string): EChartsCoreOption {
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
        return `${name}: ${value.toFixed(0)}${suffix} (${percent.toFixed(0)}%)`
      },
    },
    graphic: [
      {
        type: 'text',
        left: 'center',
        top: '42%',
        style: { text: centerLabel, textAlign: 'center', fill: CHART_MUTED, fontSize: 12 },
      },
      {
        type: 'text',
        left: 'center',
        top: '52%',
        style: { text: `${Math.round(total)}`, textAlign: 'center', fill: CHART_TEXT, fontSize: 16, fontWeight: 700 },
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

function buildMealsTimelineOption(
  data: NutritionDay[],
  mealStats: Array<{ key: string; name: string; color: string }>,
): EChartsCoreOption {
  return {
    color: mealStats.map(stat => stat.color),
    animationDuration: 450,
    legend: {
      top: 0,
      itemWidth: 10,
      itemHeight: 10,
      textStyle: { color: CHART_MUTED, fontSize: 12 },
      data: mealStats.map(stat => stat.name),
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
              value: toNumber(readScalar(item.value)),
              axisValue: readString(item.axisValue) ?? '',
            }
          })
          .filter(point => point && point.value > 0)

        const total = points.reduce((sum, point) => sum + point!.value, 0)
        const lines = points.map(point => `${point!.marker}${point!.seriesName}: ${point!.value.toFixed(0)} ккал`)
        return [
          `<div>${fmtDate(points[0]?.axisValue ?? '')}</div>`,
          ...lines,
          `<div style="margin-top:6px;font-weight:600">Итого: ${total.toFixed(0)} ккал</div>`,
        ].join('<br/>')
      },
    },
    xAxis: {
      type: 'category',
      data: data.map(point => point.date),
      axisLabel: { color: CHART_MUTED, formatter: (value: string) => fmtShort(value) },
      axisLine: { lineStyle: { color: CHART_GRID } },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: CHART_MUTED },
      splitLine: { lineStyle: { color: CHART_GRID } },
    },
    series: mealStats.map(stat => ({
      name: stat.name,
      type: 'bar',
      stack: 'meals',
      barMaxWidth: 24,
      emphasis: { focus: 'series' },
      itemStyle: { borderRadius: [4, 4, 0, 0] },
      data: data.map(day => {
        const meal = day.meals.find(entry => entry.meal_type === stat.key)
        return meal ? meal.items.reduce((sum, item) => sum + item.calories, 0) : 0
      }),
    })),
  }
}

const GOLDEN_TONE_STYLES: Record<NutritionGoldenCard['tone'], string> = {
  success: 'border-emerald-500/20 bg-emerald-500/[0.08]',
  warning: 'border-amber-500/20 bg-amber-500/[0.08]',
  danger: 'border-rose-500/20 bg-rose-500/[0.08]',
  muted: 'border-border bg-card/90',
}

const GOLDEN_ICONS: Record<string, typeof Activity> = {
  consistency: Activity,
  calories: Flame,
  protein: Target,
  hydration: Droplets,
  structure: UtensilsCrossed,
}

function NutritionGoldenMetricCard({
  card,
  loading,
}: {
  card: NutritionGoldenCard
  loading: boolean
}) {
  const Icon = GOLDEN_ICONS[card.key] ?? Target
  return (
    <div className={cn('rounded-2xl border p-4 shadow-sm flex min-h-[148px] flex-col gap-3', GOLDEN_TONE_STYLES[card.tone])}>
      <div className="flex items-start justify-between gap-3">
        <div className="space-y-1">
          <span className="text-[10px] uppercase tracking-[0.24em] text-muted-foreground">{card.title}</span>
          {loading ? <div className="h-7 w-24 bg-muted rounded animate-pulse" /> : (
            <div className="text-2xl font-semibold tracking-tight text-foreground">{card.value}</div>
          )}
        </div>
        <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-background/60">
          <Icon className="h-4 w-4 text-foreground/80" />
        </div>
      </div>
      {loading ? <div className="h-4 w-full bg-muted rounded animate-pulse" /> : (
        <p className="text-sm leading-6 text-muted-foreground">{card.detail}</p>
      )}
    </div>
  )
}

function filterNutritionDayByMeal(day: NutritionDay, mealType: string): NutritionDay | null {
  if (!mealType) return day

  const meals = day.meals.filter(meal => meal.meal_type === mealType)
  if (meals.length === 0) return null

  let calories = 0
  let protein = 0
  let carbs = 0
  let fat = 0
  let fiber = 0

  meals.forEach(meal => {
    meal.items.forEach(item => {
      calories += item.calories
      protein += item.macros?.protein ?? 0
      carbs += item.macros?.carbs ?? 0
      fat += item.macros?.fat ?? 0
      fiber += item.macros?.fiber ?? 0
    })
  })

  return {
    ...day,
    calories,
    protein,
    carbs,
    fat,
    fiber,
    water_ml: day.water_ml,
    hydration_ml: day.hydration_ml,
    counted_drinks_ml: day.counted_drinks_ml,
    other_drinks_ml: day.other_drinks_ml,
    beverages: day.beverages ?? [],
    meals,
  }
}

function buildDailyNutritionTimelineOption(
  data: NutritionDay[],
  calorieReference: number,
  calorieTarget: number | undefined,
  selectedDate: string | null,
): EChartsCoreOption {
  const reference = Math.max(
    calorieReference,
    calorieTarget ?? 0,
    ...data.map(day => day.calories),
    1,
  )
  const roundedMax = Math.ceil(reference / 250) * 250

  return {
    animationDuration: 350,
    grid: { top: 12, right: 84, bottom: 12, left: 12, containLabel: true },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      backgroundColor: CHART_TOOLTIP,
      borderColor: CHART_GRID,
      textStyle: { color: CHART_TEXT },
      formatter: (params: unknown) => {
        const points = toTooltipList(params)
        const barPoint = points.find(item => isRecord(item) && readString(item.seriesName) === 'Калории')
        const dataIndex = isRecord(barPoint) ? toNumber(readScalar(barPoint.dataIndex)) : 0
        const day = data[dataIndex]
        if (!day) return ''

        const beverageLines = (day.beverages ?? []).map(beverage => {
          const prefix = beverage.counts_toward_goal ? 'в цель' : 'отдельно'
          return `${hydrationBeverageEmoji(beverage.beverage_type)} ${hydrationBeverageLabel(beverage.beverage_type)}: ${Math.round(beverage.amount_ml)} мл · ${prefix}`
        })

        return [
          `<div>${fmtDate(day.date)}</div>`,
          `<div style="margin-top:4px;font-weight:600">${Math.round(day.calories)} ккал</div>`,
          `<div>Б ${Math.round(day.protein)} г · Ж ${Math.round(day.fat)} г · У ${Math.round(day.carbs)} г${day.fiber > 0 ? ` · К ${Math.round(day.fiber)} г` : ''}</div>`,
          `<div>💧 Вода: ${Math.round(day.water_ml)} мл · гидратация: ${Math.round(day.hydration_ml)} мл</div>`,
          ...beverageLines.map(line => `<div>${line}</div>`),
          `<div style="margin-top:6px;color:${CHART_MUTED}">Кликни по дню, чтобы посмотреть состав ниже</div>`,
        ].join('<br/>')
      },
    },
    xAxis: {
      type: 'value',
      max: roundedMax,
      axisLabel: { color: CHART_MUTED, formatter: (value: number) => `${Math.round(value)}` },
      splitLine: { lineStyle: { color: CHART_GRID } },
    },
    yAxis: {
      type: 'category',
      data: data.map(day => day.date),
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: {
        color: CHART_TEXT,
        formatter: (value: string) => fmtDate(value),
      },
    },
    series: [
      {
        name: 'Коридор',
        type: 'bar',
        silent: true,
        barWidth: 12,
        barGap: '-100%',
        itemStyle: {
          color: 'rgba(148, 163, 184, 0.12)',
          borderRadius: 999,
        },
        data: data.map(() => calorieTarget ?? roundedMax),
      },
      {
        name: 'Калории',
        type: 'bar',
        barWidth: 12,
        z: 3,
        label: {
          show: true,
          position: 'right',
          distance: 10,
          color: CHART_TEXT,
          fontSize: 12,
          formatter: ({ value }: { value: number }) => `${Math.round(value)} ккал`,
        },
        itemStyle: {
          borderRadius: 999,
          color: (params: { dataIndex: number }) => {
            const day = data[params.dataIndex]
            const isSelected = day?.date === selectedDate
            const isOver = typeof calorieTarget === 'number' && day.calories > calorieTarget
            if (isOver && isSelected) return '#fb7185'
            if (isOver) return 'rgba(251, 113, 133, 0.72)'
            if (isSelected) return '#7dd3fc'
            return 'rgba(125, 211, 252, 0.72)'
          },
          shadowBlur: 12,
          shadowColor: 'rgba(14, 165, 233, 0.14)',
        },
        emphasis: {
          itemStyle: {
            opacity: 1,
          },
        },
        data: data.map(day => day.calories),
      },
    ],
  }
}

function NutritionDayDetails({ day }: { day: NutritionDay }) {
  const beverages = day.beverages ?? []

  return (
    <div className="rounded-2xl border bg-background/35 p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-sm font-semibold text-foreground">{fmtDate(day.date)}</p>
          <p className="mt-1 text-xs text-muted-foreground">
            Б {Math.round(day.protein)} г · Ж {Math.round(day.fat)} г · У {Math.round(day.carbs)} г{day.fiber > 0 ? ` · К ${Math.round(day.fiber)} г` : ''}
          </p>
        </div>
        <div className="text-right">
          <p className="text-lg font-semibold text-foreground">{Math.round(day.calories)} ккал</p>
          <p className="mt-1 text-xs text-muted-foreground">гидратация {Math.round(day.hydration_ml)} мл</p>
        </div>
      </div>

      <div className="mt-3 flex flex-wrap gap-2 text-xs text-muted-foreground">
        {day.water_ml > 0 ? <span className="rounded-full border border-cyan-500/20 bg-cyan-500/10 px-2.5 py-1 text-cyan-100">💧 Вода {Math.round(day.water_ml)} мл</span> : null}
        {beverages.map(beverage => (
          <span
            key={beverage.beverage_type}
            className={cn(
              'rounded-full border px-2.5 py-1',
              beverage.counts_toward_goal
                ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-100'
                : 'border-amber-500/20 bg-amber-500/10 text-amber-100',
            )}
          >
            {hydrationBeverageEmoji(beverage.beverage_type)} {hydrationBeverageLabel(beverage.beverage_type)} {Math.round(beverage.amount_ml)} мл
          </span>
        ))}
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        {day.meals.map(meal => (
          <div key={meal.meal_type} className="rounded-xl border bg-background/45 p-3">
            <div className="mb-2 flex items-center justify-between gap-2">
              <p className="text-xs font-semibold uppercase tracking-wide text-foreground">{MEAL_LABELS[meal.meal_type] ?? meal.meal_type}</p>
              <span className="text-xs text-muted-foreground">
                {Math.round(meal.items.reduce((sum, item) => sum + item.calories, 0))} ккал
              </span>
            </div>
            <div className="flex flex-col gap-2">
              {meal.items.map((item, idx) => (
                <div key={idx} className="rounded-lg border border-border/70 bg-background/60 px-3 py-2">
                  <div className="flex items-center justify-between gap-3 text-xs">
                    <span className="min-w-0 flex-1 truncate text-foreground">{item.food_name}</span>
                    <span className="shrink-0 text-muted-foreground">{item.serving}</span>
                    <span className="shrink-0 font-medium text-foreground">{Math.round(item.calories)} ккал</span>
                  </div>
                  {item.macros && (item.macros.protein > 0 || item.macros.carbs > 0 || item.macros.fat > 0) ? (
                    <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
                      <span>Б {item.macros.protein?.toFixed(1)} г</span>
                      <span>Ж {item.macros.fat?.toFixed(1)} г</span>
                      <span>У {item.macros.carbs?.toFixed(1)} г</span>
                      {(item.macros.fiber ?? 0) > 0 ? <span>Клетч {item.macros.fiber?.toFixed(1)} г</span> : null}
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

export function Nutrition() {
  const globalRange = useGlobalDateRange()
  const navigate = useNavigate()
  const [summary, setSummary] = useState<NutritionSummary | null>(null)
  const [golden, setGolden] = useState<NutritionGoldenMetrics | null>(null)
  const [daily, setDaily] = useState<NutritionDay[]>([])
  const [loading, setLoading] = useState(true)
  const [period, setPeriod] = useState(14)
  const [mealFilter, setMealFilter] = useState('')
  const [syncing, setSyncing] = useState(false)
  const [integrations, setIntegrations] = useState<Integration[]>([])
  const [savingTargets, setSavingTargets] = useState(false)
  const [targetsError, setTargetsError] = useState('')
  const [targetsNotice, setTargetsNotice] = useState('')
  const [showTargetsPanel, setShowTargetsPanel] = useState(false)
  const [savingWater, setSavingWater] = useState(false)
  const [waterError, setWaterError] = useState('')
  const [waterNotice, setWaterNotice] = useState('')
  const [customWaterInput, setCustomWaterInput] = useState('')
  const [customHydrationType, setCustomHydrationType] = useState<HydrationBeverageType>('tea')
  const [customHydrationInput, setCustomHydrationInput] = useState('')
  const [hydrationMode, setHydrationMode] = useState<HydrationMode>('strict')
  const [selectedDayDate, setSelectedDayDate] = useState<string | null>(null)
  const [targetsForm, setTargetsForm] = useState({
    targetWeightKg: '',
    targetCalories: '',
    targetProteinG: '',
    targetCarbsG: '',
    targetFatG: '',
    targetWaterMl: '',
  })

  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const [s, g, d] = await Promise.all([
        api.getNutritionSummary(globalRange.params),
        api.getNutritionGolden(period, globalRange.params),
        api.getNutritionDaily(period, globalRange.params),
      ])
      setSummary(s)
      setGolden(g)
      setDaily(d)
    } catch (error) {
      console.error(error)
    } finally {
      setLoading(false)
    }
  }, [period, globalRange.params])

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
    const manual = summary?.targets?.manual
    setTargetsForm({
      targetWeightKg: numberInputValue(manual?.target_weight_kg),
      targetCalories: numberInputValue(manual?.target_calories),
      targetProteinG: numberInputValue(manual?.target_protein_g),
      targetCarbsG: numberInputValue(manual?.target_carbs_g),
      targetFatG: numberInputValue(manual?.target_fat_g),
      targetWaterMl: numberInputValue(manual?.target_water_ml),
    })
    setHydrationMode(summary?.targets?.hydration_mode ?? 'strict')
  }, [summary])

  const chartData = [...daily].reverse()
  const enabledNutritionIntegrations = integrations.filter(i =>
    (i.name === 'myfitnesspal' || i.name === 'fatsecret') && i.enabled
  )
  const nutritionSyncLabel = enabledNutritionIntegrations.length === 1
    ? `Синхронизировать ${enabledNutritionIntegrations[0].display_name}`
    : 'Синхронизировать питание'
  const nutritionSyncCaption = syncCaptionForSources(enabledNutritionIntegrations)

  // Macro averages for summary
  const avgProtein = chartData.length ? chartData.reduce((s, d) => s + d.protein, 0) / chartData.length : 0
  const avgFat = chartData.length ? chartData.reduce((s, d) => s + d.fat, 0) / chartData.length : 0
  const avgCarbs = chartData.length ? chartData.reduce((s, d) => s + d.carbs, 0) / chartData.length : 0
  const hydrationTrackedDays = chartData.filter(day => day.hydration_ml > 0).length
  const hasHydrationData = chartData.some(day => day.water_ml > 0 || day.counted_drinks_ml > 0 || day.other_drinks_ml > 0 || day.hydration_ml > 0)
  const avgHydration = hydrationTrackedDays > 0
    ? chartData.reduce((sum, day) => sum + day.hydration_ml, 0) / hydrationTrackedDays
    : 0

  // Macro distribution pie
  const macroPie = [
    { name: 'Белки', value: avgProtein * 4, color: MACRO_COLORS.protein },
    { name: 'Жиры', value: avgFat * 9, color: MACRO_COLORS.fat },
    { name: 'Углеводы', value: avgCarbs * 4, color: MACRO_COLORS.carbs },
  ].filter(m => m.value > 0)

  const mealStats = MEAL_ORDER.map((mealType) => {
    let totalCalories = 0
    let daysPresent = 0

    chartData.forEach(day => {
      const meal = day.meals.find(entry => entry.meal_type === mealType)
      if (!meal) return

      const mealCalories = meal.items.reduce((sum, item) => sum + item.calories, 0)
      if (mealCalories <= 0) return

      totalCalories += mealCalories
      daysPresent += 1
    })

    return {
      key: mealType,
      name: MEAL_LABELS[mealType],
      color: MEAL_COLORS[mealType],
      totalCalories: Math.round(totalCalories),
      daysPresent,
      avgCaloriesWhenPresent: daysPresent > 0 ? Math.round(totalCalories / daysPresent) : 0,
      avgCaloriesPerTrackedDay: chartData.length > 0 ? Math.round(totalCalories / chartData.length) : 0,
    }
  }).filter(stat => stat.totalCalories > 0)
  const targets = summary?.targets
  const calorieTarget = targets?.target_calories
  const waterTarget = targets?.target_water_ml
  const todayWater = summary?.today_water_ml ?? 0
  const todayHydration = summary?.today_hydration_ml ?? todayWater
  const todayCountedDrinks = summary?.today_counted_drinks_ml ?? 0
  const todayOtherDrinks = summary?.today_other_drinks_ml ?? 0
  const waterProgress = typeof waterTarget === 'number' && waterTarget > 0 ? Math.min(todayHydration / waterTarget, 1) : null
  const waterTargetLeft = typeof waterTarget === 'number' ? Math.max(waterTarget - todayHydration, 0) : null
  const waterGoalDays = typeof waterTarget === 'number'
    ? chartData.filter(day => day.hydration_ml >= waterTarget).length
    : 0
  const filteredDaily = mealFilter
    ? daily
      .map(day => filterNutritionDayByMeal(day, mealFilter))
      .filter((day): day is NutritionDay => day !== null)
    : daily
  const goldenCards = golden?.cards ?? []
  const selectedDay = useMemo(
    () => filteredDaily.find(day => day.date === selectedDayDate) ?? filteredDaily[0] ?? null,
    [filteredDaily, selectedDayDate],
  )

  function openNutritionRaw(filters: Record<string, string | undefined> = {}) {
    navigate(rawDataHref('nutrition.days', { ...globalRange.params, ...filters }))
  }

  const calorieReference = Math.max(
    calorieTarget ?? 0,
    ...filteredDaily.map(day => day.calories),
    1,
  )

  useEffect(() => {
    if (filteredDaily.length === 0) {
      setSelectedDayDate(null)
      return
    }

    if (!selectedDayDate || !filteredDaily.some(day => day.date === selectedDayDate)) {
      setSelectedDayDate(filteredDaily[0].date)
    }
  }, [filteredDaily, selectedDayDate])

  async function handleSyncNutrition() {
    if (enabledNutritionIntegrations.length === 0) return
    setSyncing(true)
    try {
      for (const integration of enabledNutritionIntegrations) {
        await api.syncIntegration(integration.name)
      }
      await Promise.all([loadData(), loadIntegrations()])
    } catch (error) {
      console.error(error)
    } finally {
      setSyncing(false)
    }
  }

  function setTargetsField(field: keyof typeof targetsForm, value: string) {
    setTargetsForm(current => ({ ...current, [field]: value }))
  }

  function buildSavedTargetsPayload(nextMode: HydrationMode): NutritionTargetsInput {
    const manual = summary?.targets?.manual
    return {
      target_weight_kg: manual?.target_weight_kg ?? null,
      target_calories: manual?.target_calories ?? null,
      target_protein_g: manual?.target_protein_g ?? null,
      target_carbs_g: manual?.target_carbs_g ?? null,
      target_fat_g: manual?.target_fat_g ?? null,
      target_water_ml: manual?.target_water_ml ?? null,
      hydration_mode: nextMode,
    }
  }

  async function handleSaveTargets(clear = false) {
    setSavingTargets(true)
    setTargetsError('')
    setTargetsNotice('')

    try {
      const payload: NutritionTargetsInput = clear ? {
        target_weight_kg: null,
        target_calories: null,
        target_protein_g: null,
        target_carbs_g: null,
        target_fat_g: null,
        target_water_ml: null,
        hydration_mode: null,
      } : {
        target_weight_kg: parseRequiredDecimalOrNull('Целевой вес', targetsForm.targetWeightKg),
        target_calories: parseRequiredDecimalOrNull('Калории', targetsForm.targetCalories),
        target_protein_g: parseRequiredDecimalOrNull('Белки', targetsForm.targetProteinG),
        target_carbs_g: parseRequiredDecimalOrNull('Углеводы', targetsForm.targetCarbsG),
        target_fat_g: parseRequiredDecimalOrNull('Жиры', targetsForm.targetFatG),
        target_water_ml: parseRequiredDecimalOrNull('Вода', targetsForm.targetWaterMl),
        hydration_mode: hydrationMode,
      }
      await api.saveNutritionTargets(payload)
      if (clear) {
        setTargetsForm({
          targetWeightKg: '',
          targetCalories: '',
          targetProteinG: '',
          targetCarbsG: '',
          targetFatG: '',
          targetWaterMl: '',
        })
        setHydrationMode('strict')
      }
      await loadData()
      setTargetsNotice(clear ? 'Ручные цели очищены' : 'Ручные цели сохранены')
    } catch (error) {
      setTargetsError(error instanceof Error ? error.message : 'Не удалось сохранить ручные цели')
    } finally {
      setSavingTargets(false)
    }
  }

  async function handleAddWater(deltaMl: number) {
    setSavingWater(true)
    setWaterError('')
    setWaterNotice('')
    try {
      const next = await api.saveNutritionWater({ delta_ml: deltaMl })
      await loadData()
      setWaterNotice(`Добавлено ${Math.round(deltaMl)} мл воды · в цель сегодня ${Math.round(next.hydration_ml)} мл`)
    } catch (error) {
      setWaterError(error instanceof Error ? error.message : 'Не удалось сохранить воду')
    } finally {
      setSavingWater(false)
    }
  }

  async function handleSetWaterAbsolute(nextWaterMl: number) {
    setSavingWater(true)
    setWaterError('')
    setWaterNotice('')
    try {
      const next = await api.saveNutritionWater({ water_ml: nextWaterMl })
      await loadData()
      setWaterNotice(nextWaterMl === 0 ? 'Сегодняшняя вода сброшена' : `Вода обновлена до ${Math.round(next.water_ml)} мл · в цель идёт ${Math.round(next.hydration_ml)} мл`)
      if (nextWaterMl > 0) setCustomWaterInput('')
    } catch (error) {
      setWaterError(error instanceof Error ? error.message : 'Не удалось обновить воду')
    } finally {
      setSavingWater(false)
    }
  }

  async function handleSubmitCustomWater() {
    try {
      const parsed = parseRequiredDecimalOrNull('Вода', customWaterInput)
      if (parsed == null) {
        setWaterError('Введи количество воды в мл')
        return
      }
      await handleSetWaterAbsolute(parsed)
    } catch (error) {
      setWaterError(error instanceof Error ? error.message : 'Не удалось обновить воду')
    }
  }

  async function handleAddHydration(beverageType: HydrationBeverageType, deltaMl: number) {
    setSavingWater(true)
    setWaterError('')
    setWaterNotice('')
    try {
      const next = await api.saveNutritionHydration({ beverage_type: beverageType, delta_ml: deltaMl })
      await loadData()
      setWaterNotice(`${hydrationBeverageLabel(beverageType)} +${Math.round(deltaMl)} мл · в цель сегодня ${Math.round(next.hydration_ml)} мл`)
    } catch (error) {
      setWaterError(error instanceof Error ? error.message : 'Не удалось сохранить напиток')
    } finally {
      setSavingWater(false)
    }
  }

  async function handleSubmitCustomHydration() {
    try {
      const parsed = parseRequiredDecimalOrNull('Напиток', customHydrationInput)
      if (parsed == null) {
        setWaterError('Введи количество напитка в мл')
        return
      }
      await handleAddHydration(customHydrationType, parsed)
      setCustomHydrationInput('')
    } catch (error) {
      setWaterError(error instanceof Error ? error.message : 'Не удалось сохранить напиток')
    }
  }

  async function handleHydrationModeChange(nextMode: HydrationMode) {
    const previousMode = hydrationMode
    setHydrationMode(nextMode)
    setSavingTargets(true)
    setTargetsError('')
    setTargetsNotice('')
    setWaterError('')
    setWaterNotice('')
    try {
      await api.saveNutritionTargets(buildSavedTargetsPayload(nextMode))
      await loadData()
      setTargetsNotice(`Режим гидратации: ${hydrationModeLabel(nextMode).toLowerCase()}`)
    } catch (error) {
      setHydrationMode(previousMode)
      setTargetsError(error instanceof Error ? error.message : 'Не удалось сохранить режим гидратации')
    } finally {
      setSavingTargets(false)
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        eyebrow="Nutrition"
        title="Питание"
        description="Калории, БЖУ, вода и дневник питания в одном месте. Ежедневный hydration-трекинг живёт рядом с питанием, а редкие настройки убраны ниже."
        badges={[
          { label: new Date().toLocaleDateString('ru-RU', { month: 'long', year: 'numeric' }), tone: 'primary' },
          ...(globalRange.label ? [{ label: globalRange.label, tone: 'primary' as const }] : []),
          { label: enabledNutritionIntegrations.length > 0 ? `${enabledNutritionIntegrations.length} активных источника питания` : 'Источник питания не подключён', tone: enabledNutritionIntegrations.length > 0 ? 'success' : 'warning' },
          { label: globalRange.isActive ? 'Период: глобальный фильтр' : `Период: ${period} дней`, tone: 'muted' },
        ]}
        actions={(
          <>
            <PageSyncButton
              label={nutritionSyncLabel}
              syncCaption={nutritionSyncCaption}
              syncing={syncing}
              disabled={enabledNutritionIntegrations.length === 0}
              onClick={handleSyncNutrition}
            />
            <div className="flex gap-1 rounded-2xl border bg-card/90 p-1 shadow-sm">
              {PERIODS.map(p => (
                <button key={p.days} onClick={() => setPeriod(p.days)} disabled={globalRange.isActive}
                  className={cn('px-3 py-1.5 text-xs rounded-xl transition-colors',
                    period === p.days && !globalRange.isActive ? 'bg-primary text-primary-foreground' : globalRange.isActive ? 'cursor-not-allowed text-muted-foreground/50' : 'text-muted-foreground hover:bg-accent')}>
                  {p.label}
                </button>
              ))}
            </div>
          </>
        )}
      />

      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-5">
        {(loading && goldenCards.length === 0
          ? Array.from({ length: 5 }).map((_, index) => ({ key: `skeleton-${index}`, title: '—', value: '—', detail: '—', tone: 'muted' as const }))
          : goldenCards
        ).map(card => (
          <NutritionGoldenMetricCard
            key={card.key}
            card={card}
            loading={loading}
          />
        ))}
      </div>

      <div className="rounded-2xl border bg-card/90 p-5 shadow-sm">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Гидратация</h2>
            <p className="mt-1 text-xs text-muted-foreground">
              Чистая вода живёт отдельно, а режим гидратации решает, считать ли чай и кофе частью цели.
            </p>
          </div>
          <div className={cn('rounded-full border px-3 py-1 text-xs font-medium', HYDRATION_MODE_ACCENT[hydrationMode])}>
            {hydrationModeLabel(hydrationMode)} · {typeof waterTarget === 'number' ? `цель ${Math.round(waterTarget)} мл` : 'цель не задана'}
          </div>
        </div>

        <div className="mt-4 flex flex-wrap gap-2">
          {(['strict', 'flexible'] as HydrationMode[]).map(mode => (
            <button
              key={mode}
              onClick={() => void handleHydrationModeChange(mode)}
              disabled={savingTargets}
              className={cn(
                'rounded-xl border px-3 py-2 text-sm transition-colors',
                hydrationMode === mode ? 'border-primary/30 bg-primary/10 text-primary' : 'bg-background/50 text-muted-foreground hover:bg-accent',
              )}
            >
              {hydrationModeLabel(mode)}
            </button>
          ))}
          <span className="self-center text-xs text-muted-foreground">{HYDRATION_MODE_NOTES[hydrationMode]}</span>
        </div>

        <div className="mt-5 grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1.2fr)_minmax(340px,0.8fr)] xl:items-start">
          <div className="rounded-2xl border border-cyan-500/10 bg-cyan-500/5 p-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <p className="text-[11px] uppercase tracking-wide text-cyan-200/80">Сегодня в цель</p>
                <p className="mt-2 text-3xl font-bold text-foreground">{Math.round(todayHydration)} <span className="text-base font-medium text-muted-foreground">мл</span></p>
                <p className="mt-2 text-xs text-muted-foreground">
                  {typeof waterTarget === 'number'
                    ? waterTargetLeft === 0
                      ? 'Цель по гидратации на сегодня закрыта.'
                      : `До цели осталось ${Math.round(waterTargetLeft ?? 0)} мл.`
                    : 'Можно сохранить цель по воде в ручных целях и видеть прогресс.'}
                </p>
                <div className="mt-3 flex flex-wrap gap-2 text-xs text-muted-foreground">
                  <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1 text-cyan-200">💧 Вода {Math.round(todayWater)} мл</span>
                  <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1 text-emerald-200">🍵/☕ В цель {Math.round(todayCountedDrinks)} мл</span>
                  {todayOtherDrinks > 0 ? (
                    <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1 text-amber-200">🧃 Отдельно {Math.round(todayOtherDrinks)} мл</span>
                  ) : null}
                </div>
              </div>
              <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-cyan-500/15 text-cyan-200">
                <GlassWater className="h-5 w-5" />
              </div>
            </div>

            <div className="mt-4 h-3 overflow-hidden rounded-full bg-muted/70">
              <div
                className="h-full rounded-full bg-cyan-400 transition-[width]"
                style={{ width: `${waterProgress == null ? 0 : Math.min(100, Math.round(waterProgress * 100))}%` }}
              />
            </div>

            <div className="mt-4 grid grid-cols-2 gap-3 md:grid-cols-4">
              {[
                { label: 'Ср. по дням', value: fmtWaterMl(avgHydration) },
                { label: 'Дней с гидратацией', value: `${hydrationTrackedDays}/${chartData.length || 0}` },
                { label: 'Цель выполнена', value: typeof waterTarget === 'number' ? `${waterGoalDays}/${chartData.length || 0}` : '—' },
                { label: 'Режим', value: hydrationModeLabel(hydrationMode) },
              ].map(metric => (
                <div key={metric.label} className="rounded-xl border bg-background/45 px-3 py-2">
                  <p className="text-[10px] uppercase tracking-wide text-muted-foreground/80">{metric.label}</p>
                  <p className="mt-1 text-sm font-semibold text-foreground">{metric.value}</p>
                </div>
              ))}
            </div>
          </div>

          <div className="rounded-2xl border bg-background/45 p-4">
            <p className="text-[11px] uppercase tracking-wide text-muted-foreground">Быстрый лог</p>
            <p className="mt-2 text-[11px] text-muted-foreground">Вода</p>
            <div className="mt-3 grid grid-cols-2 gap-2">
              {[250, 500, 750, 1000].map(amount => (
                <button
                  key={amount}
                  onClick={() => void handleAddWater(amount)}
                  disabled={savingWater}
                  className="inline-flex items-center justify-center gap-2 rounded-xl border border-cyan-500/20 bg-cyan-500/10 px-3 py-2 text-sm font-medium text-cyan-100 transition-colors hover:bg-cyan-500/15 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  <Droplets className="h-3.5 w-3.5" />
                  +{amount}
                </button>
              ))}
            </div>
            <p className="mt-4 text-[11px] text-muted-foreground">Напитки</p>
            <div className="mt-2 grid grid-cols-2 gap-2">
              {HYDRATION_BEVERAGES.map(beverage => (
                <button
                  key={beverage.type}
                  onClick={() => void handleAddHydration(beverage.type, beverage.amount)}
                  disabled={savingWater}
                  className="inline-flex items-center justify-center gap-2 rounded-xl border px-3 py-2 text-sm font-medium text-foreground transition-colors hover:bg-accent disabled:cursor-not-allowed disabled:opacity-60"
                >
                  <span>{beverage.emoji}</span>
                  {beverage.short} +{beverage.amount}
                </button>
              ))}
            </div>
            <div className="mt-4 grid gap-2 sm:grid-cols-2">
              <select
                value={customHydrationType}
                onChange={e => setCustomHydrationType(e.target.value as HydrationBeverageType)}
                className="rounded-xl border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
              >
                {HYDRATION_BEVERAGES.map(beverage => (
                  <option key={beverage.type} value={beverage.type}>{beverage.label}</option>
                ))}
              </select>
              <input
                type="text"
                inputMode="decimal"
                value={customHydrationInput}
                onChange={e => setCustomHydrationInput(e.target.value)}
                placeholder="например, 250"
                className="min-w-0 rounded-xl border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
              />
              <button
                onClick={() => void handleSubmitCustomHydration()}
                disabled={savingWater}
                className="rounded-xl bg-primary px-3 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-60 sm:col-span-2"
              >
                Добавить
              </button>
            </div>
            <div className="mt-3 grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto]">
              <input
                type="text"
                inputMode="decimal"
                value={customWaterInput}
                onChange={e => setCustomWaterInput(e.target.value)}
                placeholder="вода, например 1800"
                className="min-w-0 rounded-xl border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
              />
              <button
                onClick={() => void handleSubmitCustomWater()}
                disabled={savingWater}
                className="rounded-xl border border-cyan-500/20 bg-cyan-500/10 px-3 py-2 text-sm font-medium text-cyan-100 transition-colors hover:bg-cyan-500/15 disabled:cursor-not-allowed disabled:opacity-60"
              >
                Задать воду
              </button>
              <button
                onClick={() => void handleSetWaterAbsolute(0)}
                disabled={savingWater}
                className="inline-flex items-center gap-2 rounded-xl border px-3 py-2 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent disabled:cursor-not-allowed disabled:opacity-60 sm:col-span-2"
              >
                <RotateCcw className="h-3.5 w-3.5" />
                Сбросить воду
              </button>
            </div>
            {(waterError || waterNotice) ? (
              <p className={cn('mt-3 text-xs', waterError ? 'text-rose-400' : 'text-cyan-200')}>
                {waterError || waterNotice}
              </p>
            ) : null}
          </div>
        </div>

        <div className="mt-5 border-t border-border/70 pt-5">
          <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
            <div>
              <h3 className="text-sm font-semibold text-foreground uppercase tracking-wider">Гидратация по дням</h3>
              <p className="mt-1 text-xs text-muted-foreground">
                Видно отдельно чистую воду, counted hydration и напитки, которые в цель не идут. Это не смешивается с калориями и БЖУ.
              </p>
            </div>
            <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
              <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
                Среднее: {fmtWaterMl(avgHydration)}
              </span>
              <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
                Цель выполнена: {typeof waterTarget === 'number' ? `${waterGoalDays}/${chartData.length || 0}` : '—'}
              </span>
            </div>
          </div>
          {loading ? <div className="h-56 rounded bg-muted animate-pulse" /> : chartData.length === 0 ? (
            <p className="py-10 text-center text-sm text-muted-foreground">Нет данных</p>
          ) : !hasHydrationData ? (
            <div className="flex h-60 items-center justify-center rounded-2xl border border-dashed border-border/70 bg-background/35 px-6 text-center">
              <div className="max-w-sm">
                <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl bg-cyan-500/10 text-cyan-200">
                  <GlassWater className="h-5 w-5" />
                </div>
                <p className="mt-4 text-sm font-medium text-foreground">Пока нет логов по гидратации</p>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">
                  Добавь воду, чай или кофе в quick log выше. Когда появятся первые записи, здесь будет видно, что идёт в цель, а что считается отдельно.
                </p>
              </div>
            </div>
          ) : (
            <EChart
              option={buildHydrationOption(chartData, waterTarget)}
              height={240}
              onClick={(params) => {
                const day = String(params.name ?? '')
                if (day) openNutritionRaw({ day, metric: 'hydration' })
              }}
            />
          )}
        </div>
      </div>

      {/* Charts row */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Calorie chart */}
        <div className="rounded-2xl border bg-card/90 p-5 shadow-sm">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Калории</h2>
          {loading ? <div className="h-48 bg-muted rounded animate-pulse" /> : chartData.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-8">Нет данных</p>
          ) : (
            <EChart
              option={buildCaloriesOption(chartData, calorieTarget)}
              height={200}
              onClick={(params) => {
                const day = String(params.name ?? '')
                if (day) openNutritionRaw({ day, metric: 'calories' })
              }}
            />
          )}
        </div>

        {/* Macros trend */}
        <div className="rounded-2xl border bg-card/90 p-5 shadow-sm">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">БЖУ тренд</h2>
          {loading ? <div className="h-48 bg-muted rounded animate-pulse" /> : chartData.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-8">Нет данных</p>
          ) : (
            <EChart
              option={buildMacrosTrendOption(chartData)}
              height={200}
              onClick={(params) => {
                const day = String(params.name ?? '')
                const metric = String(params.seriesName ?? '')
                if (day) openNutritionRaw({ day, metric })
              }}
            />
          )}
        </div>
      </div>

      {/* Pie charts row */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
        {/* Macro distribution */}
        <div className="rounded-2xl border bg-card/90 p-5 shadow-sm">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Распределение БЖУ (ккал)</h2>
          {loading || macroPie.length === 0 ? <div className="h-40 flex items-center justify-center text-sm text-muted-foreground">Нет данных</div> : (
            <div className="flex flex-col gap-5 md:flex-row md:items-start">
              <EChart
                option={buildNutritionDonutOption(macroPie, 'Всего', ' ккал')}
                height={160}
                width={160}
                className="mx-auto shrink-0 md:mx-0"
                onClick={(params) => {
                  const metric = String(params.name ?? '')
                  if (metric) openNutritionRaw({ metric })
                }}
              />
              <div className="flex min-w-0 flex-1 flex-col gap-2">
                {macroPie.map(m => (
                  <div key={m.name} className="rounded-xl border bg-background/45 px-3 py-2">
                    <div className="flex items-center gap-3 text-xs">
                      <div className="w-2.5 h-2.5 rounded-full shrink-0" style={{ backgroundColor: m.color }} />
                      <span className="min-w-0 flex-1 text-sm text-foreground">{m.name}</span>
                      <span className="shrink-0 text-[11px] font-medium text-muted-foreground">
                        {((m.value / macroPie.reduce((s, x) => s + x.value, 0)) * 100).toFixed(0)}%
                      </span>
                      <span className="shrink-0 text-sm font-medium text-foreground">{Math.round(m.value)} ккал</span>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* Meal structure */}
        <div className="rounded-2xl border bg-card/90 p-5 shadow-sm">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-1">Структура приёмов пищи</h2>
          <p className="mb-4 text-xs text-muted-foreground">
            Не просто сумма за период, а расклад по дням: видно, когда завтрак/обед/ужин реально присутствуют и какой приём пищи тащит калории.
          </p>
          {loading || mealStats.length === 0 ? <div className="h-40 flex items-center justify-center text-sm text-muted-foreground">Нет данных</div> : (
            <div className="flex flex-col gap-5">
              <EChart
                option={buildMealsTimelineOption(chartData, mealStats)}
                height={220}
                onClick={(params) => {
                  const day = String(params.name ?? '')
                  const label = String(params.seriesName ?? '')
                  const mealType = MEAL_KEYS_BY_LABEL[label] ?? label
                  if (day) openNutritionRaw({ day, meal_type: mealType })
                }}
              />
              <div className="grid gap-2 md:grid-cols-2">
                {mealStats.map((stat) => (
                  <button key={stat.key} onClick={() => openNutritionRaw({ meal_type: stat.key })}
                    className={cn(
                      'rounded-xl border bg-background/45 px-3 py-2 text-left transition-colors hover:border-border hover:bg-accent/40',
                      mealFilter === stat.key && 'border-primary/30 bg-primary/10'
                    )}>
                    <div className="flex items-center gap-3">
                      <div className="h-2.5 w-2.5 rounded-full shrink-0" style={{ backgroundColor: stat.color }} />
                      <span className={cn('min-w-0 flex-1 text-sm text-foreground', mealFilter === stat.key && 'font-medium text-primary')}>
                        {stat.name}
                      </span>
                      <span className="shrink-0 text-sm font-semibold text-foreground">{stat.totalCalories} ккал</span>
                    </div>
                    <div className="mt-2 grid grid-cols-3 gap-2 text-[11px] text-muted-foreground">
                      <div>
                        <p className="text-[10px] uppercase tracking-wide text-muted-foreground/70">Дней</p>
                        <p className="mt-1 text-sm text-foreground">{stat.daysPresent}/{chartData.length}</p>
                      </div>
                      <div>
                        <p className="text-[10px] uppercase tracking-wide text-muted-foreground/70">Ср. когда был</p>
                        <p className="mt-1 text-sm text-foreground">{stat.avgCaloriesWhenPresent} ккал</p>
                      </div>
                      <div>
                        <p className="text-[10px] uppercase tracking-wide text-muted-foreground/70">Ср. в день</p>
                        <p className="mt-1 text-sm text-foreground">{stat.avgCaloriesPerTrackedDay} ккал</p>
                      </div>
                    </div>
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>

      <ExpandablePanel
        title="Цели питания и ручные настройки"
        description="Редко меняется. Основные цели уже участвуют в UI и AI, поэтому форму можно держать свернутой."
        open={showTargetsPanel}
        onToggle={() => setShowTargetsPanel(current => !current)}
        summary={(
          <>
            <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
              Цель: {fmtOptionalNumber(targets?.target_calories, ' ккал')}
            </span>
            <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
              Б/Ж/У: {fmtOptionalNumber(targets?.target_protein_g, ' г')} · {fmtOptionalNumber(targets?.target_fat_g, ' г')} · {fmtOptionalNumber(targets?.target_carbs_g, ' г')}
            </span>
            <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
              Вода: {fmtOptionalNumber(targets?.target_water_ml, ' мл')}
            </span>
            <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
              Гидратация: {hydrationModeLabel(hydrationMode)}
            </span>
            <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
              Вес: {fmtWeight(targets?.current_weight_kg)} → {fmtWeight(targets?.target_weight_kg)}
            </span>
            <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
              До цели: {fmtTargetDelta(targets?.current_weight_kg, targets?.target_weight_kg)}
            </span>
            {targets?.source ? (
              <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1 uppercase tracking-wide">
                {targets.source}
              </span>
            ) : null}
          </>
        )}
      >
        <div className="flex flex-col gap-4">
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-7">
            <div className="rounded-xl bg-muted/40 p-3">
              <p className="text-[10px] text-muted-foreground">Текущий вес</p>
              <p className="mt-1 text-lg font-bold text-foreground">{fmtWeight(targets?.current_weight_kg)}</p>
              {targets?.current_weight_date && <p className="mt-1 text-[10px] text-muted-foreground">{fmtDate(targets.current_weight_date)}</p>}
            </div>
            <div className="rounded-xl bg-muted/40 p-3">
              <p className="text-[10px] text-muted-foreground">Целевой вес</p>
              <p className="mt-1 text-lg font-bold text-foreground">{fmtWeight(targets?.target_weight_kg)}</p>
            </div>
            <div className="rounded-xl bg-muted/40 p-3">
              <p className="text-[10px] text-muted-foreground">До цели</p>
              <p className="mt-1 text-lg font-bold text-foreground">{fmtTargetDelta(targets?.current_weight_kg, targets?.target_weight_kg)}</p>
            </div>
            <div className="rounded-xl bg-muted/40 p-3">
              <p className="text-[10px] text-muted-foreground">Рост</p>
              <p className="mt-1 text-lg font-bold text-foreground">{fmtOptionalNumber(targets?.height_cm, ' см')}</p>
            </div>
            <div className="rounded-xl bg-muted/40 p-3">
              <p className="text-[10px] text-muted-foreground">Цель ккал</p>
              <p className="mt-1 text-lg font-bold text-foreground">{fmtOptionalNumber(targets?.target_calories, ' ккал')}</p>
            </div>
            <div className="rounded-xl bg-cyan-500/10 p-3">
              <p className="text-[10px] text-cyan-200/80">Цель воды</p>
              <p className="mt-1 text-lg font-bold text-cyan-100">{fmtOptionalNumber(targets?.target_water_ml, ' мл')}</p>
            </div>
            <div className={cn('rounded-xl p-3', hydrationMode === 'flexible' ? 'bg-emerald-500/10' : 'bg-cyan-500/10')}>
              <p className="text-[10px] text-muted-foreground">Режим гидратации</p>
              <p className="mt-1 text-lg font-bold text-foreground">{hydrationModeLabel(hydrationMode)}</p>
              <p className="mt-1 text-[10px] text-muted-foreground">{HYDRATION_MODE_NOTES[hydrationMode]}</p>
            </div>
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div className="rounded-xl bg-blue-500/10 p-3">
              <p className="text-[10px] text-blue-300">Белки</p>
              <p className="mt-1 text-base font-bold text-blue-200">{fmtOptionalNumber(targets?.target_protein_g, ' г')}</p>
            </div>
            <div className="rounded-xl bg-orange-500/10 p-3">
              <p className="text-[10px] text-orange-300">Жиры</p>
              <p className="mt-1 text-base font-bold text-orange-200">{fmtOptionalNumber(targets?.target_fat_g, ' г')}</p>
            </div>
            <div className="rounded-xl bg-emerald-500/10 p-3">
              <p className="text-[10px] text-emerald-300">Углеводы</p>
              <p className="mt-1 text-base font-bold text-emerald-200">{fmtOptionalNumber(targets?.target_carbs_g, ' г')}</p>
            </div>
          </div>

          <div className="rounded-xl border border-dashed bg-muted/20 p-4">
            <div className="mb-4">
              <h3 className="text-xs font-semibold uppercase tracking-wider text-foreground">Ручные цели</h3>
              <p className="mt-1 text-xs text-muted-foreground">
                Заполняй только то, чего не хватает в FatSecret, или то, что хочешь переопределить вручную.
              </p>
              {targets?.manual?.updated_at ? (
                <p className="mt-1 text-[11px] text-muted-foreground">Последнее ручное обновление: {fmtSyncTime(targets.manual.updated_at)}</p>
              ) : null}
              {targets?.synced_at ? (
                <p className="mt-1 text-[11px] text-muted-foreground">Последнее обновление целей: {fmtSyncTime(targets.synced_at)}</p>
              ) : null}
            </div>

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-6">
              <label className="flex flex-col gap-1">
                <span className="text-[11px] text-muted-foreground">Целевой вес, кг</span>
                <input
                  type="text"
                  inputMode="decimal"
                  value={targetsForm.targetWeightKg}
                  onChange={e => setTargetsField('targetWeightKg', e.target.value)}
                  placeholder="например, 78"
                  className="rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
                />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-[11px] text-muted-foreground">Калории, ккал</span>
                <input
                  type="text"
                  inputMode="decimal"
                  value={targetsForm.targetCalories}
                  onChange={e => setTargetsField('targetCalories', e.target.value)}
                  placeholder="например, 2400"
                  className="rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
                />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-[11px] text-muted-foreground">Белки, г</span>
                <input
                  type="text"
                  inputMode="decimal"
                  value={targetsForm.targetProteinG}
                  onChange={e => setTargetsField('targetProteinG', e.target.value)}
                  placeholder="например, 160"
                  className="rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
                />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-[11px] text-muted-foreground">Жиры, г</span>
                <input
                  type="text"
                  inputMode="decimal"
                  value={targetsForm.targetFatG}
                  onChange={e => setTargetsField('targetFatG', e.target.value)}
                  placeholder="например, 70"
                  className="rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
                />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-[11px] text-muted-foreground">Углеводы, г</span>
                <input
                  type="text"
                  inputMode="decimal"
                  value={targetsForm.targetCarbsG}
                  onChange={e => setTargetsField('targetCarbsG', e.target.value)}
                  placeholder="например, 250"
                  className="rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
                />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-[11px] text-muted-foreground">Вода, мл</span>
                <input
                  type="text"
                  inputMode="decimal"
                  value={targetsForm.targetWaterMl}
                  onChange={e => setTargetsField('targetWaterMl', e.target.value)}
                  placeholder="например, 2500"
                  className="rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
                />
              </label>
            </div>

            {(targetsError || targetsNotice) ? (
              <p className={cn('mt-3 text-xs', targetsError ? 'text-rose-400' : 'text-emerald-400')}>
                {targetsError || targetsNotice}
              </p>
            ) : null}

            <div className="mt-4 flex flex-wrap gap-2">
              <button
                onClick={() => void handleSaveTargets(false)}
                disabled={savingTargets}
                className="rounded-lg bg-primary px-3 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
              >
                {savingTargets ? 'Сохраняю...' : 'Сохранить ручные цели'}
              </button>
              <button
                onClick={() => void handleSaveTargets(true)}
                disabled={savingTargets}
                className="rounded-lg border px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent disabled:opacity-50"
              >
                Очистить ручные цели
              </button>
            </div>
          </div>

          {targets?.api_notes?.map(note => (
            <p key={note} className="rounded-xl border border-dashed px-3 py-2 text-xs text-muted-foreground">{note}</p>
          ))}
        </div>
      </ExpandablePanel>

      {/* Daily log */}
      <div className="rounded-2xl border bg-card/90 overflow-hidden shadow-sm">
        <div className="px-5 py-4 border-b flex items-center justify-between">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Дневник питания и воды</h2>
          <div className="flex gap-1">
            <button onClick={() => setMealFilter('')}
              className={cn('px-2 py-1 text-xs rounded-lg transition-colors', !mealFilter ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:bg-accent')}>
              Все
            </button>
            {Object.entries(MEAL_LABELS).map(([k, v]) => (
              <button key={k} onClick={() => setMealFilter(mealFilter === k ? '' : k)}
                className={cn('px-2 py-1 text-xs rounded-lg transition-colors', mealFilter === k ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:bg-accent')}>
                {v}
              </button>
            ))}
          </div>
        </div>
        {loading ? (
          <div className="divide-y">
            {[1, 2, 3, 4, 5].map(i => (
              <div key={i} className="px-5 py-3 flex gap-3">
                <div className="h-9 w-9 bg-muted rounded-xl animate-pulse shrink-0" />
                <div className="flex-1 flex flex-col gap-1.5">
                  <div className="h-4 w-24 bg-muted rounded animate-pulse" />
                  <div className="h-3 w-full bg-muted rounded animate-pulse" />
                </div>
              </div>
            ))}
          </div>
        ) : filteredDaily.length === 0 ? (
          <div className="px-5 py-8 text-sm text-muted-foreground text-center">Нет данных. Подключи FatSecret в настройках.</div>
        ) : (
          <div className="p-5">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <p className="text-xs font-medium text-muted-foreground">Таймлайн по дням</p>
                <p className="mt-1 text-xs text-muted-foreground">
                  Кликни по бару, чтобы раскрыть состав конкретного дня ниже.
                </p>
              </div>
              {selectedDay ? (
                <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1 text-xs text-muted-foreground">
                  Выбран день: {fmtDate(selectedDay.date)}
                </span>
              ) : null}
            </div>

            <div className="mt-4">
              <EChart
                option={buildDailyNutritionTimelineOption(filteredDaily, calorieReference, calorieTarget, selectedDayDate)}
                height={Math.max(280, filteredDaily.length * 44)}
                onClick={(params) => {
                  const dataIndex = typeof params.dataIndex === 'number' ? params.dataIndex : null
                  const day = dataIndex == null ? null : filteredDaily[dataIndex]
                  if (!day) return
                  openNutritionRaw({ day: day.date })
                }}
              />
            </div>

            {selectedDay ? (
              <div className="mt-5 border-t border-border/70 pt-5">
                <NutritionDayDetails day={selectedDay} />
              </div>
            ) : null}
          </div>
        )}
      </div>
    </div>
  )
}
