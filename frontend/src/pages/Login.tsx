import { useState, type FormEvent } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { AuthLayout } from '@/components/AuthLayout'
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
    <AuthLayout
      title={needsTotp ? 'Подтверждение входа' : 'Войти'}
      description={needsTotp
        ? 'Подтверди вход кодом из приложения-аутентификатора.'
        : 'Войди в свой workspace, чтобы работать с данными, интеграциями и AI-анализом.'}
      footer={(
        <p className="text-center">
          Нет аккаунта?{' '}
          <Link to="/register" className="font-medium text-primary hover:underline">
            Зарегистрироваться
          </Link>
        </p>
      )}
    >
      <form onSubmit={handleSubmit} className="flex flex-col gap-5">
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
                className="rounded-xl border bg-background px-3.5 py-3 text-sm outline-none transition-colors focus:border-primary focus:ring-2 focus:ring-primary/30"
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
                className="rounded-xl border bg-background px-3.5 py-3 text-sm outline-none transition-colors focus:border-primary focus:ring-2 focus:ring-primary/30"
                placeholder="••••••"
              />
            </div>
          </>
        ) : (
          <div className="flex flex-col gap-1.5">
            <label className="text-sm font-medium text-foreground">Код из приложения</label>
            <p className="text-xs text-muted-foreground">Введите 6-значный код из Google Authenticator или Aegis</p>
            <input
              type="text"
              autoFocus
              inputMode="numeric"
              maxLength={6}
              value={totpCode}
              onChange={e => setTotpCode(e.target.value.replace(/\D/g, ''))}
              required
              className="rounded-xl border bg-background px-3 py-3 text-center text-lg tracking-[0.4em] outline-none transition-colors focus:border-primary focus:ring-2 focus:ring-primary/30"
              placeholder="000000"
            />
          </div>
        )}

        {error && <p className="text-sm text-red-500">{error}</p>}

        <button
          type="submit"
          disabled={loading}
          className="rounded-xl bg-primary px-4 py-3 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
        >
          {loading ? 'Загрузка...' : needsTotp ? 'Подтвердить' : 'Войти'}
        </button>

        {needsTotp && (
          <button
            type="button"
            onClick={() => { setNeedsTotp(false); setTotpCode('') }}
            className="text-sm text-muted-foreground transition-colors hover:text-foreground"
          >
            ← Назад
          </button>
        )}
      </form>
    </AuthLayout>
  )
}
