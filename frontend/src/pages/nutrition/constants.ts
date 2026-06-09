import type { HydrationBeverageType, HydrationMode } from '@/lib/api'

export const MEAL_LABELS: Record<string, string> = {
  breakfast: 'Завтрак', lunch: 'Обед', dinner: 'Ужин', snacks: 'Перекус', other: 'Прочее',
}

export const MEAL_KEYS_BY_LABEL = Object.fromEntries(Object.entries(MEAL_LABELS).map(([key, label]) => [label, key]))

export const MEAL_COLORS: Record<string, string> = {
  breakfast: '#f97316',
  lunch: '#10b981',
  dinner: '#3b82f6',
  snacks: '#8b5cf6',
  other: '#f43f5e',
}

export const MEAL_ORDER = ['breakfast', 'lunch', 'dinner', 'snacks', 'other'] as const

export const MACRO_COLORS = { protein: '#3b82f6', fat: '#f97316', carbs: '#10b981', fiber: '#8b5cf6' }

export const HYDRATION_MODE_LABELS: Record<HydrationMode, string> = {
  strict: 'Строго',
  flexible: 'Гибко',
}

export const HYDRATION_MODE_NOTES: Record<HydrationMode, string> = {
  strict: 'В цель воды идёт только чистая вода.',
  flexible: 'В цель идут вода, чай и кофе. Энергетики и молочные напитки считаются отдельно.',
}

export const HYDRATION_MODE_ACCENT: Record<HydrationMode, string> = {
  strict: 'border-cyan-500/20 bg-cyan-500/10 text-cyan-200',
  flexible: 'border-emerald-500/20 bg-emerald-500/10 text-emerald-200',
}

export const HYDRATION_BEVERAGES: Array<{
  type: HydrationBeverageType
  label: string
  short: string
  emoji: string
  amount: number
  color: string
}> = [
  { type: 'tea', label: 'Чай', short: 'Чай', emoji: '🍵', amount: 250, color: '#10b981' },
  { type: 'coffee', label: 'Кофе', short: 'Кофе', emoji: '☕', amount: 250, color: '#c084fc' },
  { type: 'energy', label: 'Энергетик', short: 'Энерджи', emoji: '⚡', amount: 250, color: '#f59e0b' },
  { type: 'milkshake', label: 'Коктейль', short: 'Коктейль', emoji: '🥤', amount: 330, color: '#fb7185' },
  { type: 'other', label: 'Прочее', short: 'Прочее', emoji: '🧃', amount: 250, color: '#94a3b8' },
]
