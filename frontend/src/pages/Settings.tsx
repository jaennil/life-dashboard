import { startTransition, useEffect, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { RefreshCw, CheckCircle, XCircle, AlertCircle, Power, ShieldCheck, ShieldOff, ExternalLink } from 'lucide-react'
import { cn } from '@/lib/utils'
import { api, type Integration } from '@/lib/api'
import { useAuth } from '@/lib/auth'

const OAUTH_INTEGRATIONS: Record<string, string> = {
  strava: '/api/v1/auth/strava',
  fatsecret: '/api/v1/auth/fatsecret',
  google_calendar: '/api/v1/auth/google',
  notion: '/api/v1/auth/notion',
}

const MANAGED_CONNECTION_INTEGRATIONS = new Set(['myfitnesspal'])

const TOKEN_INTEGRATIONS: Record<string, { placeholder: string; help: React.ReactNode; extraField?: { key: string; placeholder: string } }> = {
  zenmoney: {
    placeholder: 'Bearer токен от ZenMoney',
    help: <>Получите токен на <a href="https://zerro.app/token" target="_blank" rel="noopener noreferrer" className="text-primary hover:underline">zerro.app/token</a> — войдите через ZenMoney аккаунт и скопируйте токен. Токен действует 24 часа, потом нужно обновить.</>,
  },
  hevy: {
    placeholder: 'API ключ от Hevy',
    help: <>Откройте Hevy → Settings → API, скопируйте API Key.</>,
  },
  notion: {
    placeholder: 'Notion Integration Token (ntn_...)',
    help: <>1. Создайте <a href="https://www.notion.so/profile/integrations" target="_blank" rel="noopener noreferrer" className="text-primary hover:underline">Internal Integration</a> в Notion. 2. Скопируйте токен. 3. Откройте базу данных дневника в Notion → «...» → Connections → добавьте интеграцию. 4. Скопируйте ID базы данных из URL (32 символа после последнего /).</>,
    extraField: { key: 'database_id', placeholder: 'Database ID (32 символа из URL)' },
  },
}

const ICONS: Record<string, string> = {
  strava: '🚴',
  hevy: '🏋️',
  zenmoney: '💰',
  myfitnesspal: '🥗',
  google_calendar: '📅',
  notion: '📓',
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

function IntegrationCard({ integration, onToggle, onSync, onRefresh, syncPending = false }: {
  integration: Integration
  onToggle: (enabled: boolean) => void
  onSync: () => void
  onRefresh: () => void
  syncPending?: boolean
}) {
  const [syncing, setSyncing] = useState(false)
  const [toggling, setToggling] = useState(false)
  const [tokenInput, setTokenInput] = useState('')
  const [extraInput, setExtraInput] = useState('')
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
    const extra = tokenMeta?.extraField ? extraInput.trim() : undefined
    if (tokenMeta?.extraField && !extra) return
    setSavingToken(true)
    try {
      const result = await api.saveToken(integration.name, tokenInput.trim(), extra ? { [tokenMeta!.extraField!.key]: extra } : undefined)
      setTokenInput('')
      setExtraInput('')
      setShowTokenForm(false)
      onRefresh()
      if (result.sync_started_at) {
        window.dispatchEvent(new CustomEvent('integration-sync-started', {
          detail: {
            name: result.source ?? integration.name,
            startedAt: result.sync_started_at,
          },
        }))
      }
    } catch { /* ignore */ } finally {
      setSavingToken(false)
    }
  }

  const isOAuth = !!OAUTH_INTEGRATIONS[integration.name]
  const tokenMeta = TOKEN_INTEGRATIONS[integration.name]
  const isDual = isOAuth && !!tokenMeta
  const requiresCredentials = isOAuth || !!tokenMeta || MANAGED_CONNECTION_INTEGRATIONS.has(integration.name)
  const canSelfConnect = isOAuth || !!tokenMeta
  const hasSyncData = integration.record_count > 0 || !!integration.last_sync_at
  const hasConnectionData = integration.has_credentials || hasSyncData
  const isConnected = requiresCredentials ? hasConnectionData : integration.enabled
  const isActive = integration.enabled && (requiresCredentials ? hasConnectionData : integration.configured)
  const showSyncPending = syncPending && integration.enabled

  const statusIcon = !integration.configured
    ? <AlertCircle className="w-4 h-4 text-muted-foreground" />
    : showSyncPending
      ? <RefreshCw className="w-4 h-4 text-primary animate-spin" />
      : isActive
      ? <CheckCircle className="w-4 h-4 text-emerald-500" />
      : <XCircle className="w-4 h-4 text-muted-foreground" />

  const statusText = !integration.configured
    ? 'Не настроено'
    : showSyncPending
      ? 'Синхронизация...'
    : requiresCredentials
      ? isConnected
        ? integration.enabled ? 'Подключено' : 'Отключено'
        : 'Не подключено'
      : integration.enabled
        ? 'Включено'
        : 'Отключено'

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

        {isOAuth && !isDual ? (
          isConnected ? (
            <button
              onClick={handleToggle}
              disabled={toggling}
              className={cn(
                'flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-lg border transition-colors shrink-0',
                integration.enabled
                  ? 'border-emerald-500/30 text-emerald-500 hover:bg-emerald-500/10'
                  : 'border-border text-muted-foreground hover:bg-muted/50'
              )}
            >
              <Power className="w-3 h-3" />
              {toggling ? '...' : integration.enabled ? 'Отключить' : 'Включить'}
            </button>
          ) : (
            <a
              href={OAUTH_INTEGRATIONS[integration.name]}
              className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-lg border border-primary/30 text-primary hover:bg-primary/10 transition-colors shrink-0"
            >
              <ExternalLink className="w-3 h-3" />
              Подключить
            </a>
          )
        ) : isDual ? (
          isConnected ? (
            <button
              onClick={handleToggle}
              disabled={toggling}
              className={cn(
                'flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-lg border transition-colors shrink-0',
                integration.enabled
                  ? 'border-emerald-500/30 text-emerald-500 hover:bg-emerald-500/10'
                  : 'border-border text-muted-foreground hover:bg-muted/50'
              )}
            >
              <Power className="w-3 h-3" />
              {toggling ? '...' : integration.enabled ? 'Отключить' : 'Включить'}
            </button>
          ) : (
            <div className="flex items-center gap-2 shrink-0">
              <a
                href={OAUTH_INTEGRATIONS[integration.name]}
                className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-lg border border-primary/30 text-primary hover:bg-primary/10 transition-colors"
              >
                <ExternalLink className="w-3 h-3" />
                OAuth
              </a>
              <button
                onClick={() => setShowTokenForm(!showTokenForm)}
                className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-lg border border-border text-muted-foreground hover:bg-muted/50 transition-colors"
              >
                Токен
              </button>
            </div>
          )
        ) : tokenMeta ? (
          isConnected ? (
            <button
              onClick={handleToggle}
              disabled={toggling}
              className={cn(
                'flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-lg border transition-colors shrink-0',
                integration.enabled
                  ? 'border-emerald-500/30 text-emerald-500 hover:bg-emerald-500/10'
                  : 'border-border text-muted-foreground hover:bg-muted/50'
              )}
            >
              <Power className="w-3 h-3" />
              {toggling ? '...' : integration.enabled ? 'Отключить' : 'Включить'}
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
        ) : requiresCredentials && !canSelfConnect ? (
          isConnected ? (
            <button
              onClick={handleToggle}
              disabled={toggling}
              className={cn(
                'flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-lg border transition-colors shrink-0',
                integration.enabled
                  ? 'border-emerald-500/30 text-emerald-500 hover:bg-emerald-500/10'
                  : 'border-border text-muted-foreground hover:bg-muted/50'
              )}
            >
              <Power className="w-3 h-3" />
              {toggling ? '...' : integration.enabled ? 'Отключить' : 'Включить'}
            </button>
          ) : (
            <button
              disabled
              className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-lg border border-border text-muted-foreground/60 cursor-not-allowed shrink-0"
            >
              <Power className="w-3 h-3" />
              Требуется настройка
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
          <div className="flex flex-col gap-2">
            <input
              type="text"
              value={tokenInput}
              onChange={e => setTokenInput(e.target.value)}
              placeholder={tokenMeta.placeholder}
              className="rounded-lg border bg-background px-3 py-1.5 text-xs outline-none focus:ring-2 focus:ring-ring"
            />
            {tokenMeta.extraField && (
              <input
                type="text"
                value={extraInput}
                onChange={e => setExtraInput(e.target.value)}
                placeholder={tokenMeta.extraField.placeholder}
                className="rounded-lg border bg-background px-3 py-1.5 text-xs outline-none focus:ring-2 focus:ring-ring"
              />
            )}
            <button
              onClick={handleSaveToken}
              disabled={savingToken || !tokenInput.trim() || (!!tokenMeta.extraField && !extraInput.trim())}
              className="rounded-lg bg-primary text-primary-foreground px-3 py-1.5 text-xs font-medium hover:bg-primary/90 disabled:opacity-50 transition-colors self-start"
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
          {showSyncPending ? (
            <div className="text-xs text-muted-foreground">
              Данные обновляются, карточка обновится автоматически
            </div>
          ) : (requiresCredentials ? isConnected : integration.enabled) && (
            <div className="flex items-center gap-3 text-xs text-muted-foreground">
              <span>Синхр.: {fmtDate(integration.last_sync_at)}</span>
              <span>•</span>
              <span>{fmtCount(integration.record_count, integration.name)}</span>
            </div>
          )}
        </div>

        {isActive && (
          <button
            onClick={handleSync}
            disabled={syncing || showSyncPending}
            className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors px-3 py-1.5 rounded-lg hover:bg-muted/50"
          >
            <RefreshCw className={cn('w-3 h-3', (syncing || showSyncPending) && 'animate-spin')} />
            {syncing || showSyncPending ? 'Синхронизация...' : 'Синхронизировать'}
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

function AppleHealthSection() {
  const [apiKey, setApiKey] = useState('')
  const [loading, setLoading] = useState(true)
  const [generating, setGenerating] = useState(false)

  useEffect(() => {
    api.getAPIKey().then(d => setApiKey(d.api_key)).catch(() => {}).finally(() => setLoading(false))
  }, [])

  async function generate() {
    setGenerating(true)
    try {
      const d = await api.generateAPIKey()
      setApiKey(d.api_key)
    } catch { /* ignore */ } finally {
      setGenerating(false)
    }
  }

  return (
    <div>
      <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-3">Apple Health</h2>
      <div className="rounded-xl border bg-card p-5 flex flex-col gap-4">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-muted flex items-center justify-center text-xl shrink-0">❤️</div>
          <div>
            <p className="text-sm font-semibold text-foreground">iOS Shortcuts → Webhook</p>
            <p className="text-xs text-muted-foreground mt-0.5">Шаги, вес, пульс через бесплатные Быстрые команды iOS</p>
          </div>
        </div>

        {loading ? (
          <div className="h-8 animate-pulse bg-muted/30 rounded" />
        ) : apiKey ? (
          <div className="flex flex-col gap-2 border-t pt-3">
            <p className="text-xs font-medium text-foreground">API Key:</p>
            <code className="text-xs bg-muted px-3 py-2 rounded-lg font-mono break-all select-all">{apiKey}</code>
            <div className="text-xs text-muted-foreground flex flex-col gap-1 mt-1">
              <p className="font-medium text-foreground">Инструкция:</p>
              <p>1. Откройте <a href="https://www.icloud.com/shortcuts/" target="_blank" rel="noopener noreferrer" className="text-primary hover:underline">Быстрые команды</a> на iPhone</p>
              <p>2. Создайте Shortcut: Find Health Samples → Steps/Weight → Get Contents of URL</p>
              <p>3. POST на <code className="bg-muted px-1 rounded">https://lifedash.dubrovskih.ru/api/v1/webhook/health</code></p>
              <p>4. JSON body: <code className="bg-muted px-1 rounded">{'{"api_key":"ваш_ключ","data":[{"type":"steps","value":8500,"date":"2026-04-01"}]}'}</code></p>
              <p>5. Настройте Automation для ежедневного запуска</p>
            </div>
            <button onClick={generate} disabled={generating}
              className="text-xs text-muted-foreground hover:text-foreground transition-colors mt-1 self-start">
              {generating ? '...' : 'Перегенерировать ключ'}
            </button>
          </div>
        ) : (
          <div className="flex flex-col gap-2 border-t pt-3">
            <p className="text-xs text-muted-foreground">Сгенерируйте API ключ для подключения iOS Shortcuts</p>
            <button onClick={generate} disabled={generating}
              className="rounded-lg bg-primary text-primary-foreground px-4 py-1.5 text-xs font-medium hover:bg-primary/90 disabled:opacity-50 transition-colors self-start">
              {generating ? '...' : 'Сгенерировать API Key'}
            </button>
          </div>
        )}
      </div>
    </div>
  )
}

export function Settings() {
  const [integrations, setIntegrations] = useState<Integration[]>([])
  const [loading, setLoading] = useState(true)
  const location = useLocation()
  const navigate = useNavigate()
  const syncTarget = new URLSearchParams(location.search).get('sync')
  const syncStartedAtRaw = new URLSearchParams(location.search).get('started_at')
  const [pendingSync, setPendingSync] = useState<{ name: string; startedAt: number; clearUrl: boolean } | null>(null)

  function load() {
    return api.getIntegrations()
      .then(setIntegrations)
      .catch(console.error)
  }

  useEffect(() => {
    load().finally(() => setLoading(false))
  }, [location.key])

  useEffect(() => {
    const handler = (event: Event) => {
      const customEvent = event as CustomEvent<{ name?: string; startedAt?: string }>
      const name = customEvent.detail?.name
      const startedAtRaw = customEvent.detail?.startedAt
      if (!name || !startedAtRaw) return

      const startedAt = Date.parse(startedAtRaw)
      if (Number.isNaN(startedAt)) return

      setPendingSync({ name, startedAt, clearUrl: false })
    }

    window.addEventListener('integration-sync-started', handler as EventListener)
    return () => window.removeEventListener('integration-sync-started', handler as EventListener)
  }, [])

  useEffect(() => {
    if (!syncTarget || !syncStartedAtRaw) {
      return
    }

    const syncStartedAt = Date.parse(syncStartedAtRaw)
    if (Number.isNaN(syncStartedAt)) {
      navigate(location.pathname, { replace: true })
      return
    }
    startTransition(() => {
      setPendingSync({ name: syncTarget, startedAt: syncStartedAt, clearUrl: true })
    })
  }, [location.pathname, navigate, syncStartedAtRaw, syncTarget])

  useEffect(() => {
    if (!pendingSync) {
      return
    }

    let cancelled = false
    let timer: number | undefined
    let attempts = 0
    const maxAttempts = 30

    const finish = () => {
      if (cancelled) return
      setPendingSync(current =>
        current?.name === pendingSync.name && current.startedAt === pendingSync.startedAt ? null : current
      )
      if (pendingSync.clearUrl) {
        navigate(location.pathname, { replace: true })
      }
    }

    const poll = async () => {
      try {
        const next = await api.getIntegrations()
        if (cancelled) return

        setIntegrations(next)
        setLoading(false)

        const target = next.find(integration => integration.name === pendingSync.name)
        const lastSyncedAt = target?.last_sync_at ? Date.parse(target.last_sync_at) : NaN
        if (!target || (Number.isFinite(lastSyncedAt) && lastSyncedAt >= pendingSync.startedAt)) {
          finish()
          return
        }
      } catch (err) {
        console.error(err)
      }

      attempts += 1
      if (attempts >= maxAttempts) {
        finish()
        return
      }

      timer = window.setTimeout(poll, 2000)
    }

    poll()

    return () => {
      cancelled = true
      if (timer) window.clearTimeout(timer)
    }
  }, [location.pathname, navigate, pendingSync])

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

      <AppleHealthSection />

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
                syncPending={pendingSync?.name === integration.name}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
