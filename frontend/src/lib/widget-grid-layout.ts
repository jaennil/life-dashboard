import type { Layout, LayoutItem } from 'react-grid-layout'

export const EDITABLE_WIDGET_GRID_COLS = {
  lg: 12,
  md: 8,
  sm: 1,
} as const

export const EDITABLE_WIDGET_GRID_ROW_HEIGHT = 32

export type EditableWidgetBounds = {
  minW: number
  maxW: number
  minH: number
  maxH: number
}

export type EditableWidgetLayoutItem<TId extends string = string> = {
  id: TId
  x: number
  y: number
  w: number
  h: number
  hidden: boolean
}

export type EditableWidgetLayoutState<TId extends string = string> = {
  widgets: Record<TId, EditableWidgetLayoutItem<TId>>
}

export type EditableWidgetDefaults<TId extends string = string> = Record<TId, EditableWidgetLayoutItem<TId>>
export type EditableWidgetBoundsMap<TId extends string = string> = Record<TId, EditableWidgetBounds>
export type EditableGridLayout = Layout
export type EditableGridLayoutItem = LayoutItem
export type EditableGridLayouts = Record<keyof typeof EDITABLE_WIDGET_GRID_COLS, EditableGridLayout>

function clamp(value: number, min: number, max: number) {
  return Math.max(min, Math.min(max, value))
}

function coerceInteger(value: unknown, fallback: number) {
  return typeof value === 'number' && Number.isFinite(value) ? Math.round(value) : fallback
}

function hasWidgetId<TId extends string>(ids: readonly TId[], id: string): id is TId {
  return (ids as readonly string[]).includes(id)
}

export function normalizeEditableWidgetLayout<TId extends string>({
  input,
  ids,
  defaults,
  bounds,
}: {
  input: unknown
  ids: readonly TId[]
  defaults: EditableWidgetDefaults<TId>
  bounds: EditableWidgetBoundsMap<TId>
}): EditableWidgetLayoutState<TId> {
  const maybeWidgets =
    input && typeof input === 'object' && 'widgets' in input
      ? (input as { widgets?: unknown }).widgets
      : undefined

  const sourceWidgets = maybeWidgets && typeof maybeWidgets === 'object'
    ? maybeWidgets as Record<string, Partial<EditableWidgetLayoutItem<TId>>>
    : {}

  const widgets = Object.fromEntries(
    ids.map(id => {
      const fallback = { ...defaults[id] }
      const source = sourceWidgets[id]
      if (!source || !hasWidgetId(ids, String(source.id ?? id))) {
        return [id, fallback]
      }

      const widgetBounds = bounds[id]
      const w = clamp(coerceInteger(source.w, fallback.w), widgetBounds.minW, widgetBounds.maxW)
      const h = clamp(coerceInteger(source.h, fallback.h), widgetBounds.minH, widgetBounds.maxH)
      const maxX = Math.max(0, EDITABLE_WIDGET_GRID_COLS.lg - w)

      return [id, {
        id,
        x: clamp(coerceInteger(source.x, fallback.x), 0, maxX),
        y: Math.max(0, coerceInteger(source.y, fallback.y)),
        w,
        h,
        hidden: typeof source.hidden === 'boolean' ? source.hidden : fallback.hidden,
      }]
    })
  ) as Record<TId, EditableWidgetLayoutItem<TId>>

  return { widgets }
}

export function visibleEditableWidgetIds<TId extends string>(
  ids: readonly TId[],
  layout: EditableWidgetLayoutState<TId>,
): TId[] {
  return ids
    .filter(id => !layout.widgets[id].hidden)
    .sort((a, b) => {
      const widgetA = layout.widgets[a]
      const widgetB = layout.widgets[b]
      if (widgetA.y !== widgetB.y) return widgetA.y - widgetB.y
      return widgetA.x - widgetB.x
    })
}

function toGridItem<TId extends string>(
  widget: EditableWidgetLayoutItem<TId>,
  bounds: EditableWidgetBoundsMap<TId>,
  cols: number,
): EditableGridLayoutItem {
  const widgetBounds = bounds[widget.id]
  const w = clamp(widget.w, Math.min(widgetBounds.minW, cols), Math.min(widgetBounds.maxW, cols))
  const x = clamp(widget.x, 0, Math.max(0, cols - w))

  return {
    i: widget.id,
    x,
    y: widget.y,
    w,
    h: widget.h,
    minW: Math.min(widgetBounds.minW, cols),
    maxW: Math.min(widgetBounds.maxW, cols),
    minH: widgetBounds.minH,
    maxH: widgetBounds.maxH,
  }
}

function overlaps(a: EditableGridLayoutItem, b: EditableGridLayoutItem) {
  return a.x < b.x + b.w
    && a.x + a.w > b.x
    && a.y < b.y + b.h
    && a.y + a.h > b.y
}

function packGridItems<TId extends string>(
  widgets: Array<EditableWidgetLayoutItem<TId>>,
  bounds: EditableWidgetBoundsMap<TId>,
  cols: number,
): EditableGridLayout {
  const placed: EditableGridLayoutItem[] = []

  widgets.forEach(widget => {
    const item = toGridItem(widget, bounds, cols)
    let y = 0

    while (true) {
      for (let x = 0; x <= cols - item.w; x += 1) {
        const candidate = { ...item, x, y }
        if (!placed.some(placedItem => overlaps(candidate, placedItem))) {
          placed.push(candidate)
          return
        }
      }
      y += 1
    }
  })

  return placed
}

export function toEditableResponsiveLayouts<TId extends string>({
  ids,
  layout,
  bounds,
}: {
  ids: readonly TId[]
  layout: EditableWidgetLayoutState<TId>
  bounds: EditableWidgetBoundsMap<TId>
}): EditableGridLayouts {
  const visible = visibleEditableWidgetIds(ids, layout).map(id => layout.widgets[id])

  return {
    lg: visible.map(widget => toGridItem(widget, bounds, EDITABLE_WIDGET_GRID_COLS.lg)),
    md: packGridItems(visible, bounds, EDITABLE_WIDGET_GRID_COLS.md),
    sm: visible.map((widget, index) => ({
      ...toGridItem(widget, bounds, EDITABLE_WIDGET_GRID_COLS.sm),
      x: 0,
      y: index * widget.h,
      w: 1,
      minW: 1,
      maxW: 1,
    })),
  }
}

export function applyEditableGridLayout<TId extends string>({
  state,
  gridLayout,
  ids,
  defaults,
  bounds,
}: {
  state: EditableWidgetLayoutState<TId>
  gridLayout: EditableGridLayout
  ids: readonly TId[]
  defaults: EditableWidgetDefaults<TId>
  bounds: EditableWidgetBoundsMap<TId>
}): EditableWidgetLayoutState<TId> {
  const normalized = normalizeEditableWidgetLayout({ input: state, ids, defaults, bounds })
  const widgets = { ...normalized.widgets }

  for (const item of gridLayout) {
    if (!hasWidgetId(ids, item.i)) continue

    const current = widgets[item.i]
    const widgetBounds = bounds[item.i]
    const w = clamp(coerceInteger(item.w, current.w), widgetBounds.minW, widgetBounds.maxW)
    const h = clamp(coerceInteger(item.h, current.h), widgetBounds.minH, widgetBounds.maxH)

    widgets[item.i] = {
      ...current,
      x: clamp(coerceInteger(item.x, current.x), 0, Math.max(0, EDITABLE_WIDGET_GRID_COLS.lg - w)),
      y: Math.max(0, coerceInteger(item.y, current.y)),
      w,
      h,
    }
  }

  return normalizeEditableWidgetLayout({ input: { widgets }, ids, defaults, bounds })
}

export function setEditableWidgetHidden<TId extends string>({
  state,
  id,
  hidden,
  ids,
  defaults,
  bounds,
}: {
  state: EditableWidgetLayoutState<TId>
  id: TId
  hidden: boolean
  ids: readonly TId[]
  defaults: EditableWidgetDefaults<TId>
  bounds: EditableWidgetBoundsMap<TId>
}): EditableWidgetLayoutState<TId> {
  const normalized = normalizeEditableWidgetLayout({ input: state, ids, defaults, bounds })
  return {
    widgets: {
      ...normalized.widgets,
      [id]: { ...normalized.widgets[id], hidden },
    },
  }
}
