import { NavLink } from 'react-router-dom'
import {
  LayoutDashboard,
  Wallet,
  Dumbbell,
  Salad,
  MessageSquare,
  Settings,
  Sun,
  Moon,
  Activity,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useTheme } from '@/hooks/useTheme'

const nav = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/finance', icon: Wallet, label: 'Finance' },
  { to: '/fitness', icon: Dumbbell, label: 'Fitness' },
  { to: '/nutrition', icon: Salad, label: 'Nutrition' },
  { to: '/ai', icon: MessageSquare, label: 'AI Chat' },
]

export function Sidebar() {
  const { theme, toggle } = useTheme()

  return (
    <aside className="flex flex-col w-16 lg:w-56 h-screen bg-card border-r shrink-0 fixed top-0 left-0">
      {/* Logo */}
      <div className="flex items-center gap-3 px-4 h-16 border-b">
        <div className="flex items-center justify-center w-8 h-8 rounded-lg bg-primary">
          <Activity className="w-4 h-4 text-primary-foreground" />
        </div>
        <span className="hidden lg:block font-semibold text-sm text-foreground">Life Dashboard</span>
      </div>

      {/* Nav */}
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
            <span className="hidden lg:block">{label}</span>
          </NavLink>
        ))}
      </nav>

      {/* Bottom */}
      <div className="flex flex-col gap-1 p-2 border-t">
        <button
          onClick={toggle}
          className="flex items-center gap-3 px-3 py-2 rounded-md text-sm font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground transition-colors"
        >
          {theme === 'dark'
            ? <Sun className="w-4 h-4 shrink-0" />
            : <Moon className="w-4 h-4 shrink-0" />}
          <span className="hidden lg:block">{theme === 'dark' ? 'Light mode' : 'Dark mode'}</span>
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
          <span className="hidden lg:block">Settings</span>
        </NavLink>
      </div>
    </aside>
  )
}
