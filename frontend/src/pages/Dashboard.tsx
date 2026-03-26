import { useEffect, useState } from 'react'
import { Wallet, Dumbbell, TrendingUp, TrendingDown, Zap, Route } from 'lucide-react'
import { cn } from '@/lib/utils'
import { api, type DashboardSummary, type Transaction } from '@/lib/api'

function StatCard({
  title,
  value,
  sub,
  icon: Icon,
  trend,
  color,
  loading,
}: {
  title: string
  value: string
  sub: string
  icon: React.ElementType
  trend?: 'up' | 'down'
  color: string
  loading?: boolean
}) {
  return (
    <div className="rounded-xl border bg-card p-5 flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-muted-foreground">{title}</span>
        <div className={cn('flex items-center justify-center w-8 h-8 rounded-lg', color)}>
          <Icon className="w-4 h-4 text-white" />
        </div>
      </div>
      <div>
        {loading ? (
          <div className="h-8 w-24 bg-muted rounded animate-pulse" />
        ) : (
          <div className="text-2xl font-bold text-foreground">{value}</div>
        )}
        <div className="flex items-center gap-1 mt-1">
          {trend === 'up' && <TrendingUp className="w-3 h-3 text-emerald-500" />}
          {trend === 'down' && <TrendingDown className="w-3 h-3 text-red-500" />}
          <span className="text-xs text-muted-foreground">{sub}</span>
        </div>
      </div>
    </div>
  )
}

function InsightCard({ text, type }: { text: string; type: 'info' | 'warn' | 'good' }) {
  const colors = {
    info: 'border-l-blue-500 bg-blue-500/5',
    warn: 'border-l-amber-500 bg-amber-500/5',
    good: 'border-l-emerald-500 bg-emerald-500/5',
  }
  return (
    <div className={cn('border-l-2 rounded-r-lg px-4 py-3 text-sm text-foreground', colors[type])}>
      <Zap className="inline w-3 h-3 mr-1 opacity-70" />
      {text}
    </div>
  )
}

function fmt(amount: number, currency: string) {
  return new Intl.NumberFormat('ru-RU', { style: 'currency', currency, maximumFractionDigits: 0 }).format(amount)
}

function fmtDate(iso: string) {
  return new Date(iso).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })
}

export function Dashboard() {
  const [summary, setSummary] = useState<DashboardSummary | null>(null)
  const [txs, setTxs] = useState<Transaction[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    Promise.all([api.getDashboardSummary(), api.getRecentTransactions()])
      .then(([s, t]) => { setSummary(s); setTxs(t) })
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [])

  const insights: { text: string; type: 'info' | 'warn' | 'good' }[] = []
  if (summary) {
    if (summary.fitness.activities_this_week === 0 && summary.fitness.workouts_this_week === 0) {
      insights.push({ type: 'warn', text: 'На этой неделе нет активностей. Самое время размяться!' })
    }
    if (summary.finance.monthly_spending > summary.finance.monthly_income && summary.finance.monthly_income > 0) {
      insights.push({ type: 'warn', text: `Расходы (${fmt(summary.finance.monthly_spending, 'RUB')}) превышают доходы в этом месяце.` })
    }
    if (summary.fitness.total_distance_km > 0) {
      insights.push({ type: 'good', text: `За эту неделю пройдено ${summary.fitness.total_distance_km.toFixed(1)} км.` })
    }
  }
  if (insights.length === 0) {
    insights.push({ type: 'info', text: 'Подключи больше источников данных чтобы получать персональные инсайты.' })
  }

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-bold text-foreground">Dashboard</h1>
        <p className="text-sm text-muted-foreground mt-1">
          {new Date().toLocaleDateString('ru-RU', { weekday: 'long', day: 'numeric', month: 'long' })}
        </p>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
        <StatCard
          title="Баланс"
          value={summary ? fmt(summary.finance.total_balance, 'RUB') : '—'}
          sub="по всем счетам"
          icon={Wallet}
          color="bg-blue-500"
          loading={loading}
        />
        <StatCard
          title="Расходы за месяц"
          value={summary ? fmt(summary.finance.monthly_spending, 'RUB') : '—'}
          sub={summary ? `доходы: ${fmt(summary.finance.monthly_income, 'RUB')}` : 'нет данных'}
          icon={TrendingDown}
          color="bg-rose-500"
          loading={loading}
        />
        <StatCard
          title="Активности"
          value={summary ? String(summary.fitness.activities_this_week) : '—'}
          sub={summary && summary.fitness.total_distance_km > 0
            ? `${summary.fitness.total_distance_km.toFixed(1)} км на этой неделе`
            : 'за эту неделю'}
          icon={Route}
          trend={summary && summary.fitness.activities_this_week > 0 ? 'up' : undefined}
          color="bg-orange-500"
          loading={loading}
        />
        <StatCard
          title="Тренировки"
          value={summary ? String(summary.fitness.workouts_this_week) : '—'}
          sub="за эту неделю"
          icon={Dumbbell}
          trend={summary && summary.fitness.workouts_this_week > 0 ? 'up' : undefined}
          color="bg-violet-500"
          loading={loading}
        />
      </div>

      {/* AI Insights */}
      <div className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">AI Инсайты</h2>
        <div className="flex flex-col gap-2">
          {insights.map((ins, i) => <InsightCard key={i} {...ins} />)}
        </div>
      </div>

      {/* Recent transactions */}
      <div className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Последние транзакции</h2>
        <div className="rounded-xl border bg-card overflow-hidden">
          {loading ? (
            <div className="divide-y">
              {Array.from({ length: 5 }).map((_, i) => (
                <div key={i} className="px-5 py-3 flex items-center gap-3">
                  <div className="h-4 w-20 bg-muted rounded animate-pulse" />
                  <div className="flex-1 h-4 bg-muted rounded animate-pulse" />
                  <div className="h-4 w-16 bg-muted rounded animate-pulse" />
                </div>
              ))}
            </div>
          ) : txs.length === 0 ? (
            <div className="px-5 py-4 text-sm text-muted-foreground text-center">Нет транзакций</div>
          ) : (
            <div className="divide-y">
              {txs.map(tx => (
                <div key={tx.id} className="px-5 py-3 flex items-center gap-4">
                  <span className="text-xs text-muted-foreground w-16 shrink-0">{fmtDate(tx.occurred_at)}</span>
                  <span className="flex-1 text-sm text-foreground truncate">
                    {tx.payee || tx.comment || '—'}
                  </span>
                  <span className={cn(
                    'text-sm font-medium tabular-nums',
                    tx.amount > 0 ? 'text-emerald-500' : 'text-rose-500'
                  )}>
                    {tx.amount > 0 ? '+' : ''}{fmt(tx.amount, tx.currency)}
                  </span>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
