import { createContext, useContext, useEffect, useState } from 'react'
import { api, type User } from '@/lib/api'

interface AuthContextValue {
  user: User | null
  loading: boolean
  login: (username: string, password: string, totpCode?: string) => Promise<{ needs_totp?: boolean }>
  logout: () => Promise<void>
  refresh: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)

  async function refresh() {
    try {
      const u = await api.me()
      setUser(u)
    } catch {
      setUser(null)
    }
  }

  useEffect(() => {
    refresh().finally(() => setLoading(false))
  }, [])

  async function login(username: string, password: string, totpCode?: string) {
    const result = await api.login(username, password, totpCode)
    if (result.needs_totp) return { needs_totp: true }
    const u = await api.me()
    setUser(u)
    return {}
  }

  async function logout() {
    await api.logout()
    setUser(null)
  }

  return (
    <AuthContext.Provider value={{ user, loading, login, logout, refresh }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used inside AuthProvider')
  return ctx
}
