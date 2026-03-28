import { useEffect, useState } from 'react'
import { RefreshCw, CheckCircle, XCircle, AlertCircle, Power } from 'lucide-react'
import { cn } from '@/lib/utils'
import { api, type Integration } from '@/lib/api'

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

function IntegrationCard({ integration, onToggle, onSync }: {
  integration: Integration
  onToggle: (enabled: boolean) => void
  onSync: () => void
}) {
  const [syncing, setSyncing] = useState(false)
  const [toggling, setToggling] = useState(false)

  async function handleSync() {
    setSyncing(true)
    try { await onSync() } finally { setSyncing(false) }
  }

  async function handleToggle() {
    setToggling(true)
    try { await onToggle(!integration.enabled) } finally { setToggling(false) }
  }

  const statusIcon = !integration.configured
    ? <AlertCircle className="w-4 h-4 text-muted-foreground" />
    : integration.enabled
      ? <CheckCircle className="w-4 h-4 text-emerald-500" />
      : <XCircle className="w-4 h-4 text-muted-foreground" />

  const statusText = !integration.configured
    ? 'Не настроено'
    : integration.enabled ? 'Активно' : 'Отключено'

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
      </div>

      <div className="flex items-center justify-between gap-3 border-t pt-3">
        <div className="flex flex-col gap-1">
          <div className="flex items-center gap-1.5">
            {statusIcon}
            <span className="text-xs font-medium text-foreground">{statusText}</span>
          </div>
          {integration.configured && (
            <div className="flex items-center gap-3 text-xs text-muted-foreground">
              <span>Синхр.: {fmtDate(integration.last_sync_at)}</span>
              <span>•</span>
              <span>{fmtCount(integration.record_count, integration.name)}</span>
            </div>
          )}
        </div>

        {integration.configured && integration.enabled && (
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

export function Settings() {
  const [integrations, setIntegrations] = useState<Integration[]>([])
  const [loading, setLoading] = useState(true)

  function load() {
    return api.getIntegrations()
      .then(setIntegrations)
      .catch(console.error)
  }

  useEffect(() => {
    load().finally(() => setLoading(false))
  }, [])

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
              />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
