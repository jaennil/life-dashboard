import * as Sentry from '@sentry/react'
import { lazy, Suspense, type ReactNode } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { InstallPrompt } from '@/components/InstallPrompt'
import { GlobalDateFilters } from '@/components/GlobalDateFilters'
import { Sidebar } from '@/components/Sidebar'
import { AuthProvider, useAuth } from '@/lib/auth'
import { sentryEnabled } from '@/lib/sentry'
import { WidgetEditProvider } from '@/lib/widget-edit'

const Dashboard = lazy(() => import('@/pages/Dashboard').then(module => ({ default: module.Dashboard })))
const Finance = lazy(() => import('@/pages/Finance').then(module => ({ default: module.Finance })))
const Fitness = lazy(() => import('@/pages/Fitness').then(module => ({ default: module.Fitness })))
const Nutrition = lazy(() => import('@/pages/Nutrition').then(module => ({ default: module.Nutrition })))
const QuickInput = lazy(() => import('@/pages/QuickInput').then(module => ({ default: module.QuickInput })))
const Productivity = lazy(() => import('@/pages/Productivity').then(module => ({ default: module.Productivity })))
const AiChat = lazy(() => import('@/pages/AiChat').then(module => ({ default: module.AiChat })))
const Settings = lazy(() => import('@/pages/Settings').then(module => ({ default: module.Settings })))
const RawData = lazy(() => import('@/pages/RawData').then(module => ({ default: module.RawData })))
const Login = lazy(() => import('@/pages/Login').then(module => ({ default: module.Login })))
const Register = lazy(() => import('@/pages/Register').then(module => ({ default: module.Register })))

function PageFallback() {
  return (
    <div className="flex min-h-48 items-center justify-center">
      <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
    </div>
  )
}

function PageSuspense({ children }: { children: ReactNode }) {
  return (
    <Suspense fallback={<PageFallback />}>
      {children}
    </Suspense>
  )
}

function Layout({ children }: { children: ReactNode }) {
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

function ProtectedRoute({ children }: { children: ReactNode }) {
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
      <Route path="/login" element={user ? <Navigate to="/" replace /> : <PageSuspense><Login /></PageSuspense>} />
      <Route path="/register" element={user ? <Navigate to="/" replace /> : <PageSuspense><Register /></PageSuspense>} />
      <Route path="/*" element={
        <ProtectedRoute>
          <WidgetEditProvider>
            <Layout>
              <PageSuspense>
                <Routes>
                  <Route path="/" element={<Dashboard />} />
                  <Route path="/finance" element={<Finance />} />
                  <Route path="/fitness" element={<Fitness />} />
                  <Route path="/productivity" element={<Productivity />} />
                  <Route path="/nutrition" element={<Nutrition />} />
                  <Route path="/input" element={<QuickInput />} />
                  <Route path="/ai" element={<AiChat />} />
                  <Route path="/settings" element={<Settings />} />
                  <Route path="/raw-data" element={<RawData />} />
                </Routes>
              </PageSuspense>
            </Layout>
          </WidgetEditProvider>
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
