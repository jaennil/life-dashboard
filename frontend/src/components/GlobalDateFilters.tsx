import { useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import { CalendarDays, ChevronLeft, ChevronRight, RotateCcw } from 'lucide-react'
import { cn } from '@/lib/utils'

const MONTHS = [
  { value: 1, label: 'Январь' },
  { value: 2, label: 'Февраль' },
  { value: 3, label: 'Март' },
  { value: 4, label: 'Апрель' },
  { value: 5, label: 'Май' },
  { value: 6, label: 'Июнь' },
  { value: 7, label: 'Июль' },
  { value: 8, label: 'Август' },
  { value: 9, label: 'Сентябрь' },
  { value: 10, label: 'Октябрь' },
  { value: 11, label: 'Ноябрь' },
  { value: 12, label: 'Декабрь' },
]

const QUICK_RANGES = [
  { label: 'Месяц', mode: 'month' },
  { label: 'Год', mode: 'year' },
  { label: '30 дней', mode: 'rolling' },
] as const

type RangeMode = typeof QUICK_RANGES[number]['mode'] | 'custom'

function pad(value: number) {
  return String(value).padStart(2, '0')
}

function formatDate(date: Date) {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

function monthRange(year: number, month: number) {
  return {
    from: `${year}-${pad(month)}-01`,
    to: formatDate(new Date(year, month, 0)),
  }
}

function yearRange(year: number) {
  return { from: `${year}-01-01`, to: `${year}-12-31` }
}

function rollingRange(days: number) {
  const to = new Date()
  const from = new Date()
  from.setDate(to.getDate() - (days - 1))
  return { from: formatDate(from), to: formatDate(to) }
}

function readNumber(value: string | null, fallback: number, min: number, max: number) {
  const parsed = Number(value)
  if (!Number.isInteger(parsed) || parsed < min || parsed > max) return fallback
  return parsed
}

function readMode(value: string | null): RangeMode {
  if (value === 'month' || value === 'year' || value === 'rolling' || value === 'custom') return value
  return 'rolling'
}

export function GlobalDateFilters() {
  const [searchParams, setSearchParams] = useSearchParams()
  const today = useMemo(() => new Date(), [])
  const year = readNumber(searchParams.get('year'), today.getFullYear(), 2000, 2100)
  const month = readNumber(searchParams.get('month'), today.getMonth() + 1, 1, 12)
  const mode = readMode(searchParams.get('range'))
  const fallbackRange = mode === 'month'
    ? monthRange(year, month)
    : mode === 'year'
      ? yearRange(year)
      : mode === 'rolling'
        ? rollingRange(30)
        : { from: '', to: '' }
  const selectedFrom = searchParams.get('from') ?? fallbackRange.from
  const selectedTo = searchParams.get('to') ?? fallbackRange.to
  const years = useMemo(
    () => Array.from({ length: 8 }, (_, index) => today.getFullYear() + 1 - index),
    [today]
  )

  function updateRange(next: {
    mode: RangeMode
    month?: number
    year?: number
    from: string
    to: string
  }) {
    const params = new URLSearchParams(searchParams)
    params.set('range', next.mode)
    params.set('year', String(next.year ?? year))
    params.set('month', String(next.month ?? month))
    params.set('from', next.from)
    params.set('to', next.to)
    setSearchParams(params)
  }

  function applyMonth(nextYear = year, nextMonth = month) {
    const range = monthRange(nextYear, nextMonth)
    updateRange({ mode: 'month', year: nextYear, month: nextMonth, ...range })
  }

  function applyYear(nextYear = year) {
    updateRange({ mode: 'year', year: nextYear, month, ...yearRange(nextYear) })
  }

  function applyRolling() {
    updateRange({ mode: 'rolling', year, month, ...rollingRange(30) })
  }

  function moveMonth(delta: number) {
    const next = new Date(year, month - 1 + delta, 1)
    applyMonth(next.getFullYear(), next.getMonth() + 1)
  }

  function reset() {
    const params = new URLSearchParams(searchParams)
    for (const key of ['range', 'year', 'month', 'from', 'to']) {
      params.delete(key)
    }
    setSearchParams(params)
  }

  function updateCustom(key: 'from' | 'to', value: string) {
    const params = new URLSearchParams(searchParams)
    params.set('range', 'custom')
    params.set('year', String(year))
    params.set('month', String(month))
    if (value) params.set(key, value)
    else params.delete(key)
    setSearchParams(params)
  }

  return (
    <div className="sticky top-[calc(4.75rem+env(safe-area-inset-top))] z-30 mx-auto mb-4 w-full max-w-[1560px] lg:top-4 lg:mb-5">
      <div className="flex flex-col gap-3 rounded-lg border bg-card/95 p-3 shadow-sm backdrop-blur xl:flex-row xl:items-center xl:justify-between">
        <div className="flex min-w-0 items-center gap-3">
          <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
            <CalendarDays className="h-4 w-4" />
          </span>
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold text-foreground">Период данных</p>
            <p className="truncate text-xs text-muted-foreground">{selectedFrom || 'начало'} — {selectedTo || 'сегодня'}</p>
          </div>
        </div>

        <div className="flex min-w-0 flex-1 flex-col gap-3 xl:flex-row xl:flex-wrap xl:items-center xl:justify-end">
          <div className="flex flex-wrap gap-1 rounded-lg border bg-background/70 p-1">
            {QUICK_RANGES.map(item => (
              <button
                key={item.mode}
                onClick={() => {
                  if (item.mode === 'month') applyMonth()
                  if (item.mode === 'year') applyYear()
                  if (item.mode === 'rolling') applyRolling()
                }}
                className={cn(
                  'rounded-md px-3 py-1.5 text-xs font-medium transition-colors',
                  mode === item.mode
                    ? 'bg-primary text-primary-foreground'
                    : 'text-muted-foreground hover:bg-accent hover:text-foreground'
                )}
              >
                {item.label}
              </button>
            ))}
          </div>

          <div className="grid grid-cols-2 gap-2 sm:grid-cols-[auto_auto_auto_auto_auto]">
            <button
              onClick={() => moveMonth(-1)}
              className="flex h-10 w-10 items-center justify-center rounded-lg border bg-background text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              aria-label="Предыдущий месяц"
            >
              <ChevronLeft className="h-4 w-4" />
            </button>
            <select
              value={month}
              onChange={event => applyMonth(year, Number(event.target.value))}
              className="h-10 rounded-lg border bg-background px-3 text-sm text-foreground outline-none transition-colors focus:border-primary"
              aria-label="Месяц"
            >
              {MONTHS.map(item => (
                <option key={item.value} value={item.value}>{item.label}</option>
              ))}
            </select>
            <select
              value={year}
              onChange={event => mode === 'year' ? applyYear(Number(event.target.value)) : applyMonth(Number(event.target.value), month)}
              className="h-10 rounded-lg border bg-background px-3 text-sm text-foreground outline-none transition-colors focus:border-primary"
              aria-label="Год"
            >
              {years.map(item => (
                <option key={item} value={item}>{item}</option>
              ))}
            </select>
            <button
              onClick={() => moveMonth(1)}
              className="flex h-10 w-10 items-center justify-center rounded-lg border bg-background text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              aria-label="Следующий месяц"
            >
              <ChevronRight className="h-4 w-4" />
            </button>
            <button
              onClick={reset}
              className="flex h-10 w-10 items-center justify-center rounded-lg border bg-background text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              aria-label="Сбросить глобальный период"
            >
              <RotateCcw className="h-4 w-4" />
            </button>
          </div>

          <div className="grid grid-cols-2 gap-2">
            <input
              type="date"
              value={selectedFrom}
              onChange={event => updateCustom('from', event.target.value)}
              className="h-10 min-w-0 rounded-lg border bg-background px-3 text-sm text-foreground outline-none transition-colors focus:border-primary"
              aria-label="Начало периода"
            />
            <input
              type="date"
              value={selectedTo}
              onChange={event => updateCustom('to', event.target.value)}
              className="h-10 min-w-0 rounded-lg border bg-background px-3 text-sm text-foreground outline-none transition-colors focus:border-primary"
              aria-label="Конец периода"
            />
          </div>

          <div id="global-header-actions" className="flex min-w-0 shrink-0 justify-start empty:hidden xl:justify-end" />
        </div>
      </div>
    </div>
  )
}
