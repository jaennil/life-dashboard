import { describe, expect, it } from 'vitest'
import {
  applyEditableGridLayout,
  normalizeEditableWidgetLayout,
  setEditableWidgetHidden,
  toEditableResponsiveLayouts,
  visibleEditableWidgetIds,
  type EditableWidgetBoundsMap,
  type EditableWidgetDefaults,
} from './widget-grid-layout'

const ids = ['alpha', 'beta', 'gamma'] as const
type WidgetId = (typeof ids)[number]

const defaults: EditableWidgetDefaults<WidgetId> = {
  alpha: { id: 'alpha', x: 0, y: 0, w: 4, h: 4, hidden: false },
  beta: { id: 'beta', x: 4, y: 0, w: 4, h: 5, hidden: false },
  gamma: { id: 'gamma', x: 8, y: 0, w: 4, h: 3, hidden: false },
}

const bounds: EditableWidgetBoundsMap<WidgetId> = {
  alpha: { minW: 2, maxW: 8, minH: 3, maxH: 10 },
  beta: { minW: 3, maxW: 12, minH: 4, maxH: 14 },
  gamma: { minW: 2, maxW: 6, minH: 2, maxH: 8 },
}

function normalize(input: unknown) {
  return normalizeEditableWidgetLayout({ input, ids, defaults, bounds })
}

describe('editable widget layout model', () => {
  it('normalizes saved layouts into the full configured widget set', () => {
    const normalized = normalize({
      widgets: {
        alpha: { id: 'alpha', x: 1, y: 2, w: 5, h: 6, hidden: true },
        unknown: { id: 'unknown', x: 0, y: 0, w: 30, h: 30, hidden: false },
      },
    })

    expect(Object.keys(normalized.widgets).sort()).toEqual([...ids].sort())
    expect(normalized.widgets.alpha).toMatchObject({ x: 1, y: 2, w: 5, h: 6, hidden: true })
    expect(normalized.widgets.beta).toEqual(defaults.beta)
    expect('unknown' in normalized.widgets).toBe(false)
  })

  it('clamps positions and sizes to widget bounds', () => {
    const normalized = normalize({
      widgets: {
        alpha: { id: 'alpha', x: -10, y: -1, w: 50, h: 1, hidden: false },
        gamma: { id: 'gamma', x: 11, y: 2, w: 5, h: 20, hidden: false },
      },
    })

    expect(normalized.widgets.alpha).toMatchObject({ x: 0, y: 0, w: 8, h: 3 })
    expect(normalized.widgets.gamma).toMatchObject({ x: 7, y: 2, w: 5, h: 8 })
  })

  it('creates responsive layouts for visible widgets only', () => {
    const layout = normalize({
      widgets: {
        alpha: { ...defaults.alpha, hidden: true },
        beta: { ...defaults.beta, x: 6, y: 0, w: 4, h: 5 },
      },
    })

    const responsive = toEditableResponsiveLayouts({ ids, layout, bounds })

    expect(responsive.lg.map(item => item.i)).toEqual(['beta', 'gamma'])
    expect(responsive.lg.find(item => item.i === 'beta')).toMatchObject({ x: 6, y: 0, w: 4, h: 5 })
    expect(responsive.md.find(item => item.i === 'beta')).toMatchObject({ x: 4, w: 4 })
    expect(responsive.sm.every(item => item.x === 0 && item.w === 1)).toBe(true)
  })

  it('applies drag and resize updates while preserving hidden state', () => {
    const state = normalize({
      widgets: {
        beta: { ...defaults.beta, hidden: true },
      },
    })

    const next = applyEditableGridLayout({
      state,
      ids,
      defaults,
      bounds,
      gridLayout: [
        { i: 'alpha', x: 8, y: 7, w: 2, h: 3 },
        { i: 'beta', x: 0, y: 3, w: 12, h: 14 },
        { i: 'unknown', x: 0, y: 0, w: 12, h: 12 },
      ],
    })

    expect(next.widgets.alpha).toMatchObject({ x: 8, y: 7, w: 2, h: 3 })
    expect(next.widgets.beta).toMatchObject({ x: 0, y: 3, w: 12, h: 14, hidden: true })
    expect(visibleEditableWidgetIds(ids, next)).toEqual(['gamma', 'alpha'])
  })

  it('hides and restores widgets without changing their grid geometry', () => {
    const state = normalize({ widgets: { gamma: { ...defaults.gamma, x: 2, y: 9 } } })
    const hidden = setEditableWidgetHidden({ state, id: 'gamma', hidden: true, ids, defaults, bounds })
    const restored = setEditableWidgetHidden({ state: hidden, id: 'gamma', hidden: false, ids, defaults, bounds })

    expect(hidden.widgets.gamma).toMatchObject({ x: 2, y: 9, hidden: true })
    expect(restored.widgets.gamma).toMatchObject({ x: 2, y: 9, hidden: false })
  })
})
