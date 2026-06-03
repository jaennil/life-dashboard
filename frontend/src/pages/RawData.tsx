import { useCallback, useEffect, useMemo, useState } from 'react'
import { ArrowDown, ArrowUp, Database, RotateCcw, Search } from 'lucide-react'
import { useSearchParams } from 'react-router-dom'
import { PageHeader } from '@/components/PageHeader'
import { api, type CollectionParams } from '@/lib/api'
import { RAW_DATA_SOURCES, type RawDataSource } from '@/lib/raw-data'

type RawRecord = Record<string, unknown>
type SortValue = string | number | undefined
type RawRow = {
  id: string
  raw: RawRecord
  values: Record<string, string | number | undefined>
  sortValues: Record<string, SortValue>
}
type Column = { key: string; label: string; defaultOrder?: 'asc' | 'desc' }

const SOURCE_COLUMNS: Record<RawDataSource, Column[]> = {
  'finance.transactions': [
    { key: 'date', label: 'Дата', defaultOrder: 'desc' }, { key: 'payee', label: 'Payee', defaultOrder: 'asc' }, { key: 'category', label: 'Категория', defaultOrder: 'asc' },
    { key: 'amount', label: 'Сумма', defaultOrder: 'desc' }, { key: 'account', label: 'Счёт', defaultOrder: 'asc' }, { key: 'comment', label: 'Комментарий', defaultOrder: 'asc' },
  ],
  'fitness.activities': [
    { key: 'date', label: 'Дата', defaultOrder: 'desc' }, { key: 'type', label: 'Тип', defaultOrder: 'asc' }, { key: 'name', label: 'Название', defaultOrder: 'asc' },
    { key: 'distance', label: 'Дистанция', defaultOrder: 'desc' }, { key: 'duration', label: 'Длительность', defaultOrder: 'desc' }, { key: 'calories', label: 'Ккал', defaultOrder: 'desc' },
  ],
  'fitness.workouts': [
    { key: 'date', label: 'Дата', defaultOrder: 'desc' }, { key: 'title', label: 'Тренировка', defaultOrder: 'asc' }, { key: 'source', label: 'Источник', defaultOrder: 'asc' },
    { key: 'exercises', label: 'Упражнений', defaultOrder: 'desc' }, { key: 'sets', label: 'Сетов', defaultOrder: 'desc' }, { key: 'notes', label: 'Заметки', defaultOrder: 'asc' },
  ],
  'nutrition.days': [
    { key: 'date', label: 'Дата', defaultOrder: 'desc' }, { key: 'calories', label: 'Ккал', defaultOrder: 'desc' }, { key: 'protein', label: 'Белки', defaultOrder: 'desc' },
    { key: 'fat', label: 'Жиры', defaultOrder: 'desc' }, { key: 'carbs', label: 'Углеводы', defaultOrder: 'desc' }, { key: 'water', label: 'Вода', defaultOrder: 'desc' },
  ],
  'productivity.tasks': [
    { key: 'due', label: 'Срок', defaultOrder: 'asc' }, { key: 'content', label: 'Задача', defaultOrder: 'asc' }, { key: 'project', label: 'Проект', defaultOrder: 'asc' },
    { key: 'section', label: 'Секция', defaultOrder: 'asc' }, { key: 'priority', label: 'Приоритет', defaultOrder: 'desc' }, { key: 'bucket', label: 'Статус', defaultOrder: 'asc' },
  ],
  'ai.messages': [
    { key: 'date', label: 'Дата', defaultOrder: 'desc' }, { key: 'role', label: 'Роль', defaultOrder: 'asc' }, { key: 'content', label: 'Сообщение', defaultOrder: 'asc' },
  ],
}

const CONTEXT_KEYS = ['type', 'category', 'payee', 'day', 'meal_type', 'metric'] as const

function displayDate(value?: string | null) {
  if (!value) return '—'
  return new Date(value).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}

function stringify(value: unknown) {
  if (value == null || value === '') return '—'
  return String(value)
}

function sortDate(value?: string | null) {
  if (!value) return undefined
  const time = new Date(value).getTime()
  return Number.isFinite(time) ? time : value
}

function toSortNumber(value: unknown) {
  const number = Number(value)
  return Number.isFinite(number) ? number : undefined
}

function matchesContext(source: RawDataSource, raw: RawRecord, params: URLSearchParams) {
  const day = params.get('day')
  if (day && String(raw.date ?? raw.occurred_at ?? raw.started_at ?? '').slice(0, 10) !== day) return false
  const mealType = params.get('meal_type')
  if (source === 'nutrition.days' && mealType) {
    const meals = Array.isArray(raw.meals) ? raw.meals : []
    if (!meals.some(meal => typeof meal === 'object' && meal !== null && (meal as RawRecord).meal_type === mealType)) return false
  }
  return true
}

function recordSearch(row: RawRow) {
  return JSON.stringify(row.raw).toLocaleLowerCase('ru-RU')
}

function buildRows(source: RawDataSource, data: unknown[]): RawRow[] {
  return data.map((item, index) => {
    const raw = item as RawRecord
    switch (source) {
      case 'finance.transactions':
        return { id: stringify(raw.id), raw, values: {
          date: displayDate(raw.occurred_at as string), payee: stringify(raw.payee), category: stringify(raw.category),
          amount: `${Number(raw.amount ?? 0).toLocaleString('ru-RU')} ${stringify(raw.currency)}`, account: stringify(raw.account_title), comment: stringify(raw.comment),
        }, sortValues: {
          date: sortDate(raw.occurred_at as string), payee: stringify(raw.payee), category: stringify(raw.category),
          amount: toSortNumber(raw.amount), account: stringify(raw.account_title), comment: stringify(raw.comment),
        } }
      case 'fitness.activities':
        return { id: stringify(raw.id), raw, values: {
          date: displayDate(raw.started_at as string), type: stringify(raw.type), name: stringify(raw.name),
          distance: raw.distance_meters ? `${(Number(raw.distance_meters) / 1000).toFixed(1)} км` : '—',
          duration: raw.duration_seconds ? `${Math.round(Number(raw.duration_seconds) / 60)} мин` : '—', calories: stringify(raw.calories),
        }, sortValues: {
          date: sortDate(raw.started_at as string), type: stringify(raw.type), name: stringify(raw.name),
          distance: toSortNumber(raw.distance_meters), duration: toSortNumber(raw.duration_seconds), calories: toSortNumber(raw.calories),
        } }
      case 'fitness.workouts': {
        const exercises = Array.isArray(raw.exercises) ? raw.exercises : []
        const sets = exercises.reduce((sum, exercise) => sum + (Array.isArray((exercise as RawRecord).sets) ? ((exercise as RawRecord).sets as unknown[]).length : 0), 0)
        return { id: stringify(raw.id), raw, values: {
          date: displayDate(raw.started_at as string), title: stringify(raw.title), source: stringify(raw.source),
          exercises: exercises.length, sets, notes: stringify(raw.notes),
        }, sortValues: {
          date: sortDate(raw.started_at as string), title: stringify(raw.title), source: stringify(raw.source),
          exercises: exercises.length, sets, notes: stringify(raw.notes),
        } }
      }
      case 'nutrition.days':
        return { id: stringify(raw.date ?? index), raw, values: {
          date: stringify(raw.date), calories: Math.round(Number(raw.calories ?? 0)), protein: `${Math.round(Number(raw.protein ?? 0))} г`,
          fat: `${Math.round(Number(raw.fat ?? 0))} г`, carbs: `${Math.round(Number(raw.carbs ?? 0))} г`, water: `${Math.round(Number(raw.water_ml ?? 0))} мл`,
        }, sortValues: {
          date: sortDate(raw.date as string), calories: toSortNumber(raw.calories), protein: toSortNumber(raw.protein),
          fat: toSortNumber(raw.fat), carbs: toSortNumber(raw.carbs), water: toSortNumber(raw.water_ml),
        } }
      case 'productivity.tasks':
        return { id: stringify(raw.id), raw, values: {
          due: stringify(raw.due_at ?? raw.due_date), content: stringify(raw.content), project: stringify(raw.project_name),
          section: stringify(raw.section_name), priority: Number(raw.priority ?? 0), bucket: stringify(raw.due_bucket),
        }, sortValues: {
          due: sortDate((raw.due_at ?? raw.due_date) as string), content: stringify(raw.content), project: stringify(raw.project_name),
          section: stringify(raw.section_name), priority: toSortNumber(raw.priority), bucket: stringify(raw.due_bucket),
        } }
      case 'ai.messages':
        return { id: stringify(raw.id), raw, values: {
          date: displayDate(raw.created_at as string), role: stringify(raw.role), content: stringify(raw.content),
        }, sortValues: {
          date: sortDate(raw.created_at as string), role: stringify(raw.role), content: stringify(raw.content),
        } }
    }
  })
}

export function RawData() {
  const [params, setParams] = useSearchParams()
  const source = (RAW_DATA_SOURCES.some(item => item.value === params.get('source')) ? params.get('source') : 'finance.transactions') as RawDataSource
  const [data, setData] = useState<unknown[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const search = params.get('search') ?? ''
  const columns = SOURCE_COLUMNS[source]
  const sort = columns.some(column => column.key === params.get('sort')) ? params.get('sort') as string : columns[0].key
  const order = params.get('order') === 'asc' ? 'asc' : 'desc'

  const loadRawData = useCallback(async () => {
    const query: CollectionParams = {
      from: params.get('from') ?? undefined, to: params.get('to') ?? undefined,
      search: params.get('search') ?? undefined, type: params.get('type') ?? undefined,
      category: params.get('category') ?? undefined, payee: params.get('payee') ?? undefined,
      sort: source === 'finance.transactions' && sort === 'amount' ? 'signed_amount' : sort,
      order,
      page_size: 250,
    }
    setLoading(true)
    setError('')
    try {
      const result = source === 'finance.transactions' ? await api.getTransactions(query)
        : source === 'fitness.activities' ? await api.getActivities(query)
          : source === 'fitness.workouts' ? await api.getWorkouts(query)
            : source === 'nutrition.days' ? await api.getNutritionDaily(366, query)
              : source === 'productivity.tasks' ? await api.getProductivityTasks('all', query)
                : await api.getAIHistory(query)
      setData(result)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Не удалось загрузить данные')
    } finally {
      setLoading(false)
    }
  }, [order, params, sort, source])

  useEffect(() => {
    void loadRawData()
    return undefined
  }, [loadRawData])

  const rows = useMemo(() => {
    const needle = search.trim().toLocaleLowerCase('ru-RU')
    return buildRows(source, data)
      .filter(row => matchesContext(source, row.raw, params))
      .filter(row => !needle || recordSearch(row).includes(needle))
      .sort((left, right) => {
        const a = left.sortValues[sort] ?? ''
        const b = right.sortValues[sort] ?? ''
        const result = typeof a === 'number' && typeof b === 'number'
          ? a - b
          : String(a).localeCompare(String(b), 'ru-RU', { numeric: true })
        return order === 'asc' ? result : -result
      })
  }, [data, order, params, search, sort, source])

  function update(next: Record<string, string | undefined>) {
    const copy = new URLSearchParams(params)
    Object.entries(next).forEach(([key, value]) => value ? copy.set(key, value) : copy.delete(key))
    setParams(copy)
  }

  function sortByColumn(column: Column) {
    const nextOrder = sort === column.key
      ? order === 'asc' ? 'desc' : 'asc'
      : column.defaultOrder ?? 'asc'
    update({ sort: column.key, order: nextOrder })
  }

  const context = CONTEXT_KEYS.flatMap(key => params.get(key) ? [{ key, value: params.get(key) as string }] : [])

  return (
    <div className="flex flex-col gap-6">
      <PageHeader eyebrow="Inspect" title="Raw Data" description="Исходные записи из доменных API. Фильтры графиков открываются здесь как воспроизводимый drill-down." badges={[{ label: `${rows.length} записей`, tone: 'primary' }]} />

      <section className="rounded-2xl border bg-card/90 shadow-sm">
        <div className="flex flex-col gap-3 border-b p-4 lg:flex-row lg:items-center">
          <label className="flex min-w-0 flex-1 items-center gap-2 rounded-xl border bg-background px-3 py-2">
            <Database className="h-4 w-4 shrink-0 text-muted-foreground" />
            <select value={source} onChange={event => update({ source: event.target.value, sort: undefined, search: undefined, type: undefined, category: undefined, payee: undefined, day: undefined, meal_type: undefined, metric: undefined })} className="min-w-0 flex-1 bg-transparent text-sm text-foreground outline-none">
              {RAW_DATA_SOURCES.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}
            </select>
          </label>
          <label className="flex min-w-0 flex-[2] items-center gap-2 rounded-xl border bg-background px-3 py-2">
            <Search className="h-4 w-4 shrink-0 text-muted-foreground" />
            <input value={search} onChange={event => update({ search: event.target.value })} placeholder="Поиск по полям записи" className="min-w-0 flex-1 bg-transparent text-sm text-foreground outline-none placeholder:text-muted-foreground" />
          </label>
          <label className="flex items-center gap-2 rounded-xl border bg-background px-3 py-2">
            <span className="text-xs text-muted-foreground">Сортировка</span>
            <select value={sort} onChange={event => {
              const column = columns.find(item => item.key === event.target.value)
              update({ sort: event.target.value, order: column?.defaultOrder ?? 'asc' })
            }} className="bg-transparent text-sm text-foreground outline-none">
              {SOURCE_COLUMNS[source].map(column => <option key={column.key} value={column.key}>{column.label}</option>)}
            </select>
          </label>
          <button type="button" onClick={() => update({ order: order === 'asc' ? 'desc' : 'asc' })} className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border bg-background text-muted-foreground transition-colors hover:text-foreground" title={order === 'asc' ? 'По возрастанию' : 'По убыванию'}>
            {order === 'asc' ? <ArrowUp className="h-4 w-4" /> : <ArrowDown className="h-4 w-4" />}
          </button>
        </div>

        {context.length > 0 ? (
          <div className="flex flex-wrap items-center gap-2 border-b px-4 py-3">
            <span className="text-xs text-muted-foreground">Контекст графика:</span>
            {context.map(item => <span key={item.key} className="rounded-full border border-primary/20 bg-primary/10 px-2.5 py-1 text-xs text-primary">{item.key}: {item.value}</span>)}
            <button type="button" onClick={() => update(Object.fromEntries(CONTEXT_KEYS.map(key => [key, undefined])))} className="ml-auto flex items-center gap-1.5 text-xs text-muted-foreground transition-colors hover:text-foreground">
              <RotateCcw className="h-3.5 w-3.5" /> Сбросить
            </button>
          </div>
        ) : null}

        <div className="overflow-x-auto">
          <table className="min-w-full text-left text-sm">
            <thead className="bg-background/55 text-xs uppercase text-muted-foreground">
              <tr>
                {columns.map(column => (
                  <th key={column.key} aria-sort={sort === column.key ? order === 'asc' ? 'ascending' : 'descending' : 'none'} className="whitespace-nowrap px-4 py-3 font-medium">
                    <button type="button" onClick={() => sortByColumn(column)} className="inline-flex items-center gap-1.5 transition-colors hover:text-foreground">
                      <span>{column.label}</span>
                      {sort === column.key ? (
                        order === 'asc' ? <ArrowUp className="h-3.5 w-3.5" /> : <ArrowDown className="h-3.5 w-3.5" />
                      ) : null}
                    </button>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y">
              {loading ? <tr><td colSpan={SOURCE_COLUMNS[source].length} className="px-4 py-10 text-center text-muted-foreground">Загрузка...</td></tr>
                : error ? <tr><td colSpan={SOURCE_COLUMNS[source].length} className="px-4 py-10 text-center text-rose-300">{error}</td></tr>
                  : rows.length === 0 ? <tr><td colSpan={SOURCE_COLUMNS[source].length} className="px-4 py-10 text-center text-muted-foreground">Нет записей для выбранных фильтров</td></tr>
                    : rows.map((row, index) => (
                      <tr key={`${row.id}-${index}`} className="transition-colors hover:bg-accent/30">
                        {SOURCE_COLUMNS[source].map(column => <td key={column.key} className="max-w-[24rem] truncate whitespace-nowrap px-4 py-3 text-foreground">{row.values[column.key]}</td>)}
                      </tr>
                    ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  )
}
