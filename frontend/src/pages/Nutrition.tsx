import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Droplets, GlassWater, RotateCcw } from 'lucide-react'
import { EditableWidgetGrid } from '@/components/EditableWidgetGrid'
import { EChart } from '@/components/EChart'
import { ExpandablePanel } from '@/components/ExpandablePanel'
import { InfoTooltip } from '@/components/InfoTooltip'
import { PageSyncButton } from '@/components/PageSyncButton'
import { PageHeader } from '@/components/PageHeader'
import { StyledSelect } from '@/components/StyledSelect'
import { useGlobalDateRange } from '@/hooks/useGlobalDateRange'
import { useIntegrations } from '@/hooks/useIntegrations'
import { useNutritionData } from '@/hooks/useNutritionData'
import { usePageSync } from '@/hooks/usePageSync'
import { cn, syncCaptionForSources } from '@/lib/utils'
import { api, type HydrationBeverageType, type HydrationMode, type NutritionDay, type NutritionTargetsInput } from '@/lib/api'
import { rawDataHref } from '@/lib/raw-data'
import {
  HYDRATION_BEVERAGES,
  HYDRATION_MODE_ACCENT,
  HYDRATION_MODE_NOTES,
  MACRO_COLORS,
  MEAL_COLORS,
  MEAL_KEYS_BY_LABEL,
  MEAL_LABELS,
  MEAL_ORDER,
} from '@/pages/nutrition/constants'
import {
  buildCaloriesOption,
  buildDailyNutritionTimelineOption,
  buildHydrationOption,
  buildMacrosTrendOption,
  buildMealsTimelineOption,
  buildNutritionDonutOption,
  filterNutritionDayByMeal,
} from '@/pages/nutrition/chart-options'
import {
  fmtDate,
  fmtOptionalNumber,
  fmtSyncTime,
  fmtTargetDelta,
  fmtWaterMl,
  fmtWeight,
  hydrationBeverageLabel,
  hydrationModeLabel,
  numberInputValue,
  parseRequiredDecimalOrNull,
} from '@/pages/nutrition/format'
import { NutritionDayDetails, NutritionGoldenMetricCard } from '@/pages/nutrition/components'

export function Nutrition() {
  const globalRange = useGlobalDateRange()
  const navigate = useNavigate()
  const period = 30
  const { summary, golden, daily, loading, reload: reloadNutritionData } = useNutritionData(globalRange.params, period)
  const [mealFilter, setMealFilter] = useState('')
  const { integrations, reload: reloadIntegrations } = useIntegrations()
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

  const reloadNutrition = useCallback(async () => {
    await Promise.all([reloadNutritionData(), reloadIntegrations()])
  }, [reloadNutritionData, reloadIntegrations])

  const { syncing, syncSources } = usePageSync(reloadNutrition)

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
    await syncSources(enabledNutritionIntegrations.map(integration => integration.name))
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
      await reloadNutritionData()
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
      await reloadNutritionData()
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
      await reloadNutritionData()
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
      await reloadNutritionData()
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
      await reloadNutritionData()
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
          { label: enabledNutritionIntegrations.length > 0 ? `${enabledNutritionIntegrations.length} активных источника питания` : 'Источник питания не подключён', tone: enabledNutritionIntegrations.length > 0 ? 'success' : 'warning' },
        ]}
        actions={(
          <PageSyncButton
            label={nutritionSyncLabel}
            syncCaption={nutritionSyncCaption}
            syncing={syncing}
            disabled={enabledNutritionIntegrations.length === 0}
            onClick={handleSyncNutrition}
          />
        )}
      />

      <EditableWidgetGrid
        storageKey="nutrition_widget_layout_v1"
        widgets={[
          { id: 'golden', label: 'Ключевые метрики', layout: { x: 0, y: 0, w: 12, h: 5 }, bounds: { minW: 4, minH: 4, maxH: 10 } },
          { id: 'hydration', label: 'Гидратация', layout: { x: 0, y: 5, w: 12, h: 14 }, bounds: { minW: 6, minH: 8, maxH: 30 } },
          { id: 'trends', label: 'Калории и БЖУ', layout: { x: 0, y: 19, w: 6, h: 8 }, bounds: { minW: 4, minH: 6, maxH: 18 } },
          { id: 'analysis', label: 'Распределение и приёмы пищи', layout: { x: 6, y: 19, w: 6, h: 10 }, bounds: { minW: 4, minH: 6, maxH: 24 } },
          { id: 'targets', label: 'Цели питания', layout: { x: 0, y: 29, w: 12, h: 10 }, bounds: { minW: 5, minH: 5, maxH: 24 } },
          { id: 'daily-log', label: 'Дневник питания и воды', layout: { x: 0, y: 39, w: 12, h: 12 }, bounds: { minW: 5, minH: 6, maxH: 30 } },
        ]}
      >
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
            <h2 className="inline-flex items-center gap-2 text-sm font-semibold text-foreground uppercase tracking-wider">
              Гидратация
              <InfoTooltip text="Чистая вода живёт отдельно, а режим гидратации решает, считать ли чай и кофе частью цели." />
            </h2>
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
          <InfoTooltip text={HYDRATION_MODE_NOTES[hydrationMode]} className="self-center" />
        </div>

        <div className="mt-5 grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1.2fr)_minmax(340px,0.8fr)] xl:items-start">
          <div className="rounded-2xl border border-cyan-500/10 bg-cyan-500/5 p-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <p className="text-[11px] uppercase tracking-wide text-cyan-200/80">Сегодня в цель</p>
                <p className="mt-2 text-3xl font-bold text-foreground">{Math.round(todayHydration)} <span className="text-base font-medium text-muted-foreground">мл</span></p>
                <p className="mt-2 text-xs font-medium text-cyan-100">
                  {typeof waterTarget === 'number'
                    ? waterTargetLeft === 0
                      ? 'Цель закрыта'
                      : `Осталось ${Math.round(waterTargetLeft ?? 0)} мл`
                    : 'Цель не задана'}
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
              <StyledSelect
                value={customHydrationType}
                onChange={e => setCustomHydrationType(e.target.value as HydrationBeverageType)}
                className="rounded-xl focus:ring-2 focus:ring-ring"
              >
                {HYDRATION_BEVERAGES.map(beverage => (
                  <option key={beverage.type} value={beverage.type}>{beverage.label}</option>
                ))}
              </StyledSelect>
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
              <h3 className="inline-flex items-center gap-2 text-sm font-semibold text-foreground uppercase tracking-wider">
                Гидратация по дням
                <InfoTooltip text="Показывает отдельно чистую воду, counted hydration и напитки, которые в цель не идут. Это не смешивается с калориями и БЖУ." />
              </h3>
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
          <h2 className="mb-4 inline-flex items-center gap-2 text-sm font-semibold text-foreground uppercase tracking-wider">
            Структура приёмов пищи
            <InfoTooltip text="Расклад по дням: видно, когда завтрак, обед и ужин реально присутствуют и какой приём пищи тащит калории." />
          </h2>
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
        description={undefined}
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
              <p className="inline-flex items-center gap-1 text-[10px] text-muted-foreground">
                Режим гидратации
                <InfoTooltip text={HYDRATION_MODE_NOTES[hydrationMode]} />
              </p>
              <p className="mt-1 text-lg font-bold text-foreground">{hydrationModeLabel(hydrationMode)}</p>
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
              <h3 className="inline-flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-foreground">
                Ручные цели
                <InfoTooltip text="Заполняй только то, чего не хватает в FatSecret, или то, что хочешь переопределить вручную." />
              </h3>
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
                <p className="inline-flex items-center gap-2 text-xs font-medium text-muted-foreground">
                  Таймлайн по дням
                  <InfoTooltip text="Кликни по бару, чтобы раскрыть состав конкретного дня ниже." />
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
      </EditableWidgetGrid>
    </div>
  )
}
