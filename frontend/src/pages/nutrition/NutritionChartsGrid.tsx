import { EChart } from '@/components/EChart'
import { InfoTooltip } from '@/components/InfoTooltip'
import { cn } from '@/lib/utils'
import type { NutritionDay } from '@/lib/api'
import {
  buildCaloriesOption,
  buildMacrosTrendOption,
  buildMealsTimelineOption,
  buildNutritionDonutOption,
} from '@/pages/nutrition/chart-options'
import { MEAL_KEYS_BY_LABEL } from '@/pages/nutrition/constants'
import type { NutritionMacroSlice, NutritionMealStat, OpenNutritionRaw } from '@/pages/nutrition/types'

export function NutritionTrendsPanel({
  loading,
  chartData,
  calorieTarget,
  onOpenRaw,
}: {
  loading: boolean
  chartData: NutritionDay[]
  calorieTarget?: number
  onOpenRaw: OpenNutritionRaw
}) {
  return (
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
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
              if (day) onOpenRaw({ day, metric: 'calories' })
            }}
          />
        )}
      </div>

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
              if (day) onOpenRaw({ day, metric })
            }}
          />
        )}
      </div>
    </div>
  )
}

export function NutritionAnalysisPanel({
  loading,
  chartData,
  macroPie,
  mealStats,
  mealFilter,
  onOpenRaw,
}: {
  loading: boolean
  chartData: NutritionDay[]
  macroPie: NutritionMacroSlice[]
  mealStats: NutritionMealStat[]
  mealFilter: string
  onOpenRaw: OpenNutritionRaw
}) {
  const macroTotal = macroPie.reduce((sum, item) => sum + item.value, 0)

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
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
                if (metric) onOpenRaw({ metric })
              }}
            />
            <div className="flex min-w-0 flex-1 flex-col gap-2">
              {macroPie.map(macro => (
                <div key={macro.name} className="rounded-xl border bg-background/45 px-3 py-2">
                  <div className="flex items-center gap-3 text-xs">
                    <div className="w-2.5 h-2.5 rounded-full shrink-0" style={{ backgroundColor: macro.color }} />
                    <span className="min-w-0 flex-1 text-sm text-foreground">{macro.name}</span>
                    <span className="shrink-0 text-[11px] font-medium text-muted-foreground">
                      {((macro.value / macroTotal) * 100).toFixed(0)}%
                    </span>
                    <span className="shrink-0 text-sm font-medium text-foreground">{Math.round(macro.value)} ккал</span>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

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
                if (day) onOpenRaw({ day, meal_type: mealType })
              }}
            />
            <div className="grid gap-2 md:grid-cols-2">
              {mealStats.map((stat) => (
                <button key={stat.key} onClick={() => onOpenRaw({ meal_type: stat.key })}
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
  )
}
