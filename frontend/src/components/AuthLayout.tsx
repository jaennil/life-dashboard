import type { ReactNode } from 'react'
import { Activity, MessageSquare, ShieldCheck, Wallet } from 'lucide-react'
import { Link } from 'react-router-dom'

const AUTH_FEATURES = [
  { icon: Wallet, title: 'Финансы и баланс', text: 'Один экран для счетов, трат, категорий и cashflow.' },
  { icon: Activity, title: 'Тренировки и прогресс', text: 'Strava, Hevy и checkup по активности без ручной сводки.' },
  { icon: MessageSquare, title: 'AI поверх твоих данных', text: 'Вопросы по финансам, тренировкам, задачам и питанию в одном чате.' },
]

export function AuthLayout({
  title,
  description,
  children,
  footer,
}: {
  title: string
  description: string
  children: ReactNode
  footer: ReactNode
}) {
  return (
    <div className="relative min-h-screen overflow-hidden bg-background px-4 py-6 sm:px-6 lg:px-8">
      <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_left,rgba(59,130,246,0.16),transparent_34%),radial-gradient(circle_at_bottom_right,rgba(16,185,129,0.12),transparent_28%)]" />
      <div className="relative mx-auto grid min-h-[calc(100vh-3rem)] w-full max-w-6xl overflow-hidden rounded-[28px] border bg-card/92 shadow-[0_32px_80px_rgba(2,6,23,0.24)] backdrop-blur xl:grid-cols-[1.08fr_0.92fr]">
        <aside className="hidden border-r bg-[linear-gradient(180deg,rgba(59,130,246,0.14),transparent_34%),linear-gradient(180deg,rgba(255,255,255,0.04),transparent)] p-10 xl:flex xl:flex-col xl:justify-between">
          <div className="space-y-8">
            <div className="flex items-center gap-3">
              <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-primary shadow-[0_10px_30px_rgba(59,130,246,0.35)]">
                <Activity className="h-5 w-5 text-primary-foreground" />
              </div>
              <div>
                <p className="text-sm font-semibold text-foreground">Life Dashboard</p>
                <p className="text-xs text-muted-foreground">Персональная панель данных</p>
              </div>
            </div>

            <div className="space-y-3">
              <p className="text-[11px] font-semibold uppercase tracking-[0.24em] text-primary/80">
                Personal operating system
              </p>
              <h2 className="max-w-md text-4xl font-semibold leading-tight tracking-tight text-foreground">
                Данные, интеграции и AI в одном рабочем пространстве.
              </h2>
              <p className="max-w-lg text-sm leading-6 text-muted-foreground">
                Life Dashboard собирает финансы, тренировки, задачи и питание в один контекст,
                чтобы анализ был быстрым и без ручного свода по куче приложений.
              </p>
            </div>

            <div className="grid gap-3">
              {AUTH_FEATURES.map(item => (
                <div key={item.title} className="rounded-2xl border bg-background/55 p-4">
                  <div className="mb-3 flex h-10 w-10 items-center justify-center rounded-xl bg-primary/10 text-primary">
                    <item.icon className="h-4 w-4" />
                  </div>
                  <p className="text-sm font-medium text-foreground">{item.title}</p>
                  <p className="mt-1 text-xs leading-5 text-muted-foreground">{item.text}</p>
                </div>
              ))}
            </div>
          </div>

          <div className="rounded-2xl border bg-background/60 p-4">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-emerald-500/10 text-emerald-300">
                <ShieldCheck className="h-4 w-4" />
              </div>
              <div>
                <p className="text-sm font-medium text-foreground">Локальный доступ</p>
                <p className="text-xs text-muted-foreground">Твои данные доступны только после авторизации.</p>
              </div>
            </div>
          </div>
        </aside>

        <main className="flex items-center justify-center px-5 py-8 sm:px-8 xl:px-12">
          <div className="w-full max-w-md space-y-8">
            <div className="space-y-3 xl:hidden">
              <Link to="/" className="inline-flex items-center gap-2 text-sm font-medium text-foreground">
                <span className="flex h-10 w-10 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
                  <Activity className="h-4 w-4" />
                </span>
                Life Dashboard
              </Link>
            </div>

            <div className="space-y-2">
              <p className="text-[11px] font-semibold uppercase tracking-[0.24em] text-primary/80">
                Secure access
              </p>
              <h1 className="text-3xl font-semibold tracking-tight text-foreground">{title}</h1>
              <p className="text-sm leading-6 text-muted-foreground">{description}</p>
            </div>

            <div className="rounded-[24px] border bg-background/70 p-6 shadow-[0_18px_40px_rgba(2,6,23,0.16)] sm:p-7">
              {children}
            </div>

            <div className="text-sm text-muted-foreground">
              {footer}
            </div>
          </div>
        </main>
      </div>
    </div>
  )
}
