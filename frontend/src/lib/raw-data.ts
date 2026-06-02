export type RawDataSource =
  | 'finance.transactions'
  | 'fitness.activities'
  | 'fitness.workouts'
  | 'nutrition.days'
  | 'productivity.tasks'
  | 'ai.messages'

export const RAW_DATA_SOURCES: Array<{ value: RawDataSource; label: string }> = [
  { value: 'finance.transactions', label: 'Finance / Transactions' },
  { value: 'fitness.activities', label: 'Fitness / Activities' },
  { value: 'fitness.workouts', label: 'Fitness / Workouts' },
  { value: 'nutrition.days', label: 'Nutrition / Days' },
  { value: 'productivity.tasks', label: 'Productivity / Tasks' },
  { value: 'ai.messages', label: 'AI / Messages' },
]

export function rawDataHref(source: RawDataSource, filters: Record<string, string | undefined> = {}) {
  const params = new URLSearchParams({ source })
  Object.entries(filters).forEach(([key, value]) => {
    if (value) params.set(key, value)
  })
  return `/raw-data?${params.toString()}`
}
