import { useEffect, useState, useCallback } from 'react'
import {
  BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Legend,
  PieChart, Pie, Cell, AreaChart, Area,
} from 'recharts'
import { Wallet, TrendingDown, TrendingUp, Search } from 'lucide-react'
import { cn } from '@/lib/utils'
import {
  api,
  type MonthStat, type Account, type FinanceTransaction,
  type CategoryStat, type DailyTotal, type TopExpense,
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

function formatMonth(ym: string) {
  const [, m] = ym.split('-')
  return MONTH_LABELS[m] ?? ym
}

function dateOffset(days: number) {
  const d = new Date()
  d.setDate(d.getDate() - days)
  return d.toISOString().split('T')[0]
}

const ChartTooltip = ({ active, payload, label, formatter }: any) => {
  if (!active || !payload?.length) return null
  return (
    <div className="rounded-xl border bg-card px-4 py-3 text-sm shadow-lg">
      <p className="font-medium text-foreground mb-1">{typeof label === 'string' && label.includes('-') ? fmtDate(label) : label}</p>
      {payload.map((p: any) => (
        <p key={p.name} style={{ color: p.color ?? p.fill }}>
          {formatter ? formatter(p) : `${p.name}: ${fmt(p.value)}`}
        </p>
      ))}
    </div>
  )
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

  const from = customFrom || dateOffset(period)
  const to = customTo || undefined

  useEffect(() => {
    Promise.all([
      api.getMonthlyStats(),
      api.getAccounts(),
      api.getSpendingByCategory(from),
      api.getDailyTotals(from, to),
      api.getTopExpenses(from, to),
      api.getCategoryList(),
    ])
      .then(([m, a, c, d, t, cl]) => {
        setMonthly(m); setAccounts(a); setCategories(c)
        setDaily(d); setTopExpenses(t); setCategoryList(cl)
      })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [period, customFrom, customTo])

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
  }, [filter, sort, search, catFilter, from])

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
  const totalBalance = accounts
    .filter(a => a.currency === 'RUB' && a.in_balance)
    .reduce((sum, a) => sum + a.balance, 0)

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-foreground">Финансы</h1>
          <p className="text-sm text-muted-foreground mt-1">
            {new Date().toLocaleDateString('ru-RU', { month: 'long', year: 'numeric' })}
          </p>
        </div>
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

      {/* Summary cards */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
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
          <span className="text-xs text-muted-foreground">по счетам в балансе</span>
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

      {/* Charts row 1 */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Monthly income vs expenses */}
        <div className="rounded-xl border bg-card p-5">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Доходы и расходы</h2>
          {loading ? <div className="h-48 bg-muted rounded animate-pulse" /> : (
            <ResponsiveContainer width="100%" height={220}>
              <BarChart data={monthly} barCategoryGap="30%" barGap={4}>
                <XAxis dataKey="month" tickFormatter={formatMonth} tick={{ fontSize: 12 }} axisLine={false} tickLine={false} />
                <YAxis tickFormatter={fmtShort} tick={{ fontSize: 12 }} axisLine={false} tickLine={false} width={36} />
                <Tooltip content={<ChartTooltip formatter={(p: any) => `${p.name === 'spending' ? 'Расходы' : 'Доходы'}: ${fmt(p.value)}`} />} cursor={{ opacity: 0.1 }} />
                <Legend formatter={v => v === 'spending' ? 'Расходы' : 'Доходы'} wrapperStyle={{ fontSize: 12 }} />
                <Bar dataKey="income" fill="#10b981" radius={[4, 4, 0, 0]} />
                <Bar dataKey="spending" fill="#f43f5e" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          )}
        </div>

        {/* Daily spending trend */}
        <div className="rounded-xl border bg-card p-5">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Расходы по дням</h2>
          {loading ? <div className="h-48 bg-muted rounded animate-pulse" /> : (
            <ResponsiveContainer width="100%" height={220}>
              <AreaChart data={daily}>
                <XAxis dataKey="date" tickFormatter={fmtDate} tick={{ fontSize: 11 }} axisLine={false} tickLine={false} interval="preserveStartEnd" />
                <YAxis tickFormatter={fmtShort} tick={{ fontSize: 12 }} axisLine={false} tickLine={false} width={36} />
                <Tooltip content={<ChartTooltip formatter={(p: any) => `${p.name === 'spending' ? 'Расходы' : 'Доходы'}: ${fmt(p.value)}`} />} />
                <Area type="monotone" dataKey="spending" stroke="#f43f5e" fill="#f43f5e" fillOpacity={0.15} strokeWidth={2} />
                <Area type="monotone" dataKey="income" stroke="#10b981" fill="#10b981" fillOpacity={0.1} strokeWidth={1.5} />
              </AreaChart>
            </ResponsiveContainer>
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
              <div style={{ width: 200, height: 200, flexShrink: 0 }}>
                <PieChart width={200} height={200}>
                  <Pie data={categories} dataKey="amount" nameKey="category" cx={100} cy={100} innerRadius={55} outerRadius={90} paddingAngle={2}>
                    {categories.map((_, i) => <Cell key={i} fill={CATEGORY_COLORS[i % CATEGORY_COLORS.length]} />)}
                  </Pie>
                  <Tooltip content={<ChartTooltip formatter={(p: any) => fmt(p.value)} />} />
                </PieChart>
              </div>
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
            <ResponsiveContainer width="100%" height={Math.max(200, topExpenses.length * 32)}>
              <BarChart data={topExpenses} layout="vertical" margin={{ left: 0, right: 10 }}>
                <XAxis type="number" tickFormatter={fmtShort} tick={{ fontSize: 11 }} axisLine={false} tickLine={false} />
                <YAxis type="category" dataKey="payee" tick={{ fontSize: 11 }} axisLine={false} tickLine={false} width={120} />
                <Tooltip content={<ChartTooltip formatter={(p: any) => `${fmt(p.value)} (${p.payload.count} операций)`} />} cursor={{ opacity: 0.1 }} />
                <Bar dataKey="amount" fill="#f97316" radius={[0, 4, 4, 0]} />
              </BarChart>
            </ResponsiveContainer>
          )}
        </div>
      </div>

      {/* Accounts + Transactions */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Accounts */}
        <div className="rounded-xl border bg-card overflow-hidden">
          <div className="px-5 py-4 border-b">
            <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Счета</h2>
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
            <div className="divide-y max-h-80 overflow-y-auto">
              {accounts.map(a => (
                <div key={a.id} className="px-5 py-3 flex items-center gap-3">
                  <div className="flex-1 min-w-0">
                    <p className="text-sm text-foreground truncate">{a.title}</p>
                    <div className="flex items-center gap-2">
                      <p className="text-xs text-muted-foreground">{a.type}</p>
                      {!a.in_balance ? (
                        <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-amber-200">
                          Вне баланса
                        </span>
                      ) : null}
                    </div>
                  </div>
                  <span className={cn('text-sm font-medium tabular-nums shrink-0', a.balance >= 0 ? 'text-foreground' : 'text-rose-500')}>
                    {fmt(a.balance, a.currency)}
                  </span>
                </div>
              ))}
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
