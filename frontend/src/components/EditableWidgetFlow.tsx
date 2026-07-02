import {
  Children,
  useEffect,
  useMemo,
  useState,
  type DragEvent,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
} from 'react'
import { ArrowDown, ArrowUp, EyeOff, GripVertical, RotateCcw } from 'lucide-react'
import { StyledSelect } from '@/components/StyledSelect'
import {
  moveWidgetFlowItem,
  normalizeWidgetFlowLayout,
  shiftWidgetFlowItem,
  updateWidgetFlowItem,
  WIDGET_FLOW_MAX_HEIGHT,
  WIDGET_FLOW_MIN_HEIGHT,
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
  const [resizingId, setResizingId] = useState<TId | null>(null)
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

  function startResizing(
    event: ReactPointerEvent<HTMLButtonElement>,
    id: TId,
    horizontalDirection: -1 | 0 | 1,
    verticalDirection: -1 | 0 | 1,
  ) {
    event.preventDefault()
    event.stopPropagation()
    const widget = event.currentTarget.closest<HTMLElement>('[data-widget-id]')
    const grid = event.currentTarget.closest<HTMLElement>('[data-widget-flow-grid]')
    if (!widget || !grid) return

    const startX = event.clientX
    const startY = event.clientY
    const startRect = widget.getBoundingClientRect()
    const gridRect = grid.getBoundingClientRect()
    const columnCount = window.innerWidth >= 1280 ? 3 : window.innerWidth >= 640 ? 2 : 1
    const columnGap = Number.parseFloat(getComputedStyle(grid).columnGap) || 0
    const columnWidth = (gridRect.width - columnGap * (columnCount - 1)) / columnCount
    const columnStep = columnWidth + columnGap
    setResizingId(id)

    function resize(moveEvent: PointerEvent) {
      const patch: Partial<{ width: WidgetFlowWidth; height: WidgetFlowHeight }> = {}
      if (horizontalDirection !== 0 && columnCount > 1) {
        const pixelWidth = startRect.width + (moveEvent.clientX - startX) * horizontalDirection
        patch.width = Math.max(1, Math.min(columnCount, Math.round((pixelWidth + columnGap) / columnStep))) as WidgetFlowWidth
      }
      if (verticalDirection !== 0) {
        const pixelHeight = startRect.height + (moveEvent.clientY - startY) * verticalDirection
        patch.height = Math.max(WIDGET_FLOW_MIN_HEIGHT, Math.min(WIDGET_FLOW_MAX_HEIGHT, Math.round(pixelHeight)))
      }
      setLayout(current => updateWidgetFlowItem({ layout: current, id, patch, ids, defaults }))
    }

    function stopResizing() {
      window.removeEventListener('pointermove', resize)
      window.removeEventListener('pointerup', stopResizing)
      window.removeEventListener('pointercancel', stopResizing)
      setResizingId(null)
    }

    window.addEventListener('pointermove', resize)
    window.addEventListener('pointerup', stopResizing)
    window.addEventListener('pointercancel', stopResizing)
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
        <div data-widget-flow-grid className={cn('grid min-w-0 grid-cols-1 gap-5 sm:grid-cols-2 xl:grid-cols-3 lg:gap-6', className)}>
          {visibleIds.map((id, index) => {
            const item = normalized.widgets[id]
            const fixedHeight = typeof item.height === 'number'

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
                style={{ height: fixedHeight ? item.height : undefined }}
                className={cn(
                  'relative flex min-w-0 flex-col',
                  WIDTH_CLASSES[item.width],
                  editingWidgets && 'rounded-xl border border-dashed border-primary/35 bg-primary/[0.03] p-2',
                  dropTargetId === id && 'ring-2 ring-primary/60',
                  resizingId === id && 'ring-2 ring-primary/70',
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
                      onChange={event => updateItem(id, {
                        height: event.target.value === 'auto' ? 'auto' : Number(event.target.value),
                      })}
                      aria-label={`Высота ${labels[id]}`}
                      title="Высота виджета"
                      className="h-8 w-[7.5rem] py-1 text-xs"
                    >
                      <option value="auto">Авто</option>
                      <option value={224}>224 px</option>
                      <option value={384}>384 px</option>
                      <option value={576}>576 px</option>
                      {fixedHeight && ![224, 384, 576].includes(item.height as number) ? (
                        <option value={item.height}>{item.height} px</option>
                      ) : null}
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
                {editingWidgets ? (
                  <>
                    <button
                      type="button"
                      onPointerDown={event => startResizing(event, id, -1, 0)}
                      aria-label={`Изменить ширину ${labels[id]} слева`}
                      title="Потянуть левую границу"
                      className="absolute -left-1 top-11 bottom-3 z-30 w-3 cursor-col-resize touch-none"
                    >
                      <span className="absolute left-1 top-0 h-full w-px bg-primary/30" />
                    </button>
                    <button
                      type="button"
                      onPointerDown={event => startResizing(event, id, 1, 0)}
                      aria-label={`Изменить ширину ${labels[id]} справа`}
                      title="Потянуть правую границу"
                      className="absolute -right-1 top-11 bottom-3 z-30 w-3 cursor-col-resize touch-none"
                    >
                      <span className="absolute right-1 top-0 h-full w-px bg-primary/30" />
                    </button>
                    <button
                      type="button"
                      onPointerDown={event => startResizing(event, id, 0, -1)}
                      aria-label={`Изменить высоту ${labels[id]} сверху`}
                      title="Потянуть верхнюю границу"
                      className="absolute -top-1 left-3 right-3 z-30 h-3 cursor-row-resize touch-none"
                    >
                      <span className="absolute left-0 top-1 h-px w-full bg-primary/30" />
                    </button>
                    <button
                      type="button"
                      onPointerDown={event => startResizing(event, id, 0, 1)}
                      aria-label={`Изменить высоту ${labels[id]} снизу`}
                      title="Потянуть нижнюю границу"
                      className="absolute -bottom-1 left-3 right-3 z-30 h-3 cursor-row-resize touch-none"
                    >
                      <span className="absolute bottom-1 left-0 h-px w-full bg-primary/30" />
                    </button>
                    <button
                      type="button"
                      onPointerDown={event => startResizing(event, id, 1, 1)}
                      aria-label={`Изменить размер ${labels[id]} за угол`}
                      title="Потянуть угол"
                      className="absolute -bottom-1 -right-1 z-40 h-4 w-4 cursor-nwse-resize touch-none rounded-sm border border-primary/50 bg-background"
                    />
                  </>
                ) : null}
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
