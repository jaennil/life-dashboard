import { useCallback, useEffect, useState } from 'react'
import type { EChartsCoreOption } from 'echarts/core'
import { ChevronDown, ChevronUp } from 'lucide-react'
import { EChart } from '@/components/EChart'
import { ExpandablePanel } from '@/components/ExpandablePanel'
import { PageSyncButton } from '@/components/PageSyncButton'
import { PageHeader } from '@/components/PageHeader'
import { cn, syncCaptionForSources } from '@/lib/utils'
import { api, type NutritionSummary, type NutritionDay, type Integration, type NutritionTargetsInput } from '@/lib/api'

const MEAL_LABELS: Record<string, string> = {
  breakfast: 'Завтрак', lunch: 'Обед', dinner: 'Ужин', snacks: 'Перекус', other: 'Прочее',
}

const MEAL_COLORS: Record<string, string> = {
  breakfast: '#f97316',
  lunch: '#10b981',
  dinner: '#3b82f6',
  snacks: '#8b5cf6',
  other: '#f43f5e',
}

const MEAL_ORDER = ['breakfast', 'lunch', 'dinner', 'snacks', 'other'] as const

const MACRO_COLORS = { protein: '#3b82f6', fat: '#f97316', carbs: '#10b981', fiber: '#8b5cf6' }
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
    meals,
  }
}

function DayRow({ day, calorieReference, calorieTarget }: { day: NutritionDay; calorieReference: number; calorieTarget?: number }) {
  const [open, setOpen] = useState(false)
  const hasMeals = day.meals.some(m => m.items.length > 0)
  const pct = calorieReference > 0 ? Math.min((day.calories / calorieReference) * 100, 100) : 0
  const overTarget = typeof calorieTarget === 'number' && day.calories > calorieTarget

  return (
    <div>
      <button
        onClick={() => setOpen(o => !o)}
        disabled={!hasMeals}
        className="w-full px-5 py-3 flex items-center gap-3 hover:bg-muted/40 transition-colors text-left disabled:cursor-default"
      >
        <div className="w-9 h-9 rounded-xl bg-muted flex items-center justify-center text-base shrink-0">🍽️</div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between gap-2">
            <p className="text-sm font-medium text-foreground">{fmtDate(day.date)}</p>
            <span className={cn('text-sm font-semibold tabular-nums', overTarget ? 'text-rose-400' : 'text-orange-400')}>
              {day.calories.toFixed(0)} ккал
            </span>
          </div>
          <div className="flex items-center gap-3 mt-1.5">
            <div className="flex-1 h-1.5 rounded-full bg-muted overflow-hidden">
              <div className={cn('h-full rounded-full', overTarget ? 'bg-rose-400' : 'bg-orange-400')} style={{ width: `${pct}%` }} />
            </div>
            <div className="flex gap-2 text-xs text-muted-foreground shrink-0">
              <span className="text-blue-400">Б {day.protein.toFixed(0)}г</span>
              <span className="text-orange-400">Ж {day.fat.toFixed(0)}г</span>
              <span className="text-emerald-400">У {day.carbs.toFixed(0)}г</span>
              {day.fiber > 0 && <span className="text-violet-400">К {day.fiber.toFixed(0)}г</span>}
            </div>
          </div>
        </div>
        {hasMeals && (open ? <ChevronUp className="w-4 h-4 text-muted-foreground shrink-0" /> : <ChevronDown className="w-4 h-4 text-muted-foreground shrink-0" />)}
      </button>

      {open && hasMeals && (
        <div className="px-5 pb-3 flex flex-col gap-3">
          {day.meals.map(meal => (
            <div key={meal.meal_type}>
              <p className="text-xs font-semibold text-foreground mb-1">{MEAL_LABELS[meal.meal_type] ?? meal.meal_type}</p>
              <div className="flex flex-col gap-1">
                {meal.items.map((item, idx) => (
                  <div key={idx} className="flex flex-col gap-0.5">
                    <div className="flex items-center justify-between text-xs text-muted-foreground">
                      <span className="truncate flex-1">{item.food_name}</span>
                      <span className="ml-2 shrink-0 text-foreground/70">{item.serving}</span>
                      <span className="ml-3 shrink-0 text-orange-400 tabular-nums">{item.calories.toFixed(0)} ккал</span>
                    </div>
                    {item.macros && (item.macros.protein > 0 || item.macros.carbs > 0 || item.macros.fat > 0) && (
                      <div className="flex gap-3 text-[10px] text-muted-foreground/60 ml-1">
                        <span>Б {item.macros.protein?.toFixed(1)}г</span>
                        <span>Ж {item.macros.fat?.toFixed(1)}г</span>
                        <span>У {item.macros.carbs?.toFixed(1)}г</span>
                        {(item.macros.fiber ?? 0) > 0 && <span>Клетч {item.macros.fiber?.toFixed(1)}г</span>}
                        {(item.macros.sugar ?? 0) > 0 && <span>Сахар {item.macros.sugar?.toFixed(1)}г</span>}
                        {(item.macros.sodium ?? 0) > 0 && <span>Na {item.macros.sodium?.toFixed(0)}мг</span>}
                      </div>
                    )}
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

export function Nutrition() {
  const [summary, setSummary] = useState<NutritionSummary | null>(null)
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
  const [targetsForm, setTargetsForm] = useState({
    targetWeightKg: '',
    targetCalories: '',
    targetProteinG: '',
    targetCarbsG: '',
    targetFatG: '',
  })

  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const [s, d] = await Promise.all([api.getNutritionSummary(), api.getNutritionDaily(period)])
      setSummary(s)
      setDaily(d)
    } catch (error) {
      console.error(error)
    } finally {
      setLoading(false)
    }
  }, [period])

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
    })
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
  const avgCalories = chartData.length ? chartData.reduce((s, d) => s + d.calories, 0) / chartData.length : 0

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
  const filteredDaily = mealFilter
    ? daily
      .map(day => filterNutritionDayByMeal(day, mealFilter))
      .filter((day): day is NutritionDay => day !== null)
    : daily

  const calorieReference = Math.max(
    calorieTarget ?? 0,
    ...filteredDaily.map(day => day.calories),
    1,
  )

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
      } : {
        target_weight_kg: parseRequiredDecimalOrNull('Целевой вес', targetsForm.targetWeightKg),
        target_calories: parseRequiredDecimalOrNull('Калории', targetsForm.targetCalories),
        target_protein_g: parseRequiredDecimalOrNull('Белки', targetsForm.targetProteinG),
        target_carbs_g: parseRequiredDecimalOrNull('Углеводы', targetsForm.targetCarbsG),
        target_fat_g: parseRequiredDecimalOrNull('Жиры', targetsForm.targetFatG),
      }
      await api.saveNutritionTargets(payload)
      if (clear) {
        setTargetsForm({
          targetWeightKg: '',
          targetCalories: '',
          targetProteinG: '',
          targetCarbsG: '',
          targetFatG: '',
        })
      }
      await loadData()
      setTargetsNotice(clear ? 'Ручные цели очищены' : 'Ручные цели сохранены')
    } catch (error) {
      setTargetsError(error instanceof Error ? error.message : 'Не удалось сохранить ручные цели')
    } finally {
      setSavingTargets(false)
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        eyebrow="Nutrition"
        title="Питание"
        description="Калории, БЖУ и дневник питания без ручной рутины. Цели остаются рядом, но не мешают ежедневному просмотру данных."
        badges={[
          { label: new Date().toLocaleDateString('ru-RU', { month: 'long', year: 'numeric' }), tone: 'primary' },
          { label: enabledNutritionIntegrations.length > 0 ? `${enabledNutritionIntegrations.length} активных источника питания` : 'Источник питания не подключён', tone: enabledNutritionIntegrations.length > 0 ? 'success' : 'warning' },
          { label: `Период: ${period} дней`, tone: 'muted' },
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
                <button key={p.days} onClick={() => setPeriod(p.days)}
                  className={cn('px-3 py-1.5 text-xs rounded-xl transition-colors',
                    period === p.days ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent')}>
                  {p.label}
                </button>
              ))}
            </div>
          </>
        )}
      />

      {/* Summary cards */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-5">
        {[
          { label: 'Сегодня', value: summary ? `${summary.today_kcal.toFixed(0)}` : '—', unit: 'ккал', color: 'bg-orange-500' },
          { label: `Ср. ккал/${period}д`, value: avgCalories.toFixed(0), unit: 'ккал', color: 'bg-amber-500' },
          { label: 'Ср. белки', value: avgProtein.toFixed(0), unit: 'г', color: 'bg-blue-500' },
          { label: 'Ср. жиры', value: avgFat.toFixed(0), unit: 'г', color: 'bg-orange-500' },
          { label: 'Ср. углеводы', value: avgCarbs.toFixed(0), unit: 'г', color: 'bg-emerald-500' },
        ].map(card => (
          <div key={card.label} className="rounded-2xl border bg-card/90 p-4 shadow-sm flex flex-col gap-1">
            <span className="text-[10px] text-muted-foreground">{card.label}</span>
            {loading ? <div className="h-6 w-12 bg-muted rounded animate-pulse" /> : (
              <div className="text-lg font-bold text-foreground">{card.value} <span className="text-xs font-normal text-muted-foreground">{card.unit}</span></div>
            )}
          </div>
        ))}
      </div>

      {/* Charts row */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Calorie chart */}
        <div className="rounded-2xl border bg-card/90 p-5 shadow-sm">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Калории</h2>
          {loading ? <div className="h-48 bg-muted rounded animate-pulse" /> : chartData.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-8">Нет данных</p>
          ) : (
            <EChart option={buildCaloriesOption(chartData, calorieTarget)} height={200} />
          )}
        </div>

        {/* Macros trend */}
        <div className="rounded-2xl border bg-card/90 p-5 shadow-sm">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">БЖУ тренд</h2>
          {loading ? <div className="h-48 bg-muted rounded animate-pulse" /> : chartData.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-8">Нет данных</p>
          ) : (
            <EChart option={buildMacrosTrendOption(chartData)} height={200} />
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
              <EChart option={buildNutritionDonutOption(macroPie, 'Всего', ' ккал')} height={160} width={160} className="mx-auto shrink-0 md:mx-0" />
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
              <EChart option={buildMealsTimelineOption(chartData, mealStats)} height={220} />
              <div className="grid gap-2 md:grid-cols-2">
                {mealStats.map((stat) => (
                  <button key={stat.key} onClick={() => setMealFilter(mealFilter === stat.key ? '' : stat.key)}
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
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
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

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-5">
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
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Дневник питания</h2>
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
          <div className="divide-y">
            {filteredDaily.map(day => <DayRow key={day.date} day={day} calorieReference={calorieReference} calorieTarget={calorieTarget} />)}
          </div>
        )}
      </div>
    </div>
  )
}
