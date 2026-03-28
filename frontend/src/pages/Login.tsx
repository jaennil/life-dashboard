import { useState, type FormEvent } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { useAuth } from '@/lib/auth'

export function Login() {
  const { login } = useAuth()
  const navigate = useNavigate()

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [totpCode, setTotpCode] = useState('')
  const [needsTotp, setNeedsTotp] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const result = await login(username, password, needsTotp ? totpCode : undefined)
      if (result.needs_totp) {
        setNeedsTotp(true)
      } else {
        navigate('/', { replace: true })
      }
    } catch (err) {
      setError(err instanceof Error ? err.message.trim() : 'Ошибка входа')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-background p-4">
      <div className="w-full max-w-sm">
        <div className="rounded-2xl border bg-card p-8 flex flex-col gap-6">
          <div>
            <h1 className="text-2xl font-bold text-foreground">Войти</h1>
            <p className="text-sm text-muted-foreground mt-1">Life Dashboard</p>
          </div>

          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            {!needsTotp ? (
              <>
                <div className="flex flex-col gap-1.5">
                  <label className="text-sm font-medium text-foreground">Логин</label>
                  <input
                    type="text"
                    autoFocus
                    value={username}
                    onChange={e => setUsername(e.target.value)}
                    required
                    className="rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
                    placeholder="username"
                  />
                </div>
                <div className="flex flex-col gap-1.5">
                  <label className="text-sm font-medium text-foreground">Пароль</label>
                  <input
                    type="password"
                    value={password}
                    onChange={e => setPassword(e.target.value)}
                    required
                    className="rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
                    placeholder="••••••"
                  />
                </div>
              </>
            ) : (
              <div className="flex flex-col gap-1.5">
                <label className="text-sm font-medium text-foreground">Код из приложения</label>
                <p className="text-xs text-muted-foreground">Введите 6-значный код из Google Authenticator</p>
                <input
                  type="text"
                  autoFocus
                  inputMode="numeric"
                  maxLength={6}
                  value={totpCode}
                  onChange={e => setTotpCode(e.target.value.replace(/\D/g, ''))}
                  required
                  className="rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring tracking-widest text-center text-lg"
                  placeholder="000000"
                />
              </div>
            )}

            {error && <p className="text-sm text-red-500">{error}</p>}

            <button
              type="submit"
              disabled={loading}
              className="rounded-lg bg-primary text-primary-foreground px-4 py-2 text-sm font-medium hover:bg-primary/90 disabled:opacity-50 transition-colors"
            >
              {loading ? 'Загрузка...' : needsTotp ? 'Подтвердить' : 'Войти'}
            </button>

            {needsTotp && (
              <button
                type="button"
                onClick={() => { setNeedsTotp(false); setTotpCode('') }}
                className="text-sm text-muted-foreground hover:text-foreground transition-colors"
              >
                ← Назад
              </button>
            )}
          </form>

          <p className="text-sm text-center text-muted-foreground">
            Нет аккаунта?{' '}
            <Link to="/register" className="text-primary hover:underline">
              Зарегистрироваться
            </Link>
          </p>
        </div>
      </div>
    </div>
  )
}
