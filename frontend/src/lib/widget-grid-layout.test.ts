import { describe, expect, it } from 'vitest'
import {
  applyEditableGridLayout,
  EDITABLE_WIDGET_GRID_COLS,
  editableRowsForPixels,
  growEditableLayouts,
  normalizeEditableWidgetLayout,
  setEditableWidgetHidden,
  toEditableResponsiveLayouts,
  visibleEditableWidgetIds,
  type EditableWidgetBoundsMap,
  type EditableWidgetDefaults,
} from './widget-grid-layout'

function layoutsOverlap(
  a: { x: number; y: number; w: number; h: number },
  b: { x: number; y: number; w: number; h: number },
) {
  return a.x < b.x + b.w
    && a.x + a.w > b.x
    && a.y < b.y + b.h
    && a.y + a.h > b.y
}

function expectNoOverlaps(items: ReadonlyArray<{ x: number; y: number; w: number; h: number }>) {
  items.forEach((item, index) => {
    items.slice(index + 1).forEach(other => {
      expect(layoutsOverlap(item, other)).toBe(false)
    })
  })
}

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
    expect(responsive.md.find(item => item.i === 'beta')).toMatchObject({ x: 0, y: 0, w: 3, h: 5 })
    expect(responsive.md.every(item => item.x + item.w <= EDITABLE_WIDGET_GRID_COLS.md)).toBe(true)
    expectNoOverlaps(responsive.md)
    expect(responsive.sm.every(item => item.x === 0 && item.w === 1)).toBe(true)
  })

  it('keeps a full row side by side on the medium grid', () => {
    const layout = normalize({
      widgets: {
        alpha: { ...defaults.alpha, x: 0, y: 0, w: 5, h: 5 },
        beta: { ...defaults.beta, x: 5, y: 0, w: 7, h: 10 },
        gamma: { ...defaults.gamma, hidden: true },
      },
    })

    const md = toEditableResponsiveLayouts({ ids, layout, bounds }).md

    expect(md.find(item => item.i === 'alpha')).toMatchObject({ x: 0, y: 0, w: 3 })
    expect(md.find(item => item.i === 'beta')).toMatchObject({ x: 3, y: 0, w: 5 })
  })

  it('converts a content height to grid rows', () => {
    // 5 rows are 5 * 32 + 4 * 16 = 224 px.
    expect(editableRowsForPixels(224)).toBe(5)
    expect(editableRowsForPixels(225)).toBe(6)
  })

  it('grows widgets to their content but never shrinks them', () => {
    const layouts = toEditableResponsiveLayouts({ ids, layout: normalize(null), bounds })

    const grown = growEditableLayouts(layouts, { alpha: 9, beta: 2 })

    expect(grown.lg.find(item => item.i === 'alpha')).toMatchObject({ h: 9 })
    expect(grown.md.find(item => item.i === 'alpha')).toMatchObject({ h: 9 })
    expect(grown.lg.find(item => item.i === 'beta')).toMatchObject({ h: 5 })
    expect(grown.lg.find(item => item.i === 'gamma')).toMatchObject({ h: 3 })
  })

  it('stacks one column in order, each widget under the previous one', () => {
    const layout = normalize({
      widgets: {
        alpha: { ...defaults.alpha, x: 0, y: 0, w: 12, h: 5 },
        beta: { ...defaults.beta, x: 0, y: 5, w: 12, h: 8 },
        gamma: { ...defaults.gamma, x: 0, y: 13, w: 12, h: 3 },
      },
    })

    const sm = toEditableResponsiveLayouts({ ids, layout, bounds }).sm

    expect(sm.map(item => [item.i, item.y])).toEqual([['alpha', 0], ['beta', 5], ['gamma', 13]])
    expectNoOverlaps(sm)
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
