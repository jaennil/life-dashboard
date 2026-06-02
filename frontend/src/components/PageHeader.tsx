import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export function PageHeader({
  actions,
  className,
}: {
  eyebrow?: string
  title: string
  description: string
  badges?: Array<{ label: string; tone?: 'primary' | 'muted' | 'success' | 'warning' | 'danger' }>
  actions?: ReactNode
  className?: string
  compactOnMobile?: boolean
  hideDescriptionOnMobile?: boolean
}) {
  if (!actions) return null

  return (
    <div className={cn('flex flex-wrap items-center justify-end gap-3', className)}>
      {actions}
    </div>
  )
}
