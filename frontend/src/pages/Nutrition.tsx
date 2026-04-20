import { useCallback, useEffect, useState } from 'react'
import {
  BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell,
  AreaChart, Area, Legend, PieChart, Pie, ReferenceLine,
  type TooltipValueType,
} from 'recharts'
import type { TooltipContentProps } from 'recharts/types/component/Tooltip'
import { ChevronDown, ChevronUp } from 'lucide-react'
import { PageSyncButton } from '@/components/PageSyncButton'
import { cn, syncCaptionForSources } from '@/lib/utils'
import { api, type NutritionSummary, type NutritionDay, type Integration } from '@/lib/api'

const MEAL_LABELS: Record<string, string> = {
  breakfast: 'Завтрак', lunch: 'Обед', dinner: 'Ужин', snacks: 'Перекус', other: 'Прочее',
}

const MACRO_COLORS = { protein: '#3b82f6', fat: '#f97316', carbs: '#10b981', fiber: '#8b5cf6' }

function fmtDate(iso: string) {
  return new Date(iso).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })
}

function fmtShort(iso: string) {
  const d = new Date(iso)
  return `${d.getDate()}.${String(d.getMonth() + 1).padStart(2, '0')}`
}

function tooltipNumber(value: TooltipValueType | undefined) {
  const scalar = Array.isArray(value) ? value[0] : value
  if (typeof scalar === 'number') return scalar
  if (typeof scalar === 'string') {
    const parsed = Number(scalar)
    if (Number.isFinite(parsed)) return parsed
  }
  return 0
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

  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const [s, d] = await Promise.all([api.getNutritionSummary(), api.getNutritionDaily()])
      setSummary(s)
      setDaily(d)
    } catch (error) {
      console.error(error)
    } finally {
      setLoading(false)
    }
  }, [])

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

  const chartData = [...daily].reverse().slice(-period)
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

  // Meal distribution
  const mealTotals: Record<string, number> = {}
  chartData.forEach(d => d.meals.forEach(m => {
    const cal = m.items.reduce((s, i) => s + i.calories, 0)
    mealTotals[m.meal_type] = (mealTotals[m.meal_type] || 0) + cal
  }))
  const mealPie = Object.entries(mealTotals).map(([name, value]) => ({
    name: MEAL_LABELS[name] || name, value: Math.round(value),
  })).sort((a, b) => b.value - a.value)

  const mealColors = ['#f97316', '#3b82f6', '#10b981', '#8b5cf6', '#f43f5e']
  const targets = summary?.targets
  const calorieTarget = targets?.target_calories
  const calorieReference = Math.max(
    calorieTarget ?? 0,
    ...daily.map(day => day.calories),
    1,
  )

  // Filtered daily
  const filteredDaily = mealFilter
    ? daily.map(d => ({ ...d, meals: d.meals.filter(m => m.meal_type === mealFilter) }))
    : daily

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

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div>
          <h1 className="text-2xl font-bold text-foreground">Питание</h1>
          <p className="text-sm text-muted-foreground mt-1">
            {new Date().toLocaleDateString('ru-RU', { month: 'long', year: 'numeric' })}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2 xl:justify-end">
          <PageSyncButton
            label={nutritionSyncLabel}
            syncCaption={nutritionSyncCaption}
            syncing={syncing}
            disabled={enabledNutritionIntegrations.length === 0}
            onClick={handleSyncNutrition}
          />
          <div className="flex gap-1">
            {PERIODS.map(p => (
              <button key={p.days} onClick={() => setPeriod(p.days)}
                className={cn('px-3 py-1 text-xs rounded-lg transition-colors',
                  period === p.days ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:bg-accent')}>
                {p.label}
              </button>
            ))}
          </div>
        </div>
      </div>

      {targets && (
        <div className="rounded-xl border bg-card p-5 flex flex-col gap-4">
          <div className="flex flex-col gap-1 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Цели FatSecret</h2>
              <p className="text-xs text-muted-foreground mt-1">Синхронизация профиля: {fmtSyncTime(targets.synced_at)}</p>
            </div>
            {targets.source && <span className="text-[10px] uppercase tracking-wider text-muted-foreground bg-muted rounded-lg px-2 py-1">{targets.source}</span>}
          </div>

          <div className="grid grid-cols-2 lg:grid-cols-5 gap-3">
            <div className="rounded-xl bg-muted/40 p-3">
              <p className="text-[10px] text-muted-foreground">Текущий вес</p>
              <p className="text-lg font-bold text-foreground mt-1">{fmtWeight(targets.current_weight_kg)}</p>
              {targets.current_weight_date && <p className="text-[10px] text-muted-foreground mt-1">{fmtDate(targets.current_weight_date)}</p>}
            </div>
            <div className="rounded-xl bg-muted/40 p-3">
              <p className="text-[10px] text-muted-foreground">Целевой вес</p>
              <p className="text-lg font-bold text-foreground mt-1">{fmtWeight(targets.target_weight_kg)}</p>
            </div>
            <div className="rounded-xl bg-muted/40 p-3">
              <p className="text-[10px] text-muted-foreground">До цели</p>
              <p className="text-lg font-bold text-foreground mt-1">{fmtTargetDelta(targets.current_weight_kg, targets.target_weight_kg)}</p>
            </div>
            <div className="rounded-xl bg-muted/40 p-3">
              <p className="text-[10px] text-muted-foreground">Рост</p>
              <p className="text-lg font-bold text-foreground mt-1">{fmtOptionalNumber(targets.height_cm, ' см')}</p>
            </div>
            <div className="rounded-xl bg-muted/40 p-3">
              <p className="text-[10px] text-muted-foreground">Цель ккал</p>
              <p className="text-lg font-bold text-foreground mt-1">{fmtOptionalNumber(targets.target_calories, ' ккал')}</p>
            </div>
          </div>

          <div className="grid grid-cols-3 gap-3">
            <div className="rounded-xl bg-blue-500/10 p-3">
              <p className="text-[10px] text-blue-300">Белки</p>
              <p className="text-base font-bold text-blue-200 mt-1">{fmtOptionalNumber(targets.target_protein_g, ' г')}</p>
            </div>
            <div className="rounded-xl bg-orange-500/10 p-3">
              <p className="text-[10px] text-orange-300">Жиры</p>
              <p className="text-base font-bold text-orange-200 mt-1">{fmtOptionalNumber(targets.target_fat_g, ' г')}</p>
            </div>
            <div className="rounded-xl bg-emerald-500/10 p-3">
              <p className="text-[10px] text-emerald-300">Углеводы</p>
              <p className="text-base font-bold text-emerald-200 mt-1">{fmtOptionalNumber(targets.target_carbs_g, ' г')}</p>
            </div>
          </div>

          {targets.api_notes?.map(note => (
            <p key={note} className="text-xs text-muted-foreground border border-dashed rounded-xl px-3 py-2">{note}</p>
          ))}
        </div>
      )}

      {/* Summary cards */}
      <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
        {[
          { label: 'Сегодня', value: summary ? `${summary.today_kcal.toFixed(0)}` : '—', unit: 'ккал', color: 'bg-orange-500' },
          { label: `Ср. ккал/${period}д`, value: avgCalories.toFixed(0), unit: 'ккал', color: 'bg-amber-500' },
          { label: 'Ср. белки', value: avgProtein.toFixed(0), unit: 'г', color: 'bg-blue-500' },
          { label: 'Ср. жиры', value: avgFat.toFixed(0), unit: 'г', color: 'bg-orange-500' },
          { label: 'Ср. углеводы', value: avgCarbs.toFixed(0), unit: 'г', color: 'bg-emerald-500' },
        ].map(card => (
          <div key={card.label} className="rounded-xl border bg-card p-4 flex flex-col gap-1">
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
        <div className="rounded-xl border bg-card p-5">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Калории</h2>
          {loading ? <div className="h-48 bg-muted rounded animate-pulse" /> : chartData.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-8">Нет данных</p>
          ) : (
            <ResponsiveContainer width="100%" height={200}>
              <BarChart data={chartData} barCategoryGap="25%">
                <XAxis dataKey="date" tickFormatter={fmtShort} tick={{ fontSize: 11 }} axisLine={false} tickLine={false} />
                <YAxis tick={{ fontSize: 11 }} axisLine={false} tickLine={false} width={36} />
                <Tooltip content={({ active, payload, label }: TooltipContentProps<TooltipValueType, string | number>) => active && payload?.length ? (
                  <div className="rounded-xl border bg-card px-4 py-3 text-sm shadow-lg">
                    <p className="font-medium text-foreground mb-1">{typeof label === 'string' ? fmtDate(label) : String(label ?? '')}</p>
                    <p className="text-orange-400">{tooltipNumber(payload[0]?.value).toFixed(0)} ккал</p>
                  </div>
                ) : null} cursor={{ opacity: 0.1 }} />
                <Bar dataKey="calories" radius={[4, 4, 0, 0]}>
                  {chartData.map((d, i) => <Cell key={i} fill={typeof calorieTarget === 'number' && d.calories > calorieTarget ? '#f87171' : '#f97316'} />)}
                </Bar>
                {typeof calorieTarget === 'number' && (
                  <ReferenceLine y={calorieTarget} stroke="#fbbf24" strokeDasharray="4 4" ifOverflow="extendDomain" />
                )}
              </BarChart>
            </ResponsiveContainer>
          )}
        </div>

        {/* Macros trend */}
        <div className="rounded-xl border bg-card p-5">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">БЖУ тренд</h2>
          {loading ? <div className="h-48 bg-muted rounded animate-pulse" /> : chartData.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-8">Нет данных</p>
          ) : (
            <ResponsiveContainer width="100%" height={200}>
              <AreaChart data={chartData}>
                <XAxis dataKey="date" tickFormatter={fmtShort} tick={{ fontSize: 11 }} axisLine={false} tickLine={false} />
                <YAxis tick={{ fontSize: 11 }} axisLine={false} tickLine={false} width={30} />
                <Tooltip content={({ active, payload, label }: TooltipContentProps<TooltipValueType, string | number>) => active && payload?.length ? (
                  <div className="rounded-xl border bg-card px-4 py-3 text-sm shadow-lg">
                    <p className="font-medium text-foreground mb-1">{typeof label === 'string' ? fmtDate(label) : String(label ?? '')}</p>
                    {payload.map((point, index) => <p key={`${point.name ?? 'series'}-${index}`} style={{ color: point.color }}>{point.name === 'protein' ? 'Белки' : point.name === 'fat' ? 'Жиры' : point.name === 'carbs' ? 'Углеводы' : 'Клетчатка'}: {tooltipNumber(point.value).toFixed(0)}г</p>)}
                  </div>
                ) : null} />
                <Legend formatter={v => v === 'protein' ? 'Белки' : v === 'fat' ? 'Жиры' : v === 'carbs' ? 'Углеводы' : 'Клетчатка'} wrapperStyle={{ fontSize: 11 }} />
                <Area type="monotone" dataKey="protein" stroke={MACRO_COLORS.protein} fill={MACRO_COLORS.protein} fillOpacity={0.1} strokeWidth={2} />
                <Area type="monotone" dataKey="fat" stroke={MACRO_COLORS.fat} fill={MACRO_COLORS.fat} fillOpacity={0.1} strokeWidth={2} />
                <Area type="monotone" dataKey="carbs" stroke={MACRO_COLORS.carbs} fill={MACRO_COLORS.carbs} fillOpacity={0.1} strokeWidth={2} />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </div>
      </div>

      {/* Pie charts row */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
        {/* Macro distribution */}
        <div className="rounded-xl border bg-card p-5">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Распределение БЖУ (ккал)</h2>
          {loading || macroPie.length === 0 ? <div className="h-40 flex items-center justify-center text-sm text-muted-foreground">Нет данных</div> : (
            <div className="flex items-center gap-6">
              <div style={{ width: 160, height: 160, flexShrink: 0 }}>
                <PieChart width={160} height={160}>
                  <Pie data={macroPie} dataKey="value" cx={80} cy={80} innerRadius={45} outerRadius={72} paddingAngle={3}>
                    {macroPie.map((m, i) => <Cell key={i} fill={m.color} />)}
                  </Pie>
                  <Tooltip formatter={(value) => `${tooltipNumber(value).toFixed(0)} ккал`} />
                </PieChart>
              </div>
              <div className="flex flex-col gap-2">
                {macroPie.map(m => (
                  <div key={m.name} className="flex items-center gap-2 text-xs">
                    <div className="w-2.5 h-2.5 rounded-full" style={{ backgroundColor: m.color }} />
                    <span className="text-foreground">{m.name}</span>
                    <span className="text-muted-foreground">{((m.value / macroPie.reduce((s, x) => s + x.value, 0)) * 100).toFixed(0)}%</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* Meal distribution */}
        <div className="rounded-xl border bg-card p-5">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Калории по приёмам пищи</h2>
          {loading || mealPie.length === 0 ? <div className="h-40 flex items-center justify-center text-sm text-muted-foreground">Нет данных</div> : (
            <div className="flex items-center gap-6">
              <div style={{ width: 160, height: 160, flexShrink: 0 }}>
                <PieChart width={160} height={160}>
                  <Pie data={mealPie} dataKey="value" cx={80} cy={80} innerRadius={45} outerRadius={72} paddingAngle={3}>
                    {mealPie.map((_, i) => <Cell key={i} fill={mealColors[i % mealColors.length]} />)}
                  </Pie>
                  <Tooltip formatter={(value) => `${tooltipNumber(value)} ккал`} />
                </PieChart>
              </div>
              <div className="flex flex-col gap-2">
                {mealPie.map((m, i) => (
                  <button key={m.name} onClick={() => setMealFilter(mealFilter === Object.keys(MEAL_LABELS).find(k => MEAL_LABELS[k] === m.name) ? '' : Object.keys(MEAL_LABELS).find(k => MEAL_LABELS[k] === m.name) || '')}
                    className="flex items-center gap-2 text-xs hover:bg-accent/50 rounded px-1 py-0.5 transition-colors">
                    <div className="w-2.5 h-2.5 rounded-full" style={{ backgroundColor: mealColors[i % mealColors.length] }} />
                    <span className="text-foreground">{m.name}</span>
                    <span className="text-muted-foreground">{m.value} ккал</span>
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Daily log */}
      <div className="rounded-xl border bg-card overflow-hidden">
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
