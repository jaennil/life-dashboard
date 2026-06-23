import { describe, expect, it } from 'vitest'
import {
  DEFAULT_DASHBOARD_LAYOUT,
  DASHBOARD_GRID_COLS,
  DASHBOARD_WIDGETS,
  normalizeDashboardLayout,
  toResponsiveLayouts,
  visibleWidgetIds,
  type DashboardLayoutState,
} from './dashboard-layout'

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

describe('dashboard layout model', () => {
  it('normalizes legacy saved layouts into the full widget set', () => {
    const legacy = {
      widgets: {
        weather: { id: 'weather', x: 0, y: 0, w: 4, h: 4, hidden: false },
        balance: { id: 'balance', x: 4, y: 0, w: 2, h: 2, hidden: true },
        unknown: { id: 'unknown', x: 0, y: 0, w: 20, h: 20, hidden: false },
      },
    } as unknown as DashboardLayoutState

    const normalized = normalizeDashboardLayout(legacy)

    expect(Object.keys(normalized.widgets).sort()).toEqual([...DASHBOARD_WIDGETS].sort())
    expect(normalized.widgets.balance.hidden).toBe(true)
    expect(normalized.widgets.balance.w).toBe(2)
    expect(normalized.widgets.spending.hidden).toBe(false)
    expect(normalized.widgets.overview.w).toBe(DEFAULT_DASHBOARD_LAYOUT.widgets.overview.w)
    expect('unknown' in normalized.widgets).toBe(false)
  })

  it('clamps widget sizes to the configured min and max bounds', () => {
    const normalized = normalizeDashboardLayout({
      widgets: {
        balance: { id: 'balance', x: -10, y: -1, w: 50, h: 0, hidden: false },
        weather: { id: 'weather', x: 0, y: 0, w: 1, h: 1, hidden: false },
      },
    } as unknown as DashboardLayoutState)

    expect(normalized.widgets.balance).toMatchObject({ x: 0, y: 0, w: 6, h: 2 })
    expect(normalized.widgets.weather).toMatchObject({ w: 3, h: 4 })
  })

  it('converts only visible widgets to responsive grid layouts', () => {
    const normalized = normalizeDashboardLayout({
      widgets: {
        balance: { ...DEFAULT_DASHBOARD_LAYOUT.widgets.balance, hidden: true },
        spending: { ...DEFAULT_DASHBOARD_LAYOUT.widgets.spending, x: 4, y: 1, w: 3, h: 3 },
      },
    } as DashboardLayoutState)

    const layouts = toResponsiveLayouts(normalized)

    expect(layouts.lg.map(item => item.i)).not.toContain('balance')
    expect(layouts.lg.find(item => item.i === 'spending')).toMatchObject({ x: 4, y: 1, w: 3, h: 3 })
    expect(layouts.md.find(item => item.i === 'spending')).toMatchObject({ w: 2, h: 3 })
    expect(layouts.md.every(item => item.x + item.w <= DASHBOARD_GRID_COLS.md)).toBe(true)
    expectNoOverlaps(layouts.md)
    expect(layouts.sm.every(item => item.x === 0 && item.w === 1)).toBe(true)
  })

  it('packs the default medium dashboard without staggered gaps', () => {
    const layouts = toResponsiveLayouts(DEFAULT_DASHBOARD_LAYOUT)

    expect(layouts.md.find(item => item.i === 'weather')).toMatchObject({ x: 0, y: 0, w: 6, h: 10 })
    expect(layouts.md.find(item => item.i === 'balance')).toMatchObject({ x: 6, y: 0, w: 2, h: 4 })
    expect(layouts.md.filter(item => [
      'balance',
      'spending',
      'activities',
      'workouts',
      'nutrition',
      'overdue',
    ].includes(item.i)).every(item => item.w === 2)).toBe(true)
    expectNoOverlaps(layouts.md)

    const occupiedHeight = Math.max(...layouts.md.map(item => item.y + item.h))
    for (let row = 0; row < occupiedHeight; row += 1) {
      expect(layouts.md.some(item => item.y <= row && item.y + item.h > row)).toBe(true)
    }
  })

  it('orders visible widget ids by grid position', () => {
    const normalized = normalizeDashboardLayout({
      widgets: {
        spending: { ...DEFAULT_DASHBOARD_LAYOUT.widgets.spending, x: 6, y: 0 },
        balance: { ...DEFAULT_DASHBOARD_LAYOUT.widgets.balance, x: 3, y: 0 },
        weather: { ...DEFAULT_DASHBOARD_LAYOUT.widgets.weather, x: 0, y: 0 },
        overdue: { ...DEFAULT_DASHBOARD_LAYOUT.widgets.overdue, hidden: true },
      },
    } as DashboardLayoutState)

    expect(visibleWidgetIds(normalized).slice(0, 3)).toEqual(['weather', 'balance', 'spending'])
    expect(visibleWidgetIds(normalized)).not.toContain('overdue')
  })
})
