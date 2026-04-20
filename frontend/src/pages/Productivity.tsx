import { useCallback, useEffect, useState, type ElementType } from 'react'
import { AlertTriangle, CalendarClock, CheckCircle2, Clock3, ListTodo, Repeat2 } from 'lucide-react'
import { PageSyncButton } from '@/components/PageSyncButton'
import { PageHeader } from '@/components/PageHeader'
import { api, type Integration, type ProductivitySummary, type ProductivityTask } from '@/lib/api'
import { cn, syncCaptionForSources } from '@/lib/utils'

type TaskFilter = 'all' | 'overdue' | 'today' | 'upcoming' | 'stale'

const FILTERS: Array<{ key: TaskFilter; label: string }> = [
  { key: 'overdue', label: 'Overdue' },
  { key: 'today', label: 'Сегодня' },
  { key: 'upcoming', label: '7 дней' },
  { key: 'stale', label: 'Висят давно' },
  { key: 'all', label: 'Все активные' },
]

function StatCard({ title, value, sub, icon: Icon, color }: {
  title: string
  value: string
  sub: string
  icon: ElementType
  color: string
}) {
  return (
    <div className="rounded-2xl border bg-card/90 p-5 shadow-sm flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-muted-foreground">{title}</span>
        <div className={cn('flex items-center justify-center w-8 h-8 rounded-lg', color)}>
          <Icon className="w-4 h-4 text-white" />
        </div>
      </div>
      <div>
        <div className="text-2xl font-bold text-foreground">{value}</div>
        <div className="text-xs text-muted-foreground mt-1">{sub}</div>
      </div>
    </div>
  )
}

function formatDate(iso: string | null) {
  if (!iso) return 'без даты'
  const date = new Date(iso)
  if (!Number.isFinite(date.getTime())) return 'без даты'
  return new Intl.DateTimeFormat('ru-RU', {
    day: '2-digit',
    month: '2-digit',
    hour: iso.includes('T') ? '2-digit' : undefined,
    minute: iso.includes('T') ? '2-digit' : undefined,
  }).format(date)
}

function priorityBadge(priority: number) {
  switch (priority) {
    case 4:
      return 'border-red-500/30 text-red-400'
    case 3:
      return 'border-amber-500/30 text-amber-400'
    case 2:
      return 'border-blue-500/30 text-blue-400'
    default:
      return 'border-border text-muted-foreground'
  }
}

function bucketLabel(task: ProductivityTask) {
  switch (task.due_bucket) {
    case 'overdue':
      return 'overdue'
    case 'today':
      return 'сегодня'
    case 'upcoming':
      return 'скоро'
    case 'later':
      return 'позже'
    default:
      return task.added_at ? 'без срока' : 'без даты'
  }
}

export function Productivity() {
  const [summary, setSummary] = useState<ProductivitySummary | null>(null)
  const [tasks, setTasks] = useState<ProductivityTask[]>([])
  const [integrations, setIntegrations] = useState<Integration[]>([])
  const [loading, setLoading] = useState(true)
  const [taskLoading, setTaskLoading] = useState(true)
  const [syncing, setSyncing] = useState(false)
  const [filter, setFilter] = useState<TaskFilter>('overdue')

  const loadIntegrations = useCallback(async () => {
    try {
      setIntegrations(await api.getIntegrations())
    } catch (error) {
      console.error(error)
    }
  }, [])

  const loadSummary = useCallback(async () => {
    try {
      setSummary(await api.getProductivitySummary())
    } catch (error) {
      console.error(error)
    }
  }, [])

  const loadTasks = useCallback(async (currentFilter: TaskFilter) => {
    setTaskLoading(true)
    try {
      setTasks(await api.getProductivityTasks(currentFilter))
    } catch (error) {
      console.error(error)
    } finally {
      setTaskLoading(false)
    }
  }, [])

  useEffect(() => {
    setLoading(true)
    Promise.all([loadSummary(), loadIntegrations(), loadTasks(filter)])
      .finally(() => setLoading(false))
  }, [filter, loadIntegrations, loadSummary, loadTasks])

  const todoistIntegration = integrations.find(integration => integration.name === 'todoist')
  const syncCaption = todoistIntegration ? syncCaptionForSources([todoistIntegration]) : undefined

  async function handleSync() {
    if (!todoistIntegration?.enabled) return
    setSyncing(true)
    try {
      await api.syncIntegration('todoist')
      await Promise.all([loadSummary(), loadIntegrations(), loadTasks(filter)])
    } catch (error) {
      console.error(error)
    } finally {
      setSyncing(false)
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        eyebrow="Productivity"
        title="Productivity"
        description="Todoist как слой задач и регулярных дел: overdue, близкая нагрузка, recurring и всё, что зависло дольше разумного."
        badges={[
          { label: todoistIntegration?.enabled ? 'Todoist подключён' : 'Todoist не подключён', tone: todoistIntegration?.enabled ? 'success' : 'warning' },
          ...(summary ? [{ label: `${summary.active_total} активных задач`, tone: 'muted' as const }] : []),
          ...(summary ? [{ label: `${summary.completed_today_total} закрыто сегодня`, tone: 'primary' as const }] : []),
        ]}
        actions={(
          <PageSyncButton
            label="Синхронизировать Todoist"
            syncCaption={syncCaption}
            syncing={syncing}
            disabled={!todoistIntegration?.enabled}
            onClick={handleSync}
          />
        )}
      />

      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4">
        <StatCard
          title="Активные"
          value={loading || !summary ? '—' : String(summary.active_total)}
          sub={loading || !summary ? 'Todoist' : `${summary.recurring_total} recurring`}
          icon={ListTodo}
          color="bg-blue-500"
        />
        <StatCard
          title="Overdue"
          value={loading || !summary ? '—' : String(summary.overdue_total)}
          sub={loading || !summary ? 'Просрочено' : `${summary.stale_total} висят давно`}
          icon={AlertTriangle}
          color="bg-rose-500"
        />
        <StatCard
          title="Сегодня"
          value={loading || !summary ? '—' : String(summary.due_today_total)}
          sub={loading || !summary ? 'На сегодня' : `${summary.completed_today_total} закрыто`}
          icon={CalendarClock}
          color="bg-amber-500"
        />
        <StatCard
          title="7 дней"
          value={loading || !summary ? '—' : String(summary.due_next_7_days_total)}
          sub={loading || !summary ? 'Ближайшая неделя' : `${summary.completed_7_days_total} закрыто за 7 дн.`}
          icon={CheckCircle2}
          color="bg-emerald-500"
        />
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-[1.2fr_1.8fr] gap-4">
        <div className="rounded-2xl border bg-card/90 p-5 shadow-sm">
          <h2 className="text-sm font-semibold text-foreground uppercase tracking-wider">Нагрузка по дням</h2>
          <div className="mt-4 flex flex-col gap-3">
            {summary?.upcoming_load?.length ? summary.upcoming_load.map(day => (
              <div key={day.date} className="flex items-center gap-3">
                <span className="w-16 shrink-0 text-xs text-muted-foreground">
                  {new Date(day.date).toLocaleDateString('ru-RU', { day: '2-digit', month: 'short' })}
                </span>
                <div className="h-2 flex-1 rounded-full bg-muted overflow-hidden">
                  <div
                    className="h-full bg-primary"
                    style={{ width: `${Math.max(8, Math.min(100, day.count * 18))}%` }}
                  />
                </div>
                <span className="text-xs text-foreground tabular-nums">{day.count}</span>
              </div>
            )) : (
              <p className="text-sm text-muted-foreground">На ближайшие дни задач с дедлайнами нет.</p>
            )}
          </div>
        </div>

        <div className="rounded-2xl border bg-card/90 p-5 shadow-sm flex flex-col gap-4">
          <div className="flex flex-wrap items-center gap-2">
            {FILTERS.map(item => (
              <button
                key={item.key}
                onClick={() => setFilter(item.key)}
                className={cn(
                  'rounded-lg px-3 py-1.5 text-xs border transition-colors',
                  filter === item.key
                    ? 'border-primary/30 bg-primary/10 text-primary'
                    : 'border-border text-muted-foreground hover:bg-muted/50'
                )}
              >
                {item.label}
              </button>
            ))}
          </div>

          {taskLoading ? (
            <div className="flex flex-col gap-3">
              {Array.from({ length: 6 }).map((_, index) => (
                <div key={index} className="h-16 rounded-lg bg-muted/30 animate-pulse" />
              ))}
            </div>
          ) : tasks.length === 0 ? (
            <div className="text-sm text-muted-foreground">По этому фильтру задач нет.</div>
          ) : (
            <div className="flex flex-col gap-3">
              {tasks.map(task => (
                <div key={task.id} className="rounded-2xl border bg-background/40 px-4 py-3 shadow-sm">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <p className="text-sm font-medium text-foreground break-words">{task.content}</p>
                        {task.is_recurring && (
                          <span className="inline-flex items-center gap-1 rounded-full border border-emerald-500/20 px-2 py-0.5 text-[10px] text-emerald-400">
                            <Repeat2 className="w-3 h-3" />
                            recurring
                          </span>
                        )}
                      </div>
                      {(task.project_name || task.section_name) && (
                        <p className="mt-1 text-xs text-muted-foreground">
                          {[task.project_name, task.section_name].filter(Boolean).join(' / ')}
                        </p>
                      )}
                      {task.description && (
                        <p className="mt-2 text-xs text-muted-foreground line-clamp-2">{task.description}</p>
                      )}
                    </div>
                    <span className={cn('shrink-0 rounded-full border px-2 py-0.5 text-[10px]', priorityBadge(task.priority))}>
                      p{task.priority}
                    </span>
                  </div>

                  <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                    <span className={cn(task.is_overdue && 'text-rose-400')}>{bucketLabel(task)}</span>
                    <span>•</span>
                    <span>дедлайн: {formatDate(task.due_at ?? task.due_date)}</span>
                    {task.added_at && (
                      <>
                        <span>•</span>
                        <span>добавлена {formatDate(task.added_at)}</span>
                      </>
                    )}
                    {task.last_completed_at && (
                      <>
                        <span>•</span>
                        <span>последний раз закрыта {formatDate(task.last_completed_at)}</span>
                      </>
                    )}
                    {!task.due_at && !task.due_date && (
                      <>
                        <span>•</span>
                        <span className="inline-flex items-center gap-1"><Clock3 className="w-3 h-3" />без срока</span>
                      </>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
