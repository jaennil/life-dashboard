import { useEffect, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { RefreshCw, CheckCircle, XCircle, AlertCircle, Power, ShieldCheck, ShieldOff, ExternalLink } from 'lucide-react'
import { PageHeader } from '@/components/PageHeader'
import { cn, formatLastSyncAt } from '@/lib/utils'
import { api, type HealthAPIKeyInfo, type Integration } from '@/lib/api'
import { useAuth } from '@/lib/auth'

const OAUTH_INTEGRATIONS: Record<string, string> = {
  strava: '/api/v1/auth/strava',
  fatsecret: '/api/v1/auth/fatsecret',
  google_calendar: '/api/v1/auth/google',
  notion: '/api/v1/auth/notion',
  todoist: '/api/v1/auth/todoist',
}

const MANAGED_CONNECTION_INTEGRATIONS = new Set(['myfitnesspal'])
const WEBHOOK_ONLY_INTEGRATIONS = new Set(['apple_health'])

const TOKEN_INTEGRATIONS: Record<string, { placeholder: string; help: React.ReactNode; extraField?: { key: string; placeholder: string } }> = {
  zenmoney: {
    placeholder: 'Bearer токен от ZenMoney',
    help: <>Получите токен на <a href="https://zerro.app/token" target="_blank" rel="noopener noreferrer" className="text-primary hover:underline">zerro.app/token</a> — войдите через ZenMoney аккаунт и скопируйте токен. Токен действует 24 часа, потом нужно обновить.</>,
  },
  hevy: {
    placeholder: 'API ключ от Hevy',
    help: <>Откройте Hevy → Settings → API, скопируйте API Key.</>,
  },
  habitify: {
    placeholder: 'API ключ от Habitify',
    help: <>Откройте Habitify → Settings → API Key и скопируйте ключ. Подробности есть в <a href="https://docs.habitify.me/" target="_blank" rel="noopener noreferrer" className="text-primary hover:underline">официальной документации API</a>.</>,
  },
  todoist: {
    placeholder: 'Personal API token от Todoist',
    help: <>Fallback без OAuth: откройте Todoist → Settings → Integrations → Developer и скопируйте personal API token. Подробности есть в <a href="https://developer.todoist.com/api/v1/" target="_blank" rel="noopener noreferrer" className="text-primary hover:underline">официальной API документации</a>.</>,
  },
  vikunja: {
    placeholder: 'API-токен Vikunja (tk_...)',
    help: <>Откройте Vikunja → Settings → API Tokens и создайте токен с правами Tasks (Read All, Create, Update), Projects (Read All) и User (Read One) - без них синхронизация и создание задач не работают. Адрес инстанса тот же, что в браузере.</>,
    extraField: { key: 'base_url', placeholder: 'Адрес инстанса (https://vikunja.example.com)' },
  },
  notion: {
    placeholder: 'Notion Integration Token (ntn_...)',
    help: <>1. Создайте <a href="https://www.notion.so/profile/integrations" target="_blank" rel="noopener noreferrer" className="text-primary hover:underline">Internal Integration</a> в Notion. 2. Скопируйте токен. 3. Откройте базу данных дневника в Notion → «...» → Connections → добавьте интеграцию. 4. Скопируйте ID базы данных из URL (32 символа после последнего /).</>,
    extraField: { key: 'database_id', placeholder: 'Database ID (32 символа из URL)' },
  },
  xiaomi_scale: {
    placeholder: 'passToken от аккаунта Xiaomi',
    help: <>У Xiaomi нет публичного API, поэтому вход по логину и паролю упирается в капчу и код из почты — для фоновой синхронизации не годится. Получите passToken один раз через <a href="https://github.com/PiotrMachowski/Xiaomi-cloud-tokens-extractor" target="_blank" rel="noopener noreferrer" className="text-primary hover:underline">Xiaomi-cloud-tokens-extractor</a>: он же покажет числовой user ID. Токен обновляется при каждой синхронизации, так что заново вводить не придётся.</>,
    extraField: { key: 'account_id', placeholder: 'Xiaomi user ID (числовой)' },
  },
}

const ICONS: Record<string, string> = {
  strava: '🚴',
  hevy: '🏋️',
  apple_health: '❤️',
  habitify: '✅',
  todoist: '☑️',
  vikunja: '🦙',
  zenmoney: '💰',
  myfitnesspal: '🥗',
  google_calendar: '📅',
  notion: '📓',
  xiaomi_scale: '⚖️',
}

function fmtCount(n: number, name: string) {
  if (n === 0) return 'нет данных'
  const labels: Record<string, [string, string, string]> = {
    strava:   ['активность', 'активности', 'активностей'],
    hevy:     ['тренировка', 'тренировки', 'тренировок'],
    apple_health: ['запись здоровья', 'записи здоровья', 'записей здоровья'],
    habitify: ['привычка', 'привычки', 'привычек'],
    todoist: ['задача', 'задачи', 'задач'],
    vikunja: ['задача', 'задачи', 'задач'],
    zenmoney: ['транзакция', 'транзакции', 'транзакций'],
    xiaomi_scale: ['измерение', 'измерения', 'измерений'],
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
  const [extraInput, setExtraInput] = useState('')
  const [showTokenForm, setShowTokenForm] = useState(false)
  const [savingToken, setSavingToken] = useState(false)
  const [saveError, setSaveError] = useState('')

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
    setSaveError('')
    try {
      const result = await api.saveToken(integration.name, tokenInput.trim(), extra ? { [tokenMeta!.extraField!.key]: extra } : undefined)
      setTokenInput('')
      setExtraInput('')
      setShowTokenForm(false)
      await onRefresh()
      if (result.sync_error) {
        setSaveError(`Подключение сохранено, но первичная синхронизация упала: ${result.sync_error}`)
      }
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : 'Не удалось сохранить подключение')
    } finally {
      setSavingToken(false)
    }
  }

  const isOAuth = !!OAUTH_INTEGRATIONS[integration.name] && integration.oauth_configured
  const tokenMeta = TOKEN_INTEGRATIONS[integration.name]
  const isDual = isOAuth && !!tokenMeta
  const isWebhookOnly = WEBHOOK_ONLY_INTEGRATIONS.has(integration.name)
  const requiresCredentials = isOAuth || !!tokenMeta || MANAGED_CONNECTION_INTEGRATIONS.has(integration.name) || isWebhookOnly
  const canSelfConnect = isOAuth || !!tokenMeta
  const hasSyncData = integration.record_count > 0 || !!integration.last_sync_at
  const isConnected = requiresCredentials ? integration.has_credentials : integration.enabled
  const isActive = integration.enabled && (requiresCredentials ? integration.has_credentials : integration.configured)

  const statusIcon = !integration.configured
    ? <AlertCircle className="w-4 h-4 text-muted-foreground" />
    : isActive
      ? <CheckCircle className="w-4 h-4 text-emerald-500" />
      : <XCircle className="w-4 h-4 text-muted-foreground" />

  const statusText = !integration.configured
    ? 'Не настроено'
    : requiresCredentials
      ? isConnected
        ? integration.enabled ? 'Подключено' : 'Отключено'
        : 'Не подключено'
      : integration.enabled
        ? 'Включено'
        : 'Отключено'

  return (
    <div className={cn(
      'rounded-2xl border bg-card/90 p-5 shadow-sm flex flex-col gap-4 transition-opacity',
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
          {saveError ? (
            <div className="rounded-lg border border-rose-500/30 bg-rose-500/10 px-3 py-2 text-xs text-rose-200">
              {saveError}
            </div>
          ) : null}
        </div>
      )}

      <div className="flex items-center justify-between gap-3 border-t pt-3">
        <div className="flex flex-col gap-1">
          <div className="flex items-center gap-1.5">
            {statusIcon}
            <span className="text-xs font-medium text-foreground">{statusText}</span>
          </div>
          {(requiresCredentials ? (isConnected || hasSyncData) : integration.enabled) && (
            <div className="flex items-center gap-3 text-xs text-muted-foreground">
              <span>Синхр.: {formatLastSyncAt(integration.last_sync_at)}</span>
              <span>•</span>
              <span>{fmtCount(integration.record_count, integration.name)}</span>
            </div>
          )}
        </div>

        {isActive && !isWebhookOnly && (
          <button
            onClick={handleSync}
            disabled={syncing}
            className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors px-3 py-1.5 rounded-lg hover:bg-muted/50"
          >
            <RefreshCw className={cn('w-3 h-3', syncing && 'animate-spin')} />
            {syncing ? 'Синхронизация...' : 'Синхронизировать'}
          </button>
        )}
        {isActive && isWebhookOnly && (
          <span className="text-xs text-muted-foreground px-3 py-1.5">
            Обновляется входящим webhook
          </span>
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
      <div className="rounded-2xl border bg-card/90 p-5 shadow-sm flex flex-col gap-4">
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
      <div className="rounded-2xl border bg-card/90 p-5 shadow-sm flex flex-col gap-4">
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

function AppleHealthSection({ onChanged, reloadKey }: { onChanged: () => void; reloadKey: number }) {
  const [info, setInfo] = useState<HealthAPIKeyInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [generating, setGenerating] = useState(false)

  function load() {
    setLoading(true)
    api.getAPIKey()
      .then(setInfo)
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    load()
  }, [reloadKey])

  async function generate() {
    setGenerating(true)
    try {
      const d = await api.generateAPIKey()
      setInfo(d)
      onChanged()
    } catch { /* ignore */ } finally {
      setGenerating(false)
    }
  }

  const apiKey = info?.api_key ?? ''
  const webhookURL = info?.webhook_url || 'https://lifedash.dubrovskih.ru/api/v1/webhook/health'
  const sampleMetricPayload = JSON.stringify({
    api_key: 'ваш_ключ',
    source: 'apple_health',
    metrics: [
      { type: 'steps', value: 8500, unit: 'count', date: '2026-04-17' },
      { type: 'resting_heart_rate', value: 58, unit: 'bpm', date: '2026-04-17T09:00:00+03:00' },
    ],
  })
  const sampleSleepPayload = JSON.stringify({
    api_key: 'ваш_ключ',
    source: 'apple_health',
    sleep: [
      {
        date: '2026-04-17',
        start_date: '2026-04-16T23:40:00+03:00',
        end_date: '2026-04-17T07:20:00+03:00',
        total_sleep_minutes: 460,
        deep_sleep_minutes: 80,
        rem_sleep_minutes: 95,
        awake_minutes: 20,
      },
    ],
  })

  return (
    <div>
      <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider mb-3">Apple Health</h2>
      <div className="rounded-2xl border bg-card/90 p-5 shadow-sm flex flex-col gap-4">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-muted flex items-center justify-center text-xl shrink-0">❤️</div>
          <div>
            <p className="text-sm font-semibold text-foreground">Apple Health / Zepp → Webhook</p>
            <p className="text-xs text-muted-foreground mt-0.5">Факт по шагам, сну, пульсу, HRV и весу. Zepp сначала пишет в Apple Health, потом Shortcut отправляет сюда.</p>
          </div>
        </div>

        {loading ? (
          <div className="h-8 animate-pulse bg-muted/30 rounded" />
        ) : apiKey ? (
          <div className="flex flex-col gap-2 border-t pt-3">
            <div className="grid gap-2 md:grid-cols-2">
              <div className="flex flex-col gap-1">
                <p className="text-xs font-medium text-foreground">Webhook URL</p>
                <code className="text-xs bg-muted px-3 py-2 rounded-lg font-mono break-all select-all">{webhookURL}</code>
              </div>
              <div className="flex flex-col gap-1">
                <p className="text-xs font-medium text-foreground">API Key</p>
                <code className="text-xs bg-muted px-3 py-2 rounded-lg font-mono break-all select-all">{apiKey}</code>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
              <span>Синхр.: {formatLastSyncAt(info?.last_sync_at ?? null)}</span>
              <span>•</span>
              <span>{(info?.metric_count ?? 0).toLocaleString('ru-RU')} метрик</span>
              <span>•</span>
              <span>{(info?.sleep_count ?? 0).toLocaleString('ru-RU')} записей сна</span>
            </div>
            <div className="text-xs text-muted-foreground flex flex-col gap-1 mt-1">
              <p className="font-medium text-foreground">Инструкция:</p>
              <p>1. Zepp → Profile → Add accounts / Apple Health, включить запись нужных типов данных в Health.</p>
              <p>2. iPhone → Быстрые команды → Automation, запускать ежедневно после пробуждения и вечером.</p>
              <p>3. В Shortcut собрать Health Samples и отправить через Get Contents of URL: Method POST, Header Content-Type = application/json.</p>
              <p>4. Поддержанные метрики: steps, weight, heart_rate, resting_heart_rate, hrv, active_energy, walking_running_distance, body_fat, spo2, vo2max.</p>
            </div>
            <details className="text-xs text-muted-foreground rounded-lg border bg-muted/20 p-3">
              <summary className="cursor-pointer text-foreground font-medium">Примеры JSON payload</summary>
              <div className="mt-2 flex flex-col gap-2">
                <code className="block bg-background px-3 py-2 rounded font-mono break-all select-all">{sampleMetricPayload}</code>
                <code className="block bg-background px-3 py-2 rounded font-mono break-all select-all">{sampleSleepPayload}</code>
              </div>
            </details>
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
  const [healthReloadKey, setHealthReloadKey] = useState(0)
  const location = useLocation()
  const navigate = useNavigate()
  const searchParams = new URLSearchParams(location.search)
  const syncStatus = searchParams.get('sync_status')
  const syncSource = searchParams.get('sync_source')
  const syncError = searchParams.get('sync_error')

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
    if (name === 'apple_health') {
      setHealthReloadKey(key => key + 1)
    }
  }

  async function handleSync(name: string) {
    await api.syncIntegration(name)
    await load()
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        eyebrow="Control center"
        title="Настройки"
        description="Управление интеграциями, вебхуками, ключами доступа и безопасностью аккаунта. Здесь видно, какие источники реально подключены и когда обновлялись."
        badges={[
          { label: `${integrations.filter(integration => integration.enabled).length} активных интеграций`, tone: 'primary' },
          { label: `${integrations.filter(integration => integration.has_credentials).length} подключено с доступом`, tone: 'muted' },
          { label: syncStatus === 'success' && syncSource ? `Подключено и синхронизировано: ${syncSource}` : 'Ручной sync запускается отдельно', tone: syncStatus === 'success' ? 'success' : 'muted' },
        ]}
      />

      {syncStatus === 'success' && syncSource ? (
        <div className="rounded-2xl border border-emerald-500/25 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-100">
          <div className="flex items-start justify-between gap-3">
            <div>
              <p className="font-medium text-emerald-50">Первичная синхронизация завершена</p>
              <p className="mt-1 text-xs text-emerald-200/90">
                Источник <span className="font-medium">{syncSource}</span> подключён и уже успел подтянуть данные.
              </p>
            </div>
            <button
              onClick={() => navigate(location.pathname, { replace: true })}
              className="text-xs text-emerald-200 transition-colors hover:text-emerald-50"
            >
              Скрыть
            </button>
          </div>
        </div>
      ) : null}

      {syncStatus === 'error' && syncSource ? (
        <div className="rounded-2xl border border-rose-500/25 bg-rose-500/10 px-4 py-3 text-sm text-rose-100">
          <div className="flex items-start justify-between gap-3">
            <div>
              <p className="font-medium text-rose-50">Первичная синхронизация не завершилась</p>
              <p className="mt-1 text-xs text-rose-200/90">
                Подключение <span className="font-medium">{syncSource}</span> сохранено, но данные автоматически не подтянулись.
              </p>
              {syncError ? <p className="mt-2 text-xs text-rose-200/90">{syncError}</p> : null}
            </div>
            <button
              onClick={() => navigate(location.pathname, { replace: true })}
              className="text-xs text-rose-200 transition-colors hover:text-rose-50"
            >
              Скрыть
            </button>
          </div>
        </div>
      ) : null}

      <div className="grid gap-6 xl:grid-cols-2 xl:items-start">
        <div>
          <h2 className="mb-3 text-sm font-semibold uppercase tracking-wider text-foreground">Аккаунт</h2>
          <TOTPSection />
        </div>

        <AppleHealthSection onChanged={load} reloadKey={healthReloadKey} />
      </div>

      <section>
        <h2 className="mb-3 text-sm font-semibold uppercase tracking-wider text-foreground">Интеграции</h2>
        {loading ? (
          <div className="flex flex-col gap-3">
            {[1, 2, 3].map(i => (
              <div key={i} className="h-28 rounded-2xl border bg-muted/30 p-5 shadow-sm animate-pulse" />
            ))}
          </div>
        ) : (
          <div className="grid gap-3 xl:grid-cols-2">
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
      </section>
    </div>
  )
}
