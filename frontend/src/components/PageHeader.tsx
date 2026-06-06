import { useEffect, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { InfoTooltip } from '@/components/InfoTooltip'
import { cn } from '@/lib/utils'

export function PageHeader({
  eyebrow,
  title,
  description,
  badges = [],
  actions,
  className,
  compactOnMobile = false,
  hideDescriptionOnMobile = false,
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

  return (
    <>
      {actions && target ? createPortal(actions, target) : null}
      <header className={cn('flex min-w-0 flex-col gap-3', compactOnMobile && 'gap-2 sm:gap-3', className)}>
        {eyebrow ? (
          <p className="text-[11px] font-semibold uppercase tracking-[0.18em] text-primary">{eyebrow}</p>
        ) : null}
        <div className="flex min-w-0 flex-col gap-1 sm:flex-row sm:items-end sm:gap-4">
          <h1 className="inline-flex shrink-0 items-center gap-2 text-2xl font-semibold text-foreground">
            {title}
            <InfoTooltip
              text={description}
              className={cn(hideDescriptionOnMobile && 'hidden sm:inline-flex')}
            />
          </h1>
        </div>
        {badges.length > 0 ? (
          <div className="flex flex-wrap gap-2">
            {badges.map((badge, index) => (
              <span
                key={`${badge.label}-${index}`}
                className={cn(
                  'rounded-full border px-2.5 py-1 text-xs',
                  badge.tone === 'primary' && 'border-primary/25 bg-primary/10 text-primary',
                  badge.tone === 'success' && 'border-emerald-500/25 bg-emerald-500/10 text-emerald-300',
                  badge.tone === 'warning' && 'border-amber-500/25 bg-amber-500/10 text-amber-200',
                  badge.tone === 'danger' && 'border-rose-500/25 bg-rose-500/10 text-rose-300',
                  (!badge.tone || badge.tone === 'muted') && 'border-border bg-card text-muted-foreground',
                )}
              >
                {badge.label}
              </span>
            ))}
          </div>
        ) : null}
        {actions && !target && typeof document === 'undefined' ? (
          <div className="flex flex-wrap items-center gap-3">{actions}</div>
        ) : null}
      </header>
    </>
  )
}
