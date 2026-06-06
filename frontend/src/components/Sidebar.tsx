import { useEffect, useState } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import {
  LayoutDashboard,
  Wallet,
  Dumbbell,
  CheckSquare,
  Salad,
  MessageSquare,
  Settings,
  Sun,
  Moon,
  Activity,
  User,
  Database,
  MoreHorizontal,
  X,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useTheme } from '@/hooks/useTheme'
import { useAuth } from '@/lib/auth'

const nav = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/finance', icon: Wallet, label: 'Finance' },
  { to: '/fitness', icon: Dumbbell, label: 'Fitness' },
  { to: '/productivity', icon: CheckSquare, label: 'Productivity' },
  { to: '/nutrition', icon: Salad, label: 'Nutrition' },
  { to: '/ai', icon: MessageSquare, label: 'AI Chat' },
  { to: '/raw-data', icon: Database, label: 'Raw Data' },
]

const mobilePrimary = nav.filter(item => ['/', '/finance', '/fitness', '/nutrition'].includes(item.to))
const mobileMore = [
  ...nav.filter(item => ['/productivity', '/ai', '/raw-data'].includes(item.to)),
  { to: '/settings', icon: Settings, label: 'Settings' },
]

export function Sidebar() {
  const [moreOpen, setMoreOpen] = useState(false)
  const location = useLocation()
  const { theme, toggle } = useTheme()
  const { user } = useAuth()

  useEffect(() => {
    if (!moreOpen) return
    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === 'Escape') setMoreOpen(false)
    }
    document.addEventListener('keydown', closeOnEscape)
    return () => document.removeEventListener('keydown', closeOnEscape)
  }, [moreOpen])

  return (
    <>
      <header
        className="fixed inset-x-0 top-0 z-40 flex items-center justify-between border-b bg-background/95 px-4 backdrop-blur lg:hidden"
        style={{ paddingTop: 'env(safe-area-inset-top)', height: 'calc(4rem + env(safe-area-inset-top))' }}
      >
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-primary shadow-sm shadow-primary/25">
            <Activity className="h-4 w-4 text-primary-foreground" />
          </div>
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold text-foreground">Life Dashboard</p>
            <p className="truncate text-xs text-muted-foreground">{user?.username ?? 'Mobile mode'}</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={toggle}
            className="flex h-10 w-10 items-center justify-center rounded-xl border bg-background text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
            aria-label={theme === 'dark' ? 'Включить светлую тему' : 'Включить тёмную тему'}
          >
            {theme === 'dark'
              ? <Sun className="h-4 w-4" />
              : <Moon className="h-4 w-4" />}
          </button>
        </div>
      </header>

      <aside className="fixed left-0 top-0 hidden h-screen w-56 shrink-0 flex-col border-r bg-background/95 backdrop-blur lg:flex">
        <div className="border-b px-4 py-4">
          <div className="flex items-center gap-3 rounded-2xl border bg-background/70 px-3 py-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-2xl bg-primary">
              <Activity className="h-4 w-4 text-primary-foreground" />
            </div>
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold text-foreground">Life Dashboard</p>
              <p className="truncate text-[11px] text-muted-foreground">personal operating system</p>
            </div>
          </div>
        </div>

        <div className="px-4 pt-4">
          <p className="text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">
            Навигация
          </p>
        </div>

        <nav className="flex flex-1 flex-col gap-1 p-3 pt-3">
          {nav.map(({ to, icon: Icon, label }) => (
            <NavLink
              key={to}
              to={to}
              end={to === '/'}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-3 rounded-2xl px-3.5 py-3 text-sm font-medium transition-all',
                  isActive
                    ? 'bg-primary/10 text-primary shadow-[inset_0_0_0_1px_rgba(59,130,246,0.12)]'
                    : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
                )
              }
            >
              <span className={cn(
                'flex h-9 w-9 shrink-0 items-center justify-center rounded-xl',
                'bg-background/80'
              )}>
                <Icon className="h-4 w-4 shrink-0" />
              </span>
              <span>{label}</span>
            </NavLink>
          ))}
        </nav>

        <div className="border-t p-3">
          <div className="rounded-2xl border bg-background/70 p-3">
            <div className="mb-3 flex items-center gap-3">
              <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl bg-muted">
                <User className="h-4 w-4 text-muted-foreground" />
              </div>
              <div className="min-w-0">
                <p className="truncate text-sm font-medium text-foreground">{user?.username ?? 'Аккаунт'}</p>
                <p className="truncate text-[11px] text-muted-foreground">Личный workspace</p>
              </div>
            </div>
            <div className="flex flex-col gap-1">
              <button
                onClick={toggle}
                className="flex items-center gap-3 rounded-xl px-3 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
              >
                {theme === 'dark'
                  ? <Sun className="h-4 w-4 shrink-0" />
                  : <Moon className="h-4 w-4 shrink-0" />}
                <span>{theme === 'dark' ? 'Light mode' : 'Dark mode'}</span>
              </button>
              <NavLink
                to="/settings"
                className={({ isActive }) =>
                  cn(
                    'flex items-center gap-3 rounded-xl px-3 py-2 text-sm font-medium transition-colors',
                    isActive
                      ? 'bg-accent text-accent-foreground'
                      : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
                  )
                }
              >
                <Settings className="h-4 w-4 shrink-0" />
                <span>Settings</span>
              </NavLink>
            </div>
          </div>
        </div>
      </aside>

      {moreOpen ? (
        <div className="fixed inset-0 z-50 bg-black/55 lg:hidden" onClick={() => setMoreOpen(false)}>
          <div className="absolute inset-x-3 bottom-[calc(5.25rem+env(safe-area-inset-bottom))] rounded-lg border bg-card p-3 shadow-2xl" onClick={event => event.stopPropagation()}>
            <div className="mb-2 flex items-center justify-between px-2 py-1">
              <p className="text-sm font-medium text-foreground">More</p>
              <button type="button" onClick={() => setMoreOpen(false)} className="flex h-9 w-9 items-center justify-center rounded-lg text-muted-foreground hover:bg-accent hover:text-foreground" aria-label="Закрыть меню">
                <X className="h-4 w-4" />
              </button>
            </div>
            <div className="grid grid-cols-2 gap-2">
              {mobileMore.map(({ to, icon: Icon, label }) => (
                <NavLink key={to} to={to} onClick={() => setMoreOpen(false)} className={({ isActive }) => cn('flex items-center gap-3 rounded-lg border px-3 py-3 text-sm', isActive ? 'border-primary/30 bg-primary/10 text-primary' : 'bg-background text-muted-foreground')}>
                  <Icon className="h-4 w-4" />
                  {label}
                </NavLink>
              ))}
              <button type="button" onClick={toggle} className="flex items-center gap-3 rounded-lg border bg-background px-3 py-3 text-left text-sm text-muted-foreground">
                {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
                {theme === 'dark' ? 'Light mode' : 'Dark mode'}
              </button>
            </div>
          </div>
        </div>
      ) : null}

      <nav
        className="fixed inset-x-0 bottom-0 z-40 border-t bg-background/95 px-2 pt-2 backdrop-blur lg:hidden"
        style={{ paddingBottom: 'max(0.75rem, env(safe-area-inset-bottom))' }}
      >
        <div className="flex gap-1 overflow-x-auto">
          {mobilePrimary.map(({ to, icon: Icon, label }) => (
            <NavLink
              key={to}
              to={to}
              end={to === '/'}
              className={({ isActive }) =>
                cn(
                  'flex min-w-[72px] flex-1 flex-col items-center gap-1 rounded-2xl px-2 py-2 text-[11px] font-medium transition-colors',
                  isActive
                    ? 'bg-primary/10 text-primary'
                    : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
                )
              }
            >
              <Icon className="h-4 w-4 shrink-0" />
              <span className="truncate">{label}</span>
            </NavLink>
          ))}
          <button
            type="button"
            onClick={() => setMoreOpen(current => !current)}
            aria-expanded={moreOpen}
            className={cn(
              'flex min-w-[72px] flex-1 flex-col items-center gap-1 rounded-lg px-2 py-2 text-[11px] font-medium transition-colors',
              mobileMore.some(item => location.pathname === item.to) || moreOpen ? 'bg-primary/10 text-primary' : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
            )}
          >
            <MoreHorizontal className="h-4 w-4" />
            <span>More</span>
          </button>
        </div>
      </nav>
    </>
  )
}
