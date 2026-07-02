import {
  Children,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { EyeOff } from 'lucide-react'
import {
  Responsive as ResponsiveGridLayout,
  useContainerWidth,
  verticalCompactor,
  type Layout,
  type LayoutItem,
  type ResponsiveLayouts,
} from 'react-grid-layout'
import 'react-grid-layout/css/styles.css'
import 'react-resizable/css/styles.css'
import {
  ADAPTIVE_BREAKPOINTS,
  ADAPTIVE_COLS,
  ADAPTIVE_MARGIN,
  ADAPTIVE_ROW_HEIGHT,
  mergeAdaptiveBreakpointLayout,
  normalizeAdaptiveWidgetLayout,
  pixelsToAdaptiveRows,
  updateAdaptiveWidget,
  type AdaptiveBreakpoint,
  type AdaptiveWidgetDefaults,
  type AdaptiveWidgetLayoutState,
} from '@/lib/adaptive-widget-layout'
import { useWidgetEdit } from '@/lib/widget-edit'
import { cn } from '@/lib/utils'

export type AdaptiveWidgetDefinition<TId extends string = string> = {
  id: TId
  label: string
  layouts: Record<AdaptiveBreakpoint, LayoutItem>
  content?: ReactNode
}

function loadLayout<TId extends string>(
  storageKey: string,
  ids: readonly TId[],
  defaults: AdaptiveWidgetDefaults<TId>,
): AdaptiveWidgetLayoutState<TId> {
  if (typeof localStorage === 'undefined') {
    return normalizeAdaptiveWidgetLayout({ input: null, ids, defaults })
  }

  try {
    return normalizeAdaptiveWidgetLayout({
      input: JSON.parse(localStorage.getItem(storageKey) || 'null'),
      ids,
      defaults,
    })
  } catch {
    return normalizeAdaptiveWidgetLayout({ input: null, ids, defaults })
  }
}

function MeasuredWidget({
  id,
  label,
  editing,
  autoHeight,
  onMeasure,
  onHide,
  children,
}: {
  id: string
  label: string
  editing: boolean
  autoHeight: boolean
  onMeasure: (id: string, pixelHeight: number) => void
  onHide: () => void
  children: ReactNode
}) {
  const measureRef = useRef<HTMLDivElement | null>(null)
  const onMeasureRef = useRef(onMeasure)

  useLayoutEffect(() => {
    onMeasureRef.current = onMeasure
  }, [onMeasure])

  useLayoutEffect(() => {
    if (!autoHeight || !measureRef.current) return
    const element = measureRef.current
    let frame = 0
    const report = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => {
        const pixelHeight = Math.ceil(Math.max(element.scrollHeight, element.getBoundingClientRect().height))
        if (pixelHeight > 0) onMeasureRef.current(id, pixelHeight)
      })
    }
    const observer = new ResizeObserver(report)
    observer.observe(element)
    report()
    return () => {
      cancelAnimationFrame(frame)
      observer.disconnect()
    }
  }, [autoHeight, id])

  return (
    <section
      data-widget-id={id}
      className={cn(
        'dashboard-grid-widget adaptive-grid-widget h-full min-w-0',
        editing && 'is-editing',
      )}
    >
      {editing ? (
        <button
          type="button"
          onClick={onHide}
          aria-label={`Скрыть ${label}`}
          title="Скрыть"
          className="dashboard-widget-action absolute right-2 top-2 z-40 inline-flex h-8 w-8 items-center justify-center rounded-lg border bg-background/95 text-muted-foreground shadow-sm transition-colors hover:bg-muted hover:text-foreground"
        >
          <EyeOff className="h-4 w-4" />
        </button>
      ) : null}
      <div className={cn(
        'dashboard-widget-content h-full min-h-0 min-w-0',
        autoHeight ? 'overflow-visible' : 'overflow-auto overscroll-contain',
      )}>
        <div ref={measureRef} className="adaptive-widget-measure min-w-0">
          {children}
        </div>
      </div>
    </section>
  )
}

export function AdaptiveWidgetGrid<TId extends string>({
  storageKey,
  widgets,
  className,
  emptyLabel = 'Все виджеты скрыты',
  children,
}: {
  storageKey: string
  widgets: Array<AdaptiveWidgetDefinition<TId>>
  className?: string
  emptyLabel?: string
  children?: ReactNode
}) {
  const { editingWidgets, registerWidgetReset } = useWidgetEdit()
  const { width, containerRef, mounted } = useContainerWidth({ initialWidth: 1280 })
  const ids = useMemo(() => widgets.map(widget => widget.id), [widgets])
  const defaults = useMemo(() => Object.fromEntries(
    widgets.map(widget => [widget.id, widget.layouts]),
  ) as AdaptiveWidgetDefaults<TId>, [widgets])
  const labels = useMemo(
    () => Object.fromEntries(widgets.map(widget => [widget.id, widget.label])) as Record<TId, string>,
    [widgets],
  )
  const childContent = Children.toArray(children)
  const widgetContent = useMemo(
    () => new Map(widgets.map((widget, index) => [widget.id, widget.content ?? childContent[index]])),
    [childContent, widgets],
  )
  const [state, setState] = useState(() => loadLayout(storageKey, ids, defaults))
  const [breakpoint, setBreakpoint] = useState<AdaptiveBreakpoint>('lg')
  const stateRef = useRef(state)
  const breakpointRef = useRef<AdaptiveBreakpoint>('lg')
  const resizingIdRef = useRef<TId | null>(null)

  function applyState(next: AdaptiveWidgetLayoutState<TId>, persist: boolean) {
    stateRef.current = next
    setState(next)
    if (persist) localStorage.setItem(storageKey, JSON.stringify(next))
  }

  function mergeLayout(layout: Layout, persist: boolean) {
    applyState(mergeAdaptiveBreakpointLayout({
      state: stateRef.current,
      breakpoint: breakpointRef.current,
      layout,
      ids,
      defaults,
    }), persist)
  }

  function updateWidget(
    id: TId,
    options: { hidden?: boolean; autoHeight?: boolean; height?: number },
    persist = true,
  ) {
    applyState(updateAdaptiveWidget({
      state: stateRef.current,
      id,
      breakpoint: breakpointRef.current,
      ids,
      defaults,
      ...options,
    }), persist)
  }

  function handleMeasure(rawId: string, pixelHeight: number) {
    const id = rawId as TId
    const breakpoint = breakpointRef.current
    if (resizingIdRef.current === id || !stateRef.current.autoHeight[breakpoint][id]) return
    const height = pixelsToAdaptiveRows(pixelHeight)
    const currentItem = stateRef.current.layouts[breakpoint].find(item => item.i === id)
    if (!currentItem || currentItem.h === height) return
    applyState(updateAdaptiveWidget({
      state: stateRef.current,
      id,
      breakpoint,
      height,
      ids,
      defaults,
    }), true)
  }

  const responsiveLayouts = useMemo(() => Object.fromEntries(
    (Object.keys(ADAPTIVE_BREAKPOINTS) as AdaptiveBreakpoint[]).map(breakpoint => [
      breakpoint,
      state.layouts[breakpoint].filter(item => !state.hidden[item.i as TId]),
    ]),
  ) as ResponsiveLayouts<AdaptiveBreakpoint>, [state])
  const hiddenIds = ids.filter(id => state.hidden[id])
  const visibleIds = ids.filter(id => !state.hidden[id])

  useEffect(() => registerWidgetReset(() => {
    const next = normalizeAdaptiveWidgetLayout({ input: null, ids, defaults })
    stateRef.current = next
    setState(next)
    localStorage.setItem(storageKey, JSON.stringify(next))
  }), [defaults, ids, registerWidgetReset, storageKey])

  return (
    <div className="flex min-w-0 flex-col gap-3">
      {editingWidgets && hiddenIds.length > 0 ? (
        <div className="flex flex-wrap items-center gap-2 rounded-xl border border-dashed bg-card/60 px-3 py-2">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">Скрытые</span>
            {hiddenIds.map(id => (
              <button
                key={id}
                type="button"
                onClick={() => updateWidget(id, { hidden: false })}
                className="rounded-lg border bg-background/50 px-2.5 py-1.5 text-xs font-medium text-foreground transition-colors hover:bg-muted"
              >
                Показать: {labels[id]}
              </button>
            ))}
          </div>
        </div>
      ) : null}

      {visibleIds.length > 0 ? (
        <div ref={containerRef} className="min-w-0">
          {mounted ? (
            <ResponsiveGridLayout<AdaptiveBreakpoint>
              className={cn('dashboard-grid adaptive-widget-grid', editingWidgets && 'is-editing', className)}
              width={width}
              layouts={responsiveLayouts}
              breakpoints={ADAPTIVE_BREAKPOINTS}
              cols={ADAPTIVE_COLS}
              rowHeight={ADAPTIVE_ROW_HEIGHT}
              margin={ADAPTIVE_MARGIN}
              containerPadding={[0, 0]}
              compactor={verticalCompactor}
              dragConfig={{
                enabled: editingWidgets,
                bounded: true,
                cancel: '.dashboard-widget-action',
                threshold: 4,
              }}
              resizeConfig={{
                enabled: editingWidgets,
                handles: ['n', 's', 'e', 'w', 'ne', 'nw', 'se', 'sw'],
              }}
              onBreakpointChange={(breakpoint) => {
                breakpointRef.current = breakpoint
                setBreakpoint(breakpoint)
              }}
              onLayoutChange={(layout) => {
                if (editingWidgets && !resizingIdRef.current) mergeLayout(layout, false)
              }}
              onDragStop={(layout) => mergeLayout(layout, true)}
              onResizeStart={(_layout, _oldItem, newItem) => {
                resizingIdRef.current = newItem?.i as TId ?? null
              }}
              onResizeStop={(layout, oldItem, newItem) => {
                let next = mergeAdaptiveBreakpointLayout({
                  state: stateRef.current,
                  breakpoint: breakpointRef.current,
                  layout,
                  ids,
                  defaults,
                })
                const id = newItem?.i as TId | undefined
                if (id && oldItem && newItem && oldItem.h !== newItem.h) {
                  next = updateAdaptiveWidget({
                    state: next,
                    id,
                    breakpoint: breakpointRef.current,
                    autoHeight: false,
                    ids,
                    defaults,
                  })
                }
                resizingIdRef.current = null
                applyState(next, true)
              }}
            >
              {visibleIds.map(id => (
                <div key={id} className="min-w-0">
                  <MeasuredWidget
                    id={id}
                    label={labels[id]}
                    editing={editingWidgets}
                    autoHeight={state.autoHeight[breakpoint][id]}
                    onMeasure={handleMeasure}
                    onHide={() => updateWidget(id, { hidden: true })}
                  >
                    {widgetContent.get(id)}
                  </MeasuredWidget>
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
