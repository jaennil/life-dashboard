import { useCallback, useState, type ElementType } from 'react'
import {
  AlertTriangle,
  CalendarClock,
  Check,
  CheckCircle2,
  Clock3,
  ListTodo,
  Moon,
  Pencil,
  Plus,
  Repeat2,
  Sparkles,
  Sun,
  Trash2,
} from 'lucide-react'
import { ExpandablePanel } from '@/components/ExpandablePanel'
import { EditableWidgetGrid } from '@/components/EditableWidgetGrid'
import { InfoTooltip } from '@/components/InfoTooltip'
import { PageSyncButton } from '@/components/PageSyncButton'
import { PageHeader } from '@/components/PageHeader'
import { StyledSelect } from '@/components/StyledSelect'
import { useGlobalDateRange } from '@/hooks/useGlobalDateRange'
import { useIntegrations } from '@/hooks/useIntegrations'
import { usePageSync } from '@/hooks/usePageSync'
import { useProductivityData } from '@/hooks/useProductivityData'
import {
  api,
  type ProductivityHabit,
  type ProductivityHabitInput,
  type ProductivityTask,
} from '@/lib/api'
import { cn, syncCaptionForSources } from '@/lib/utils'

type TaskFilter = 'all' | 'overdue' | 'today' | 'upcoming' | 'stale'
type HabitRoutine = 'morning' | 'evening' | 'anytime'

const FILTERS: Array<{ key: TaskFilter; label: string }> = [
  { key: 'overdue', label: 'Overdue' },
  { key: 'today', label: 'Сегодня' },
  { key: 'upcoming', label: '7 дней' },
  { key: 'stale', label: 'Висят давно' },
  { key: 'all', label: 'Все активные' },
]

const ROUTINES: Array<{ key: HabitRoutine; label: string; icon: ElementType; accent: string }> = [
  { key: 'morning', label: 'Утро', icon: Sun, accent: 'text-amber-300' },
  { key: 'evening', label: 'Вечер', icon: Moon, accent: 'text-violet-300' },
  { key: 'anytime', label: 'В течение дня', icon: Sparkles, accent: 'text-cyan-300' },
]

function StatCard({ title, value, sub, icon: Icon, color }: {
  title: string
  value: string
  sub?: string
  icon: ElementType
  color: string
}) {
  return (
    <div className="rounded-2xl border bg-card/90 p-5 shadow-sm flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <span className="inline-flex items-center gap-2 text-sm font-medium text-muted-foreground">
          {title}
          {sub ? <InfoTooltip text={sub} /> : null}
        </span>
        <div className={cn('flex items-center justify-center w-8 h-8 rounded-lg', color)}>
          <Icon className="w-4 h-4 text-white" />
        </div>
      </div>
      <div>
        <div className="text-2xl font-bold text-foreground">{value}</div>
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

function routineAccent(routine: HabitRoutine) {
  return ROUTINES.find(item => item.key === routine)?.accent ?? 'text-muted-foreground'
}

function HabitColumn({
  title,
  accent,
  icon: Icon,
  habits,
  savingHabitID,
  deletingHabitID,
  onToggle,
  onEdit,
  onDelete,
}: {
  title: string
  accent: string
  icon: ElementType
  habits: ProductivityHabit[]
  savingHabitID: string | null
  deletingHabitID: string | null
  onToggle: (habit: ProductivityHabit) => void
  onEdit: (habit: ProductivityHabit) => void
  onDelete: (habit: ProductivityHabit) => void
}) {
  return (
    <div className="rounded-2xl border bg-background/30 p-4 shadow-sm">
      <div className="flex items-center gap-2">
        <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-muted/70">
          <Icon className={cn('h-4 w-4', accent)} />
        </div>
        <div>
          <h3 className="text-sm font-semibold text-foreground">{title}</h3>
          <p className="text-xs text-muted-foreground">{habits.length > 0 ? `${habits.length} в списке` : 'Пока пусто'}</p>
        </div>
      </div>

      <div className="mt-4 flex flex-col gap-3">
        {habits.length === 0 ? (
          <div className="rounded-xl border border-dashed px-3 py-4 text-sm text-muted-foreground">
            Здесь пока нет привычек.
          </div>
        ) : habits.map((habit) => {
          const isCompleted = habit.status === 'completed'
          const isSaving = savingHabitID === habit.id
          const isDeleting = deletingHabitID === habit.id

          return (
            <div
              key={habit.id}
              className={cn(
                'rounded-xl border px-3 py-3 transition-colors',
                isCompleted ? 'border-emerald-500/20 bg-emerald-500/5' : 'border-border bg-card/60',
              )}
            >
              <div className="flex items-start gap-3">
                <button
                  type="button"
                  disabled={isSaving || isDeleting}
                  onClick={() => onToggle(habit)}
                  className={cn(
                    'mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full border transition-colors',
                    isCompleted
                      ? 'border-emerald-500/40 bg-emerald-500/15 text-emerald-300'
                      : 'border-border bg-background/50 text-muted-foreground hover:border-primary/30 hover:text-primary',
                  )}
                >
                  {isCompleted ? <Check className="h-3.5 w-3.5" /> : <span className="h-2 w-2 rounded-full bg-current opacity-60" />}
                </button>

                <div className="min-w-0 flex-1">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className={cn('text-sm font-medium break-words', isCompleted ? 'text-emerald-100' : 'text-foreground')}>
                        {habit.name}
                      </p>
                      <div className="mt-1 flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground">
                        <span className={cn('rounded-full border px-2 py-0.5', isCompleted ? 'border-emerald-500/20 text-emerald-300' : 'border-border')}>
                          {isCompleted ? 'сделано сегодня' : 'ожидает'}
                        </span>
                        <span>{habit.completed_7_days}/7 за неделю</span>
                        <span>стрик {habit.current_streak}д</span>
                        {habit.area_name && <span>{habit.area_name}</span>}
                      </div>
                    </div>

                    <div className="flex items-center gap-1">
                      <button
                        type="button"
                        disabled={isSaving || isDeleting}
                        onClick={() => onEdit(habit)}
                        className="rounded-lg border border-border px-2 py-1 text-muted-foreground transition-colors hover:bg-muted/60"
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </button>
                      <button
                        type="button"
                        disabled={isSaving || isDeleting}
                        onClick={() => onDelete(habit)}
                        className="rounded-lg border border-border px-2 py-1 text-muted-foreground transition-colors hover:bg-rose-500/10 hover:text-rose-300"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </div>

                  {habit.last_completed_at && (
                    <p className="mt-2 text-[11px] text-muted-foreground">
                      Последний раз отмечено: {formatDate(habit.last_completed_at)}
                    </p>
                  )}
                </div>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

export function Productivity() {
  const globalRange = useGlobalDateRange()
  const { integrations, reload: reloadIntegrations } = useIntegrations()
  const [habitSavingID, setHabitSavingID] = useState<string | null>(null)
  const [habitDeletingID, setHabitDeletingID] = useState<string | null>(null)
  const [habitFormSaving, setHabitFormSaving] = useState(false)
  const [habitFormError, setHabitFormError] = useState('')
  const [filter, setFilter] = useState<TaskFilter>('overdue')
  const [editingHabitID, setEditingHabitID] = useState<string | null>(null)
  const [showHabitComposer, setShowHabitComposer] = useState(false)
  const [habitForm, setHabitForm] = useState<ProductivityHabitInput>({
    name: '',
    routine: 'morning',
    area_name: '',
  })
  const {
    summary,
    tasks,
    habitsData,
    loading,
    taskLoading,
    habitsLoading,
    reload: reloadProductivityData,
    reloadHabits,
  } = useProductivityData(globalRange.params, globalRange.targetDate, filter)

  const reloadProductivity = useCallback(async () => {
    await Promise.all([reloadProductivityData(), reloadIntegrations()])
  }, [reloadProductivityData, reloadIntegrations])

  const { syncing, syncSources } = usePageSync(reloadProductivity)

  const todoistIntegration = integrations.find(integration => integration.name === 'todoist')
  const syncCaption = todoistIntegration ? syncCaptionForSources([todoistIntegration]) : undefined

  async function handleSync() {
    if (!todoistIntegration?.enabled) return
    await syncSources('todoist')
  }

  async function handleSaveHabit() {
    setHabitFormSaving(true)
    setHabitFormError('')
    try {
      if (editingHabitID) {
        await api.updateProductivityHabit(editingHabitID, habitForm)
      } else {
        await api.createProductivityHabit(habitForm)
      }
      setEditingHabitID(null)
      setShowHabitComposer(false)
      setHabitForm({ name: '', routine: 'morning', area_name: '' })
      await reloadHabits()
    } catch (error) {
      setHabitFormError(error instanceof Error ? error.message : 'Не удалось сохранить привычку')
    } finally {
      setHabitFormSaving(false)
    }
  }

  async function handleToggleHabit(habit: ProductivityHabit) {
    setHabitSavingID(habit.id)
    try {
      await api.setProductivityHabitStatus(habit.id, habit.status === 'completed' ? 'none' : 'completed', globalRange.targetDate)
      await reloadHabits()
    } catch (error) {
      console.error(error)
    } finally {
      setHabitSavingID(null)
    }
  }

  async function handleDeleteHabit(habit: ProductivityHabit) {
    setHabitDeletingID(habit.id)
    try {
      if (editingHabitID === habit.id) {
        setEditingHabitID(null)
        setShowHabitComposer(false)
        setHabitForm({ name: '', routine: 'morning', area_name: '' })
      }
      await api.deleteProductivityHabit(habit.id)
      await reloadHabits()
    } catch (error) {
      console.error(error)
    } finally {
      setHabitDeletingID(null)
    }
  }

  function startEditingHabit(habit: ProductivityHabit) {
    setEditingHabitID(habit.id)
    setShowHabitComposer(true)
    setHabitForm({
      name: habit.name,
      routine: habit.routine,
      area_name: habit.area_name,
    })
    setHabitFormError('')
  }

  const habits = habitsData?.habits ?? []
  const morningHabits = habits.filter(habit => habit.routine === 'morning')
  const eveningHabits = habits.filter(habit => habit.routine === 'evening')
  const anytimeHabits = habits.filter(habit => habit.routine === 'anytime')

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        eyebrow="Productivity"
        title="Productivity"
        description="Todoist отвечает за задачи, а встроенные рутины закрывают ежедневный уход и привычки без лишнего шума на экране."
        badges={[
          { label: todoistIntegration?.enabled ? 'Todoist подключён' : 'Todoist не подключён', tone: todoistIntegration?.enabled ? 'success' : 'warning' },
          ...(habitsData ? [{ label: `${habitsData.summary.total} локальных привычек`, tone: 'primary' as const }] : []),
          ...(summary ? [{ label: `${summary.active_total} активных задач`, tone: 'muted' as const }] : []),
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

      <EditableWidgetGrid
        storageKey="productivity_widget_layout_v2"
        widgets={[
          { id: 'routines', label: 'Рутины и уход', layout: { x: 0, y: 0, w: 12, h: 12 }, bounds: { minW: 4, minH: 6, maxH: 24 } },
          { id: 'habit-editor', label: 'Редактор привычек', layout: { x: 0, y: 12, w: 12, h: 3 }, bounds: { minW: 4, minH: 3, maxH: 14 } },
          { id: 'task-summary', label: 'Сводка задач', layout: { x: 0, y: 15, w: 5, h: 5 }, bounds: { minW: 3, minH: 4, maxH: 10 } },
          { id: 'task-workload', label: 'Нагрузка и задачи', layout: { x: 5, y: 15, w: 7, h: 10 }, bounds: { minW: 4, minH: 6, maxH: 22 } },
        ]}
      >
      <div className="rounded-2xl border bg-card/90 p-5 shadow-sm">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h2 className="inline-flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-foreground">
              Рутины и уход
              <InfoTooltip text="Свои ежедневные чеклисты внутри Life Dashboard. Отмечай факт выполнения, а AI увидит утренние и вечерние привычки как отдельный источник." />
            </h2>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {habitsData ? (
              <>
                <span className="rounded-full border px-3 py-1 text-xs text-muted-foreground">{globalRange.isActive ? 'сделано на дату' : 'сделано сегодня'} {habitsData.summary.completed_today}/{habitsData.summary.total}</span>
                <span className="rounded-full border px-3 py-1 text-xs text-muted-foreground">completion rate 7д {habitsData.summary.completion_rate_7_days.toFixed(0)}%</span>
              </>
            ) : null}
            <button
              type="button"
              onClick={() => {
                if (showHabitComposer && editingHabitID) {
                  setEditingHabitID(null)
                  setHabitForm({ name: '', routine: 'morning', area_name: '' })
                  setHabitFormError('')
                }
                setShowHabitComposer(current => !current)
              }}
              className="rounded-xl border bg-background/70 px-3 py-2 text-sm font-medium text-foreground transition-colors hover:bg-accent"
            >
              {showHabitComposer ? 'Скрыть редактор' : 'Новая привычка'}
            </button>
          </div>
        </div>

        <div className="mt-5 grid grid-cols-2 gap-3 xl:grid-cols-5">
          <StatCard
            title="Сделано сегодня"
            value={habitsLoading || !habitsData ? '—' : String(habitsData.summary.completed_today)}
            sub={habitsLoading || !habitsData ? 'Локальные привычки' : `${habitsData.summary.pending_today} осталось`}
            icon={CheckCircle2}
            color="bg-emerald-500"
          />
          <StatCard
            title="Утро"
            value={habitsLoading || !habitsData ? '—' : String(morningHabits.length)}
            sub={habitsLoading || !habitsData ? 'Рутина' : `${habitsData.summary.morning_pending} не закрыто`}
            icon={Sun}
            color="bg-amber-500"
          />
          <StatCard
            title="Вечер"
            value={habitsLoading || !habitsData ? '—' : String(eveningHabits.length)}
            sub={habitsLoading || !habitsData ? 'Рутина' : `${habitsData.summary.evening_pending} не закрыто`}
            icon={Moon}
            color="bg-violet-500"
          />
          <StatCard
            title="День"
            value={habitsLoading || !habitsData ? '—' : String(anytimeHabits.length)}
            sub={habitsLoading || !habitsData ? 'По ситуации' : `${habitsData.summary.anytime_pending} не закрыто`}
            icon={Sparkles}
            color="bg-cyan-500"
          />
          <StatCard
            title="7 дней"
            value={habitsLoading || !habitsData ? '—' : `${habitsData.summary.completion_rate_7_days.toFixed(0)}%`}
            sub="Общая дисциплина по рутинам"
            icon={Repeat2}
            color="bg-blue-500"
          />
        </div>

        <div className="mt-5 grid grid-cols-1 gap-4 xl:grid-cols-3">
          <HabitColumn
            title="Утренний блок"
            accent={routineAccent('morning')}
            icon={Sun}
            habits={morningHabits}
            savingHabitID={habitSavingID}
            deletingHabitID={habitDeletingID}
            onToggle={habit => { void handleToggleHabit(habit) }}
            onEdit={startEditingHabit}
            onDelete={habit => { void handleDeleteHabit(habit) }}
          />
          <HabitColumn
            title="Вечерний блок"
            accent={routineAccent('evening')}
            icon={Moon}
            habits={eveningHabits}
            savingHabitID={habitSavingID}
            deletingHabitID={habitDeletingID}
            onToggle={habit => { void handleToggleHabit(habit) }}
            onEdit={startEditingHabit}
            onDelete={habit => { void handleDeleteHabit(habit) }}
          />
          <HabitColumn
            title="В течение дня"
            accent={routineAccent('anytime')}
            icon={Sparkles}
            habits={anytimeHabits}
            savingHabitID={habitSavingID}
            deletingHabitID={habitDeletingID}
            onToggle={habit => { void handleToggleHabit(habit) }}
            onEdit={startEditingHabit}
            onDelete={habit => { void handleDeleteHabit(habit) }}
          />
        </div>
      </div>

      <ExpandablePanel
        title={editingHabitID ? 'Редактирование привычки' : 'Новая привычка'}
        description="Редактор нужен редко, поэтому живёт отдельно от самой доски рутин и не мешает ежедневным отметкам."
        open={showHabitComposer}
        onToggle={() => {
          if (showHabitComposer && editingHabitID) {
            setEditingHabitID(null)
            setHabitForm({ name: '', routine: 'morning', area_name: '' })
            setHabitFormError('')
          }
          setShowHabitComposer(current => !current)
        }}
        summary={(
          <>
            <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
              {editingHabitID ? 'Режим: редактирование' : 'Режим: создание'}
            </span>
            <span className="rounded-full border border-border/80 bg-background/70 px-2.5 py-1">
              Утро {morningHabits.length} · Вечер {eveningHabits.length} · День {anytimeHabits.length}
            </span>
          </>
        )}
      >
        <div className="flex flex-col gap-3 lg:flex-row">
          <label className="flex-1">
            <span className="mb-1 block text-[11px] text-muted-foreground">Привычка</span>
            <input
              type="text"
              value={habitForm.name}
              onChange={(event) => setHabitForm(current => ({ ...current, name: event.target.value }))}
              placeholder="Например: чистка зубов, крем CeraVe, душ"
              className="w-full rounded-xl border bg-card px-3 py-2.5 text-sm outline-none transition focus:ring-2 focus:ring-ring"
            />
          </label>
          <label className="w-full lg:w-48">
            <span className="mb-1 block text-[11px] text-muted-foreground">Когда</span>
            <StyledSelect
              value={habitForm.routine}
              onChange={(event) => setHabitForm(current => ({ ...current, routine: event.target.value as HabitRoutine }))}
              wrapperClassName="w-full"
              className="h-11 rounded-xl bg-card focus:ring-2 focus:ring-ring"
            >
              {ROUTINES.map(routine => (
                <option key={routine.key} value={routine.key}>{routine.label}</option>
              ))}
            </StyledSelect>
          </label>
          <label className="w-full lg:w-56">
            <span className="mb-1 block text-[11px] text-muted-foreground">Группа</span>
            <input
              type="text"
              value={habitForm.area_name ?? ''}
              onChange={(event) => setHabitForm(current => ({ ...current, area_name: event.target.value }))}
              placeholder="Опционально: уход, гигиена"
              className="w-full rounded-xl border bg-card px-3 py-2.5 text-sm outline-none transition focus:ring-2 focus:ring-ring"
            />
          </label>
        </div>

        {habitFormError ? (
          <p className="mt-3 text-sm text-rose-400">{habitFormError}</p>
        ) : null}

        <div className="mt-4 flex flex-wrap gap-2">
          <button
            type="button"
            onClick={() => void handleSaveHabit()}
            disabled={habitFormSaving}
            className="inline-flex items-center gap-2 rounded-xl bg-primary px-3 py-2 text-sm font-medium text-primary-foreground transition hover:bg-primary/90 disabled:opacity-50"
          >
            <Plus className="h-4 w-4" />
            {habitFormSaving ? 'Сохраняю...' : editingHabitID ? 'Сохранить привычку' : 'Добавить привычку'}
          </button>
          {editingHabitID ? (
            <button
              type="button"
              onClick={() => {
                setEditingHabitID(null)
                setShowHabitComposer(false)
                setHabitForm({ name: '', routine: 'morning', area_name: '' })
                setHabitFormError('')
              }}
              className="rounded-xl border px-3 py-2 text-sm text-muted-foreground transition hover:bg-muted/50"
            >
              Отменить редактирование
            </button>
          ) : null}
        </div>
      </ExpandablePanel>

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
      </EditableWidgetGrid>
    </div>
  )
}
