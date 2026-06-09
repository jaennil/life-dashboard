import { Activity, Droplets, Flame, Target, UtensilsCrossed } from 'lucide-react'
import { InfoTooltip } from '@/components/InfoTooltip'
import type { NutritionDay, NutritionGoldenCard } from '@/lib/api'
import { cn } from '@/lib/utils'
import { MEAL_LABELS } from '@/pages/nutrition/constants'
import {
  fmtDate,
  hydrationBeverageEmoji,
  hydrationBeverageLabel,
} from '@/pages/nutrition/format'

const GOLDEN_TONE_STYLES: Record<NutritionGoldenCard['tone'], string> = {
  success: 'border-emerald-500/25 bg-card/90',
  warning: 'border-amber-500/25 bg-card/90',
  danger: 'border-rose-500/25 bg-card/90',
  muted: 'border-border bg-card/90',
}

const GOLDEN_ICONS: Record<string, typeof Activity> = {
  consistency: Activity,
  calories: Flame,
  protein: Target,
  hydration: Droplets,
  structure: UtensilsCrossed,
}

export function NutritionGoldenMetricCard({
  card,
  loading,
}: {
  card: NutritionGoldenCard
  loading: boolean
}) {
  const Icon = GOLDEN_ICONS[card.key] ?? Target
  return (
    <div className={cn('rounded-2xl border p-4 shadow-sm flex min-h-[112px] flex-col gap-3', GOLDEN_TONE_STYLES[card.tone])}>
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

export function NutritionDayDetails({ day }: { day: NutritionDay }) {
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
