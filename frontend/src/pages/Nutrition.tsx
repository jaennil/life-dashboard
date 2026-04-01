import { useEffect, useState } from 'react'
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell } from 'recharts'
import { ChevronDown, ChevronUp, Flame } from 'lucide-react'
import { cn } from '@/lib/utils'
import { api, type NutritionSummary, type NutritionDay } from '@/lib/api'

const MEAL_LABELS: Record<string, string> = {
  breakfast: 'Завтрак',
  lunch: 'Обед',
  dinner: 'Ужин',
  snacks: 'Перекус',
  other: 'Прочее',
}

function fmtDate(iso: string) {
  return new Date(iso).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })
}

function fmtShort(iso: string) {
  const d = new Date(iso)
  return `${d.getDate()}.${String(d.getMonth() + 1).padStart(2, '0')}`
}

const CALORIE_TARGET = 2000

const CustomTooltip = ({ active, payload, label }: any) => {
  if (!active || !payload?.length) return null
  return (
    <div className="rounded-xl border bg-card px-4 py-3 text-sm shadow-lg">
      <p className="font-medium text-foreground mb-1">{fmtDate(label)}</p>
      <p className="text-orange-400">{payload[0]?.value?.toFixed(0)} ккал</p>
    </div>
  )
}

function DayRow({ day }: { day: NutritionDay }) {
  const [open, setOpen] = useState(false)
  const hasMeals = day.meals.some(m => m.items.length > 0)
  const pct = Math.min((day.calories / CALORIE_TARGET) * 100, 100)
  const overTarget = day.calories > CALORIE_TARGET

  return (
    <div>
      <button
        onClick={() => setOpen(o => !o)}
        disabled={!hasMeals}
        className="w-full px-5 py-3 flex items-center gap-3 hover:bg-muted/40 transition-colors text-left disabled:cursor-default"
      >
        <div className="w-9 h-9 rounded-xl bg-muted flex items-center justify-center text-base shrink-0">
          🍽️
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between gap-2">
            <p className="text-sm font-medium text-foreground">{fmtDate(day.date)}</p>
            <span className={cn('text-sm font-semibold tabular-nums', overTarget ? 'text-rose-400' : 'text-orange-400')}>
              {day.calories.toFixed(0)} ккал
            </span>
          </div>
          <div className="flex items-center gap-3 mt-1.5">
            <div className="flex-1 h-1.5 rounded-full bg-muted overflow-hidden">
              <div
                className={cn('h-full rounded-full', overTarget ? 'bg-rose-400' : 'bg-orange-400')}
                style={{ width: `${pct}%` }}
              />
            </div>
            <div className="flex gap-2 text-xs text-muted-foreground shrink-0">
              <span>Б {day.protein.toFixed(0)}г</span>
              <span>Ж {day.fat.toFixed(0)}г</span>
              <span>У {day.carbs.toFixed(0)}г</span>
            </div>
          </div>
        </div>
        {hasMeals && (
          open
            ? <ChevronUp className="w-4 h-4 text-muted-foreground shrink-0" />
            : <ChevronDown className="w-4 h-4 text-muted-foreground shrink-0" />
        )}
      </button>

      {open && hasMeals && (
        <div className="px-5 pb-3 flex flex-col gap-3">
          {day.meals.map(meal => (
            <div key={meal.meal_type}>
              <p className="text-xs font-semibold text-foreground mb-1">
                {MEAL_LABELS[meal.meal_type] ?? meal.meal_type}
              </p>
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
                        {item.macros.fiber > 0 && <span>Клетч {item.macros.fiber?.toFixed(1)}г</span>}
                        {item.macros.sugar > 0 && <span>Сахар {item.macros.sugar?.toFixed(1)}г</span>}
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

  useEffect(() => {
    Promise.all([api.getNutritionSummary(), api.getNutritionDaily()])
      .then(([s, d]) => { setSummary(s); setDaily(d) })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [])

  const chartData = [...daily].reverse()

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-bold text-foreground">Питание</h1>
        <p className="text-sm text-muted-foreground mt-1">
          {new Date().toLocaleDateString('ru-RU', { month: 'long', year: 'numeric' })}
        </p>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        {[
          { label: 'Сегодня', value: summary ? `${summary.today_kcal.toFixed(0)} ккал` : '—', color: 'bg-orange-500' },
          { label: 'Ср. за 7 дней', value: summary ? `${summary.avg_calories.toFixed(0)} ккал` : '—', color: 'bg-amber-500' },
          { label: 'Ср. белки', value: summary ? `${summary.avg_protein.toFixed(0)} г` : '—', color: 'bg-blue-500' },
          { label: 'Ср. углеводы', value: summary ? `${summary.avg_carbs.toFixed(0)} г` : '—', color: 'bg-emerald-500' },
        ].map(card => (
          <div key={card.label} className="rounded-xl border bg-card p-4 flex flex-col gap-2">
            <div className="flex items-center justify-between">
              <span className="text-xs text-muted-foreground">{card.label}</span>
              <div className={cn('flex items-center justify-center w-7 h-7 rounded-lg', card.color)}>
                <Flame className="w-3.5 h-3.5 text-white" />
              </div>
            </div>
            {loading
              ? <div className="h-7 w-16 bg-muted rounded animate-pulse" />
              : <div className="text-xl font-bold text-foreground">{card.value}</div>}
          </div>
        ))}
      </div>

      {/* Calorie chart */}
      <div className="rounded-xl border bg-card p-5">
        <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Калории за 2 недели</h2>
        {loading ? (
          <div className="h-40 bg-muted rounded animate-pulse" />
        ) : chartData.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center py-8">Нет данных. Подключи FatSecret в настройках.</p>
        ) : (
          <ResponsiveContainer width="100%" height={180}>
            <BarChart data={chartData} barCategoryGap="35%">
              <XAxis dataKey="date" tickFormatter={fmtShort} tick={{ fontSize: 11 }} axisLine={false} tickLine={false} />
              <YAxis tick={{ fontSize: 11 }} axisLine={false} tickLine={false} width={36} />
              <Tooltip content={<CustomTooltip />} cursor={{ opacity: 0.1 }} />
              <Bar dataKey="calories" radius={[4, 4, 0, 0]}>
                {chartData.map((d, i) => (
                  <Cell key={i} fill={d.calories > CALORIE_TARGET ? '#f87171' : '#f97316'} />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        )}
      </div>

      {/* Daily log */}
      <div className="rounded-xl border bg-card overflow-hidden">
        <div className="px-5 py-4 border-b">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Дневник питания</h2>
        </div>
        {loading ? (
          <div className="divide-y">
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="px-5 py-3 flex gap-3">
                <div className="h-9 w-9 bg-muted rounded-xl animate-pulse shrink-0" />
                <div className="flex-1 flex flex-col gap-1.5">
                  <div className="h-4 w-24 bg-muted rounded animate-pulse" />
                  <div className="h-3 w-full bg-muted rounded animate-pulse" />
                </div>
              </div>
            ))}
          </div>
        ) : daily.length === 0 ? (
          <div className="px-5 py-8 text-sm text-muted-foreground text-center">
            Нет данных. Подключи FatSecret в настройках.
          </div>
        ) : (
          <div className="divide-y">
            {daily.map(day => <DayRow key={day.date} day={day} />)}
          </div>
        )}
      </div>
    </div>
  )
}
