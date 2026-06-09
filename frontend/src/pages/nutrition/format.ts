import type { HydrationBeverageType, HydrationMode } from '@/lib/api'
import { HYDRATION_BEVERAGES, HYDRATION_MODE_LABELS } from '@/pages/nutrition/constants'

export function fmtDate(iso: string) {
  return new Date(iso).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })
}

export function fmtShort(iso: string) {
  const d = new Date(iso)
  return `${d.getDate()}.${String(d.getMonth() + 1).padStart(2, '0')}`
}

export function fmtWeight(value?: number) {
  return typeof value === 'number' ? `${value.toFixed(1)} кг` : '—'
}

export function fmtTargetDelta(current?: number, target?: number) {
  if (typeof current !== 'number' || typeof target !== 'number') return '—'
  const delta = current - target
  if (Math.abs(delta) < 0.05) return 'цель достигнута'
  return delta > 0 ? `сбросить ${delta.toFixed(1)} кг` : `набрать ${Math.abs(delta).toFixed(1)} кг`
}

export function fmtSyncTime(value?: string) {
  if (!value) return '—'
  return new Date(value).toLocaleString('ru-RU', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })
}

export function fmtOptionalNumber(value?: number, unit = '') {
  return typeof value === 'number' ? `${value.toFixed(0)}${unit}` : '—'
}

export function fmtWaterMl(value?: number) {
  return typeof value === 'number' ? `${Math.round(value)} мл` : '—'
}

export function hydrationBeverageLabel(type: HydrationBeverageType) {
  return HYDRATION_BEVERAGES.find(item => item.type === type)?.label ?? type
}

export function hydrationBeverageEmoji(type: HydrationBeverageType) {
  return HYDRATION_BEVERAGES.find(item => item.type === type)?.emoji ?? '🧃'
}

export function hydrationModeLabel(mode: HydrationMode | undefined) {
  return HYDRATION_MODE_LABELS[mode ?? 'strict']
}

export function numberInputValue(value?: number) {
  return typeof value === 'number' ? String(value) : ''
}

export function parseRequiredDecimalOrNull(label: string, value: string) {
  const trimmed = value.trim().replace(',', '.')
  if (!trimmed) return null
  const parsed = Number(trimmed)
  if (!Number.isFinite(parsed)) {
    throw new Error(`Некорректное значение для поля «${label}»`)
  }
  return parsed
}
