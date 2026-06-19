import { useCallback, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { EditableWidgetGrid } from '@/components/EditableWidgetGrid'
import { PageSyncButton } from '@/components/PageSyncButton'
import { PageHeader } from '@/components/PageHeader'
import { useGlobalDateRange } from '@/hooks/useGlobalDateRange'
import { useIntegrations } from '@/hooks/useIntegrations'
import { useNutritionData } from '@/hooks/useNutritionData'
import { usePageSync } from '@/hooks/usePageSync'
import { syncCaptionForSources } from '@/lib/utils'
import type { NutritionDay } from '@/lib/api'
import { rawDataHref } from '@/lib/raw-data'
import { filterNutritionDayByMeal } from '@/pages/nutrition/chart-options'
import { MACRO_COLORS, MEAL_COLORS, MEAL_LABELS, MEAL_ORDER } from '@/pages/nutrition/constants'
import { DailyNutritionLog } from '@/pages/nutrition/DailyNutritionLog'
import { GoldenMetricsGrid } from '@/pages/nutrition/GoldenMetricsGrid'
import { HydrationPanel } from '@/pages/nutrition/HydrationPanel'
import {
  NutritionAnalysisPanel,
  NutritionTrendsPanel,
} from '@/pages/nutrition/NutritionChartsGrid'
import { NutritionTargetsPanel } from '@/pages/nutrition/NutritionTargetsPanel'
import type { NutritionMacroSlice, NutritionMealStat, NutritionRawFilters } from '@/pages/nutrition/types'
import { useNutritionHydration } from '@/pages/nutrition/useNutritionHydration'
import { useNutritionTargets } from '@/pages/nutrition/useNutritionTargets'

const NUTRITION_WIDGETS = [
  { id: 'golden', label: 'Ключевые метрики', layout: { x: 0, y: 0, w: 12, h: 5 }, bounds: { minW: 4, minH: 4, maxH: 10 } },
  { id: 'hydration', label: 'Гидратация', layout: { x: 0, y: 5, w: 12, h: 24 }, bounds: { minW: 6, minH: 12, maxH: 42 } },
  { id: 'trends', label: 'Калории и БЖУ', layout: { x: 0, y: 29, w: 12, h: 10 }, bounds: { minW: 6, minH: 8, maxH: 22 } },
  { id: 'analysis', label: 'Распределение и приёмы пищи', layout: { x: 0, y: 39, w: 12, h: 14 }, bounds: { minW: 6, minH: 10, maxH: 32 } },
  { id: 'targets', label: 'Цели питания', layout: { x: 0, y: 53, w: 12, h: 10 }, bounds: { minW: 5, minH: 5, maxH: 24 } },
  { id: 'daily-log', label: 'Дневник питания и воды', layout: { x: 0, y: 63, w: 12, h: 32 }, bounds: { minW: 5, minH: 14, maxH: 54 } },
]

export function Nutrition() {
  const globalRange = useGlobalDateRange()
  const navigate = useNavigate()
  const period = 30
  const { summary, golden, daily, loading, reload: reloadNutritionData } = useNutritionData(globalRange.params, period)
  const { integrations, reload: reloadIntegrations } = useIntegrations()
  const [mealFilter, setMealFilter] = useState('')
  const targets = summary?.targets

  const reloadNutrition = useCallback(async () => {
    await Promise.all([reloadNutritionData(), reloadIntegrations()])
  }, [reloadNutritionData, reloadIntegrations])

  const { syncing, syncSources } = usePageSync(reloadNutrition)
  const hydration = useNutritionHydration({ reloadNutritionData })
  const nutritionTargets = useNutritionTargets({
    targets,
    reloadNutritionData,
    onHydrationModeChangeStart: hydration.clearHydrationMessages,
  })

  const chartData = useMemo(() => [...daily].reverse(), [daily])
  const enabledNutritionIntegrations = integrations.filter(integration =>
    (integration.name === 'myfitnesspal' || integration.name === 'fatsecret') && integration.enabled
  )
  const nutritionSyncLabel = enabledNutritionIntegrations.length === 1
    ? `Синхронизировать ${enabledNutritionIntegrations[0].display_name}`
    : 'Синхронизировать питание'
  const nutritionSyncCaption = syncCaptionForSources(enabledNutritionIntegrations)

  const avgProtein = chartData.length ? chartData.reduce((sum, day) => sum + day.protein, 0) / chartData.length : 0
  const avgFat = chartData.length ? chartData.reduce((sum, day) => sum + day.fat, 0) / chartData.length : 0
  const avgCarbs = chartData.length ? chartData.reduce((sum, day) => sum + day.carbs, 0) / chartData.length : 0
  const hydrationTrackedDays = chartData.filter(day => day.hydration_ml > 0).length
  const hasHydrationData = chartData.some(day => day.water_ml > 0 || day.counted_drinks_ml > 0 || day.other_drinks_ml > 0 || day.hydration_ml > 0)
  const avgHydration = hydrationTrackedDays > 0
    ? chartData.reduce((sum, day) => sum + day.hydration_ml, 0) / hydrationTrackedDays
    : 0

  const macroPie: NutritionMacroSlice[] = [
    { name: 'Белки', value: avgProtein * 4, color: MACRO_COLORS.protein },
    { name: 'Жиры', value: avgFat * 9, color: MACRO_COLORS.fat },
    { name: 'Углеводы', value: avgCarbs * 4, color: MACRO_COLORS.carbs },
  ].filter(macro => macro.value > 0)

  const mealStats: NutritionMealStat[] = MEAL_ORDER.map((mealType) => {
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
  const filteredDaily = useMemo(() => {
    if (!mealFilter) return daily

    return daily
      .map(day => filterNutritionDayByMeal(day, mealFilter))
      .filter((day): day is NutritionDay => day !== null)
  }, [daily, mealFilter])
  const goldenCards = golden?.cards ?? []
  const selectedDay = filteredDaily[0] ?? null
  const selectedDayDate = selectedDay?.date ?? null
  const calorieReference = Math.max(
    calorieTarget ?? 0,
    ...filteredDaily.map(day => day.calories),
    1,
  )

  const openNutritionRaw = useCallback((filters: NutritionRawFilters = {}) => {
    navigate(rawDataHref('nutrition.days', { ...globalRange.params, ...filters }))
  }, [globalRange.params, navigate])

  async function handleSyncNutrition() {
    if (enabledNutritionIntegrations.length === 0) return
    await syncSources(enabledNutritionIntegrations.map(integration => integration.name))
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
        storageKey="nutrition_widget_layout_v2"
        widgets={NUTRITION_WIDGETS}
      >
        <GoldenMetricsGrid
          cards={goldenCards}
          loading={loading}
        />
        <HydrationPanel
          chartData={chartData}
          loading={loading}
          hydrationMode={nutritionTargets.hydrationMode}
          waterTarget={waterTarget}
          todayHydration={todayHydration}
          todayWater={todayWater}
          todayCountedDrinks={todayCountedDrinks}
          todayOtherDrinks={todayOtherDrinks}
          waterProgress={waterProgress}
          waterTargetLeft={waterTargetLeft}
          waterGoalDays={waterGoalDays}
          hydrationTrackedDays={hydrationTrackedDays}
          avgHydration={avgHydration}
          hasHydrationData={hasHydrationData}
          savingTargets={nutritionTargets.savingTargets}
          savingWater={hydration.savingWater}
          waterError={hydration.waterError}
          waterNotice={hydration.waterNotice}
          customWaterInput={hydration.customWaterInput}
          customHydrationType={hydration.customHydrationType}
          customHydrationInput={hydration.customHydrationInput}
          onHydrationModeChange={nutritionTargets.handleHydrationModeChange}
          onAddWater={hydration.handleAddWater}
          onSetWaterAbsolute={hydration.handleSetWaterAbsolute}
          onSubmitCustomWater={hydration.handleSubmitCustomWater}
          onAddHydration={hydration.handleAddHydration}
          onSubmitCustomHydration={hydration.handleSubmitCustomHydration}
          onCustomWaterInputChange={hydration.setCustomWaterInput}
          onCustomHydrationTypeChange={hydration.setCustomHydrationType}
          onCustomHydrationInputChange={hydration.setCustomHydrationInput}
          onOpenRaw={openNutritionRaw}
        />
        <NutritionTrendsPanel
          loading={loading}
          chartData={chartData}
          calorieTarget={calorieTarget}
          onOpenRaw={openNutritionRaw}
        />
        <NutritionAnalysisPanel
          loading={loading}
          chartData={chartData}
          macroPie={macroPie}
          mealStats={mealStats}
          mealFilter={mealFilter}
          onOpenRaw={openNutritionRaw}
        />
        <NutritionTargetsPanel
          targets={targets}
          targetsForm={nutritionTargets.targetsForm}
          hydrationMode={nutritionTargets.hydrationMode}
          savingTargets={nutritionTargets.savingTargets}
          targetsError={nutritionTargets.targetsError}
          targetsNotice={nutritionTargets.targetsNotice}
          onTargetsFieldChange={nutritionTargets.setTargetsField}
          onSaveTargets={nutritionTargets.handleSaveTargets}
        />
        <DailyNutritionLog
          loading={loading}
          filteredDaily={filteredDaily}
          selectedDay={selectedDay}
          selectedDayDate={selectedDayDate}
          mealFilter={mealFilter}
          calorieReference={calorieReference}
          calorieTarget={calorieTarget}
          onMealFilterChange={setMealFilter}
          onOpenRaw={openNutritionRaw}
        />
      </EditableWidgetGrid>
    </div>
  )
}
