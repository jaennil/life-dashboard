import { NavLink } from 'react-router-dom'
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
]

export function Sidebar() {
  const { theme, toggle } = useTheme()
  const { user } = useAuth()

  return (
    <>
      <header
        className="fixed inset-x-0 top-0 z-40 flex items-center justify-between border-b bg-card/95 px-4 backdrop-blur lg:hidden"
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
          <NavLink
            to="/settings"
            className={({ isActive }) =>
              cn(
                'flex h-10 w-10 items-center justify-center rounded-xl border bg-background text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground',
                isActive && 'border-primary/30 bg-primary/10 text-primary'
              )
            }
            aria-label="Открыть настройки"
          >
            <Settings className="h-4 w-4" />
          </NavLink>
        </div>
      </header>

      <aside className="hidden h-screen w-56 shrink-0 flex-col border-r bg-card fixed top-0 left-0 lg:flex">
        <div className="flex items-center gap-3 px-4 h-16 border-b">
          <div className="flex items-center justify-center w-8 h-8 rounded-lg bg-primary">
            <Activity className="w-4 h-4 text-primary-foreground" />
          </div>
          <span className="font-semibold text-sm text-foreground">Life Dashboard</span>
        </div>

        <nav className="flex flex-col gap-1 p-2 flex-1">
          {nav.map(({ to, icon: Icon, label }) => (
            <NavLink
              key={to}
              to={to}
              end={to === '/'}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-3 px-3 py-2 rounded-md text-sm font-medium transition-colors',
                  isActive
                    ? 'bg-accent text-accent-foreground'
                    : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
                )
              }
            >
              <Icon className="w-4 h-4 shrink-0" />
              <span>{label}</span>
            </NavLink>
          ))}
        </nav>

        <div className="flex flex-col gap-1 p-2 border-t">
          <button
            onClick={toggle}
            className="flex items-center gap-3 px-3 py-2 rounded-md text-sm font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground transition-colors"
          >
            {theme === 'dark'
              ? <Sun className="w-4 h-4 shrink-0" />
              : <Moon className="w-4 h-4 shrink-0" />}
            <span>{theme === 'dark' ? 'Light mode' : 'Dark mode'}</span>
          </button>
          <NavLink
            to="/settings"
            className={({ isActive }) =>
              cn(
                'flex items-center gap-3 px-3 py-2 rounded-md text-sm font-medium transition-colors',
                isActive
                  ? 'bg-accent text-accent-foreground'
                  : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
              )
            }
          >
            <Settings className="w-4 h-4 shrink-0" />
            <span>Settings</span>
          </NavLink>
          <NavLink
            to="/settings"
            className="flex items-center gap-3 px-3 py-2 rounded-md text-sm transition-colors text-muted-foreground hover:bg-accent hover:text-accent-foreground"
          >
            <div className="w-4 h-4 shrink-0 rounded-full bg-muted flex items-center justify-center">
              <User className="w-3 h-3" />
            </div>
            <span className="text-xs truncate">{user?.username}</span>
          </NavLink>
        </div>
      </aside>

      <nav
        className="fixed inset-x-0 bottom-0 z-40 border-t bg-card/95 px-2 pt-2 backdrop-blur lg:hidden"
        style={{ paddingBottom: 'max(0.75rem, env(safe-area-inset-bottom))' }}
      >
        <div className="grid grid-cols-6 gap-1">
          {nav.map(({ to, icon: Icon, label }) => (
            <NavLink
              key={to}
              to={to}
              end={to === '/'}
              className={({ isActive }) =>
                cn(
                  'flex min-w-0 flex-col items-center gap-1 rounded-2xl px-2 py-2 text-[11px] font-medium transition-colors',
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
        </div>
      </nav>
    </>
  )
}
