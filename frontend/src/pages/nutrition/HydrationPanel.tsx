import { Droplets, GlassWater, RotateCcw } from 'lucide-react'
import { EChart } from '@/components/EChart'
import { InfoTooltip } from '@/components/InfoTooltip'
import { StyledSelect } from '@/components/StyledSelect'
import { cn } from '@/lib/utils'
import type { HydrationBeverageType, HydrationMode, NutritionDay } from '@/lib/api'
import { buildHydrationOption } from '@/pages/nutrition/chart-options'
import {
  HYDRATION_BEVERAGES,
  HYDRATION_MODE_ACCENT,
  HYDRATION_MODE_NOTES,
} from '@/pages/nutrition/constants'
import { fmtWaterMl, hydrationModeLabel } from '@/pages/nutrition/format'
import type { OpenNutritionRaw } from '@/pages/nutrition/types'

type HydrationPanelMetrics = {
  hydrationMode: HydrationMode
  waterTarget?: number
  focusHydration: number
  focusWater: number
  focusCountedDrinks: number
  focusOtherDrinks: number
  waterProgress: number | null
  waterTargetLeft: number | null
  waterGoalDays: number
  hydrationTrackedDays: number
  avgHydration: number
  hasHydrationData: boolean
}

type HydrationPanelForm = {
  customWaterInput: string
  customHydrationType: HydrationBeverageType
  customHydrationInput: string
}

type HydrationPanelStatus = {
  savingTargets: boolean
  savingWater: boolean
  error: string
  notice: string
}

type HydrationPanelActions = {
  onHydrationModeChange: (mode: HydrationMode) => void | Promise<void>
  onAddWater: (deltaMl: number) => void | Promise<void>
  onSetWaterAbsolute: (nextWaterMl: number) => void | Promise<void>
  onSubmitCustomWater: () => void | Promise<void>
  onAddHydration: (beverageType: HydrationBeverageType, deltaMl: number) => void | Promise<void>
  onSubmitCustomHydration: () => void | Promise<void>
  onCustomWaterInputChange: (value: string) => void
  onCustomHydrationTypeChange: (type: HydrationBeverageType) => void
  onCustomHydrationInputChange: (value: string) => void
}

export function HydrationPanel({
  chartData,
  loading,
  focusDateLabel,
  metrics,
  form,
  status,
  actions,
  onOpenRaw,
}: {
  chartData: NutritionDay[]
  loading: boolean
  focusDateLabel: string
  metrics: HydrationPanelMetrics
  form: HydrationPanelForm
  status: HydrationPanelStatus
  actions: HydrationPanelActions
  onOpenRaw: OpenNutritionRaw
}) {
  const {
    hydrationMode,
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
  } = metrics
  const { customWaterInput, customHydrationType, customHydrationInput } = form
  const { savingTargets, savingWater, error, notice } = status
  const {
    onHydrationModeChange,
    onAddWater,
    onSetWaterAbsolute,
    onSubmitCustomWater,
    onAddHydration,
    onSubmitCustomHydration,
    onCustomWaterInputChange,
    onCustomHydrationTypeChange,
    onCustomHydrationInputChange,
  } = actions

  return (
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
            onClick={() => void onHydrationModeChange(mode)}
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
              <p className="text-[11px] uppercase tracking-wide text-cyan-200/80">В цель · {focusDateLabel}</p>
              <p className="mt-2 text-3xl font-bold text-foreground">{Math.round(focusHydration)} <span className="text-base font-medium text-muted-foreground">мл</span></p>
              <p className="mt-2 text-xs font-medium text-cyan-100">
                {typeof waterTarget === 'number'
                  ? waterTargetLeft === 0
                    ? 'Цель закрыта'
                    : `Осталось ${Math.round(waterTargetLeft ?? 0)} мл`
                  : 'Цель не задана'}
              </p>
              <div className="mt-3 flex flex-wrap gap-2 text-xs text-muted-foreground">
                <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1 text-cyan-200">💧 Вода {Math.round(focusWater)} мл</span>
                <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1 text-emerald-200">🍵/☕ В цель {Math.round(focusCountedDrinks)} мл</span>
                {focusOtherDrinks > 0 ? (
                  <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1 text-amber-200">🧃 Отдельно {Math.round(focusOtherDrinks)} мл</span>
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
          <p className="text-[11px] uppercase tracking-wide text-muted-foreground">Быстрый лог · {focusDateLabel}</p>
          <p className="mt-2 text-[11px] text-muted-foreground">Вода</p>
          <div className="mt-3 grid grid-cols-2 gap-2">
            {[250, 500, 750, 1000].map(amount => (
              <button
                key={amount}
                onClick={() => void onAddWater(amount)}
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
                onClick={() => void onAddHydration(beverage.type, beverage.amount)}
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
              onChange={e => onCustomHydrationTypeChange(e.target.value as HydrationBeverageType)}
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
              onChange={e => onCustomHydrationInputChange(e.target.value)}
              placeholder="например, 250"
              className="min-w-0 rounded-xl border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
            />
            <button
              onClick={() => void onSubmitCustomHydration()}
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
              onChange={e => onCustomWaterInputChange(e.target.value)}
              placeholder="вода, например 1800"
              className="min-w-0 rounded-xl border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
            />
            <button
              onClick={() => void onSubmitCustomWater()}
              disabled={savingWater}
              className="rounded-xl border border-cyan-500/20 bg-cyan-500/10 px-3 py-2 text-sm font-medium text-cyan-100 transition-colors hover:bg-cyan-500/15 disabled:cursor-not-allowed disabled:opacity-60"
            >
              Задать воду
            </button>
            <button
              onClick={() => void onSetWaterAbsolute(0)}
              disabled={savingWater}
              className="inline-flex items-center gap-2 rounded-xl border px-3 py-2 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent disabled:cursor-not-allowed disabled:opacity-60 sm:col-span-2"
            >
              <RotateCcw className="h-3.5 w-3.5" />
              Сбросить воду
            </button>
          </div>
          {(error || notice) ? (
            <p className={cn('mt-3 text-xs', error ? 'text-rose-400' : 'text-cyan-200')}>
              {error || notice}
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
              if (day) onOpenRaw({ day, metric: 'hydration' })
            }}
          />
        )}
      </div>
    </div>
  )
}
