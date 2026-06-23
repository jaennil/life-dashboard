import { useCallback, useMemo, useState } from 'react'
import { AlertTriangle, RefreshCw } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { PageSyncButton } from '@/components/PageSyncButton'
import { PageHeader } from '@/components/PageHeader'
import { useGlobalDateRange } from '@/hooks/useGlobalDateRange'
import { useIntegrations } from '@/hooks/useIntegrations'
import { useNutritionData } from '@/hooks/useNutritionData'
import { usePageSync } from '@/hooks/usePageSync'
import { syncCaptionForSources } from '@/lib/utils'
import type { NutritionDay } from '@/lib/api'
import { rawDataHref } from '@/lib/raw-data'
import { DailyNutritionLog } from '@/pages/nutrition/DailyNutritionLog'
import { fmtDate } from '@/pages/nutrition/format'
import { GoldenMetricsGrid } from '@/pages/nutrition/GoldenMetricsGrid'
import { HydrationPanel } from '@/pages/nutrition/HydrationPanel'
import {
  NutritionAnalysisPanel,
  NutritionTrendsPanel,
} from '@/pages/nutrition/NutritionChartsGrid'
import { NutritionTargetsPanel } from '@/pages/nutrition/NutritionTargetsPanel'
import {
  buildNutritionMacroSlices,
  buildNutritionMealStats,
  filterNutritionDayByMeal,
} from '@/pages/nutrition/selectors'
import type { NutritionRawFilters } from '@/pages/nutrition/types'
import { useNutritionHydration } from '@/pages/nutrition/useNutritionHydration'
import { useNutritionTargets } from '@/pages/nutrition/useNutritionTargets'

export function Nutrition() {
  const globalRange = useGlobalDateRange()
  const navigate = useNavigate()
  const period = 30
  const {
    summary,
    golden,
    daily,
    loading,
    error,
    reload: reloadNutritionData,
  } = useNutritionData(globalRange.params, period)
  const { integrations, reload: reloadIntegrations } = useIntegrations()
  const [mealFilter, setMealFilter] = useState('')
  const [preferredDayDate, setPreferredDayDate] = useState<string | null>(null)
  const targets = summary?.targets

  const reloadNutrition = useCallback(async () => {
    await Promise.all([reloadNutritionData(), reloadIntegrations()])
  }, [reloadNutritionData, reloadIntegrations])

  const { syncing, syncSources } = usePageSync(reloadNutrition)
  const focusDate = summary?.focus_date ?? (globalRange.to || undefined)
  const focusDateLabel = focusDate ? fmtDate(focusDate) : 'сегодня'
  const hydration = useNutritionHydration({
    reloadNutritionData,
    targetDate: focusDate,
    targetDateLabel: focusDateLabel,
  })
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

  const hydrationTrackedDays = chartData.filter(day => day.hydration_ml > 0).length
  const hasHydrationData = chartData.some(day => day.water_ml > 0 || day.counted_drinks_ml > 0 || day.other_drinks_ml > 0 || day.hydration_ml > 0)
  const avgHydration = hydrationTrackedDays > 0
    ? chartData.reduce((sum, day) => sum + day.hydration_ml, 0) / hydrationTrackedDays
    : 0

  const macroPie = useMemo(() => buildNutritionMacroSlices(chartData), [chartData])
  const mealStats = useMemo(() => buildNutritionMealStats(chartData), [chartData])

  const calorieTarget = targets?.target_calories
  const waterTarget = targets?.target_water_ml
  const focusWater = summary?.today_water_ml ?? 0
  const focusHydration = summary?.today_hydration_ml ?? focusWater
  const focusCountedDrinks = summary?.today_counted_drinks_ml ?? 0
  const focusOtherDrinks = summary?.today_other_drinks_ml ?? 0
  const waterProgress = typeof waterTarget === 'number' && waterTarget > 0 ? Math.min(focusHydration / waterTarget, 1) : null
  const waterTargetLeft = typeof waterTarget === 'number' ? Math.max(waterTarget - focusHydration, 0) : null
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
  const selectedDay = filteredDaily.find(day => day.date === preferredDayDate)
    ?? filteredDaily[0]
    ?? null
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
    <div className="flex min-w-0 flex-col gap-5 lg:gap-6">
      <PageHeader
        eyebrow="Nutrition"
        title="Питание"
        description="Калории, БЖУ, вода и дневник питания в одном месте. Ежедневный hydration-трекинг живёт рядом с питанием, а редкие настройки убраны ниже."
        compactOnMobile
        hideDescriptionOnMobile
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

      {error ? (
        <div role="alert" className="flex flex-col gap-3 rounded-xl border border-rose-500/25 bg-rose-500/10 p-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex min-w-0 items-start gap-3">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-rose-300" />
            <p className="text-sm text-rose-100">{error}</p>
          </div>
          <button
            type="button"
            onClick={() => void reloadNutritionData()}
            disabled={loading}
            className="inline-flex items-center justify-center gap-2 rounded-lg border border-rose-400/25 px-3 py-2 text-sm font-medium text-rose-100 transition-colors hover:bg-rose-400/10 disabled:opacity-50"
          >
            <RefreshCw className="h-4 w-4" />
            Повторить
          </button>
        </div>
      ) : null}

      <GoldenMetricsGrid cards={goldenCards} loading={loading} />

      <HydrationPanel
        chartData={chartData}
        loading={loading}
        focusDateLabel={focusDateLabel}
        metrics={{
          hydrationMode: nutritionTargets.hydrationMode,
          waterTarget,
          focusHydration,
          focusWater,
          focusCountedDrinks,
          focusOtherDrinks,
          waterProgress,
          waterTargetLeft,
          waterGoalDays,
          hydrationTrackedDays,
          avgHydration,
          hasHydrationData,
        }}
        form={{
          customWaterInput: hydration.customWaterInput,
          customHydrationType: hydration.customHydrationType,
          customHydrationInput: hydration.customHydrationInput,
        }}
        status={{
          savingTargets: loading || nutritionTargets.savingTargets,
          savingWater: hydration.savingWater,
          error: hydration.waterError,
          notice: hydration.waterNotice,
        }}
        actions={{
          onHydrationModeChange: nutritionTargets.handleHydrationModeChange,
          onAddWater: hydration.handleAddWater,
          onSetWaterAbsolute: hydration.handleSetWaterAbsolute,
          onSubmitCustomWater: hydration.handleSubmitCustomWater,
          onAddHydration: hydration.handleAddHydration,
          onSubmitCustomHydration: hydration.handleSubmitCustomHydration,
          onCustomWaterInputChange: hydration.setCustomWaterInput,
          onCustomHydrationTypeChange: hydration.setCustomHydrationType,
          onCustomHydrationInputChange: hydration.setCustomHydrationInput,
        }}
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

      <DailyNutritionLog
        loading={loading}
        filteredDaily={filteredDaily}
        selectedDay={selectedDay}
        selectedDayDate={selectedDayDate}
        mealFilter={mealFilter}
        calorieReference={calorieReference}
        calorieTarget={calorieTarget}
        onMealFilterChange={setMealFilter}
        onSelectDay={setPreferredDayDate}
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
    </div>
  )
}
