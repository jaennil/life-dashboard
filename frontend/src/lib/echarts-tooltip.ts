export type TooltipScalar = number | string | null | undefined

export function toTooltipNumber(value: TooltipScalar): number {
  if (typeof value === 'number') return value
  if (typeof value === 'string') {
    const parsed = Number(value)
    if (Number.isFinite(parsed)) return parsed
  }
  return 0
}

export function isTooltipRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

export function readTooltipString(value: unknown): string | undefined {
  return typeof value === 'string' ? value : undefined
}

export function readTooltipScalar(value: unknown): TooltipScalar {
  if (typeof value === 'number' || typeof value === 'string' || value == null) return value
  return undefined
}

export function readTooltipNumber(value: unknown): number | undefined {
  return typeof value === 'number' ? value : undefined
}

export function toTooltipList(params: unknown): unknown[] {
  return Array.isArray(params) ? params : [params]
}
