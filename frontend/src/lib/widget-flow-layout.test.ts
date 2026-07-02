import { describe, expect, it } from 'vitest'
import {
  moveWidgetFlowItem,
  normalizeWidgetFlowLayout,
  shiftWidgetFlowItem,
  updateWidgetFlowItem,
  type WidgetFlowDefaults,
} from './widget-flow-layout'

const ids = ['balance', 'trends', 'accounts'] as const
type WidgetId = (typeof ids)[number]

const defaults: WidgetFlowDefaults<WidgetId> = {
  balance: { id: 'balance', width: 1, height: 'auto', hidden: false },
  trends: { id: 'trends', width: 3, height: 'auto', hidden: false },
  accounts: { id: 'accounts', width: 3, height: 'auto', hidden: false },
}

function normalize(input: unknown) {
  return normalizeWidgetFlowLayout({ input, ids, defaults })
}

describe('widget flow layout', () => {
  it('repairs invalid persistence and appends new widgets', () => {
    const layout = normalize({
      order: ['accounts', 'unknown', 'accounts'],
      widgets: {
        accounts: { width: 2, height: 'compact', hidden: true },
        balance: { width: 9, height: 'huge', hidden: false },
      },
    })

    expect(layout.order).toEqual(['accounts', 'balance', 'trends'])
    expect(layout.widgets.accounts).toMatchObject({ width: 2, height: 'compact', hidden: true })
    expect(layout.widgets.balance).toEqual(defaults.balance)
  })

  it('moves a dragged widget before its drop target', () => {
    const layout = moveWidgetFlowItem({
      layout: normalize(null),
      activeId: 'accounts',
      targetId: 'balance',
      ids,
      defaults,
    })

    expect(layout.order).toEqual(['accounts', 'balance', 'trends'])
  })

  it('swaps visible neighbours while skipping hidden widgets', () => {
    const hidden = updateWidgetFlowItem({
      layout: normalize(null),
      id: 'trends',
      patch: { hidden: true },
      ids,
      defaults,
    })
    const shifted = shiftWidgetFlowItem({
      layout: hidden,
      id: 'accounts',
      direction: -1,
      ids,
      defaults,
    })

    expect(shifted.order).toEqual(['accounts', 'trends', 'balance'])
  })

  it('updates width, height and visibility within valid values', () => {
    const layout = updateWidgetFlowItem({
      layout: normalize(null),
      id: 'trends',
      patch: { width: 2, height: 'tall', hidden: true },
      ids,
      defaults,
    })

    expect(layout.widgets.trends).toMatchObject({ width: 2, height: 'tall', hidden: true })
  })
})
