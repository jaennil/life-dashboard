import { describe, expect, it } from 'vitest'
import {
  mergeAdaptiveBreakpointLayout,
  normalizeAdaptiveWidgetLayout,
  pixelsToAdaptiveRows,
  updateAdaptiveWidget,
  type AdaptiveWidgetDefaults,
} from './adaptive-widget-layout'

const ids = ['balance', 'trends'] as const
type WidgetId = (typeof ids)[number]

const defaults: AdaptiveWidgetDefaults<WidgetId> = {
  balance: {
    lg: { i: 'balance', x: 0, y: 0, w: 16, h: 6, minW: 8, maxW: 48, minH: 4, maxH: 80 },
    md: { i: 'balance', x: 0, y: 0, w: 12, h: 6, minW: 6, maxW: 24, minH: 4, maxH: 80 },
    sm: { i: 'balance', x: 0, y: 0, w: 1, h: 6, minW: 1, maxW: 1, minH: 4, maxH: 80 },
  },
  trends: {
    lg: { i: 'trends', x: 0, y: 6, w: 48, h: 18, minW: 12, maxW: 48, minH: 8, maxH: 120 },
    md: { i: 'trends', x: 0, y: 6, w: 24, h: 18, minW: 8, maxW: 24, minH: 8, maxH: 120 },
    sm: { i: 'trends', x: 0, y: 6, w: 1, h: 18, minW: 1, maxW: 1, minH: 8, maxH: 120 },
  },
}

function normalize(input: unknown) {
  return normalizeAdaptiveWidgetLayout({ input, ids, defaults })
}

describe('adaptive widget layout', () => {
  it('creates complete independent breakpoint layouts', () => {
    const state = normalize({
      layouts: {
        lg: [{ i: 'balance', x: 47, y: -1, w: 99, h: 1 }],
      },
    })

    expect(state.layouts.lg[0]).toMatchObject({ i: 'balance', x: 0, y: 0, w: 48, h: 4 })
    expect(state.layouts.md).toHaveLength(2)
    expect(state.layouts.sm.every(item => item.x === 0 && item.w === 1)).toBe(true)
  })

  it('merges only the active breakpoint geometry', () => {
    const state = normalize(null)
    const next = mergeAdaptiveBreakpointLayout({
      state,
      breakpoint: 'lg',
      layout: [{ i: 'balance', x: 16, y: 4, w: 24, h: 10 }],
      ids,
      defaults,
    })

    expect(next.layouts.lg[0]).toMatchObject({ x: 16, y: 4, w: 24, h: 10 })
    expect(next.layouts.md[0]).toMatchObject({ x: 0, y: 0, w: 12, h: 6 })
  })

  it('tracks hidden and manual height independently', () => {
    const state = updateAdaptiveWidget({
      state: normalize(null),
      id: 'trends',
      breakpoint: 'lg',
      hidden: true,
      autoHeight: false,
      height: 30,
      ids,
      defaults,
    })

    expect(state.hidden.trends).toBe(true)
    expect(state.autoHeight.lg.trends).toBe(false)
    expect(state.autoHeight.md.trends).toBe(true)
    expect(state.layouts.lg[1].h).toBe(30)
  })

  it('converts measured pixels to grid rows', () => {
    expect(pixelsToAdaptiveRows(128)).toBe(6)
    expect(pixelsToAdaptiveRows(129)).toBe(7)
  })
})
