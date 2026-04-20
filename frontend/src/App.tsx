import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { InstallPrompt } from '@/components/InstallPrompt'
import { Sidebar } from '@/components/Sidebar'
import { Dashboard } from '@/pages/Dashboard'
import { Finance } from '@/pages/Finance'
import { Fitness } from '@/pages/Fitness'
import { Nutrition } from '@/pages/Nutrition'
import { Productivity } from '@/pages/Productivity'
import { AiChat } from '@/pages/AiChat'
import { Settings } from '@/pages/Settings'
import { Login } from '@/pages/Login'
import { Register } from '@/pages/Register'
import { AuthProvider, useAuth } from '@/lib/auth'

function Layout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen bg-background">
      <Sidebar />
      <InstallPrompt />
      <main className="app-shell-main flex-1 lg:ml-56">
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
            </Routes>
          </Layout>
        </ProtectedRoute>
      } />
    </Routes>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </BrowserRouter>
  )
}
