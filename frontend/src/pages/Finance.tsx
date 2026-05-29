import { useEffect, useState, useCallback } from 'react'
import {
  BadgeRussianRuble,
  Ban,
  CalendarClock,
  CreditCard,
  EyeOff,
  HandCoins,
  Landmark,
  Pin,
  PiggyBank,
  RotateCcw,
  Search,
  ShieldAlert,
  ShieldCheck,
  TrendingDown,
  TrendingUp,
  Wallet,
  WalletCards,
  type LucideIcon,
} from 'lucide-react'
import type { EChartsCoreOption } from 'echarts/core'
import { EChart } from '@/components/EChart'
import { ExpandablePanel } from '@/components/ExpandablePanel'
import { PageSyncButton } from '@/components/PageSyncButton'
import { PageHeader } from '@/components/PageHeader'
import { cn, syncCaptionForSources } from '@/lib/utils'
import {
  api,
  type MonthStat, type Account, type FinanceTransaction,
  type CategoryStat, type DailyTotal, type TopExpense, type Integration, type FinanceObligationsSummary, type FinanceObligationRule,
} from '@/lib/api'

const CATEGORY_COLORS = [
  '#f97316', '#3b82f6', '#10b981', '#8b5cf6', '#f43f5e',
  '#06b6d4', '#eab308', '#ec4899', '#14b8a6', '#a855f7',
]

const INCOME_CATEGORY_COLORS = [
  '#10b981', '#22c55e', '#06b6d4', '#3b82f6', '#84cc16',
  '#14b8a6', '#0ea5e9', '#a3e635', '#2dd4bf', '#60a5fa',
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

function fmtSigned(amount: number, currency = 'RUB') {
  if (amount === 0) return fmt(0, currency)
  const abs = fmt(Math.abs(amount), currency)
  return amount > 0 ? `+${abs}` : `−${abs}`
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

function formatRangeLabel(from: string, to?: string) {
  const end = to || new Date().toISOString().split('T')[0]
  return `${fmtDate(from)} — ${fmtDate(end)}`
}

function formatPercent(part: number, total: number) {
  if (total <= 0) return '0%'
  return `${Math.round((part / total) * 100)}%`
}

function formatRatioPercent(ratio: number | null) {
  if (ratio == null || !Number.isFinite(ratio)) return '—'
  return `${Math.round(ratio * 100)}%`
}

function parseDateOnly(value: string) {
  const [year, month, day] = value.split('-').map(Number)
  return new Date(year, (month || 1) - 1, day || 1)
}

function daysInclusive(from: string, to: string) {
  const start = parseDateOnly(from)
  const end = parseDateOnly(to)
  const diff = end.getTime() - start.getTime()
  return Math.max(1, Math.floor(diff / (24 * 60 * 60 * 1000)) + 1)
}

function formatRunway(days: number | null) {
  if (days == null) return '—'
  if (!Number.isFinite(days)) return '∞'
  if (days <= 0) return '0 дн'
  if (days >= 180) return `${Math.round(days / 30)} мес`
  if (days >= 45) return `${(days / 30).toFixed(1)} мес`
  return `${Math.round(days)} дн`
}

function formatCoverageMultiple(ratio: number | null) {
  if (ratio == null) return '∞'
  if (!Number.isFinite(ratio)) return '∞'
  if (ratio < 0) return '0x'
  if (ratio >= 10) return `${ratio.toFixed(0)}x`
  return `${ratio.toFixed(1)}x`
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

function buildCategoriesOption(categories: CategoryStat[], colors = CATEGORY_COLORS): EChartsCoreOption {
  const total = categories.reduce((sum, point) => sum + point.amount, 0)
  return {
    color: colors,
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
  const [incomeCategories, setIncomeCategories] = useState<CategoryStat[]>([])
  const [daily, setDaily] = useState<DailyTotal[]>([])
  const [topExpenses, setTopExpenses] = useState<TopExpense[]>([])
  const [obligations, setObligations] = useState<FinanceObligationsSummary | null>(null)
  const [categoryList, setCategoryList] = useState<string[]>([])
  const [txs, setTxs] = useState<FinanceTransaction[]>([])
  const [filter, setFilter] = useState<FilterType>('')
  const [sort, setSort] = useState<SortType>('')
  const [search, setSearch] = useState('')
  const [catFilter, setCatFilter] = useState('')
  const [period, setPeriod] = useState(30)
  const [customFrom, setCustomFrom] = useState('')
  const [customTo, setCustomTo] = useState('')
  const [showRangePanel, setShowRangePanel] = useState(false)
  const [page, setPage] = useState(1)
  const [hasMore, setHasMore] = useState(true)
  const [loading, setLoading] = useState(true)
  const [txLoading, setTxLoading] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [ruleSavingKey, setRuleSavingKey] = useState<string | null>(null)
  const [ruleError, setRuleError] = useState('')
  const [integrations, setIntegrations] = useState<Integration[]>([])

  const from = customFrom || dateOffset(period)
  const to = customTo || undefined

  const loadPageData = useCallback(async () => {
    setLoading(true)
    try {
      const [m, a, c, ic, d, t, o, cl] = await Promise.all([
        api.getMonthlyStats(),
        api.getAccounts(),
        api.getSpendingByCategory(from, to),
        api.getIncomeByCategory(from, to),
        api.getDailyTotals(from, to),
        api.getTopExpenses(from, to),
        api.getFinanceObligations(30),
        api.getCategoryList(),
      ])

      setMonthly(m); setAccounts(a); setCategories(c)
      setIncomeCategories(ic); setDaily(d); setTopExpenses(t); setObligations(o); setCategoryList(cl)
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
  const hasCustomRange = Boolean(customFrom || customTo)
  const effectiveTo = to || new Date().toISOString().split('T')[0]
  const rangeLabel = formatRangeLabel(from, to)
  const activePeriodLabel = hasCustomRange
    ? 'Произвольный диапазон'
    : PERIODS.find(item => item.days === period)?.label ?? 'Месяц'
  const totalCategorySpend = categories.reduce((sum, category) => sum + category.amount, 0)
  const topCategory = categories[0]
  const totalCategoryIncome = incomeCategories.reduce((sum, category) => sum + category.amount, 0)
  const topIncomeCategory = incomeCategories[0]
  const topThreeCategories = categories.slice(0, 3)
  const totalDailySpending = daily.reduce((sum, day) => sum + day.spending, 0)
  const totalDailyIncome = daily.reduce((sum, day) => sum + day.income, 0)
  const rangeDays = daysInclusive(from, effectiveTo)
  const avgDailySpending = rangeDays > 0 ? totalDailySpending / rangeDays : 0
  const periodNet = totalDailyIncome - totalDailySpending
  const savingsRate = totalDailyIncome > 0 ? periodNet / totalDailyIncome : null
  const monthlyBurnProjection = avgDailySpending * 30
  const runwayDays = avgDailySpending > 0 ? totalBalance / avgDailySpending : Number.POSITIVE_INFINITY
  const topThreeShare = totalCategorySpend > 0
    ? topThreeCategories.reduce((sum, category) => sum + category.amount, 0) / totalCategorySpend
    : null
  const peakExpenseDay = daily.reduce<DailyTotal | null>((peak, day) => {
    if (!peak || day.spending > peak.spending) return day
    return peak
  }, null)
  const topPayee = topExpenses[0]
  const activeTransactionFilters = [filter, search, catFilter, sort].filter(Boolean).length
  const concentrationLeaders = topThreeCategories.map(category => category.category).join(' • ')
  const obligationsWindowDays = obligations?.window_days ?? 30
  const obligationItems = obligations?.items ?? []
  const obligationRules = obligations?.rules ?? []
  const upcomingObligationsTotal = obligations?.upcoming_total ?? 0
  const nextObligation = obligationItems[0]
  const obligationCoverageRatio = upcomingObligationsTotal > 0 ? totalBalance / upcomingObligationsTotal : null
  const obligationCoverageGap = totalBalance - upcomingObligationsTotal
  const coverageTone = obligationCoverageRatio == null || !Number.isFinite(obligationCoverageRatio) || obligationCoverageRatio >= 1.5
    ? 'safe'
    : obligationCoverageRatio >= 1
      ? 'tight'
      : 'shortfall'

  useEffect(() => {
    if (hasCustomRange) setShowRangePanel(true)
  }, [hasCustomRange])

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

  async function handleSaveObligationRule(input: { key: string; label: string; action: 'ignore' | 'force' }) {
    setRuleSavingKey(`${input.action}:${input.key}`)
    setRuleError('')
    try {
      await api.saveFinanceObligationRule(input)
      await loadPageData()
    } catch (error) {
      setRuleError(error instanceof Error ? error.message : 'Не удалось сохранить правило')
    } finally {
      setRuleSavingKey(null)
    }
  }

  async function handleDeleteObligationRule(rule: FinanceObligationRule) {
    setRuleSavingKey(`delete:${rule.key}`)
    setRuleError('')
    try {
      await api.deleteFinanceObligationRule(rule.key)
      await loadPageData()
    } catch (error) {
      setRuleError(error instanceof Error ? error.message : 'Не удалось удалить правило')
    } finally {
      setRuleSavingKey(null)
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-4">
        <PageHeader
          eyebrow="Finance"
          title="Финансы"
          description="Сначала ключевые сигналы: баланс, net cashflow, savings rate, burn rate и runway. Ниже — структура расходов и детализация по транзакциям."
          badges={[
            { label: new Date().toLocaleDateString('ru-RU', { month: 'long', year: 'numeric' }), tone: 'primary' },
            { label: zenmoneyIntegration?.enabled ? 'ZenMoney подключён' : 'ZenMoney не подключён', tone: zenmoneyIntegration?.enabled ? 'success' : 'warning' },
            { label: formatAccountCount(includedAccounts.length), tone: 'muted' },
            ...(totalDailyIncome > 0 ? [{ label: `Savings rate · ${formatRatioPercent(savingsRate)}`, tone: savingsRate != null && savingsRate >= 0.2 ? 'success' as const : savingsRate != null && savingsRate >= 0 ? 'warning' as const : 'danger' as const }] : []),
            ...(upcomingObligationsTotal > 0 ? [{ label: `Обязательства 30д · ${fmt(upcomingObligationsTotal)}`, tone: coverageTone === 'safe' ? 'success' as const : coverageTone === 'tight' ? 'warning' as const : 'danger' as const }] : []),
            ...(topCategory ? [{ label: `Топ: ${topCategory.category} · ${formatPercent(topCategory.amount, totalCategorySpend)}`, tone: 'muted' as const }] : []),
          ]}
          actions={(
            <PageSyncButton
              label="Синхронизировать ZenMoney"
              syncCaption={financeSyncCaption}
              syncing={syncing}
              disabled={!zenmoneyIntegration?.enabled}
              onClick={handleSyncFinance}
            />
          )}
        />

        <ExpandablePanel
          title="Период и диапазон"
          description="Меняй быстрый период или выставляй произвольные даты только когда реально нужно копнуть глубже."
          open={showRangePanel}
          onToggle={() => setShowRangePanel(current => !current)}
          summary={(
            <>
              <span className="rounded-full border border-primary/20 bg-primary/10 px-2.5 py-1 font-medium uppercase tracking-wide text-primary">
                {activePeriodLabel}
              </span>
              <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
                {rangeLabel}
              </span>
              <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
                {formatAccountCount(includedAccounts.length)} в балансе
              </span>
              {topCategory ? (
                <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
                  Топ категория: {topCategory.category} · {formatPercent(topCategory.amount, totalCategorySpend)}
                </span>
              ) : null}
            </>
          )}
        >
          <div className="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
            <p className="max-w-2xl text-sm text-foreground">
              {hasCustomRange
                ? 'Показываем произвольный диапазон без усреднений по календарным периодам.'
                : `Быстрый срез за ${activePeriodLabel.toLowerCase()} с фокусом на реальный cashflow и структуру трат.`}
            </p>

            <div className="flex flex-wrap items-end gap-3">
              <div className="flex flex-wrap gap-1 rounded-xl border bg-background/60 p-1">
                {PERIODS.map(p => (
                  <button
                    key={p.days}
                    onClick={() => { setPeriod(p.days); setCustomFrom(''); setCustomTo('') }}
                    className={cn(
                      'rounded-lg px-3 py-1.5 text-xs font-medium transition-colors',
                      period === p.days && !customFrom
                        ? 'bg-primary text-primary-foreground shadow-sm'
                        : 'text-muted-foreground hover:bg-accent hover:text-foreground'
                    )}
                  >
                    {p.label}
                  </button>
                ))}
              </div>

              <div className="grid grid-cols-1 gap-2 sm:grid-cols-[auto_auto_auto]">
                <label className="flex min-w-[128px] flex-col gap-1">
                  <span className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">С даты</span>
                  <input
                    type="date"
                    value={customFrom}
                    onChange={e => setCustomFrom(e.target.value)}
                    className="rounded-lg border bg-background px-3 py-2 text-xs outline-none transition-colors focus:border-primary"
                  />
                </label>
                <label className="flex min-w-[128px] flex-col gap-1">
                  <span className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">По дату</span>
                  <input
                    type="date"
                    value={customTo}
                    onChange={e => setCustomTo(e.target.value)}
                    className="rounded-lg border bg-background px-3 py-2 text-xs outline-none transition-colors focus:border-primary"
                  />
                </label>
                {hasCustomRange ? (
                  <button
                    onClick={() => { setCustomFrom(''); setCustomTo('') }}
                    className="self-end rounded-lg border px-3 py-2 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                  >
                    Сбросить
                  </button>
                ) : null}
              </div>
            </div>
          </div>
        </ExpandablePanel>
      </div>

      <div className="rounded-2xl border bg-card/70 px-5 py-4 shadow-sm">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <p className="text-sm font-semibold uppercase tracking-wider text-foreground">Golden Metrics</p>
            <p className="mt-1 text-sm text-muted-foreground">
              Верхняя линия теперь считает не только баланс, а реальное состояние периода: сколько ты сохранил, как быстро сжигаешь деньги и на сколько хватит ликвидного остатка.
            </p>
          </div>
          <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
            <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
              Период: {activePeriodLabel}
            </span>
            <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
              Диапазон: {rangeLabel}
            </span>
            <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
              Дней в окне: {rangeDays}
            </span>
          </div>
        </div>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
        <FinanceSummaryCard
          title="Ликвидный баланс"
          icon={Wallet}
          iconClassName="bg-blue-500"
          loading={loading}
          value={fmt(totalBalance)}
          caption={`${formatAccountCount(includedAccounts.length)} участвуют в общем балансе`}
          hint={excludedAccounts.length > 0 ? `${fmt(excludedBalance)} ещё лежит вне общего баланса` : 'Счета вне баланса сейчас не влияют на цифру сверху'}
        />

        <FinanceSummaryCard
          title="Net cashflow"
          icon={periodNet >= 0 ? TrendingUp : TrendingDown}
          iconClassName={periodNet >= 0 ? 'bg-emerald-500' : 'bg-rose-500'}
          loading={loading}
          value={fmtSigned(periodNet)}
          valueClassName={periodNet >= 0 ? 'text-emerald-300' : 'text-rose-300'}
          caption={`${activePeriodLabel} · ${rangeLabel}`}
          hint={`Доходы ${fmt(totalDailyIncome)} • расходы ${fmt(totalDailySpending)}`}
        />

        <FinanceSummaryCard
          title="Savings rate"
          icon={PiggyBank}
          iconClassName={savingsRate != null && savingsRate >= 0.2 ? 'bg-emerald-500' : savingsRate != null && savingsRate >= 0 ? 'bg-amber-500' : 'bg-rose-500'}
          loading={loading}
          value={formatRatioPercent(savingsRate)}
          valueClassName={savingsRate != null && savingsRate >= 0.2 ? 'text-emerald-300' : savingsRate != null && savingsRate >= 0 ? 'text-amber-200' : 'text-rose-300'}
          caption="доля дохода, которую ты сохранил"
          hint={totalDailyIncome > 0 ? `Если опускаться ниже 0%, ты уже проедаешь доход` : 'В выбранном диапазоне нет доходов, savings rate не считается'}
        />

        <FinanceSummaryCard
          title="Burn rate"
          icon={TrendingDown}
          iconClassName="bg-orange-500"
          loading={loading}
          value={fmt(avgDailySpending)}
          caption="средний расход в день"
          hint={`За ${rangeDays} дн. это даёт темп ${fmt(monthlyBurnProjection)} / 30 дн.`}
        />

        <FinanceSummaryCard
          title="Runway"
          icon={Landmark}
          iconClassName={Number.isFinite(runwayDays) && runwayDays >= 90 ? 'bg-violet-500' : Number.isFinite(runwayDays) && runwayDays >= 30 ? 'bg-amber-500' : 'bg-rose-500'}
          loading={loading}
          value={formatRunway(runwayDays)}
          valueClassName={Number.isFinite(runwayDays) && runwayDays >= 90 ? 'text-violet-200' : Number.isFinite(runwayDays) && runwayDays >= 30 ? 'text-amber-200' : 'text-rose-300'}
          caption="на сколько хватит ликвидного баланса"
          hint={avgDailySpending > 0 ? 'Расчёт по текущему burn rate, без новых доходов' : 'Расходов в диапазоне нет, runway не ограничен'}
        />

        <FinanceSummaryCard
          title="Концентрация расходов"
          icon={EyeOff}
          iconClassName={topThreeShare != null && topThreeShare > 0.7 ? 'bg-rose-500' : topThreeShare != null && topThreeShare > 0.55 ? 'bg-amber-500' : 'bg-cyan-500'}
          loading={loading}
          value={formatRatioPercent(topThreeShare)}
          valueClassName={topThreeShare != null && topThreeShare > 0.7 ? 'text-rose-300' : topThreeShare != null && topThreeShare > 0.55 ? 'text-amber-200' : 'text-cyan-200'}
          caption="топ-3 категории в выбранном периоде"
          hint={topThreeCategories.length > 0 ? `${concentrationLeaders} формируют основную массу трат` : 'Появится, когда в диапазоне будут категории расходов'}
        />
      </div>

      {!loading && excludedAccounts.length > 0 ? (
        <div className="rounded-2xl border border-amber-500/20 bg-amber-500/5 px-5 py-4">
          <p className="text-sm font-medium text-foreground">Часть счетов исключена из общего баланса</p>
          <p className="text-sm text-muted-foreground mt-1">
            ZenMoney помечает {formatAccountCount(excludedAccounts.length)} как не участвующие в общем балансе.
            Их сумма: {fmt(excludedBalance)}. Эти счета показаны ниже отдельной секцией и не участвуют
            в карточке "Баланс" и агрегатах по финансам.
          </p>
        </div>
      ) : null}

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-[0.92fr_1.38fr]">
        <div className="space-y-4">
          <div className="rounded-2xl border bg-card/70 px-5 py-4 shadow-sm">
            <div className="flex flex-col gap-3">
              <div>
                <p className="text-sm font-semibold uppercase tracking-wider text-foreground">Обязательные платежи</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Смотрим вперёд на {obligationsWindowDays} дней и автодетектим recurring списания по истории транзакций:
                  подписки, кредиты, связь, аренду и похожие платежи. Это прогноз по паттернам, а не данные банка о будущих списаниях.
                </p>
              </div>
              <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
                <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
                  Окно прогноза: {obligationsWindowDays} дней
                </span>
                <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
                  Найдено: {obligationItems.length}
                </span>
                <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
                  Ручных правил: {obligationRules.length}
                </span>
                {nextObligation ? (
                  <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
                    Следующий платёж: {fmtDate(nextObligation.next_due_at)}
                  </span>
                ) : null}
              </div>
            </div>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-1">
            <FinanceSummaryCard
              title={`Обязательства ${obligationsWindowDays}д`}
              icon={CalendarClock}
              iconClassName={upcomingObligationsTotal > 0 ? 'bg-rose-500' : 'bg-slate-500'}
              loading={loading}
              value={fmt(upcomingObligationsTotal)}
              caption="прогноз recurring списаний от сегодня"
              hint={nextObligation
                ? `${nextObligation.name} придёт ${fmtDate(nextObligation.next_due_at)} и даст ${fmt(nextObligation.projected_total)} в окне`
                : 'Пока не нашли устойчивых recurring списаний в истории транзакций'}
            />

            <FinanceSummaryCard
              title={`Coverage ${obligationsWindowDays}д`}
              icon={coverageTone === 'shortfall' ? ShieldAlert : ShieldCheck}
              iconClassName={coverageTone === 'safe' ? 'bg-emerald-500' : coverageTone === 'tight' ? 'bg-amber-500' : 'bg-rose-500'}
              loading={loading}
              value={formatCoverageMultiple(obligationCoverageRatio)}
              valueClassName={coverageTone === 'safe' ? 'text-emerald-300' : coverageTone === 'tight' ? 'text-amber-200' : 'text-rose-300'}
              caption="ликвидный баланс / обязательства ближайших 30 дней"
              hint={upcomingObligationsTotal > 0
                ? `Баланс ${fmt(totalBalance)} против обязательств ${fmt(upcomingObligationsTotal)} → ${obligationCoverageGap >= 0 ? `запас ${fmt(obligationCoverageGap)}` : `дефицит ${fmt(Math.abs(obligationCoverageGap))}`}`
                : 'Обязательства не найдены, поэтому coverage сейчас не ограничен'}
            />
          </div>

          <div className="rounded-2xl border bg-card/70 p-5 shadow-sm">
            <div className="flex flex-col gap-3">
              <div>
                <p className="text-sm font-semibold uppercase tracking-wider text-foreground">Ручные правила</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Если эвристика ошиблась, можно закрепить recurring-платёж или навсегда выкинуть шумный merchant из прогноза.
                </p>
              </div>
              {ruleError ? (
                <div className="rounded-xl border border-rose-500/20 bg-rose-500/10 px-3 py-2 text-xs text-rose-200">
                  {ruleError}
                </div>
              ) : null}
              {obligationRules.length === 0 ? (
                <div className="rounded-xl border border-dashed border-border/80 bg-background/40 px-4 py-4 text-sm text-muted-foreground">
                  Пока нет ручных правил. Если увидишь шум или захочешь зафиксировать recurring-платёж, управление появится прямо на карточках ниже.
                </div>
              ) : (
                <div className="space-y-2">
                  {obligationRules.map(rule => (
                    <div key={rule.key} className="flex flex-col gap-3 rounded-xl border border-border/80 bg-background/50 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium text-foreground">{rule.label}</p>
                        <div className="mt-1 flex flex-wrap gap-2 text-[11px] text-muted-foreground">
                          <span className={cn(
                            'rounded-full border px-2.5 py-1',
                            rule.action === 'force'
                              ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-200'
                              : 'border-amber-500/30 bg-amber-500/10 text-amber-100'
                          )}>
                            {rule.action === 'force' ? 'Зафиксирован' : 'Игнорируется'}
                          </span>
                          <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
                            обновлено {fmtDate(rule.updated_at)}
                          </span>
                        </div>
                      </div>
                      <button
                        type="button"
                        onClick={() => void handleDeleteObligationRule(rule)}
                        disabled={ruleSavingKey === `delete:${rule.key}`}
                        className="inline-flex items-center justify-center gap-2 rounded-lg border px-3 py-2 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
                      >
                        <RotateCcw className="h-3.5 w-3.5" />
                        {ruleSavingKey === `delete:${rule.key}` ? 'Убираю…' : 'Убрать правило'}
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>

        <div className="rounded-2xl border bg-card p-5 shadow-sm">
          <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-sm font-semibold uppercase tracking-wider text-foreground">Что именно попадёт в ближайшие {obligationsWindowDays} дней</h2>
              <p className="mt-1 text-xs text-muted-foreground">
                Список отсортирован по ближайшей дате списания. Если регулярный платёж будет повторяться несколько раз за окно, это видно в projected total.
              </p>
            </div>
            {obligationItems.length > 0 ? (
              <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1 text-[11px] font-medium text-muted-foreground">
                Всего: {fmt(upcomingObligationsTotal)}
              </span>
            ) : null}
          </div>

          {loading ? (
            <div className="space-y-3">
              <div className="h-20 animate-pulse rounded-xl bg-muted" />
              <div className="h-20 animate-pulse rounded-xl bg-muted" />
              <div className="h-20 animate-pulse rounded-xl bg-muted" />
            </div>
          ) : obligationItems.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-border/80 bg-background/40 px-5 py-6 text-sm text-muted-foreground">
              Пока не нашли recurring платежей с достаточно устойчивым ритмом. Когда в истории появятся повторяющиеся подписки, кредиты или коммуналка, они окажутся здесь.
            </div>
          ) : (
            <div className="space-y-3">
              {obligationItems.map(item => (
                <div key={`${item.name}-${item.next_due_at}`} className="rounded-2xl border border-border/80 bg-background/50 px-4 py-4">
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium text-foreground">{item.name}</p>
                      <div className="mt-2 flex flex-wrap gap-2 text-[11px] text-muted-foreground">
                        <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
                          {item.cadence_label}
                        </span>
                        {item.category ? (
                          <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
                            {item.category}
                          </span>
                        ) : null}
                        <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
                          История: {item.occurrences} списания
                        </span>
                        {item.rule_action === 'force' ? (
                          <span className="rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2.5 py-1 text-emerald-200">
                            Зафиксирован вручную
                          </span>
                        ) : (
                          <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
                            Автоэвристика
                          </span>
                        )}
                      </div>
                    </div>

                    <div className="shrink-0 text-left sm:text-right">
                      <p className="text-base font-semibold text-foreground">{fmt(item.projected_total)}</p>
                      <p className="text-xs text-muted-foreground">
                        ≈ {fmt(item.amount)} × {item.expected_occurrences}
                      </p>
                    </div>
                  </div>

                  <div className="mt-3 flex flex-wrap gap-3 text-xs text-muted-foreground">
                    <span>Следующее: {fmtDate(item.next_due_at)}</span>
                    {item.expected_occurrences > 1 ? (
                      <span>В окне повторится {item.expected_occurrences} раза</span>
                    ) : (
                      <span>В окне один платёж</span>
                    )}
                  </div>

                  <div className="mt-4 flex flex-wrap gap-2">
                    {item.rule_action === 'force' ? null : (
                      <button
                        type="button"
                        onClick={() => void handleSaveObligationRule({ key: item.key, label: item.name, action: 'force' })}
                        disabled={ruleSavingKey === `force:${item.key}` || ruleSavingKey === `ignore:${item.key}`}
                        className="inline-flex items-center justify-center gap-2 rounded-lg border border-emerald-500/30 bg-emerald-500/10 px-3 py-2 text-xs font-medium text-emerald-100 transition-colors hover:bg-emerald-500/15 disabled:cursor-not-allowed disabled:opacity-60"
                      >
                        <Pin className="h-3.5 w-3.5" />
                        {ruleSavingKey === `force:${item.key}` ? 'Фиксирую…' : 'Зафиксировать'}
                      </button>
                    )}
                    <button
                      type="button"
                      onClick={() => void handleSaveObligationRule({ key: item.key, label: item.name, action: 'ignore' })}
                      disabled={ruleSavingKey === `force:${item.key}` || ruleSavingKey === `ignore:${item.key}`}
                      className="inline-flex items-center justify-center gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs font-medium text-amber-100 transition-colors hover:bg-amber-500/15 disabled:cursor-not-allowed disabled:opacity-60"
                    >
                      <Ban className="h-3.5 w-3.5" />
                      {ruleSavingKey === `ignore:${item.key}` ? 'Исключаю…' : 'Игнорировать'}
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Charts row 1 */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Monthly income vs expenses */}
        <div className="rounded-2xl border bg-card p-5 shadow-sm">
          <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-sm font-semibold uppercase tracking-wider text-foreground">Доходы и расходы</h2>
              <p className="mt-1 text-xs text-muted-foreground">
                Тренд по месяцам с фокусом на разрыв между поступлениями и тратами.
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <span className="rounded-full border border-emerald-500/20 bg-emerald-500/10 px-2.5 py-1 text-[11px] font-medium text-emerald-300">
                Доходы: {currentMonth ? fmt(currentMonth.income) : '—'}
              </span>
              <span className="rounded-full border border-rose-500/20 bg-rose-500/10 px-2.5 py-1 text-[11px] font-medium text-rose-300">
                Расходы: {currentMonth ? fmt(currentMonth.spending) : '—'}
              </span>
            </div>
          </div>
          {loading ? <div className="h-48 bg-muted rounded animate-pulse" /> : (
            <EChart option={buildMonthlyOption(monthly)} height={220} />
          )}
        </div>

        {/* Daily spending trend */}
        <div className="rounded-2xl border bg-card p-5 shadow-sm">
          <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-sm font-semibold uppercase tracking-wider text-foreground">Расходы по дням</h2>
              <p className="mt-1 text-xs text-muted-foreground">
                Помогает увидеть всплески трат и редкие доходные дни.
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              {peakExpenseDay ? (
                <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1 text-[11px] font-medium text-muted-foreground">
                  Пик расходов: {fmtDate(peakExpenseDay.date)} · {fmt(peakExpenseDay.spending)}
                </span>
              ) : null}
            </div>
          </div>
          {loading ? <div className="h-48 bg-muted rounded animate-pulse" /> : (
            <EChart option={buildDailyOption(daily)} height={220} />
          )}
        </div>
      </div>

      {/* Charts row 2 */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Categories pie */}
        <div className="rounded-2xl border bg-card p-5 shadow-sm">
          <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-sm font-semibold uppercase tracking-wider text-foreground">Расходы по категориям</h2>
              <p className="mt-1 text-xs text-muted-foreground">
                Легенда справа кликабельна: можно быстро отфильтровать транзакции по категории.
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              {topCategory ? (
                <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1 text-[11px] font-medium text-muted-foreground">
                  Лидер: {topCategory.category} · {formatPercent(topCategory.amount, totalCategorySpend)}
                </span>
              ) : null}
              {catFilter ? (
                <button
                  onClick={() => setCatFilter('')}
                  className="rounded-full border border-primary/20 bg-primary/10 px-2.5 py-1 text-[11px] font-medium text-primary transition-colors hover:bg-primary/15"
                >
                  Фильтр: {catFilter} ×
                </button>
              ) : null}
            </div>
          </div>
          {loading ? <div className="h-56 bg-muted rounded animate-pulse" /> : categories.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-8">Нет данных</p>
          ) : (
            <div className="flex flex-col gap-6 lg:flex-row lg:items-start">
              <EChart option={buildCategoriesOption(categories)} height={200} width={200} className="shrink-0" />
              <div className="flex max-h-[280px] flex-1 min-w-0 flex-col gap-2 overflow-y-auto py-1 pr-1">
                {categories.map((c, i) => (
                  <button
                    key={c.category}
                    onClick={() => setCatFilter(catFilter === c.category ? '' : c.category)}
                    className={cn(
                      'rounded-xl border border-transparent bg-background/45 px-3 py-2 text-left transition-colors hover:border-border hover:bg-accent/40',
                      catFilter === c.category && 'border-primary/30 bg-primary/10'
                    )}
                  >
                    <div className="flex items-center gap-3 text-xs">
                      <div
                        className="h-2.5 w-2.5 shrink-0 rounded-full"
                        style={{ backgroundColor: CATEGORY_COLORS[i % CATEGORY_COLORS.length] }}
                      />
                      <span className={cn('min-w-0 flex-1 truncate text-sm', catFilter === c.category && 'font-medium text-primary')}>
                        {c.category}
                      </span>
                      <span className="shrink-0 text-[11px] font-medium text-muted-foreground">
                        {formatPercent(c.amount, totalCategorySpend)}
                      </span>
                      <span className="shrink-0 tabular-nums text-sm text-foreground">{fmt(c.amount)}</span>
                    </div>
                    <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-muted/70">
                      <div
                        className="h-full rounded-full"
                        style={{
                          width: formatPercent(c.amount, totalCategorySpend),
                          backgroundColor: CATEGORY_COLORS[i % CATEGORY_COLORS.length],
                        }}
                      />
                    </div>
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* Income categories */}
        <div className="rounded-2xl border bg-card p-5 shadow-sm">
          <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-sm font-semibold uppercase tracking-wider text-foreground">Доходы по категориям</h2>
              <p className="mt-1 text-xs text-muted-foreground">
                Отдельный срез поступлений за выбранный период без переводов между счетами.
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              {topIncomeCategory ? (
                <span className="rounded-full border border-emerald-500/20 bg-emerald-500/10 px-2.5 py-1 text-[11px] font-medium text-emerald-300">
                  Лидер: {topIncomeCategory.category} · {formatPercent(topIncomeCategory.amount, totalCategoryIncome)}
                </span>
              ) : null}
              {filter === 'income' && catFilter ? (
                <button
                  onClick={() => {
                    setFilter('')
                    setCatFilter('')
                  }}
                  className="rounded-full border border-emerald-500/20 bg-emerald-500/10 px-2.5 py-1 text-[11px] font-medium text-emerald-300 transition-colors hover:bg-emerald-500/15"
                >
                  Фильтр: {catFilter} ×
                </button>
              ) : null}
            </div>
          </div>
          {loading ? <div className="h-56 bg-muted rounded animate-pulse" /> : incomeCategories.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-8">Нет данных</p>
          ) : (
            <div className="flex flex-col gap-6 lg:flex-row lg:items-start">
              <EChart option={buildCategoriesOption(incomeCategories, INCOME_CATEGORY_COLORS)} height={200} width={200} className="shrink-0" />
              <div className="flex max-h-[280px] flex-1 min-w-0 flex-col gap-2 overflow-y-auto py-1 pr-1">
                {incomeCategories.map((c, i) => (
                  <button
                    key={c.category}
                    onClick={() => {
                      const isSameFilter = filter === 'income' && catFilter === c.category
                      setFilter(isSameFilter ? '' : 'income')
                      setCatFilter(isSameFilter ? '' : c.category)
                    }}
                    className={cn(
                      'rounded-xl border border-transparent bg-background/45 px-3 py-2 text-left transition-colors hover:border-border hover:bg-accent/40',
                      filter === 'income' && catFilter === c.category && 'border-emerald-500/30 bg-emerald-500/10'
                    )}
                  >
                    <div className="flex items-center gap-3 text-xs">
                      <div
                        className="h-2.5 w-2.5 shrink-0 rounded-full"
                        style={{ backgroundColor: INCOME_CATEGORY_COLORS[i % INCOME_CATEGORY_COLORS.length] }}
                      />
                      <span className={cn('min-w-0 flex-1 truncate text-sm', filter === 'income' && catFilter === c.category && 'font-medium text-emerald-300')}>
                        {c.category}
                      </span>
                      <span className="shrink-0 text-[11px] font-medium text-muted-foreground">
                        {formatPercent(c.amount, totalCategoryIncome)}
                      </span>
                      <span className="shrink-0 tabular-nums text-sm text-foreground">{fmt(c.amount)}</span>
                    </div>
                    <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-muted/70">
                      <div
                        className="h-full rounded-full"
                        style={{
                          width: formatPercent(c.amount, totalCategoryIncome),
                          backgroundColor: INCOME_CATEGORY_COLORS[i % INCOME_CATEGORY_COLORS.length],
                        }}
                      />
                    </div>
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* Top expenses */}
        <div className="rounded-2xl border bg-card p-5 shadow-sm">
          <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-sm font-semibold uppercase tracking-wider text-foreground">Топ расходов</h2>
              <p className="mt-1 text-xs text-muted-foreground">
                Группировка по payee из транзакций, чтобы быстро увидеть главные утечки.
              </p>
            </div>
            {topPayee ? (
              <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1 text-[11px] font-medium text-muted-foreground">
                Лидер: {truncateLabel(topPayee.payee, 18)} · {fmt(topPayee.amount)}
              </span>
            ) : null}
          </div>
          {loading ? <div className="h-56 bg-muted rounded animate-pulse" /> : topExpenses.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-8">Нет данных</p>
          ) : (
            <div className="space-y-4">
              <EChart option={buildTopExpensesOption(topExpenses)} height={Math.max(220, topExpenses.length * 32)} />
              <div className="grid gap-2 sm:grid-cols-3">
                {topExpenses.slice(0, 3).map((expense, index) => (
                  <div key={expense.payee} className="rounded-xl border bg-background/50 px-3 py-2">
                    <p className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">#{index + 1}</p>
                    <p className="mt-1 truncate text-sm font-medium text-foreground">{expense.payee}</p>
                    <p className="mt-1 text-xs text-muted-foreground">{expense.count} операций</p>
                    <p className="mt-2 text-sm font-semibold tabular-nums text-foreground">{fmt(expense.amount)}</p>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Accounts + Transactions */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3 lg:items-start">
        {/* Accounts */}
        <div className="self-start overflow-hidden rounded-2xl border bg-card shadow-sm">
          <div className="border-b px-5 py-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h2 className="text-sm font-semibold uppercase tracking-wider text-foreground">Счета</h2>
                <p className="mt-1 text-xs text-muted-foreground">
                  Быстрый обзор всех кошельков, карт и счетов из ZenMoney.
                </p>
              </div>
              {!loading ? (
                <div className="flex flex-wrap gap-2">
                  <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1 text-[11px] font-medium text-muted-foreground">
                    В балансе: {fmt(totalBalance)}
                  </span>
                  {excludedAccounts.length > 0 ? (
                    <span className="rounded-full border border-amber-500/20 bg-amber-500/10 px-2.5 py-1 text-[11px] font-medium text-amber-200">
                      Вне баланса: {fmt(excludedBalance)}
                    </span>
                  ) : null}
                </div>
              ) : null}
            </div>
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
        <div className="overflow-hidden rounded-2xl border bg-card shadow-sm lg:col-span-2">
          <div className="border-b px-5 py-4">
            <div className="flex flex-col gap-3">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h2 className="text-sm font-semibold uppercase tracking-wider text-foreground">Транзакции</h2>
                  <p className="mt-1 text-xs text-muted-foreground">
                    Живой список операций за выбранный период. Поиск и фильтры применяются сразу.
                  </p>
                </div>
                <div className="flex flex-wrap gap-2">
                  <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1 text-[11px] font-medium text-muted-foreground">
                    Показано: {txs.length}
                  </span>
                  {activeTransactionFilters > 0 ? (
                    <span className="rounded-full border border-primary/20 bg-primary/10 px-2.5 py-1 text-[11px] font-medium text-primary">
                      Активных фильтров: {activeTransactionFilters}
                    </span>
                  ) : null}
                </div>
              </div>

              <div className="flex gap-1">
                {(['', 'income', 'expense'] as FilterType[]).map(f => (
                  <button key={f} onClick={() => setFilter(f)}
                    className={cn(
                      'rounded-lg px-3 py-1 text-xs font-medium transition-colors',
                      filter === f ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:bg-accent'
                    )}
                  >
                    {f === '' ? 'Все' : f === 'income' ? 'Доходы' : 'Расходы'}
                  </button>
                ))}
              </div>

              <div className="grid gap-2 md:grid-cols-[minmax(0,1fr)_180px_160px]">
                <div className="relative">
                  <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
                  <input
                    type="text"
                    value={search}
                    onChange={e => setSearch(e.target.value)}
                    placeholder="Поиск по payee или комментарию..."
                    className="w-full rounded-lg border bg-background pl-8 pr-3 py-2 text-xs outline-none transition-colors focus:border-primary"
                  />
                </div>
                <select
                  value={catFilter}
                  onChange={e => setCatFilter(e.target.value)}
                  className="rounded-lg border bg-background px-3 py-2 text-xs outline-none transition-colors focus:border-primary"
                >
                  <option value="">Все категории</option>
                  {categoryList.map(c => <option key={c} value={c}>{c}</option>)}
                </select>
                <select
                  value={sort}
                  onChange={e => setSort(e.target.value as SortType)}
                  className="rounded-lg border bg-background px-3 py-2 text-xs outline-none transition-colors focus:border-primary"
                >
                  <option value="">По дате ↓</option>
                  <option value="date_asc">По дате ↑</option>
                  <option value="amount">По сумме ↓</option>
                  <option value="amount_asc">По сумме ↑</option>
                  <option value="category">По категории</option>
                </select>
              </div>
            </div>
          </div>

          <div className="divide-y max-h-[480px] overflow-y-auto">
            {txs.length === 0 && !txLoading ? (
              <div className="px-5 py-4 text-sm text-muted-foreground text-center">Нет транзакций</div>
            ) : (
              txs.map(tx => (
                <div key={tx.id} className="flex items-center gap-3 px-5 py-3 transition-colors hover:bg-accent/30">
                  <span className="text-xs text-muted-foreground w-16 shrink-0">{fmtDate(tx.occurred_at)}</span>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm text-foreground truncate">{tx.payee || tx.comment || '—'}</p>
                    {tx.category && <p className="text-[10px] text-muted-foreground/70">{tx.category}</p>}
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

function FinanceSummaryCard({
  title,
  value,
  caption,
  hint,
  valueClassName,
  icon: Icon,
  iconClassName,
  loading,
  panelClassName,
}: {
  title: string
  value: string
  caption: string
  hint?: string
  valueClassName?: string
  icon: LucideIcon
  iconClassName: string
  loading: boolean
  panelClassName?: string
}) {
  return (
    <div className={cn('rounded-2xl border bg-card/90 p-5 shadow-sm', panelClassName)}>
      <div className="flex items-start justify-between gap-3">
        <div className="space-y-1">
          <p className="text-sm font-medium text-muted-foreground">{title}</p>
          {loading ? (
            <div className="h-8 w-28 animate-pulse rounded bg-muted" />
          ) : (
            <p className={cn('text-3xl font-semibold tracking-tight text-foreground', valueClassName)}>{value}</p>
          )}
        </div>
        <div className={cn('flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl text-white shadow-sm', iconClassName)}>
          <Icon className="h-5 w-5" />
        </div>
      </div>
      <div className="mt-4 space-y-1">
        <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{caption}</p>
        {hint ? (
          <p className="text-sm leading-5 text-muted-foreground">{hint}</p>
        ) : null}
      </div>
    </div>
  )
}
