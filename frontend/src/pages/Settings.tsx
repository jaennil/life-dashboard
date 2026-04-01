import { useEffect, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { RefreshCw, CheckCircle, XCircle, AlertCircle, Power, ShieldCheck, ShieldOff, ExternalLink } from 'lucide-react'
import { cn } from '@/lib/utils'
import { api, type Integration } from '@/lib/api'
import { useAuth } from '@/lib/auth'

const OAUTH_INTEGRATIONS: Record<string, string> = {
  strava: '/api/v1/auth/strava',
  fatsecret: '/api/v1/auth/fatsecret',
}

const TOKEN_INTEGRATIONS: Record<string, { placeholder: string; help: string }> = {
  zenmoney: {
    placeholder: 'Bearer токен от ZenMoney',
    help: 'Получите токен на zerro.app/token — войдите через ZenMoney аккаунт и скопируйте токен. Токен действует 24 часа, потом нужно обновить.',
  },
}

const ICONS: Record<string, string> = {
  strava: '🚴',
  hevy: '🏋️',
  zenmoney: '💰',
  myfitnesspal: '🥗',
}

function fmtDate(iso: string | null) {
  if (!iso) return 'никогда'
  const d = new Date(iso)
  return d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' }) +
    ' в ' + d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })
}

function fmtCount(n: number, name: string) {
  if (n === 0) return 'нет данных'
  const labels: Record<string, [string, string, string]> = {
    strava:   ['активность', 'активности', 'активностей'],
    hevy:     ['тренировка', 'тренировки', 'тренировок'],
    zenmoney: ['транзакция', 'транзакции', 'транзакций'],
  }
  const l = labels[name] ?? ['запись', 'записи', 'записей']
  const mod = n % 100
  const mod10 = n % 10
  let label: string
  if (mod >= 11 && mod <= 19) label = l[2]
  else if (mod10 === 1) label = l[0]
  else if (mod10 >= 2 && mod10 <= 4) label = l[1]
  else label = l[2]
  return `${n.toLocaleString('ru-RU')} ${label}`
}

function IntegrationCard({ integration, onToggle, onSync, onRefresh }: {
  integration: Integration
  onToggle: (enabled: boolean) => void
  onSync: () => void
  onRefresh: () => void
}) {
  const [syncing, setSyncing] = useState(false)
  const [toggling, setToggling] = useState(false)
  const [tokenInput, setTokenInput] = useState('')
  const [showTokenForm, setShowTokenForm] = useState(false)
  const [savingToken, setSavingToken] = useState(false)

  async function handleSync() {
    setSyncing(true)
    try { await onSync() } finally { setSyncing(false) }
  }

  async function handleToggle() {
    setToggling(true)
    try { await onToggle(!integration.enabled) } finally { setToggling(false) }
  }

  async function handleSaveToken() {
    if (!tokenInput.trim()) return
    setSavingToken(true)
    try {
      await api.saveToken(integration.name, tokenInput.trim())
      setTokenInput('')
      setShowTokenForm(false)
      onRefresh()
    } catch { /* ignore */ } finally {
      setSavingToken(false)
    }
  }

  const isOAuth = !!OAUTH_INTEGRATIONS[integration.name]
  const tokenMeta = TOKEN_INTEGRATIONS[integration.name]
  const isConnected = integration.enabled && (!isOAuth || integration.record_count > 0)

  const statusIcon = !integration.configured
    ? <AlertCircle className="w-4 h-4 text-muted-foreground" />
    : isConnected
      ? <CheckCircle className="w-4 h-4 text-emerald-500" />
      : <XCircle className="w-4 h-4 text-muted-foreground" />

  const statusText = !integration.configured
    ? 'Не настроено'
    : isConnected ? 'Подключено' : 'Отключено'

  return (
    <div className={cn(
      'rounded-xl border bg-card p-5 flex flex-col gap-4 transition-opacity',
      !integration.configured && 'opacity-60'
    )}>
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-muted flex items-center justify-center text-xl shrink-0">
            {ICONS[integration.name] ?? '🔌'}
          </div>
          <div>
            <p className="text-sm font-semibold text-foreground">{integration.display_name}</p>
            <p className="text-xs text-muted-foreground mt-0.5">{integration.description}</p>
          </div>
        </div>

        {isOAuth ? (
          isConnected ? (
            <button
              onClick={handleToggle}
              disabled={toggling}
              className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-lg border border-emerald-500/30 text-emerald-500 hover:bg-emerald-500/10 transition-colors shrink-0"
            >
              <Power className="w-3 h-3" />
              {toggling ? '...' : 'Отключить'}
            </button>
          ) : (
            <a
              href={OAUTH_INTEGRATIONS[integration.name]}
              onClick={() => { if (!integration.enabled) handleToggle() }}
              className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-lg border border-primary/30 text-primary hover:bg-primary/10 transition-colors shrink-0"
            >
              <ExternalLink className="w-3 h-3" />
              Подключить
            </a>
          )
        ) : tokenMeta ? (
          isConnected ? (
            <button
              onClick={handleToggle}
              disabled={toggling}
              className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-lg border border-emerald-500/30 text-emerald-500 hover:bg-emerald-500/10 transition-colors shrink-0"
            >
              <Power className="w-3 h-3" />
              {toggling ? '...' : 'Отключить'}
            </button>
          ) : (
            <button
              onClick={() => setShowTokenForm(!showTokenForm)}
              className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-lg border border-primary/30 text-primary hover:bg-primary/10 transition-colors shrink-0"
            >
              <ExternalLink className="w-3 h-3" />
              Подключить
            </button>
          )
        ) : (
          <button
            onClick={handleToggle}
            disabled={!integration.configured || toggling}
            className={cn(
              'flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-lg border transition-colors shrink-0',
              integration.configured && integration.enabled
                ? 'border-emerald-500/30 text-emerald-500 hover:bg-emerald-500/10'
                : integration.configured
                  ? 'border-border text-muted-foreground hover:bg-muted/50'
                  : 'border-border text-muted-foreground cursor-not-allowed'
            )}
          >
            <Power className="w-3 h-3" />
            {toggling ? '...' : integration.enabled ? 'Отключить' : 'Включить'}
          </button>
        )}
      </div>

      {showTokenForm && tokenMeta && (
        <div className="flex flex-col gap-2 border-t pt-3">
          <p className="text-xs text-muted-foreground">{tokenMeta.help}</p>
          <div className="flex gap-2">
            <input
              type="text"
              value={tokenInput}
              onChange={e => setTokenInput(e.target.value)}
              placeholder={tokenMeta.placeholder}
              className="flex-1 rounded-lg border bg-background px-3 py-1.5 text-xs outline-none focus:ring-2 focus:ring-ring"
            />
            <button
              onClick={handleSaveToken}
              disabled={savingToken || !tokenInput.trim()}
              className="rounded-lg bg-primary text-primary-foreground px-3 py-1.5 text-xs font-medium hover:bg-primary/90 disabled:opacity-50 transition-colors"
            >
              {savingToken ? '...' : 'Сохранить'}
            </button>
          </div>
        </div>
      )}

      <div className="flex items-center justify-between gap-3 border-t pt-3">
        <div className="flex flex-col gap-1">
          <div className="flex items-center gap-1.5">
            {statusIcon}
            <span className="text-xs font-medium text-foreground">{statusText}</span>
          </div>
          {isConnected && (
            <div className="flex items-center gap-3 text-xs text-muted-foreground">
              <span>Синхр.: {fmtDate(integration.last_sync_at)}</span>
              <span>•</span>
              <span>{fmtCount(integration.record_count, integration.name)}</span>
            </div>
          )}
        </div>

        {isConnected && (
          <button
            onClick={handleSync}
            disabled={syncing}
            className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors px-3 py-1.5 rounded-lg hover:bg-muted/50"
          >
            <RefreshCw className={cn('w-3 h-3', syncing && 'animate-spin')} />
            {syncing ? 'Синхронизация...' : 'Синхронизировать'}
          </button>
        )}
      </div>
    </div>
  )
}

function TOTPSection() {
  const { user, refresh, logout } = useAuth()
  const [phase, setPhase] = useState<'idle' | 'setup' | 'disable'>('idle')
  const [qr, setQr] = useState('')
  const [secret, setSecret] = useState('')
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function startSetup() {
    setLoading(true)
    setError('')
    try {
      const data = await api.totpSetup()
      setQr(data.qr)
      setSecret(data.secret)
      setPhase('setup')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Ошибка')
    } finally {
      setLoading(false)
    }
  }

  async function confirmEnable() {
    setLoading(true)
    setError('')
    try {
      await api.totpEnable(code)
      await refresh()
      setPhase('idle')
      setCode('')
    } catch (err) {
      setError(err instanceof Error ? err.message.trim() : 'Неверный код')
    } finally {
      setLoading(false)
    }
  }

  async function confirmDisable() {
    setLoading(true)
    setError('')
    try {
      await api.totpDisable(code)
      await refresh()
      setPhase('idle')
      setCode('')
    } catch (err) {
      setError(err instanceof Error ? err.message.trim() : 'Неверный код')
    } finally {
      setLoading(false)
    }
  }

  if (phase === 'setup') {
    return (
      <div className="rounded-xl border bg-card p-5 flex flex-col gap-4">
        <p className="text-sm font-semibold text-foreground">Настройка двухфакторной аутентификации</p>
        <p className="text-xs text-muted-foreground">Отсканируй QR-код в Google Authenticator или Aegis, затем введи 6-значный код для подтверждения.</p>
        {qr && <img src={qr} alt="TOTP QR" className="w-40 h-40 rounded-lg border self-start" />}
        <div className="flex flex-col gap-1">
          <p className="text-xs text-muted-foreground">Или введи ключ вручную:</p>
          <code className="text-xs bg-muted px-2 py-1 rounded font-mono break-all">{secret}</code>
        </div>
        <div className="flex flex-col gap-1.5">
          <label className="text-xs font-medium text-foreground">Код подтверждения</label>
          <input
            type="text"
            inputMode="numeric"
            maxLength={6}
            value={code}
            onChange={e => setCode(e.target.value.replace(/\D/g, ''))}
            placeholder="000000"
            className="rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring w-32 tracking-widest text-center"
          />
        </div>
        {error && <p className="text-sm text-red-500">{error}</p>}
        <div className="flex gap-2">
          <button
            onClick={confirmEnable}
            disabled={loading || code.length !== 6}
            className="rounded-lg bg-primary text-primary-foreground px-4 py-1.5 text-sm font-medium hover:bg-primary/90 disabled:opacity-50 transition-colors"
          >
            {loading ? '...' : 'Включить 2FA'}
          </button>
          <button
            onClick={() => { setPhase('idle'); setCode('') }}
            className="rounded-lg border px-4 py-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            Отмена
          </button>
        </div>
      </div>
    )
  }

  if (phase === 'disable') {
    return (
      <div className="rounded-xl border bg-card p-5 flex flex-col gap-4">
        <p className="text-sm font-semibold text-foreground">Отключить двухфакторную аутентификацию</p>
        <div className="flex flex-col gap-1.5">
          <label className="text-xs font-medium text-foreground">Код из приложения</label>
          <input
            type="text"
            inputMode="numeric"
            maxLength={6}
            autoFocus
            value={code}
            onChange={e => setCode(e.target.value.replace(/\D/g, ''))}
            placeholder="000000"
            className="rounded-lg border bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring w-32 tracking-widest text-center"
          />
        </div>
        {error && <p className="text-sm text-red-500">{error}</p>}
        <div className="flex gap-2">
          <button
            onClick={confirmDisable}
            disabled={loading || code.length !== 6}
            className="rounded-lg bg-destructive text-destructive-foreground px-4 py-1.5 text-sm font-medium hover:bg-destructive/90 disabled:opacity-50 transition-colors"
          >
            {loading ? '...' : 'Отключить 2FA'}
          </button>
          <button
            onClick={() => { setPhase('idle'); setCode('') }}
            className="rounded-lg border px-4 py-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            Отмена
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="rounded-xl border bg-card p-5 flex flex-col gap-4">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-muted flex items-center justify-center shrink-0">
            {user?.totp_enabled
              ? <ShieldCheck className="w-5 h-5 text-emerald-500" />
              : <ShieldOff className="w-5 h-5 text-muted-foreground" />}
          </div>
          <div>
            <p className="text-sm font-semibold text-foreground">Двухфакторная аутентификация</p>
            <p className="text-xs text-muted-foreground mt-0.5">
              {user?.totp_enabled ? 'Включена (TOTP)' : 'Выключена'}
            </p>
          </div>
        </div>
        <button
          onClick={() => user?.totp_enabled ? setPhase('disable') : startSetup()}
          disabled={loading}
          className={cn(
            'flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-lg border transition-colors shrink-0',
            user?.totp_enabled
              ? 'border-red-500/30 text-red-500 hover:bg-red-500/10'
              : 'border-border text-muted-foreground hover:bg-muted/50'
          )}
        >
          {loading ? '...' : user?.totp_enabled ? 'Отключить' : 'Включить'}
        </button>
      </div>
      <div className="flex items-center justify-between border-t pt-3">
        <p className="text-xs text-muted-foreground">Аккаунт: <span className="text-foreground font-medium">{user?.username}</span></p>
        <button
          onClick={logout}
          className="text-xs text-muted-foreground hover:text-foreground transition-colors"
        >
          Выйти
        </button>
      </div>
    </div>
  )
}

export function Settings() {
  const [integrations, setIntegrations] = useState<Integration[]>([])
  const [loading, setLoading] = useState(true)
  const location = useLocation()

  function load() {
    return api.getIntegrations()
      .then(setIntegrations)
      .catch(console.error)
  }

  useEffect(() => {
    load().finally(() => setLoading(false))
  }, [location.key])

  async function handleToggle(name: string, enabled: boolean) {
    await api.toggleIntegration(name, enabled)
    await load()
  }

  async function handleSync(name: string) {
    await api.syncIntegration(name)
    await load()
  }

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-bold text-foreground">Настройки</h1>
        <p className="text-sm text-muted-foreground mt-1">Управление интеграциями и источниками данных</p>
      </div>

      <div>
        <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-3">Аккаунт</h2>
        <TOTPSection />
      </div>

      <div>
        <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-3">Интеграции</h2>
        {loading ? (
          <div className="flex flex-col gap-3">
            {[1, 2, 3].map(i => (
              <div key={i} className="rounded-xl border bg-card p-5 h-28 animate-pulse bg-muted/30" />
            ))}
          </div>
        ) : (
          <div className="flex flex-col gap-3">
            {integrations.map(integration => (
              <IntegrationCard
                key={integration.name}
                integration={integration}
                onToggle={enabled => handleToggle(integration.name, enabled)}
                onSync={() => handleSync(integration.name)}
                onRefresh={load}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
