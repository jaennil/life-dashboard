import * as Sentry from '@sentry/react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { InstallPrompt } from '@/components/InstallPrompt'
import { GlobalDateFilters } from '@/components/GlobalDateFilters'
import { Sidebar } from '@/components/Sidebar'
import { Dashboard } from '@/pages/Dashboard'
import { Finance } from '@/pages/Finance'
import { Fitness } from '@/pages/Fitness'
import { Nutrition } from '@/pages/Nutrition'
import { Productivity } from '@/pages/Productivity'
import { AiChat } from '@/pages/AiChat'
import { Settings } from '@/pages/Settings'
import { RawData } from '@/pages/RawData'
import { Login } from '@/pages/Login'
import { Register } from '@/pages/Register'
import { AuthProvider, useAuth } from '@/lib/auth'
import { sentryEnabled } from '@/lib/sentry'

function Layout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen bg-background">
      <Sidebar />
      <InstallPrompt />
      <main className="app-shell-main min-w-0 flex-1 lg:ml-56">
        <GlobalDateFilters />
        <div className="page-shell">
          {children}
        </div>
      </main>
    </div>
  )
}

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { user, loading } = useAuth()
  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <div className="w-6 h-6 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }
  if (!user) return <Navigate to="/login" replace />
  return <>{children}</>
}

function AppRoutes() {
  const { user, loading } = useAuth()

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <div className="w-6 h-6 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  return (
    <Routes>
      <Route path="/login" element={user ? <Navigate to="/" replace /> : <Login />} />
      <Route path="/register" element={user ? <Navigate to="/" replace /> : <Register />} />
      <Route path="/*" element={
        <ProtectedRoute>
          <Layout>
            <Routes>
              <Route path="/" element={<Dashboard />} />
              <Route path="/finance" element={<Finance />} />
              <Route path="/fitness" element={<Fitness />} />
              <Route path="/productivity" element={<Productivity />} />
              <Route path="/nutrition" element={<Nutrition />} />
              <Route path="/ai" element={<AiChat />} />
              <Route path="/settings" element={<Settings />} />
              <Route path="/raw-data" element={<RawData />} />
            </Routes>
          </Layout>
        </ProtectedRoute>
      } />
    </Routes>
  )
}

export default function App() {
  const app = (
    <BrowserRouter>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </BrowserRouter>
  )

  if (!sentryEnabled) {
    return app
  }

  return (
    <Sentry.ErrorBoundary
      fallback={(
        <div className="flex min-h-screen items-center justify-center bg-background px-6">
          <div className="max-w-md rounded-3xl border border-border/60 bg-card/90 p-8 text-center shadow-[0_24px_80px_rgba(0,0,0,0.35)]">
            <h1 className="text-2xl font-semibold text-foreground">Приложение столкнулось с ошибкой</h1>
            <p className="mt-3 text-sm leading-6 text-muted-foreground">
              Ошибка уже отправлена в мониторинг. Обнови страницу, если экран не восстановился сам.
            </p>
          </div>
        </div>
      )}
    >
      {app}
    </Sentry.ErrorBoundary>
  )
}
