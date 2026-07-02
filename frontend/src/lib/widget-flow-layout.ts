export type WidgetFlowWidth = 1 | 2 | 3
export type WidgetFlowHeight = 'auto' | number

export const WIDGET_FLOW_MIN_HEIGHT = 160
export const WIDGET_FLOW_MAX_HEIGHT = 1600

export type WidgetFlowItem<TId extends string = string> = {
  id: TId
  width: WidgetFlowWidth
  height: WidgetFlowHeight
  hidden: boolean
}

export type WidgetFlowDefaults<TId extends string = string> = Record<TId, WidgetFlowItem<TId>>

export type WidgetFlowLayout<TId extends string = string> = {
  order: TId[]
  widgets: Record<TId, WidgetFlowItem<TId>>
}

function hasWidgetId<TId extends string>(ids: readonly TId[], value: unknown): value is TId {
  return typeof value === 'string' && (ids as readonly string[]).includes(value)
}

function normalizeWidth(value: unknown, fallback: WidgetFlowWidth): WidgetFlowWidth {
  return value === 1 || value === 2 || value === 3 ? value : fallback
}

function normalizeHeight(value: unknown, fallback: WidgetFlowHeight): WidgetFlowHeight {
  if (value === 'auto') return value
  if (value === 'compact') return 224
  if (value === 'medium') return 384
  if (value === 'tall') return 576
  if (typeof value === 'number' && Number.isFinite(value)) {
    return Math.max(WIDGET_FLOW_MIN_HEIGHT, Math.min(WIDGET_FLOW_MAX_HEIGHT, Math.round(value)))
  }
  return fallback
}

export function normalizeWidgetFlowLayout<TId extends string>({
  input,
  ids,
  defaults,
}: {
  input: unknown
  ids: readonly TId[]
  defaults: WidgetFlowDefaults<TId>
}): WidgetFlowLayout<TId> {
  const source = input && typeof input === 'object' ? input as {
    order?: unknown
    widgets?: unknown
  } : {}
  const sourceOrder = Array.isArray(source.order) ? source.order : []
  const order = sourceOrder.reduce<TId[]>((result, value) => {
    if (hasWidgetId(ids, value) && !result.includes(value)) result.push(value)
    return result
  }, [])

  ids.forEach(id => {
    if (!order.includes(id)) order.push(id)
  })

  const sourceWidgets = source.widgets && typeof source.widgets === 'object'
    ? source.widgets as Record<string, Partial<WidgetFlowItem<TId>>>
    : {}
  const widgets = Object.fromEntries(ids.map(id => {
    const fallback = defaults[id]
    const item = sourceWidgets[id]
    return [id, {
      id,
      width: normalizeWidth(item?.width, fallback.width),
      height: normalizeHeight(item?.height, fallback.height),
      hidden: typeof item?.hidden === 'boolean' ? item.hidden : fallback.hidden,
    }]
  })) as Record<TId, WidgetFlowItem<TId>>

  return { order, widgets }
}

export function moveWidgetFlowItem<TId extends string>({
  layout,
  activeId,
  targetId,
  ids,
  defaults,
}: {
  layout: WidgetFlowLayout<TId>
  activeId: TId
  targetId: TId
  ids: readonly TId[]
  defaults: WidgetFlowDefaults<TId>
}): WidgetFlowLayout<TId> {
  const normalized = normalizeWidgetFlowLayout({ input: layout, ids, defaults })
  if (activeId === targetId) return normalized

  const order = normalized.order.filter(id => id !== activeId)
  const targetIndex = order.indexOf(targetId)
  order.splice(targetIndex < 0 ? order.length : targetIndex, 0, activeId)
  return { ...normalized, order }
}

export function shiftWidgetFlowItem<TId extends string>({
  layout,
  id,
  direction,
  ids,
  defaults,
}: {
  layout: WidgetFlowLayout<TId>
  id: TId
  direction: -1 | 1
  ids: readonly TId[]
  defaults: WidgetFlowDefaults<TId>
}): WidgetFlowLayout<TId> {
  const normalized = normalizeWidgetFlowLayout({ input: layout, ids, defaults })
  const visibleOrder = normalized.order.filter(widgetId => !normalized.widgets[widgetId].hidden)
  const visibleIndex = visibleOrder.indexOf(id)
  const targetId = visibleOrder[visibleIndex + direction]
  if (!targetId) return normalized

  const order = [...normalized.order]
  const sourceIndex = order.indexOf(id)
  const targetIndex = order.indexOf(targetId)
  ;[order[sourceIndex], order[targetIndex]] = [order[targetIndex], order[sourceIndex]]
  return { ...normalized, order }
}

export function updateWidgetFlowItem<TId extends string>({
  layout,
  id,
  patch,
  ids,
  defaults,
}: {
  layout: WidgetFlowLayout<TId>
  id: TId
  patch: Partial<Pick<WidgetFlowItem<TId>, 'width' | 'height' | 'hidden'>>
  ids: readonly TId[]
  defaults: WidgetFlowDefaults<TId>
}): WidgetFlowLayout<TId> {
  const normalized = normalizeWidgetFlowLayout({ input: layout, ids, defaults })
  return normalizeWidgetFlowLayout({
    input: {
      ...normalized,
      widgets: {
        ...normalized.widgets,
        [id]: { ...normalized.widgets[id], ...patch },
      },
    },
    ids,
    defaults,
  })
}
