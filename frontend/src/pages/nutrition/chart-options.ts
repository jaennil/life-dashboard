import type { EChartsCoreOption } from 'echarts/core'
import type { NutritionDay } from '@/lib/api'
import { CHART_GRID, CHART_MUTED, CHART_TEXT, CHART_TOOLTIP } from '@/lib/chart-theme'
import { MACRO_COLORS } from '@/pages/nutrition/constants'
import {
  fmtDate,
  fmtShort,
  hydrationBeverageEmoji,
  hydrationBeverageLabel,
} from '@/pages/nutrition/format'

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

export function buildCaloriesOption(data: NutritionDay[], calorieTarget?: number): EChartsCoreOption {
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

export function buildMacrosTrendOption(data: NutritionDay[]): EChartsCoreOption {
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

export function buildHydrationOption(data: NutritionDay[], waterTarget?: number): EChartsCoreOption {
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

export function buildNutritionDonutOption(data: Array<{ name: string; value: number; color: string }>, centerLabel: string, suffix: string): EChartsCoreOption {
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

export function buildMealsTimelineOption(
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

export function filterNutritionDayByMeal(day: NutritionDay, mealType: string): NutritionDay | null {
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

export function buildDailyNutritionTimelineOption(
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
