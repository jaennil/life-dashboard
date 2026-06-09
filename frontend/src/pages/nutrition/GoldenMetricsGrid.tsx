import type { NutritionGoldenCard } from '@/lib/api'
import { NutritionGoldenMetricCard } from '@/pages/nutrition/components'

export function GoldenMetricsGrid({
  cards,
  loading,
}: {
  cards: NutritionGoldenCard[]
  loading: boolean
}) {
  const visibleCards = loading && cards.length === 0
    ? Array.from({ length: 5 }).map((_, index) => ({
      key: `skeleton-${index}`,
      title: '—',
      value: '—',
      detail: '—',
      tone: 'muted' as const,
    }))
    : cards

  return (
    <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-5">
      {visibleCards.map(card => (
        <NutritionGoldenMetricCard
          key={card.key}
          card={card}
          loading={loading}
        />
      ))}
    </div>
  )
}
