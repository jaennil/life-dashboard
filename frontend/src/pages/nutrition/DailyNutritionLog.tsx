import { Database } from 'lucide-react'
import { EChart } from '@/components/EChart'
import { InfoTooltip } from '@/components/InfoTooltip'
import { cn } from '@/lib/utils'
import type { NutritionDay } from '@/lib/api'
import { buildDailyNutritionTimelineOption } from '@/pages/nutrition/chart-options'
import { NutritionDayDetails } from '@/pages/nutrition/components'
import { MEAL_LABELS } from '@/pages/nutrition/constants'
import { fmtDate } from '@/pages/nutrition/format'
import type { OpenNutritionRaw } from '@/pages/nutrition/types'

export function DailyNutritionLog({
  loading,
  filteredDaily,
  selectedDay,
  selectedDayDate,
  mealFilter,
  calorieReference,
  calorieTarget,
  onMealFilterChange,
  onSelectDay,
  onOpenRaw,
}: {
  loading: boolean
  filteredDaily: NutritionDay[]
  selectedDay: NutritionDay | null
  selectedDayDate: string | null
  mealFilter: string
  calorieReference: number
  calorieTarget?: number
  onMealFilterChange: (mealType: string) => void
  onSelectDay: (date: string) => void
  onOpenRaw: OpenNutritionRaw
}) {
  return (
    <div className="rounded-2xl border bg-card/90 overflow-hidden shadow-sm">
      <div className="flex flex-col gap-3 border-b px-4 py-4 sm:px-5 lg:flex-row lg:items-center lg:justify-between">
        <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Дневник питания и воды</h2>
        <div className="-mx-1 flex max-w-full gap-1 overflow-x-auto px-1 pb-1">
          <button onClick={() => onMealFilterChange('')}
            className={cn('px-2 py-1 text-xs rounded-lg transition-colors', !mealFilter ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:bg-accent')}>
            Все
          </button>
          {Object.entries(MEAL_LABELS).map(([key, label]) => (
            <button key={key} onClick={() => onMealFilterChange(mealFilter === key ? '' : key)}
              className={cn('px-2 py-1 text-xs rounded-lg transition-colors', mealFilter === key ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:bg-accent')}>
              {label}
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
        <div className="p-4 sm:p-5">
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

          <div className="mt-4 max-h-[36rem] overflow-y-auto overscroll-contain pr-1">
            <EChart
              option={buildDailyNutritionTimelineOption(filteredDaily, calorieReference, calorieTarget, selectedDayDate)}
              height={Math.max(280, filteredDaily.length * 44)}
              onClick={(params) => {
                const dataIndex = typeof params.dataIndex === 'number' ? params.dataIndex : null
                const day = dataIndex == null ? null : filteredDaily[dataIndex]
                if (day) onSelectDay(day.date)
              }}
            />
          </div>

          {selectedDay ? (
            <div className="mt-5 border-t border-border/70 pt-5">
              <div className="mb-4 flex justify-end">
                <button
                  type="button"
                  onClick={() => onOpenRaw({ day: selectedDay.date })}
                  className="inline-flex items-center gap-2 rounded-lg border px-3 py-2 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  <Database className="h-3.5 w-3.5" />
                  Сырые данные дня
                </button>
              </div>
              <NutritionDayDetails day={selectedDay} />
            </div>
          ) : null}
        </div>
      )}
    </div>
  )
}
