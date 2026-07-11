import { describe, expect, it } from 'vitest'
import {
  isTooltipRecord,
  readTooltipNumber,
  readTooltipScalar,
  readTooltipString,
  toTooltipList,
  toTooltipNumber,
} from './echarts-tooltip'

describe('ECharts tooltip values', () => {
  it.each([
    [42, 42],
    [-2.5, -2.5],
    ['42', 42],
    ['-2.5', -2.5],
    ['', 0],
    ['not-a-number', 0],
    [null, 0],
    [undefined, 0],
  ])('converts %j to a chart number', (value, expected) => {
    expect(toTooltipNumber(value)).toBe(expected)
  })

  it('preserves numeric NaN like the inline helpers did', () => {
    expect(Number.isNaN(toTooltipNumber(Number.NaN))).toBe(true)
  })

  it('recognizes non-null objects, including arrays', () => {
    expect(isTooltipRecord({ value: 1 })).toBe(true)
    expect(isTooltipRecord([])).toBe(true)
    expect(isTooltipRecord(null)).toBe(false)
    expect(isTooltipRecord('value')).toBe(false)
  })

  it('reads only supported scalar types', () => {
    expect(readTooltipString('label')).toBe('label')
    expect(readTooltipString(1)).toBeUndefined()
    expect(readTooltipScalar(1)).toBe(1)
    expect(readTooltipScalar('1')).toBe('1')
    expect(readTooltipScalar(null)).toBeNull()
    expect(readTooltipScalar(undefined)).toBeUndefined()
    expect(readTooltipScalar(false)).toBeUndefined()
    expect(readTooltipScalar({ value: 1 })).toBeUndefined()
    expect(readTooltipNumber(1)).toBe(1)
    expect(readTooltipNumber('1')).toBeUndefined()
  })

  it('keeps arrays and wraps single tooltip values', () => {
    const points = [{ value: 1 }]
    expect(toTooltipList(points)).toBe(points)
    expect(toTooltipList(points[0])).toEqual(points)
    expect(toTooltipList(undefined)).toEqual([undefined])
  })
})
