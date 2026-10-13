import { Children, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Responsive as ResponsiveGridLayout, useContainerWidth } from 'react-grid-layout'
import { EyeOff } from 'lucide-react'
import 'react-grid-layout/css/styles.css'
import 'react-resizable/css/styles.css'
import {
  applyEditableGridLayout,
  EDITABLE_WIDGET_GRID_COLS,
  EDITABLE_WIDGET_GRID_MARGIN,
  EDITABLE_WIDGET_GRID_ROW_HEIGHT,
  editableRowsForPixels,
  growEditableLayouts,
  normalizeEditableWidgetLayout,
  setEditableWidgetHidden,
  toEditableResponsiveLayouts,
  visibleEditableWidgetIds,
  type EditableGridLayout,
  type EditableWidgetBounds,
  type EditableWidgetBoundsMap,
  type EditableWidgetDefaults,
  type EditableWidgetLayoutItem,
  type EditableWidgetLayoutState,
} from '@/lib/widget-grid-layout'
import { useWidgetEdit } from '@/lib/widget-edit'
import { cn } from '@/lib/utils'

export type EditableWidgetDefinition<TId extends string = string> = {
  id: TId
  label: string
  layout: Omit<EditableWidgetLayoutItem<TId>, 'id' | 'hidden'> & { hidden?: boolean }
  bounds?: Partial<EditableWidgetBounds>
  content?: ReactNode
  // Widgets grow to their content. A widget that scrolls inside itself, like
  // a chat history, keeps its grid height and sets a phone height here.
  phoneHeight?: string
}

// Below this width the grid is a single column. There a height chosen for a
// row of five cards on a desktop cut the same cards off once they stacked.
const PHONE_MAX_WIDTH = 768

const DEFAULT_BOUNDS: EditableWidgetBounds = {
  minW: 2,
  maxW: 12,
  minH: 3,
  maxH: 24,
}

function loadLayout<TId extends string>(
  storageKey: string,
  ids: readonly TId[],
  defaults: EditableWidgetDefaults<TId>,
  bounds: EditableWidgetBoundsMap<TId>,
): EditableWidgetLayoutState<TId> {
  if (typeof localStorage === 'undefined') {
    return normalizeEditableWidgetLayout({ input: null, ids, defaults, bounds })
  }

  try {
    return normalizeEditableWidgetLayout({
      input: JSON.parse(localStorage.getItem(storageKey) || 'null'),
      ids,
      defaults,
      bounds,
    })
  } catch {
    return normalizeEditableWidgetLayout({ input: null, ids, defaults, bounds })
  }
}

function persistLayout<TId extends string>(
  storageKey: string,
  state: EditableWidgetLayoutState<TId>,
  ids: readonly TId[],
  defaults: EditableWidgetDefaults<TId>,
  bounds: EditableWidgetBoundsMap<TId>,
) {
  localStorage.setItem(storageKey, JSON.stringify(normalizeEditableWidgetLayout({ input: state, ids, defaults, bounds })))
}

export function EditableWidgetGrid<TId extends string>({
  storageKey,
  widgets,
  className,
  emptyLabel = 'Все виджеты скрыты',
  children,
}: {
  storageKey: string
  widgets: Array<EditableWidgetDefinition<TId>>
  className?: string
  emptyLabel?: string
  children?: ReactNode
}) {
  const { editingWidgets } = useWidgetEdit()
  const { width: gridWidth, containerRef: gridContainerRef, mounted: gridMounted } = useContainerWidth({ initialWidth: 1200 })
  const ids = widgets.map(widget => widget.id)
  const defaults = Object.fromEntries(
    widgets.map(widget => [
      widget.id,
      { id: widget.id, ...widget.layout, hidden: widget.layout.hidden ?? false },
    ])
  ) as EditableWidgetDefaults<TId>
  const bounds = Object.fromEntries(
    widgets.map(widget => [
      widget.id,
      { ...DEFAULT_BOUNDS, ...widget.bounds },
    ])
  ) as EditableWidgetBoundsMap<TId>
  const labels = Object.fromEntries(widgets.map(widget => [widget.id, widget.label])) as Record<TId, string>
  const phoneHeights = new Map(widgets.map(widget => [widget.id, widget.phoneHeight]))
  const childContent = Children.toArray(children)
  const widgetContent = new Map(widgets.map((widget, index) => [widget.id, widget.content ?? childContent[index]]))
  const [layout, setLayout] = useState(() => loadLayout(storageKey, ids, defaults, bounds))
  const normalizedLayout = useMemo(
    () => normalizeEditableWidgetLayout({ input: layout, ids, defaults, bounds }),
    [bounds, defaults, ids, layout]
  )

  useEffect(() => {
    persistLayout(storageKey, normalizedLayout, ids, defaults, bounds)
  }, [bounds, defaults, ids, normalizedLayout, storageKey])

  const savedLayouts = useMemo(
    () => toEditableResponsiveLayouts({ ids, layout: normalizedLayout, bounds }),
    [bounds, ids, normalizedLayout]
  )
  const contentRefs = useRef(new Map<string, HTMLDivElement>())
  // Growth only goes up, so it is kept per width and a new width starts over
  // from the saved heights.
  const [grown, setGrown] = useState<{ width: number; rows: Partial<Record<string, number>> }>({ width: 0, rows: {} })
  const grownRows = useMemo(() => grown.width === gridWidth ? grown.rows : {}, [grown, gridWidth])
  const responsiveLayouts = useMemo(
    () => editingWidgets ? savedLayouts : growEditableLayouts(savedLayouts, grownRows),
    [editingWidgets, grownRows, savedLayouts]
  )

  function measureOverflow() {
    if (editingWidgets) return
    setGrown(current => {
      const base = current.width === gridWidth ? current.rows : {}
      let next = base
      contentRefs.current.forEach((content, id) => {
        if (phoneHeights.get(id as TId)) return
        const overflow = content.scrollHeight - content.clientHeight
        const item = content.closest('.react-grid-item')
        if (overflow <= 1 || !item) return
        const rows = editableRowsForPixels(item.getBoundingClientRect().height + overflow)
        if (rows > (next[id] ?? 0)) next = { ...next, [id]: rows }
      })
      return next === base && current.width === gridWidth ? current : { width: gridWidth, rows: next }
    })
  }

  useLayoutEffect(measureOverflow)

  useEffect(() => {
    const observer = new ResizeObserver(() => measureOverflow())
    contentRefs.current.forEach(content => {
      observer.observe(content)
      if (content.firstElementChild) observer.observe(content.firstElementChild)
    })
    return () => observer.disconnect()
  })
  const visibleIds = visibleEditableWidgetIds(ids, normalizedLayout)
  const stacked = gridMounted && gridWidth < PHONE_MAX_WIDTH && !editingWidgets
  const hiddenIds = ids.filter(id => normalizedLayout.widgets[id]?.hidden)

  function handleGridLayoutChange(gridLayout: EditableGridLayout) {
    if (!editingWidgets) return

    setLayout(current => applyEditableGridLayout({
      state: current,
      gridLayout,
      ids,
      defaults,
      bounds,
    }))
  }

  function setHidden(id: TId, hidden: boolean) {
    setLayout(current => setEditableWidgetHidden({
      state: current,
      id,
      hidden,
      ids,
      defaults,
      bounds,
    }))
  }

  return (
    <div className={cn('flex min-w-0 flex-col gap-3', className)}>
      {editingWidgets && hiddenIds.length > 0 ? (
        <div className="flex flex-wrap items-center gap-2 rounded-xl border border-dashed bg-card/60 px-3 py-2">
          <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">Скрытые</span>
          {hiddenIds.map(id => (
            <button
              key={id}
              type="button"
              onClick={() => setHidden(id, false)}
              className="inline-flex items-center gap-1.5 rounded-lg border bg-background/40 px-2.5 py-1.5 text-xs font-medium text-foreground transition-colors hover:bg-muted"
            >
              {labels[id]}
            </button>
          ))}
        </div>
      ) : null}

      {visibleIds.length > 0 ? (
        <div ref={gridContainerRef} className="min-w-0">
          {stacked ? (
            <div className="flex min-w-0 flex-col gap-4">
              {visibleIds.map(id => {
                const height = phoneHeights.get(id)
                return (
                  <section
                    key={id}
                    data-widget-id={id}
                    className="dashboard-grid-widget min-w-0"
                    style={height ? { height } : undefined}
                  >
                    <div className={cn('dashboard-widget-content min-w-0', height && 'h-full overflow-y-auto')}>
                      {widgetContent.get(id)}
                    </div>
                  </section>
                )
              })}
            </div>
          ) : gridMounted ? (
            <ResponsiveGridLayout
              className={cn('dashboard-grid', editingWidgets && 'is-editing')}
              width={gridWidth}
              layouts={responsiveLayouts}
              breakpoints={{ lg: 1200, md: 768, sm: 0 }}
              cols={EDITABLE_WIDGET_GRID_COLS}
              rowHeight={EDITABLE_WIDGET_GRID_ROW_HEIGHT}
              margin={[EDITABLE_WIDGET_GRID_MARGIN, EDITABLE_WIDGET_GRID_MARGIN]}
              containerPadding={[0, 0]}
              dragConfig={{
                enabled: editingWidgets,
                bounded: true,
                cancel: '.dashboard-widget-action',
                threshold: 4,
              }}
              resizeConfig={{
                enabled: editingWidgets,
                handles: ['se', 'sw', 'ne', 'nw'],
              }}
              onLayoutChange={handleGridLayoutChange}
            >
              {visibleIds.map(id => (
                <div key={id} className="min-w-0">
                  <section
                    data-widget-id={id}
                    className={cn(
                      'dashboard-grid-widget h-full min-w-0',
                      editingWidgets && 'is-editing cursor-grab active:cursor-grabbing'
                    )}
                  >
                    {editingWidgets ? (
                      <button
                        type="button"
                        onClick={() => setHidden(id, true)}
                        aria-label={`Скрыть ${labels[id]}`}
                        title="Скрыть"
                        className="dashboard-widget-action absolute right-2 top-2 z-30 inline-flex h-8 w-8 items-center justify-center rounded-lg border bg-background/95 text-muted-foreground shadow-sm transition-colors hover:bg-muted hover:text-foreground"
                      >
                        <EyeOff className="h-4 w-4" />
                      </button>
                    ) : null}
                    <div
                      ref={element => {
                        if (element) contentRefs.current.set(id, element)
                        else contentRefs.current.delete(id)
                      }}
                      className="dashboard-widget-content h-full min-w-0 overflow-x-hidden overflow-y-auto"
                    >
                      {widgetContent.get(id)}
                    </div>
                  </section>
                </div>
              ))}
            </ResponsiveGridLayout>
          ) : null}
        </div>
      ) : (
        <div className="rounded-2xl border border-dashed bg-card/60 px-5 py-8 text-center text-sm text-muted-foreground">
          {emptyLabel}
        </div>
      )}
    </div>
  )
}
