import { useEffect, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
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
  const [target, setTarget] = useState<HTMLElement | null>(() =>
    typeof document === 'undefined' ? null : document.getElementById('global-header-actions')
  )

  useEffect(() => {
    if (target || typeof document === 'undefined') return

    const frame = window.requestAnimationFrame(() => {
      setTarget(document.getElementById('global-header-actions'))
    })

    return () => window.cancelAnimationFrame(frame)
  }, [target])

  if (!actions) return null

  if (target) return createPortal(actions, target)

  if (typeof document !== 'undefined') return null

  return (
    <div className={cn('flex flex-wrap items-center justify-end gap-3', className)}>
      {actions}
    </div>
  )
}
