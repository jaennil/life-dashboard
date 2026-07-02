import { Children, useEffect, useMemo, useState, type DragEvent, type ReactNode } from 'react'
import { ArrowDown, ArrowUp, EyeOff, GripVertical, RotateCcw } from 'lucide-react'
import { StyledSelect } from '@/components/StyledSelect'
import {
  moveWidgetFlowItem,
  normalizeWidgetFlowLayout,
  shiftWidgetFlowItem,
  updateWidgetFlowItem,
  type WidgetFlowDefaults,
  type WidgetFlowHeight,
  type WidgetFlowLayout,
  type WidgetFlowWidth,
} from '@/lib/widget-flow-layout'
import { useWidgetEdit } from '@/lib/widget-edit'
import { cn } from '@/lib/utils'

export type EditableFlowWidgetDefinition<TId extends string = string> = {
  id: TId
  label: string
  width: WidgetFlowWidth
  height?: WidgetFlowHeight
  hidden?: boolean
  content?: ReactNode
}

const WIDTH_CLASSES: Record<WidgetFlowWidth, string> = {
  1: 'sm:col-span-1 xl:col-span-1',
  2: 'sm:col-span-2 xl:col-span-2',
  3: 'sm:col-span-2 xl:col-span-3',
}

const HEIGHT_CLASSES: Record<WidgetFlowHeight, string> = {
  auto: '',
  compact: 'h-56',
  medium: 'h-96',
  tall: 'h-[36rem]',
}

function loadLayout<TId extends string>(
  storageKey: string,
  ids: readonly TId[],
  defaults: WidgetFlowDefaults<TId>,
): WidgetFlowLayout<TId> {
  if (typeof localStorage === 'undefined') {
    return normalizeWidgetFlowLayout({ input: null, ids, defaults })
  }

  try {
    return normalizeWidgetFlowLayout({
      input: JSON.parse(localStorage.getItem(storageKey) || 'null'),
      ids,
      defaults,
    })
  } catch {
    return normalizeWidgetFlowLayout({ input: null, ids, defaults })
  }
}

export function EditableWidgetFlow<TId extends string>({
  storageKey,
  widgets,
  className,
  emptyLabel = 'Все виджеты скрыты',
  children,
}: {
  storageKey: string
  widgets: Array<EditableFlowWidgetDefinition<TId>>
  className?: string
  emptyLabel?: string
  children?: ReactNode
}) {
  const { editingWidgets } = useWidgetEdit()
  const ids = useMemo(() => widgets.map(widget => widget.id), [widgets])
  const defaults = useMemo(() => Object.fromEntries(widgets.map(widget => [widget.id, {
    id: widget.id,
    width: widget.width,
    height: widget.height ?? 'auto',
    hidden: widget.hidden ?? false,
  }])) as WidgetFlowDefaults<TId>, [widgets])
  const labels = useMemo(
    () => Object.fromEntries(widgets.map(widget => [widget.id, widget.label])) as Record<TId, string>,
    [widgets],
  )
  const childContent = Children.toArray(children)
  const widgetContent = useMemo(
    () => new Map(widgets.map((widget, index) => [widget.id, widget.content ?? childContent[index]])),
    [childContent, widgets],
  )
  const [layout, setLayout] = useState(() => loadLayout(storageKey, ids, defaults))
  const [draggingId, setDraggingId] = useState<TId | null>(null)
  const [dropTargetId, setDropTargetId] = useState<TId | null>(null)
  const normalized = useMemo(
    () => normalizeWidgetFlowLayout({ input: layout, ids, defaults }),
    [defaults, ids, layout],
  )
  const visibleIds = normalized.order.filter(id => !normalized.widgets[id].hidden)
  const hiddenIds = normalized.order.filter(id => normalized.widgets[id].hidden)

  useEffect(() => {
    localStorage.setItem(storageKey, JSON.stringify(normalized))
  }, [normalized, storageKey])

  function updateItem(
    id: TId,
    patch: Partial<{ width: WidgetFlowWidth; height: WidgetFlowHeight; hidden: boolean }>,
  ) {
    setLayout(current => updateWidgetFlowItem({ layout: current, id, patch, ids, defaults }))
  }

  function shiftItem(id: TId, direction: -1 | 1) {
    setLayout(current => shiftWidgetFlowItem({ layout: current, id, direction, ids, defaults }))
  }

  function startDragging(event: DragEvent<HTMLButtonElement>, id: TId) {
    setDraggingId(id)
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', id)
  }

  function dropOn(event: DragEvent<HTMLDivElement>, targetId: TId) {
    event.preventDefault()
    const activeId = draggingId ?? event.dataTransfer.getData('text/plain') as TId
    if ((ids as readonly string[]).includes(activeId)) {
      setLayout(current => moveWidgetFlowItem({ layout: current, activeId, targetId, ids, defaults }))
    }
    setDraggingId(null)
    setDropTargetId(null)
  }

  return (
    <div className="flex min-w-0 flex-col gap-3">
      {editingWidgets ? (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-dashed bg-card/60 px-3 py-2">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">Виджеты</span>
            {hiddenIds.map(id => (
              <button
                key={id}
                type="button"
                onClick={() => updateItem(id, { hidden: false })}
                className="rounded-lg border bg-background/50 px-2.5 py-1.5 text-xs font-medium text-foreground transition-colors hover:bg-muted"
              >
                Показать: {labels[id]}
              </button>
            ))}
          </div>
          <button
            type="button"
            onClick={() => setLayout(normalizeWidgetFlowLayout({ input: null, ids, defaults }))}
            title="Сбросить расположение"
            className="inline-flex h-8 items-center gap-2 rounded-lg border bg-background/50 px-2.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <RotateCcw className="h-3.5 w-3.5" />
            Сбросить
          </button>
        </div>
      ) : null}

      {visibleIds.length > 0 ? (
        <div className={cn('grid min-w-0 grid-cols-1 gap-5 sm:grid-cols-2 xl:grid-cols-3 lg:gap-6', className)}>
          {visibleIds.map((id, index) => {
            const item = normalized.widgets[id]
            const fixedHeight = item.height !== 'auto'

            return (
              <div
                key={id}
                data-widget-id={id}
                onDragOver={event => {
                  if (!editingWidgets || !draggingId || draggingId === id) return
                  event.preventDefault()
                  event.dataTransfer.dropEffect = 'move'
                  setDropTargetId(id)
                }}
                onDragLeave={() => setDropTargetId(current => current === id ? null : current)}
                onDrop={event => dropOn(event, id)}
                className={cn(
                  'flex min-w-0 flex-col',
                  WIDTH_CLASSES[item.width],
                  HEIGHT_CLASSES[item.height],
                  editingWidgets && 'rounded-xl border border-dashed border-primary/35 bg-primary/[0.03] p-2',
                  dropTargetId === id && 'ring-2 ring-primary/60',
                )}
              >
                {editingWidgets ? (
                  <div className="mb-2 flex flex-wrap items-center gap-1.5 rounded-lg border bg-background/90 p-1.5 shadow-sm">
                    <button
                      type="button"
                      draggable
                      onDragStart={event => startDragging(event, id)}
                      onDragEnd={() => {
                        setDraggingId(null)
                        setDropTargetId(null)
                      }}
                      aria-label={`Перетащить ${labels[id]}`}
                      title="Перетащить"
                      className="inline-flex h-8 w-8 cursor-grab items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground active:cursor-grabbing"
                    >
                      <GripVertical className="h-4 w-4" />
                    </button>
                    <span className="min-w-24 flex-1 truncate px-1 text-xs font-medium text-foreground">{labels[id]}</span>
                    <button
                      type="button"
                      onClick={() => shiftItem(id, -1)}
                      disabled={index === 0}
                      aria-label={`Переместить ${labels[id]} выше`}
                      title="Переместить выше"
                      className="inline-flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-30"
                    >
                      <ArrowUp className="h-4 w-4" />
                    </button>
                    <button
                      type="button"
                      onClick={() => shiftItem(id, 1)}
                      disabled={index === visibleIds.length - 1}
                      aria-label={`Переместить ${labels[id]} ниже`}
                      title="Переместить ниже"
                      className="inline-flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-30"
                    >
                      <ArrowDown className="h-4 w-4" />
                    </button>
                    <StyledSelect
                      value={item.width}
                      onChange={event => updateItem(id, { width: Number(event.target.value) as WidgetFlowWidth })}
                      aria-label={`Ширина ${labels[id]}`}
                      title="Ширина виджета"
                      className="h-8 w-[7.5rem] py-1 text-xs"
                    >
                      <option value={1}>1 колонка</option>
                      <option value={2}>2 колонки</option>
                      <option value={3}>3 колонки</option>
                    </StyledSelect>
                    <StyledSelect
                      value={item.height}
                      onChange={event => updateItem(id, { height: event.target.value as WidgetFlowHeight })}
                      aria-label={`Высота ${labels[id]}`}
                      title="Высота виджета"
                      className="h-8 w-[7.5rem] py-1 text-xs"
                    >
                      <option value="auto">Авто</option>
                      <option value="compact">224 px</option>
                      <option value="medium">384 px</option>
                      <option value="tall">576 px</option>
                    </StyledSelect>
                    <button
                      type="button"
                      onClick={() => updateItem(id, { hidden: true })}
                      aria-label={`Скрыть ${labels[id]}`}
                      title="Скрыть"
                      className="inline-flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
                    >
                      <EyeOff className="h-4 w-4" />
                    </button>
                  </div>
                ) : null}
                <div className={cn('min-h-0 min-w-0 flex-1', fixedHeight && 'overflow-y-auto overscroll-contain')}>
                  {widgetContent.get(id)}
                </div>
              </div>
            )
          })}
        </div>
      ) : (
        <div className="rounded-2xl border border-dashed bg-card/60 px-5 py-8 text-center text-sm text-muted-foreground">
          {emptyLabel}
        </div>
      )}
    </div>
  )
}
