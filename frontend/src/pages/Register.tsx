import { useState, type FormEvent } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { AuthLayout } from '@/components/AuthLayout'
import { useAuth } from '@/lib/auth'
import { api } from '@/lib/api'

export function Register() {
  const { refresh } = useAuth()
  const navigate = useNavigate()

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await api.register(username, password)
      await api.login(username, password)
      await refresh()
      navigate('/', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message.trim() : 'Ошибка регистрации')
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthLayout
      title="Создать аккаунт"
      description="Новый workspace для интеграций, личных данных и AI-анализов. После регистрации вход выполнится автоматически."
      footer={(
        <p className="text-center">
          Уже есть аккаунт?{' '}
          <Link to="/login" className="font-medium text-primary hover:underline">
            Войти
          </Link>
        </p>
      )}
    >
      <form onSubmit={handleSubmit} className="flex flex-col gap-5">
        <div className="flex flex-col gap-1.5">
          <label className="text-sm font-medium text-foreground">Логин</label>
          <input
            type="text"
            autoFocus
            value={username}
            onChange={e => setUsername(e.target.value)}
            required
            minLength={3}
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
            minLength={6}
            className="rounded-xl border bg-background px-3.5 py-3 text-sm outline-none transition-colors focus:border-primary focus:ring-2 focus:ring-primary/30"
            placeholder="мин. 6 символов"
          />
        </div>

        {error && <p className="text-sm text-red-500">{error}</p>}

        <button
          type="submit"
          disabled={loading}
          className="rounded-xl bg-primary px-4 py-3 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
        >
          {loading ? 'Загрузка...' : 'Создать аккаунт'}
        </button>
      </form>
    </AuthLayout>
  )
}
