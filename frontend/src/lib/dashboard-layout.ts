import type { Layout, LayoutItem } from 'react-grid-layout'

export const DASHBOARD_WIDGETS = [
  'weather',
  'balance',
  'spending',
  'activities',
  'workouts',
  'nutrition',
  'overdue',
  'overview',
  'transactions',
] as const

export type DashboardWidgetId = (typeof DASHBOARD_WIDGETS)[number]

export type DashboardLayoutWidget = {
  id: DashboardWidgetId
  x: number
  y: number
  w: number
  h: number
  hidden: boolean
}

export type DashboardLayoutState = {
  widgets: Record<DashboardWidgetId, DashboardLayoutWidget>
}

export type DashboardGridLayout = Layout
export type DashboardGridLayoutItem = LayoutItem
export type DashboardGridLayouts = Record<keyof typeof DASHBOARD_GRID_COLS, DashboardGridLayout>

type WidgetBounds = {
  minW: number
  maxW: number
  minH: number
  maxH: number
}

export const DASHBOARD_GRID_COLS = {
  lg: 12,
  md: 8,
  sm: 1,
} as const

export const DASHBOARD_GRID_ROW_HEIGHT = 32

export const DASHBOARD_WIDGET_BOUNDS: Record<DashboardWidgetId, WidgetBounds> = {
  weather: { minW: 3, maxW: 8, minH: 4, maxH: 12 },
  balance: { minW: 2, maxW: 6, minH: 2, maxH: 8 },
  spending: { minW: 2, maxW: 6, minH: 2, maxH: 8 },
  activities: { minW: 2, maxW: 6, minH: 2, maxH: 8 },
  workouts: { minW: 2, maxW: 6, minH: 2, maxH: 8 },
  nutrition: { minW: 2, maxW: 6, minH: 2, maxH: 8 },
  overdue: { minW: 2, maxW: 6, minH: 2, maxH: 8 },
  overview: { minW: 4, maxW: 12, minH: 6, maxH: 18 },
  transactions: { minW: 3, maxW: 8, minH: 3, maxH: 12 },
}

const COMPACT_STAT_WIDGETS = new Set<DashboardWidgetId>([
  'balance',
  'spending',
  'activities',
  'workouts',
  'nutrition',
  'overdue',
])

export const DASHBOARD_WIDGET_LABELS: Record<DashboardWidgetId, string> = {
  weather: 'Погода',
  balance: 'Баланс',
  spending: 'Расходы за месяц',
  activities: 'Активности',
  workouts: 'Тренировки',
  nutrition: 'Питание',
  overdue: 'Overdue',
  overview: 'Срез по разделам',
  transactions: 'Транзакции',
}

export const DEFAULT_DASHBOARD_LAYOUT: DashboardLayoutState = {
  widgets: {
    weather: { id: 'weather', x: 0, y: 0, w: 4, h: 8, hidden: false },
    balance: { id: 'balance', x: 4, y: 0, w: 2, h: 3, hidden: false },
    spending: { id: 'spending', x: 6, y: 0, w: 3, h: 3, hidden: false },
    activities: { id: 'activities', x: 9, y: 0, w: 3, h: 3, hidden: false },
    workouts: { id: 'workouts', x: 4, y: 3, w: 2, h: 3, hidden: false },
    nutrition: { id: 'nutrition', x: 6, y: 3, w: 3, h: 3, hidden: false },
    overdue: { id: 'overdue', x: 9, y: 3, w: 3, h: 3, hidden: false },
    overview: { id: 'overview', x: 0, y: 8, w: 8, h: 14, hidden: false },
    transactions: { id: 'transactions', x: 8, y: 8, w: 4, h: 5, hidden: false },
  },
}

function isDashboardWidgetId(id: string): id is DashboardWidgetId {
  return (DASHBOARD_WIDGETS as readonly string[]).includes(id)
}

function clamp(value: number, min: number, max: number) {
  return Math.max(min, Math.min(max, value))
}

function coerceInteger(value: unknown, fallback: number) {
  return typeof value === 'number' && Number.isFinite(value) ? Math.round(value) : fallback
}

function cloneDefaultWidget(id: DashboardWidgetId): DashboardLayoutWidget {
  return { ...DEFAULT_DASHBOARD_LAYOUT.widgets[id] }
}

export function normalizeDashboardLayout(input: unknown): DashboardLayoutState {
  const maybeWidgets =
    input && typeof input === 'object' && 'widgets' in input
      ? (input as { widgets?: unknown }).widgets
      : undefined

  const sourceWidgets = maybeWidgets && typeof maybeWidgets === 'object'
    ? maybeWidgets as Record<string, Partial<DashboardLayoutWidget>>
    : {}

  const widgets = Object.fromEntries(
    DASHBOARD_WIDGETS.map(id => {
      const fallback = cloneDefaultWidget(id)
      const source = sourceWidgets[id]
      if (!source || !isDashboardWidgetId(String(source.id ?? id))) {
        return [id, fallback]
      }

      const bounds = DASHBOARD_WIDGET_BOUNDS[id]
      const w = clamp(coerceInteger(source.w, fallback.w), bounds.minW, bounds.maxW)
      const h = clamp(coerceInteger(source.h, fallback.h), bounds.minH, bounds.maxH)
      const maxX = Math.max(0, DASHBOARD_GRID_COLS.lg - w)

      return [id, {
        id,
        x: clamp(coerceInteger(source.x, fallback.x), 0, maxX),
        y: Math.max(0, coerceInteger(source.y, fallback.y)),
        w,
        h,
        hidden: typeof source.hidden === 'boolean' ? source.hidden : fallback.hidden,
      }]
    })
  ) as Record<DashboardWidgetId, DashboardLayoutWidget>

  return { widgets }
}

export function visibleWidgetIds(layout: DashboardLayoutState): DashboardWidgetId[] {
  return DASHBOARD_WIDGETS
    .filter(id => !layout.widgets[id].hidden)
    .sort((a, b) => {
      const widgetA = layout.widgets[a]
      const widgetB = layout.widgets[b]
      if (widgetA.y !== widgetB.y) return widgetA.y - widgetB.y
      return widgetA.x - widgetB.x
    })
}

function toGridItem(widget: DashboardLayoutWidget, cols: number): DashboardGridLayoutItem {
  const bounds = DASHBOARD_WIDGET_BOUNDS[widget.id]
  const preferredWidth = cols < DASHBOARD_GRID_COLS.lg && COMPACT_STAT_WIDGETS.has(widget.id)
    ? bounds.minW
    : widget.w
  const w = clamp(preferredWidth, Math.min(bounds.minW, cols), Math.min(bounds.maxW, cols))
  const x = clamp(widget.x, 0, Math.max(0, cols - w))

  return {
    i: widget.id,
    x,
    y: widget.y,
    w,
    h: widget.h,
    minW: Math.min(bounds.minW, cols),
    maxW: Math.min(bounds.maxW, cols),
    minH: bounds.minH,
    maxH: bounds.maxH,
  }
}

function overlaps(a: DashboardGridLayoutItem, b: DashboardGridLayoutItem) {
  return a.x < b.x + b.w
    && a.x + a.w > b.x
    && a.y < b.y + b.h
    && a.y + a.h > b.y
}

function packGridItems(widgets: DashboardLayoutWidget[], cols: number): DashboardGridLayout {
  const placed: DashboardGridLayoutItem[] = []

  widgets.forEach(widget => {
    const item = toGridItem(widget, cols)
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

export function toResponsiveLayouts(layout: DashboardLayoutState): DashboardGridLayouts {
  const visible = visibleWidgetIds(layout).map(id => layout.widgets[id])

  return {
    lg: visible.map(widget => toGridItem(widget, DASHBOARD_GRID_COLS.lg)),
    md: packGridItems(visible, DASHBOARD_GRID_COLS.md),
    sm: visible.map((widget, index) => ({
      ...toGridItem(widget, DASHBOARD_GRID_COLS.sm),
      x: 0,
      y: index * widget.h,
      w: 1,
      minW: 1,
      maxW: 1,
    })),
  }
}

export function applyGridLayout(
  state: DashboardLayoutState,
  gridLayout: DashboardGridLayout,
): DashboardLayoutState {
  const normalized = normalizeDashboardLayout(state)
  const widgets = { ...normalized.widgets }

  for (const item of gridLayout) {
    if (!isDashboardWidgetId(item.i)) continue

    const current = widgets[item.i]
    const bounds = DASHBOARD_WIDGET_BOUNDS[item.i]
    const w = clamp(coerceInteger(item.w, current.w), bounds.minW, bounds.maxW)
    const h = clamp(coerceInteger(item.h, current.h), bounds.minH, bounds.maxH)

    widgets[item.i] = {
      ...current,
      x: clamp(coerceInteger(item.x, current.x), 0, Math.max(0, DASHBOARD_GRID_COLS.lg - w)),
      y: Math.max(0, coerceInteger(item.y, current.y)),
      w,
      h,
    }
  }

  return normalizeDashboardLayout({ widgets })
}

export function setWidgetHidden(
  state: DashboardLayoutState,
  id: DashboardWidgetId,
  hidden: boolean,
): DashboardLayoutState {
  const normalized = normalizeDashboardLayout(state)
  return {
    widgets: {
      ...normalized.widgets,
      [id]: { ...normalized.widgets[id], hidden },
    },
  }
}
