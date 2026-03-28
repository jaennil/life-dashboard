import { useEffect, useState, useCallback } from 'react'
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Legend, PieChart, Pie, Cell } from 'recharts'
import { Wallet, TrendingDown, TrendingUp } from 'lucide-react'
import { cn } from '@/lib/utils'
import { api, type MonthStat, type Account, type FinanceTransaction, type CategoryStat } from '@/lib/api'

const CATEGORY_COLORS = [
  '#f97316', '#3b82f6', '#10b981', '#8b5cf6', '#f43f5e',
  '#06b6d4', '#eab308', '#ec4899', '#14b8a6', '#a855f7',
]

const CategoryTooltip = ({ active, payload }: any) => {
  if (!active || !payload?.length) return null
  return (
    <div className="rounded-xl border bg-card px-4 py-3 text-sm shadow-lg">
      <p className="font-medium text-foreground">{payload[0]?.payload?.category}</p>
      <p style={{ color: payload[0]?.fill }}>{fmt(payload[0]?.value, 'RUB')}</p>
    </div>
  )
}

const MONTH_LABELS: Record<string, string> = {
  '01': 'Янв', '02': 'Фев', '03': 'Мар', '04': 'Апр',
  '05': 'Май', '06': 'Июн', '07': 'Июл', '08': 'Авг',
  '09': 'Сен', '10': 'Окт', '11': 'Ноя', '12': 'Дек',
}

function fmt(amount: number, currency: string) {
  return new Intl.NumberFormat('ru-RU', {
    style: 'currency', currency, maximumFractionDigits: 0,
  }).format(amount)
}

function fmtShort(value: number) {
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

const CustomTooltip = ({ active, payload, label }: any) => {
  if (!active || !payload?.length) return null
  return (
    <div className="rounded-xl border bg-card px-4 py-3 text-sm shadow-lg">
      <p className="font-medium text-foreground mb-2">{formatMonth(label)}</p>
      {payload.map((p: any) => (
        <p key={p.name} style={{ color: p.color }}>
          {p.name === 'spending' ? 'Расходы' : 'Доходы'}: {fmt(p.value, 'RUB')}
        </p>
      ))}
    </div>
  )
}

type FilterType = '' | 'income' | 'expense'

export function Finance() {
  const [monthly, setMonthly] = useState<MonthStat[]>([])
  const [accounts, setAccounts] = useState<Account[]>([])
  const [categories, setCategories] = useState<CategoryStat[]>([])
  const [txs, setTxs] = useState<FinanceTransaction[]>([])
  const [filter, setFilter] = useState<FilterType>('')
  const [page, setPage] = useState(1)
  const [hasMore, setHasMore] = useState(true)
  const [loading, setLoading] = useState(true)
  const [txLoading, setTxLoading] = useState(false)

  useEffect(() => {
    Promise.all([api.getMonthlyStats(), api.getAccounts(), api.getSpendingByCategory()])
      .then(([m, a, c]) => { setMonthly(m); setAccounts(a); setCategories(c) })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [])

  const loadTxs = useCallback(async (f: FilterType, p: number, replace: boolean) => {
    setTxLoading(true)
    try {
      const data = await api.getTransactions(p, f)
      setTxs(prev => replace ? data : [...prev, ...data])
      setHasMore(data.length === 30)
    } catch (e) {
      console.error(e)
    } finally {
      setTxLoading(false)
    }
  }, [])

  useEffect(() => {
    setPage(1)
    loadTxs(filter, 1, true)
  }, [filter, loadTxs])

  function loadMore() {
    const next = page + 1
    setPage(next)
    loadTxs(filter, next, false)
  }

  const currentMonth = monthly[monthly.length - 1]
  const totalBalance = accounts
    .filter(a => a.currency === 'RUB' && a.balance > 0)
    .reduce((sum, a) => sum + a.balance, 0)

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-bold text-foreground">Финансы</h1>
        <p className="text-sm text-muted-foreground mt-1">
          {new Date().toLocaleDateString('ru-RU', { month: 'long', year: 'numeric' })}
        </p>
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
            : <div className="text-2xl font-bold text-foreground">{fmt(totalBalance, 'RUB')}</div>}
          <span className="text-xs text-muted-foreground">по всем счетам</span>
        </div>

        <div className="rounded-xl border bg-card p-5 flex flex-col gap-2">
          <div className="flex items-center justify-between">
            <span className="text-sm text-muted-foreground">Расходы за месяц</span>
            <div className="flex items-center justify-center w-8 h-8 rounded-lg bg-rose-500">
              <TrendingDown className="w-4 h-4 text-white" />
            </div>
          </div>
          {loading
            ? <div className="h-8 w-32 bg-muted rounded animate-pulse" />
            : <div className="text-2xl font-bold text-foreground">{currentMonth ? fmt(currentMonth.spending, 'RUB') : '—'}</div>}
          <span className="text-xs text-muted-foreground">текущий месяц</span>
        </div>

        <div className="rounded-xl border bg-card p-5 flex flex-col gap-2">
          <div className="flex items-center justify-between">
            <span className="text-sm text-muted-foreground">Доходы за месяц</span>
            <div className="flex items-center justify-center w-8 h-8 rounded-lg bg-emerald-500">
              <TrendingUp className="w-4 h-4 text-white" />
            </div>
          </div>
          {loading
            ? <div className="h-8 w-32 bg-muted rounded animate-pulse" />
            : <div className="text-2xl font-bold text-foreground">{currentMonth ? fmt(currentMonth.income, 'RUB') : '—'}</div>}
          <span className="text-xs text-muted-foreground">текущий месяц</span>
        </div>
      </div>

      {/* Chart */}
      <div className="rounded-xl border bg-card p-5">
        <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Доходы и расходы</h2>
        {loading ? (
          <div className="h-48 bg-muted rounded animate-pulse" />
        ) : (
          <ResponsiveContainer width="100%" height={220}>
            <BarChart data={monthly} barCategoryGap="30%" barGap={4}>
              <XAxis
                dataKey="month"
                tickFormatter={formatMonth}
                tick={{ fontSize: 12 }}
                axisLine={false}
                tickLine={false}
              />
              <YAxis
                tickFormatter={fmtShort}
                tick={{ fontSize: 12 }}
                axisLine={false}
                tickLine={false}
                width={36}
              />
              <Tooltip content={<CustomTooltip />} cursor={{ opacity: 0.1 }} />
              <Legend
                formatter={(v) => v === 'spending' ? 'Расходы' : 'Доходы'}
                wrapperStyle={{ fontSize: 12 }}
              />
              <Bar dataKey="income" fill="#10b981" radius={[4, 4, 0, 0]} />
              <Bar dataKey="spending" fill="#f43f5e" radius={[4, 4, 0, 0]} />
            </BarChart>
          </ResponsiveContainer>
        )}
      </div>

      {/* Category breakdown */}
      <div className="rounded-xl border bg-card p-5">
        <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-4">Расходы по категориям</h2>
        {loading ? (
          <div className="h-56 bg-muted rounded animate-pulse" />
        ) : categories.length === 0 ? (
          <p className="text-sm text-muted-foreground text-center py-8">Нет данных за текущий месяц</p>
        ) : (
          <div className="flex flex-col lg:flex-row gap-8 items-start">
            <div style={{ width: 240, height: 240, flexShrink: 0 }}>
              <PieChart width={240} height={240}>
                <Pie
                  data={categories}
                  dataKey="amount"
                  nameKey="category"
                  cx={120}
                  cy={120}
                  innerRadius={65}
                  outerRadius={108}
                  paddingAngle={2}
                >
                  {categories.map((_, i) => (
                    <Cell key={i} fill={CATEGORY_COLORS[i % CATEGORY_COLORS.length]} />
                  ))}
                </Pie>
                <Tooltip content={<CategoryTooltip />} />
              </PieChart>
            </div>
            <div className="flex flex-col gap-2 flex-1 min-w-0 py-2">
              {categories.map((c, i) => (
                <div key={c.category} className="flex items-center gap-2 text-sm">
                  <div className="w-2.5 h-2.5 rounded-full shrink-0" style={{ backgroundColor: CATEGORY_COLORS[i % CATEGORY_COLORS.length] }} />
                  <span className="flex-1 text-foreground truncate">{c.category}</span>
                  <span className="text-muted-foreground tabular-nums shrink-0">{fmt(c.amount, 'RUB')}</span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Accounts */}
        <div className="rounded-xl border bg-card overflow-hidden">
          <div className="px-5 py-4 border-b">
            <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Счета</h2>
          </div>
          {loading ? (
            <div className="divide-y">
              {Array.from({ length: 4 }).map((_, i) => (
                <div key={i} className="px-5 py-3 flex items-center gap-3">
                  <div className="flex-1 h-4 bg-muted rounded animate-pulse" />
                  <div className="h-4 w-20 bg-muted rounded animate-pulse" />
                </div>
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
                    <p className="text-xs text-muted-foreground">{a.type}</p>
                  </div>
                  <span className={cn(
                    'text-sm font-medium tabular-nums shrink-0',
                    a.balance >= 0 ? 'text-foreground' : 'text-rose-500'
                  )}>
                    {fmt(a.balance, a.currency)}
                  </span>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Transactions */}
        <div className="lg:col-span-2 rounded-xl border bg-card overflow-hidden">
          <div className="px-5 py-4 border-b flex items-center justify-between">
            <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Транзакции</h2>
            <div className="flex gap-1">
              {(['', 'income', 'expense'] as FilterType[]).map(f => (
                <button
                  key={f}
                  onClick={() => setFilter(f)}
                  className={cn(
                    'px-3 py-1 text-xs rounded-lg transition-colors',
                    filter === f
                      ? 'bg-primary text-primary-foreground'
                      : 'bg-muted text-muted-foreground hover:bg-accent hover:text-accent-foreground'
                  )}
                >
                  {f === '' ? 'Все' : f === 'income' ? 'Доходы' : 'Расходы'}
                </button>
              ))}
            </div>
          </div>

          <div className="divide-y max-h-[480px] overflow-y-auto">
            {txs.length === 0 && !txLoading ? (
              <div className="px-5 py-4 text-sm text-muted-foreground text-center">Нет транзакций</div>
            ) : (
              txs.map(tx => (
                <div key={tx.id} className="px-5 py-3 flex items-center gap-4">
                  <span className="text-xs text-muted-foreground w-16 shrink-0">{fmtDate(tx.occurred_at)}</span>
                  <span className="flex-1 text-sm text-foreground truncate">
                    {tx.payee || tx.comment || '—'}
                  </span>
                  <span className={cn(
                    'text-sm font-medium tabular-nums shrink-0',
                    tx.amount > 0 ? 'text-emerald-500' : 'text-rose-500'
                  )}>
                    {tx.amount > 0 ? '+' : ''}{fmt(tx.amount, tx.currency)}
                  </span>
                </div>
              ))
            )}
            {txLoading && (
              <div className="px-5 py-3 text-sm text-muted-foreground text-center">Загрузка...</div>
            )}
          </div>

          {hasMore && !txLoading && txs.length > 0 && (
            <div className="px-5 py-3 border-t">
              <button
                onClick={loadMore}
                className="w-full text-sm text-muted-foreground hover:text-foreground transition-colors"
              >
                Загрузить ещё
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
