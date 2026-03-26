import { Activity, Wallet, Dumbbell, TrendingUp, TrendingDown, Zap } from 'lucide-react'
import { cn } from '@/lib/utils'

function StatCard({
  title,
  value,
  sub,
  icon: Icon,
  trend,
  color,
}: {
  title: string
  value: string
  sub: string
  icon: React.ElementType
  trend?: 'up' | 'down'
  color: string
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
        <div className="text-2xl font-bold text-foreground">{value}</div>
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

export function Dashboard() {
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
          value="15 316 ₽"
          sub="по всем счетам"
          icon={Wallet}
          color="bg-blue-500"
        />
        <StatCard
          title="Расходы за месяц"
          value="—"
          sub="нет данных"
          icon={TrendingDown}
          color="bg-rose-500"
        />
        <StatCard
          title="Активности"
          value="—"
          sub="за последние 7 дней"
          icon={Activity}
          color="bg-orange-500"
        />
        <StatCard
          title="Тренировки"
          value="—"
          sub="за последние 7 дней"
          icon={Dumbbell}
          color="bg-violet-500"
        />
      </div>

      {/* AI Insights */}
      <div className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">AI Инсайты</h2>
        <div className="flex flex-col gap-2">
          <InsightCard
            type="info"
            text="Подключи больше источников данных чтобы получать персональные инсайты."
          />
        </div>
      </div>

      {/* Recent transactions */}
      <div className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Последние транзакции</h2>
        <div className="rounded-xl border bg-card overflow-hidden">
          <div className="px-5 py-4 text-sm text-muted-foreground text-center">
            Данные загружаются...
          </div>
        </div>
      </div>
    </div>
  )
}
