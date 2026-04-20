import { useEffect, useState, useCallback } from 'react'
import {
  BadgeRussianRuble,
  CreditCard,
  EyeOff,
  HandCoins,
  Landmark,
  PiggyBank,
  Search,
  TrendingDown,
  TrendingUp,
  Wallet,
  WalletCards,
  type LucideIcon,
} from 'lucide-react'
import type { EChartsCoreOption } from 'echarts/core'
import { EChart } from '@/components/EChart'
import { PageSyncButton } from '@/components/PageSyncButton'
import { cn, syncCaptionForSources } from '@/lib/utils'
import {
  api,
  type MonthStat, type Account, type FinanceTransaction,
  type CategoryStat, type DailyTotal, type TopExpense, type Integration,
} from '@/lib/api'

const CATEGORY_COLORS = [
  '#f97316', '#3b82f6', '#10b981', '#8b5cf6', '#f43f5e',
  '#06b6d4', '#eab308', '#ec4899', '#14b8a6', '#a855f7',
]

const MONTH_LABELS: Record<string, string> = {
  '01': 'Янв', '02': 'Фев', '03': 'Мар', '04': 'Апр',
  '05': 'Май', '06': 'Июн', '07': 'Июл', '08': 'Авг',
  '09': 'Сен', '10': 'Окт', '11': 'Ноя', '12': 'Дек',
}

const PERIODS = [
  { label: 'Неделя', days: 7 },
  { label: 'Месяц', days: 30 },
  { label: '3 мес', days: 90 },
  { label: 'Год', days: 365 },
]

const CHART_TEXT = '#e5eefc'
const CHART_MUTED = 'rgba(148, 163, 184, 0.85)'
const CHART_GRID = 'rgba(148, 163, 184, 0.12)'
const CHART_TOOLTIP = 'rgba(15, 23, 42, 0.96)'

type TooltipScalar = number | string | null | undefined

type AxisTooltipPoint = {
  axisValue?: string
  marker?: string
  seriesName?: string
  value?: TooltipScalar
}

type PieTooltipPoint = {
  marker?: string
  name?: string
  percent?: TooltipScalar
  value?: TooltipScalar
}

type TopExpenseTooltipData = {
  count?: number
  fullLabel?: string
  value?: TooltipScalar
}

type TopExpenseTooltipPoint = {
  data?: TopExpenseTooltipData
  marker?: string
  name?: string
  value?: TooltipScalar
}

function fmt(amount: number, currency = 'RUB') {
  return new Intl.NumberFormat('ru-RU', {
    style: 'currency', currency, maximumFractionDigits: 0,
  }).format(amount)
}

function fmtShort(value: number) {
  if (value >= 1000000) return `${(value / 1000000).toFixed(1)}М`
  if (value >= 1000) return `${(value / 1000).toFixed(0)}к`
  return String(value)
}

function fmtDate(iso: string) {
  return new Date(iso).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })
}

function fmtDayMonth(iso: string) {
  const date = new Date(iso)
  return `${date.getDate()} ${MONTH_LABELS[String(date.getMonth() + 1).padStart(2, '0')] ?? ''}`.trim()
}

function formatMonth(ym: string) {
  const [, m] = ym.split('-')
  return MONTH_LABELS[m] ?? ym
}

function dateOffset(days: number) {
  const d = new Date()
  d.setDate(d.getDate() - days)
  return d.toISOString().split('T')[0]
}

function formatAccountCount(count: number) {
  const mod10 = count % 10
  const mod100 = count % 100
  if (mod10 === 1 && mod100 !== 11) return `${count} счёт`
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return `${count} счёта`
  return `${count} счетов`
}

function getAccountVisual(type: string): { icon: LucideIcon; accent: string; bg: string } {
  switch (type) {
    case 'ccard':
      return { icon: CreditCard, accent: 'text-amber-300', bg: 'bg-amber-500/12' }
    case 'checking':
      return { icon: Landmark, accent: 'text-sky-300', bg: 'bg-sky-500/12' }
    case 'cash':
      return { icon: BadgeRussianRuble, accent: 'text-emerald-300', bg: 'bg-emerald-500/12' }
    case 'deposit':
      return { icon: PiggyBank, accent: 'text-violet-300', bg: 'bg-violet-500/12' }
    case 'loan':
      return { icon: HandCoins, accent: 'text-rose-300', bg: 'bg-rose-500/12' }
    case 'emoney':
      return { icon: WalletCards, accent: 'text-cyan-300', bg: 'bg-cyan-500/12' }
    default:
      return { icon: Wallet, accent: 'text-muted-foreground', bg: 'bg-muted' }
  }
}

function getAccountBrandBadge(account: Account): { label: string; bg: string; text: string } | null {
  const source = `${account.company_title ?? ''} ${account.title}`.toLowerCase()

  if (source.includes('тинь') || source.includes('t-bank') || source.includes('t bank')) {
    return { label: 'T', bg: 'bg-yellow-300', text: 'text-black' }
  }
  if (source.includes('альфа') || source.includes('alfa')) {
    return { label: 'A', bg: 'bg-red-500', text: 'text-white' }
  }
  if (source.includes('втб') || source.includes('vtb')) {
    return { label: 'ВТБ', bg: 'bg-sky-500', text: 'text-white' }
  }
  if (source.includes('озон') || source.includes('ozon')) {
    return { label: 'OZ', bg: 'bg-blue-600', text: 'text-white' }
  }
  if (source.includes('сбер') || source.includes('sber')) {
    return { label: 'C', bg: 'bg-emerald-500', text: 'text-white' }
  }
  if (source.includes('райфф') || source.includes('raif') || source.includes('raiffeisen')) {
    return { label: 'R', bg: 'bg-yellow-400', text: 'text-black' }
  }
  if (source.includes('яндекс') || source.includes('yandex')) {
    return { label: 'Я', bg: 'bg-red-500', text: 'text-white' }
  }

  return null
}

function getAccountTypeLabel(type: string) {
  switch (type) {
    case 'ccard':
      return 'Карта'
    case 'checking':
      return 'Счёт'
    case 'cash':
      return 'Наличные'
    case 'deposit':
      return 'Вклад'
    case 'loan':
      return 'Кредит'
    case 'emoney':
      return 'Электронный кошелёк'
    case 'debt':
      return 'Долг'
    default:
      return type || 'Счёт'
  }
}

function toTooltipNumber(value: number | string | null | undefined) {
  if (typeof value === 'number') return value
  if (typeof value === 'string') {
    const parsed = Number(value)
    if (Number.isFinite(parsed)) return parsed
  }
  return 0
}

function escapeHtml(value: string) {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;')
}

function truncateLabel(value: string, limit = 20) {
  return value.length > limit ? `${value.slice(0, limit - 1)}…` : value
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function readTooltipScalar(value: unknown): TooltipScalar {
  if (typeof value === 'number' || typeof value === 'string' || value == null) return value
  return undefined
}

function readTooltipString(value: unknown) {
  return typeof value === 'string' ? value : undefined
}

function readTooltipNumber(value: unknown) {
  return typeof value === 'number' ? value : undefined
}

function toTooltipArray(params: unknown) {
  return Array.isArray(params) ? params : [params]
}

function readAxisTooltipPoints(params: unknown): AxisTooltipPoint[] {
  return toTooltipArray(params).flatMap((item): AxisTooltipPoint[] => {
    if (!isRecord(item)) return []
    return [{
      axisValue: readTooltipString(item.axisValue),
      marker: readTooltipString(item.marker),
      seriesName: readTooltipString(item.seriesName),
      value: readTooltipScalar(item.value),
    }]
  })
}

function readPieTooltipPoint(param: unknown): PieTooltipPoint | null {
  if (!isRecord(param)) return null
  return {
    marker: readTooltipString(param.marker),
    name: readTooltipString(param.name),
    percent: readTooltipScalar(param.percent),
    value: readTooltipScalar(param.value),
  }
}

function readTopExpenseTooltipPoint(params: unknown): TopExpenseTooltipPoint | null {
  const point = toTooltipArray(params)[0]
  if (!isRecord(point)) return null

  let data: TopExpenseTooltipData | undefined
  if (isRecord(point.data)) {
    data = {
      count: readTooltipNumber(point.data.count),
      fullLabel: readTooltipString(point.data.fullLabel),
      value: readTooltipScalar(point.data.value),
    }
  }

  return {
    data,
    marker: readTooltipString(point.marker),
    name: readTooltipString(point.name),
    value: readTooltipScalar(point.value),
  }
}

function buildMonthlyOption(monthly: MonthStat[]): EChartsCoreOption {
  return {
    color: ['#10b981', '#f43f5e'],
    animationDuration: 400,
    grid: { top: 44, right: 12, bottom: 12, left: 12, containLabel: true },
    legend: {
      top: 0,
      textStyle: { color: CHART_MUTED, fontSize: 12 },
      itemWidth: 10,
      itemHeight: 10,
      data: ['Доходы', 'Расходы'],
    },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      backgroundColor: CHART_TOOLTIP,
      borderColor: CHART_GRID,
      textStyle: { color: CHART_TEXT },
      formatter: (params: unknown) => {
        const points = readAxisTooltipPoints(params)
        const lines = points.map(point => {
          const name = point.seriesName === 'Расходы' ? 'Расходы' : 'Доходы'
          return `${point.marker ?? ''}${name}: ${fmt(toTooltipNumber(point.value))}`
        })
        return [`<div>${escapeHtml(formatMonth(points[0]?.axisValue ?? ''))}</div>`, ...lines].join('<br/>')
      },
    },
    xAxis: {
      type: 'category',
      data: monthly.map(point => point.month),
      axisLabel: {
        color: CHART_MUTED,
        formatter: (value: string) => formatMonth(value),
      },
      axisLine: { lineStyle: { color: CHART_GRID } },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      axisLabel: {
        color: CHART_MUTED,
        formatter: (value: number) => fmtShort(value),
      },
      splitLine: { lineStyle: { color: CHART_GRID } },
    },
    series: [
      {
        name: 'Доходы',
        type: 'bar',
        barMaxWidth: 28,
        borderRadius: [6, 6, 0, 0],
        data: monthly.map(point => point.income),
      },
      {
        name: 'Расходы',
        type: 'bar',
        barMaxWidth: 28,
        borderRadius: [6, 6, 0, 0],
        data: monthly.map(point => point.spending),
      },
    ],
  }
}

function buildDailyOption(daily: DailyTotal[]): EChartsCoreOption {
  return {
    color: ['#10b981', '#f43f5e'],
    animationDuration: 450,
    grid: { top: 18, right: 12, bottom: 12, left: 12, containLabel: true },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'line' },
      backgroundColor: CHART_TOOLTIP,
      borderColor: CHART_GRID,
      textStyle: { color: CHART_TEXT },
      formatter: (params: unknown) => {
        const points = readAxisTooltipPoints(params)
        const lines = points.map(point => `${point.marker ?? ''}${point.seriesName ?? ''}: ${fmt(toTooltipNumber(point.value))}`)
        return [`<div>${escapeHtml(fmtDate(points[0]?.axisValue ?? ''))}</div>`, ...lines].join('<br/>')
      },
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: daily.map(point => point.date),
      axisLabel: {
        color: CHART_MUTED,
        formatter: (value: string) => fmtDayMonth(value),
      },
      axisLine: { lineStyle: { color: CHART_GRID } },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      axisLabel: {
        color: CHART_MUTED,
        formatter: (value: number) => fmtShort(value),
      },
      splitLine: { lineStyle: { color: CHART_GRID } },
    },
    series: [
      {
        name: 'Доходы',
        type: 'line',
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 2 },
        areaStyle: { color: 'rgba(16, 185, 129, 0.14)' },
        data: daily.map(point => point.income),
      },
      {
        name: 'Расходы',
        type: 'line',
        smooth: true,
        showSymbol: false,
        lineStyle: { width: 2.5 },
        areaStyle: { color: 'rgba(244, 63, 94, 0.18)' },
        data: daily.map(point => point.spending),
      },
    ],
  }
}

function buildCategoriesOption(categories: CategoryStat[]): EChartsCoreOption {
  const total = categories.reduce((sum, point) => sum + point.amount, 0)
  return {
    color: CATEGORY_COLORS,
    animationDuration: 450,
    tooltip: {
      trigger: 'item',
      backgroundColor: CHART_TOOLTIP,
      borderColor: CHART_GRID,
      textStyle: { color: CHART_TEXT },
      formatter: (param: unknown) => {
        const point = readPieTooltipPoint(param)
        const amount = toTooltipNumber(point?.value)
        const percent = Number(point?.percent ?? 0)
        return `${point?.marker ?? ''}${escapeHtml(point?.name ?? '')}: ${fmt(amount)} (${percent.toFixed(0)}%)`
      },
    },
    graphic: [
      {
        type: 'text',
        left: 'center',
        top: '42%',
        style: {
          text: 'Всего',
          textAlign: 'center',
          fill: CHART_MUTED,
          fontSize: 12,
        },
      },
      {
        type: 'text',
        left: 'center',
        top: '52%',
        style: {
          text: fmt(total),
          textAlign: 'center',
          fill: CHART_TEXT,
          fontSize: 14,
          fontWeight: 700,
        },
      },
    ],
    series: [
      {
        type: 'pie',
        radius: ['56%', '84%'],
        center: ['50%', '50%'],
        startAngle: 90,
        avoidLabelOverlap: true,
        label: { show: false },
        labelLine: { show: false },
        itemStyle: {
          borderColor: '#162033',
          borderWidth: 2,
        },
        emphasis: {
          scale: true,
          scaleSize: 6,
        },
        data: categories.map(point => ({
          name: point.category,
          value: point.amount,
        })),
      },
    ],
  }
}

function buildTopExpensesOption(topExpenses: TopExpense[]): EChartsCoreOption {
  return {
    color: ['#f97316'],
    animationDuration: 450,
    grid: { top: 6, right: 12, bottom: 8, left: 12, containLabel: true },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      backgroundColor: CHART_TOOLTIP,
      borderColor: CHART_GRID,
      textStyle: { color: CHART_TEXT },
      formatter: (params: unknown) => {
        const point = readTopExpenseTooltipPoint(params)
        const data = point?.data
        return [
          `<div>${escapeHtml(data?.fullLabel ?? point?.name ?? '')}</div>`,
          `${point?.marker ?? ''}${fmt(toTooltipNumber(point?.value))} (${data?.count ?? 0} операций)`,
        ].join('<br/>')
      },
    },
    xAxis: {
      type: 'value',
      axisLabel: {
        color: CHART_MUTED,
        formatter: (value: number) => fmtShort(value),
      },
      splitLine: { lineStyle: { color: CHART_GRID } },
    },
    yAxis: {
      type: 'category',
      inverse: true,
      data: topExpenses.map(point => truncateLabel(point.payee)),
      axisLabel: {
        color: CHART_MUTED,
        width: 132,
        overflow: 'truncate',
      },
      axisLine: { show: false },
      axisTick: { show: false },
    },
    series: [
      {
        type: 'bar',
        barMaxWidth: 18,
        roundCap: true,
        itemStyle: {
          borderRadius: [0, 6, 6, 0],
        },
        data: topExpenses.map(point => ({
          value: point.amount,
          count: point.count,
          fullLabel: point.payee,
        })),
      },
    ],
  }
}

type FilterType = '' | 'income' | 'expense'
type SortType = '' | 'amount' | 'amount_asc' | 'date_asc' | 'category'

export function Finance() {
  const [monthly, setMonthly] = useState<MonthStat[]>([])
  const [accounts, setAccounts] = useState<Account[]>([])
  const [categories, setCategories] = useState<CategoryStat[]>([])
  const [daily, setDaily] = useState<DailyTotal[]>([])
  const [topExpenses, setTopExpenses] = useState<TopExpense[]>([])
  const [categoryList, setCategoryList] = useState<string[]>([])
  const [txs, setTxs] = useState<FinanceTransaction[]>([])
  const [filter, setFilter] = useState<FilterType>('')
  const [sort, setSort] = useState<SortType>('')
  const [search, setSearch] = useState('')
  const [catFilter, setCatFilter] = useState('')
  const [period, setPeriod] = useState(30)
  const [customFrom, setCustomFrom] = useState('')
  const [customTo, setCustomTo] = useState('')
  const [page, setPage] = useState(1)
  const [hasMore, setHasMore] = useState(true)
  const [loading, setLoading] = useState(true)
  const [txLoading, setTxLoading] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [integrations, setIntegrations] = useState<Integration[]>([])

  const from = customFrom || dateOffset(period)
  const to = customTo || undefined

  const loadPageData = useCallback(async () => {
    setLoading(true)
    try {
      const [m, a, c, d, t, cl] = await Promise.all([
      api.getMonthlyStats(),
      api.getAccounts(),
      api.getSpendingByCategory(from),
      api.getDailyTotals(from, to),
      api.getTopExpenses(from, to),
      api.getCategoryList(),
      ])

      setMonthly(m); setAccounts(a); setCategories(c)
      setDaily(d); setTopExpenses(t); setCategoryList(cl)
    } catch (error) {
      console.error(error)
    } finally {
      setLoading(false)
    }
  }, [from, to])

  const loadIntegrations = useCallback(async () => {
    try {
      setIntegrations(await api.getIntegrations())
    } catch (error) {
      console.error(error)
    }
  }, [])

  useEffect(() => {
    void loadPageData()
  }, [loadPageData])

  useEffect(() => {
    void loadIntegrations()
  }, [loadIntegrations])

  const loadTxs = useCallback(async (p: number, replace: boolean) => {
    setTxLoading(true)
    try {
      const data = await api.getTransactions({
        page: p, type: filter, sort, search, category: catFilter, from, to,
      })
      setTxs(prev => replace ? data : [...prev, ...data])
      setHasMore(data.length === 30)
    } catch { /* ignore */ } finally {
      setTxLoading(false)
    }
  }, [filter, sort, search, catFilter, from, to])

  useEffect(() => {
    setPage(1)
    const t = setTimeout(() => loadTxs(1, true), search ? 300 : 0)
    return () => clearTimeout(t)
  }, [filter, sort, search, catFilter, loadTxs])

  function loadMore() {
    const next = page + 1
    setPage(next)
    loadTxs(next, false)
  }

  const currentMonth = monthly[monthly.length - 1]
  const zenmoneyIntegration = integrations.find(i => i.name === 'zenmoney')
  const financeSyncCaption = syncCaptionForSources(zenmoneyIntegration?.enabled ? [zenmoneyIntegration] : [])
  const includedAccounts = accounts.filter(a => a.in_balance)
  const excludedAccounts = accounts.filter(a => !a.in_balance)
  const totalBalance = includedAccounts
    .reduce((sum, a) => sum + a.balance, 0)
  const excludedBalance = excludedAccounts
    .filter(a => a.currency === 'RUB')
    .reduce((sum, a) => sum + a.balance, 0)

  async function handleSyncFinance() {
    if (!zenmoneyIntegration?.enabled) return
    setSyncing(true)
    try {
      await api.syncIntegration('zenmoney')
      await Promise.all([loadPageData(), loadIntegrations()])
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
          <h1 className="text-2xl font-bold text-foreground">Финансы</h1>
          <p className="text-sm text-muted-foreground mt-1">
            {new Date().toLocaleDateString('ru-RU', { month: 'long', year: 'numeric' })}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2 xl:justify-end">
          <PageSyncButton
            label="Синхронизировать ZenMoney"
            syncCaption={financeSyncCaption}
            syncing={syncing}
            disabled={!zenmoneyIntegration?.enabled}
            onClick={handleSyncFinance}
          />
          <div className="flex gap-1 items-center flex-wrap">
            {PERIODS.map(p => (
              <button
                key={p.days}
                onClick={() => { setPeriod(p.days); setCustomFrom(''); setCustomTo('') }}
                className={cn(
                  'px-3 py-1 text-xs rounded-lg transition-colors',
                  period === p.days && !customFrom
                    ? 'bg-primary text-primary-foreground'
                    : 'bg-muted text-muted-foreground hover:bg-accent'
                )}
              >
                {p.label}
              </button>
            ))}
            <span className="text-xs text-muted-foreground mx-1">|</span>
            <input type="date" value={customFrom} onChange={e => setCustomFrom(e.target.value)}
              className="text-xs rounded-lg border bg-background px-2 py-1 outline-none" />
            <span className="text-xs text-muted-foreground">—</span>
            <input type="date" value={customTo} onChange={e => setCustomTo(e.target.value)}
              className="text-xs rounded-lg border bg-background px-2 py-1 outline-none" />
          </div>
        </div>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
        <div className="rounded-xl border bg-card p-5 flex flex-col gap-2">
          <div className="flex items-center justify-between">
            <span className="text-sm text-muted-foreground">Баланс</span>
            <div className="flex items-center justify-center w-8 h-8 rounded-lg bg-blue-500">
              <Wallet className="w-4 h-4 text-white" />
            </div>
          </div>
          {loading
            ? <div className="h-8 w-32 bg-muted rounded animate-pulse" />
            : <div className="text-2xl font-bold text-foreground">{fmt(totalBalance)}</div>}
          <span className="text-xs text-muted-foreground">
            {loading ? 'загрузка...' : `${formatAccountCount(includedAccounts.length)} в балансе`}
          </span>
        </div>

        <div className="rounded-xl border border-amber-500/20 bg-amber-500/5 p-5 flex flex-col gap-2">
          <div className="flex items-center justify-between">
            <span className="text-sm text-muted-foreground">Вне баланса</span>
            <div className="flex items-center justify-center w-8 h-8 rounded-lg bg-amber-500">
              <EyeOff className="w-4 h-4 text-white" />
            </div>
          </div>
          {loading
            ? <div className="h-8 w-32 bg-muted rounded animate-pulse" />
            : <div className="text-2xl font-bold text-foreground">{fmt(excludedBalance)}</div>}
          <span className="text-xs text-muted-foreground">
            {loading ? 'загрузка...' : `${formatAccountCount(excludedAccounts.length)} исключено из общего баланса`}
          </span>
        </div>

        <div className="rounded-xl border bg-card p-5 flex flex-col gap-2">
          <div className="flex items-center justify-between">
            <span className="text-sm text-muted-foreground">Расходы</span>
            <div className="flex items-center justify-center w-8 h-8 rounded-lg bg-rose-500">
              <TrendingDown className="w-4 h-4 text-white" />
            </div>
          </div>
          {loading
            ? <div className="h-8 w-32 bg-muted rounded animate-pulse" />
            : <div className="text-2xl font-bold text-foreground">{currentMonth ? fmt(currentMonth.spending) : '—'}</div>}
          <span className="text-xs text-muted-foreground">текущий месяц</span>
        </div>

        <div className="rounded-xl border bg-card p-5 flex flex-col gap-2">
          <div className="flex items-center justify-between">
            <span className="text-sm text-muted-foreground">Доходы</span>
            <div className="flex items-center justify-center w-8 h-8 rounded-lg bg-emerald-500">
              <TrendingUp className="w-4 h-4 text-white" />
            </div>
          </div>
          {loading
            ? <div className="h-8 w-32 bg-muted rounded animate-pulse" />
            : <div className="text-2xl font-bold text-foreground">{currentMonth ? fmt(currentMonth.income) : '—'}</div>}
          <span className="text-xs text-muted-foreground">текущий месяц</span>
        </div>
      </div>

      {!loading && excludedAccounts.length > 0 ? (
        <div className="rounded-xl border border-amber-500/20 bg-amber-500/5 px-5 py-4">
          <p className="text-sm font-medium text-foreground">Часть счетов исключена из общего баланса</p>
          <p className="text-sm text-muted-foreground mt-1">
            ZenMoney помечает {formatAccountCount(excludedAccounts.length)} как не участвующие в общем балансе.
            Их сумма: {fmt(excludedBalance)}. Эти счета показаны ниже отдельной секцией и не участвуют
            в карточке "Баланс" и агрегатах по финансам.
          </p>
        </div>
      ) : null}

      {/* Charts row 1 */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Monthly income vs expenses */}
        <div className="rounded-xl border bg-card p-5">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Доходы и расходы</h2>
          {loading ? <div className="h-48 bg-muted rounded animate-pulse" /> : (
            <EChart option={buildMonthlyOption(monthly)} height={220} />
          )}
        </div>

        {/* Daily spending trend */}
        <div className="rounded-xl border bg-card p-5">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Расходы по дням</h2>
          {loading ? <div className="h-48 bg-muted rounded animate-pulse" /> : (
            <EChart option={buildDailyOption(daily)} height={220} />
          )}
        </div>
      </div>

      {/* Charts row 2 */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Categories pie */}
        <div className="rounded-xl border bg-card p-5">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Расходы по категориям</h2>
          {loading ? <div className="h-56 bg-muted rounded animate-pulse" /> : categories.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-8">Нет данных</p>
          ) : (
            <div className="flex flex-col lg:flex-row gap-6 items-start">
              <EChart option={buildCategoriesOption(categories)} height={200} className="w-[200px] shrink-0" />
              <div className="flex flex-col gap-1.5 flex-1 min-w-0 py-1">
                {categories.map((c, i) => (
                  <div key={c.category} className="flex items-center gap-2 text-xs cursor-pointer hover:bg-accent/50 rounded px-1 py-0.5 transition-colors"
                    onClick={() => setCatFilter(catFilter === c.category ? '' : c.category)}>
                    <div className="w-2 h-2 rounded-full shrink-0" style={{ backgroundColor: CATEGORY_COLORS[i % CATEGORY_COLORS.length] }} />
                    <span className={cn('flex-1 truncate', catFilter === c.category && 'text-primary font-medium')}>{c.category}</span>
                    <span className="text-muted-foreground tabular-nums shrink-0">{fmt(c.amount)}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* Top expenses */}
        <div className="rounded-xl border bg-card p-5">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Топ расходов</h2>
          {loading ? <div className="h-56 bg-muted rounded animate-pulse" /> : topExpenses.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-8">Нет данных</p>
          ) : (
            <EChart option={buildTopExpensesOption(topExpenses)} height={Math.max(220, topExpenses.length * 32)} />
          )}
        </div>
      </div>

      {/* Accounts + Transactions */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Accounts */}
        <div className="rounded-xl border bg-card overflow-hidden">
          <div className="px-5 py-4 border-b">
            <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Счета</h2>
            {!loading ? (
              <p className="mt-1 text-xs text-muted-foreground">
                {formatAccountCount(includedAccounts.length)} в балансе
                {excludedAccounts.length > 0 ? ` • ${formatAccountCount(excludedAccounts.length)} вне баланса` : ''}
              </p>
            ) : null}
          </div>
          {loading ? (
            <div className="divide-y">
              {[1, 2, 3, 4].map(i => (
                <div key={i} className="px-5 py-3 flex gap-3"><div className="flex-1 h-4 bg-muted rounded animate-pulse" /></div>
              ))}
            </div>
          ) : accounts.length === 0 ? (
            <div className="px-5 py-4 text-sm text-muted-foreground text-center">Нет данных</div>
          ) : (
            <div className="max-h-80 overflow-y-auto">
              {includedAccounts.length > 0 ? (
                <div>
                  <div className="sticky top-0 z-10 border-y bg-card/95 px-5 py-2 backdrop-blur">
                    <span className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
                      В балансе
                    </span>
                  </div>
                  <div className="divide-y">
                    {includedAccounts.map(a => (
                      <AccountRow key={a.id} account={a} />
                    ))}
                  </div>
                </div>
              ) : null}

              {excludedAccounts.length > 0 ? (
                <div>
                  <div className="sticky top-0 z-10 border-y border-amber-500/20 bg-amber-500/5 px-5 py-2 backdrop-blur">
                    <span className="text-[11px] font-medium uppercase tracking-wide text-amber-200">
                      Вне баланса
                    </span>
                  </div>
                  <div className="divide-y divide-amber-500/10">
                    {excludedAccounts.map(a => (
                      <AccountRow key={a.id} account={a} muted />
                    ))}
                  </div>
                </div>
              ) : null}
            </div>
          )}
        </div>

        {/* Transactions */}
        <div className="lg:col-span-2 rounded-xl border bg-card overflow-hidden">
          <div className="px-5 py-4 border-b flex flex-col gap-3">
            <div className="flex items-center justify-between">
              <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Транзакции</h2>
              <div className="flex gap-1">
                {(['', 'income', 'expense'] as FilterType[]).map(f => (
                  <button key={f} onClick={() => setFilter(f)}
                    className={cn('px-3 py-1 text-xs rounded-lg transition-colors',
                      filter === f ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:bg-accent')}>
                    {f === '' ? 'Все' : f === 'income' ? 'Доходы' : 'Расходы'}
                  </button>
                ))}
              </div>
            </div>
            <div className="flex gap-2">
              <div className="relative flex-1">
                <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
                <input type="text" value={search} onChange={e => setSearch(e.target.value)}
                  placeholder="Поиск..." className="w-full pl-8 pr-3 py-1.5 text-xs rounded-lg border bg-background outline-none focus:ring-2 focus:ring-ring" />
              </div>
              <select value={catFilter} onChange={e => setCatFilter(e.target.value)}
                className="text-xs rounded-lg border bg-background px-2 py-1.5 outline-none">
                <option value="">Все категории</option>
                {categoryList.map(c => <option key={c} value={c}>{c}</option>)}
              </select>
              <select value={sort} onChange={e => setSort(e.target.value as SortType)}
                className="text-xs rounded-lg border bg-background px-2 py-1.5 outline-none">
                <option value="">По дате ↓</option>
                <option value="date_asc">По дате ↑</option>
                <option value="amount">По сумме ↓</option>
                <option value="amount_asc">По сумме ↑</option>
                <option value="category">По категории</option>
              </select>
            </div>
          </div>

          <div className="divide-y max-h-[480px] overflow-y-auto">
            {txs.length === 0 && !txLoading ? (
              <div className="px-5 py-4 text-sm text-muted-foreground text-center">Нет транзакций</div>
            ) : (
              txs.map(tx => (
                <div key={tx.id} className="px-5 py-3 flex items-center gap-3">
                  <span className="text-xs text-muted-foreground w-16 shrink-0">{fmtDate(tx.occurred_at)}</span>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm text-foreground truncate">{tx.payee || tx.comment || '—'}</p>
                    {tx.category && <p className="text-[10px] text-muted-foreground/60">{tx.category}</p>}
                  </div>
                  <span className={cn('text-sm font-medium tabular-nums shrink-0', tx.amount > 0 ? 'text-emerald-500' : 'text-rose-500')}>
                    {tx.amount > 0 ? '+' : ''}{fmt(tx.amount, tx.currency)}
                  </span>
                </div>
              ))
            )}
            {txLoading && <div className="px-5 py-3 text-sm text-muted-foreground text-center">Загрузка...</div>}
          </div>

          {hasMore && !txLoading && txs.length > 0 && (
            <div className="px-5 py-3 border-t">
              <button onClick={loadMore} className="w-full text-sm text-muted-foreground hover:text-foreground transition-colors">
                Загрузить ещё
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function AccountRow({ account, muted = false }: { account: Account; muted?: boolean }) {
  const visual = getAccountVisual(account.type)
  const Icon = visual.icon
  const brandBadge = getAccountBrandBadge(account)
  const secondaryMeta = account.company_title
    ? `${account.company_title} • ${getAccountTypeLabel(account.type)}`
    : getAccountTypeLabel(account.type)

  return (
    <div className={cn('px-5 py-3 flex items-center gap-3', muted && 'bg-amber-500/5')}>
      {brandBadge ? (
        <div className={cn('flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-white/5 text-[10px] font-black uppercase tracking-tight', brandBadge.bg, brandBadge.text)}>
          {brandBadge.label}
        </div>
      ) : (
        <div className={cn('flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-white/5', visual.bg)}>
          <Icon className={cn('h-4 w-4', visual.accent)} />
        </div>
      )}
      <div className="flex-1 min-w-0">
        <p className="text-sm text-foreground truncate">{account.title}</p>
        <div className="flex items-center gap-2">
          <p className="text-xs text-muted-foreground truncate">{secondaryMeta}</p>
          {!account.in_balance ? (
            <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-amber-200">
              Вне баланса
            </span>
          ) : null}
        </div>
      </div>
      <span className={cn('text-sm font-medium tabular-nums shrink-0', account.balance >= 0 ? 'text-foreground' : 'text-rose-500')}>
        {fmt(account.balance, account.currency)}
      </span>
    </div>
  )
}
