import { useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { CalendarRange, ChevronLeft, ChevronRight, Pencil, RotateCcw, X } from 'lucide-react'
import { StyledSelect } from '@/components/StyledSelect'
import { useWidgetEdit } from '@/lib/widget-edit'
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
  const { editingWidgets, toggleWidgetEditing } = useWidgetEdit()
  const [customOpen, setCustomOpen] = useState(false)
  const [draftFrom, setDraftFrom] = useState('')
  const [draftTo, setDraftTo] = useState('')
  const customRef = useRef<HTMLDivElement | null>(null)
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

  useEffect(() => {
    if (!customOpen) return

    function close(event: MouseEvent) {
      if (!customRef.current?.contains(event.target as Node)) setCustomOpen(false)
    }

    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === 'Escape') setCustomOpen(false)
    }

    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('mousedown', close)
      document.removeEventListener('keydown', closeOnEscape)
    }
  }, [customOpen])

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

  function openCustom() {
    setDraftFrom(selectedFrom)
    setDraftTo(selectedTo)
    setCustomOpen(true)
  }

  function applyCustom() {
    if (!draftFrom || !draftTo || draftFrom > draftTo) return
    updateRange({ mode: 'custom', year, month, from: draftFrom, to: draftTo })
    setCustomOpen(false)
  }

  function rangeLabel() {
    const format = (value: string) => value.split('-').reverse().join('.')
    return `${format(selectedFrom)} — ${format(selectedTo)}`
  }

  return (
    <div className="sticky top-[calc(4.75rem+env(safe-area-inset-top))] z-30 mx-auto mb-4 w-full max-w-[1560px] lg:top-0 lg:mb-5">
      <div className="relative flex flex-wrap items-center justify-center gap-2 rounded-lg border bg-card/95 p-2 shadow-sm backdrop-blur">
        <div className="flex min-w-0 flex-wrap items-center justify-center gap-2">
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

          {mode === 'month' ? (
            <div className="flex items-center gap-2">
              <button
                onClick={() => moveMonth(-1)}
                className="flex h-10 w-10 items-center justify-center rounded-lg border bg-background text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                aria-label="Предыдущий месяц"
              >
                <ChevronLeft className="h-4 w-4" />
              </button>
              <StyledSelect
                value={month}
                onChange={event => applyMonth(year, Number(event.target.value))}
                className="w-32"
                aria-label="Месяц"
              >
                {MONTHS.map(item => (
                  <option key={item.value} value={item.value}>{item.label}</option>
                ))}
              </StyledSelect>
              <StyledSelect
                value={year}
                onChange={event => applyMonth(Number(event.target.value), month)}
                className="w-24"
                aria-label="Год"
              >
                {years.map(item => (
                  <option key={item} value={item}>{item}</option>
                ))}
              </StyledSelect>
              <button
                onClick={() => moveMonth(1)}
                className="flex h-10 w-10 items-center justify-center rounded-lg border bg-background text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                aria-label="Следующий месяц"
              >
                <ChevronRight className="h-4 w-4" />
              </button>
            </div>
          ) : null}

          {mode === 'year' ? (
            <div className="flex items-center gap-2">
              <button onClick={() => applyYear(year - 1)} className="flex h-10 w-10 items-center justify-center rounded-lg border bg-background text-muted-foreground hover:bg-accent hover:text-foreground" aria-label="Предыдущий год">
                <ChevronLeft className="h-4 w-4" />
              </button>
              <StyledSelect value={year} onChange={event => applyYear(Number(event.target.value))} className="w-24" aria-label="Год">
                {years.map(item => <option key={item} value={item}>{item}</option>)}
              </StyledSelect>
              <button onClick={() => applyYear(year + 1)} className="flex h-10 w-10 items-center justify-center rounded-lg border bg-background text-muted-foreground hover:bg-accent hover:text-foreground" aria-label="Следующий год">
                <ChevronRight className="h-4 w-4" />
              </button>
            </div>
          ) : null}

          <div ref={customRef} className="relative">
            <button
              type="button"
              onClick={openCustom}
              aria-expanded={customOpen}
              className={cn(
                'inline-flex h-10 items-center gap-2 rounded-lg border bg-background px-3 text-sm transition-colors hover:bg-accent',
                mode === 'custom' ? 'border-primary/40 text-primary' : 'text-muted-foreground hover:text-foreground',
              )}
            >
              <CalendarRange className="h-4 w-4" />
              <span className="hidden sm:inline">{rangeLabel()}</span>
              <span className="sm:hidden">Даты</span>
            </button>
            {customOpen ? (
              <div className="absolute right-0 top-full z-50 mt-2 w-[min(22rem,calc(100vw-2rem))] rounded-lg border bg-card p-3 shadow-xl">
                <div className="mb-3 flex items-center justify-between gap-3">
                  <p className="text-sm font-medium text-foreground">Произвольный период</p>
                  <button type="button" onClick={() => setCustomOpen(false)} className="text-muted-foreground hover:text-foreground" aria-label="Закрыть выбор периода"><X className="h-4 w-4" /></button>
                </div>
                <div className="grid grid-cols-2 gap-2">
                  <label className="text-xs text-muted-foreground">От<input type="date" value={draftFrom} onChange={event => setDraftFrom(event.target.value)} className="mt-1 h-10 w-full rounded-lg border bg-background px-2 text-sm text-foreground outline-none focus:border-primary" /></label>
                  <label className="text-xs text-muted-foreground">До<input type="date" value={draftTo} onChange={event => setDraftTo(event.target.value)} className="mt-1 h-10 w-full rounded-lg border bg-background px-2 text-sm text-foreground outline-none focus:border-primary" /></label>
                </div>
                <div className="mt-3 flex justify-end gap-2">
                  <button type="button" onClick={() => { reset(); setCustomOpen(false) }} className="inline-flex h-9 items-center gap-2 rounded-lg border px-3 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"><RotateCcw className="h-3.5 w-3.5" />Сбросить</button>
                  <button type="button" onClick={applyCustom} disabled={!draftFrom || !draftTo || draftFrom > draftTo} className="h-9 rounded-lg bg-primary px-3 text-xs font-medium text-primary-foreground disabled:opacity-50">Применить</button>
                </div>
              </div>
            ) : null}
          </div>

          <div className="flex min-w-0 shrink-0 flex-wrap items-center justify-start gap-2 xl:justify-end">
            <div id="global-header-actions" className="contents" />
            <button
              type="button"
              onClick={toggleWidgetEditing}
              aria-pressed={editingWidgets}
              className={cn(
                'inline-flex h-10 items-center gap-2 rounded-lg border px-3 text-sm font-medium transition-colors',
                editingWidgets
                  ? 'border-primary/40 bg-primary text-primary-foreground hover:bg-primary/90'
                  : 'bg-background text-muted-foreground hover:bg-accent hover:text-foreground'
              )}
            >
              <Pencil className="h-4 w-4" />
              {editingWidgets ? 'Готово' : 'Edit'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
