import type { Layout, LayoutItem } from 'react-grid-layout'

export const ADAPTIVE_BREAKPOINTS = {
  lg: 1024,
  md: 640,
  sm: 0,
} as const

export const ADAPTIVE_COLS = {
  lg: 48,
  md: 24,
  sm: 1,
} as const

export const ADAPTIVE_ROW_HEIGHT = 8
export const ADAPTIVE_MARGIN = [16, 16] as const

export type AdaptiveBreakpoint = keyof typeof ADAPTIVE_BREAKPOINTS
export type AdaptiveWidgetDefaults<TId extends string = string> = Record<
  TId,
  Record<AdaptiveBreakpoint, LayoutItem>
>

export type AdaptiveWidgetLayoutState<TId extends string = string> = {
  version: 1
  layouts: Record<AdaptiveBreakpoint, Layout>
  hidden: Record<TId, boolean>
  autoHeight: Record<AdaptiveBreakpoint, Record<TId, boolean>>
}

function hasWidgetId<TId extends string>(ids: readonly TId[], value: unknown): value is TId {
  return typeof value === 'string' && (ids as readonly string[]).includes(value)
}

function finiteInteger(value: unknown, fallback: number) {
  return typeof value === 'number' && Number.isFinite(value) ? Math.round(value) : fallback
}

function normalizeItem<TId extends string>(
  id: TId,
  input: Partial<LayoutItem> | undefined,
  fallback: LayoutItem,
  cols: number,
): LayoutItem {
  const minW = Math.max(1, Math.min(cols, finiteInteger(fallback.minW, 1)))
  const maxW = Math.max(minW, Math.min(cols, finiteInteger(fallback.maxW, cols)))
  const minH = Math.max(1, finiteInteger(fallback.minH, 1))
  const maxH = Math.max(minH, finiteInteger(fallback.maxH, 200))
  const w = Math.max(minW, Math.min(maxW, finiteInteger(input?.w, fallback.w)))
  const h = Math.max(minH, Math.min(maxH, finiteInteger(input?.h, fallback.h)))

  return {
    i: id,
    x: Math.max(0, Math.min(cols - w, finiteInteger(input?.x, fallback.x))),
    y: Math.max(0, finiteInteger(input?.y, fallback.y)),
    w,
    h,
    minW,
    maxW,
    minH,
    maxH,
  }
}

export function normalizeAdaptiveWidgetLayout<TId extends string>({
  input,
  ids,
  defaults,
}: {
  input: unknown
  ids: readonly TId[]
  defaults: AdaptiveWidgetDefaults<TId>
}): AdaptiveWidgetLayoutState<TId> {
  const source = input && typeof input === 'object' ? input as {
    layouts?: unknown
    hidden?: unknown
    autoHeight?: unknown
  } : {}
  const sourceLayouts = source.layouts && typeof source.layouts === 'object'
    ? source.layouts as Partial<Record<AdaptiveBreakpoint, unknown>>
    : {}
  const sourceHidden = source.hidden && typeof source.hidden === 'object'
    ? source.hidden as Record<string, unknown>
    : {}
  const sourceAutoHeight = source.autoHeight && typeof source.autoHeight === 'object'
    ? source.autoHeight as Partial<Record<AdaptiveBreakpoint, unknown>>
    : {}

  const layouts = {} as Record<AdaptiveBreakpoint, Layout>
  for (const breakpoint of Object.keys(ADAPTIVE_BREAKPOINTS) as AdaptiveBreakpoint[]) {
    const rawItems = Array.isArray(sourceLayouts[breakpoint])
      ? sourceLayouts[breakpoint] as Array<Partial<LayoutItem>>
      : []
    const byId = new Map(rawItems.filter(item => hasWidgetId(ids, item.i)).map(item => [item.i as TId, item]))
    layouts[breakpoint] = ids.map(id => normalizeItem(
      id,
      byId.get(id),
      defaults[id][breakpoint],
      ADAPTIVE_COLS[breakpoint],
    ))
  }

  const hidden = Object.fromEntries(ids.map(id => [
    id,
    typeof sourceHidden[id] === 'boolean' ? sourceHidden[id] : false,
  ])) as Record<TId, boolean>

  const autoHeight = Object.fromEntries(
    (Object.keys(ADAPTIVE_BREAKPOINTS) as AdaptiveBreakpoint[]).map(breakpoint => {
      const sourceBreakpoint = sourceAutoHeight[breakpoint] && typeof sourceAutoHeight[breakpoint] === 'object'
        ? sourceAutoHeight[breakpoint] as Record<string, unknown>
        : {}
      return [breakpoint, Object.fromEntries(ids.map(id => [
        id,
        typeof sourceBreakpoint[id] === 'boolean' ? sourceBreakpoint[id] : true,
      ]))]
    }),
  ) as Record<AdaptiveBreakpoint, Record<TId, boolean>>

  return { version: 1, layouts, hidden, autoHeight }
}

export function mergeAdaptiveBreakpointLayout<TId extends string>({
  state,
  breakpoint,
  layout,
  ids,
  defaults,
}: {
  state: AdaptiveWidgetLayoutState<TId>
  breakpoint: AdaptiveBreakpoint
  layout: Layout
  ids: readonly TId[]
  defaults: AdaptiveWidgetDefaults<TId>
}): AdaptiveWidgetLayoutState<TId> {
  const normalized = normalizeAdaptiveWidgetLayout({ input: state, ids, defaults })
  const updates = new Map(layout.filter(item => hasWidgetId(ids, item.i)).map(item => [item.i as TId, item]))
  return normalizeAdaptiveWidgetLayout({
    input: {
      ...normalized,
      layouts: {
        ...normalized.layouts,
        [breakpoint]: normalized.layouts[breakpoint].map(item => updates.get(item.i as TId) ?? item),
      },
    },
    ids,
    defaults,
  })
}

export function updateAdaptiveWidget<TId extends string>({
  state,
  id,
  breakpoint,
  hidden,
  autoHeight,
  height,
  ids,
  defaults,
}: {
  state: AdaptiveWidgetLayoutState<TId>
  id: TId
  breakpoint?: AdaptiveBreakpoint
  hidden?: boolean
  autoHeight?: boolean
  height?: number
  ids: readonly TId[]
  defaults: AdaptiveWidgetDefaults<TId>
}): AdaptiveWidgetLayoutState<TId> {
  const normalized = normalizeAdaptiveWidgetLayout({ input: state, ids, defaults })
  const layouts = breakpoint && height !== undefined
    ? {
        ...normalized.layouts,
        [breakpoint]: normalized.layouts[breakpoint].map(item => item.i === id ? { ...item, h: height } : item),
      }
    : normalized.layouts
  const nextAutoHeight = breakpoint && autoHeight !== undefined
    ? {
        ...normalized.autoHeight,
        [breakpoint]: { ...normalized.autoHeight[breakpoint], [id]: autoHeight },
      }
    : normalized.autoHeight

  return normalizeAdaptiveWidgetLayout({
    input: {
      ...normalized,
      layouts,
      hidden: hidden === undefined ? normalized.hidden : { ...normalized.hidden, [id]: hidden },
      autoHeight: nextAutoHeight,
    },
    ids,
    defaults,
  })
}

export function pixelsToAdaptiveRows(pixelHeight: number) {
  const [, marginY] = ADAPTIVE_MARGIN
  return Math.max(1, Math.ceil((pixelHeight + marginY) / (ADAPTIVE_ROW_HEIGHT + marginY)))
}
