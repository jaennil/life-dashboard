import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

type PageHeaderBadgeTone = 'primary' | 'muted' | 'success' | 'warning' | 'danger'

type PageHeaderBadge = {
  label: string
  tone?: PageHeaderBadgeTone
}

const BADGE_TONES: Record<PageHeaderBadgeTone, string> = {
  primary: 'border-primary/20 bg-primary/10 text-primary',
  muted: 'border-border/70 bg-background/70 text-muted-foreground',
  success: 'border-emerald-500/20 bg-emerald-500/10 text-emerald-300',
  warning: 'border-amber-500/20 bg-amber-500/10 text-amber-200',
  danger: 'border-rose-500/20 bg-rose-500/10 text-rose-300',
}

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
  badges?: PageHeaderBadge[]
  actions?: ReactNode
  className?: string
  compactOnMobile?: boolean
  hideDescriptionOnMobile?: boolean
}) {
  return (
    <section
      className={cn(
        'page-header-panel',
        compactOnMobile && 'p-4 sm:p-6',
        className
      )}
    >
      <div className={cn('flex flex-col gap-5 xl:flex-row xl:items-start xl:justify-between', compactOnMobile && 'gap-4 sm:gap-5')}>
        <div className={cn('min-w-0 space-y-3', compactOnMobile && 'space-y-2.5 sm:space-y-3')}>
          {eyebrow ? (
            <p className={cn('text-[11px] font-semibold uppercase tracking-[0.24em] text-primary/80', compactOnMobile && 'text-[10px] sm:text-[11px]')}>
              {eyebrow}
            </p>
          ) : null}
          <div className={cn('space-y-2', compactOnMobile && 'space-y-1.5 sm:space-y-2')}>
            <h1 className={cn('text-3xl font-semibold tracking-tight text-foreground sm:text-[2rem]', compactOnMobile && 'text-[1.75rem] leading-none sm:text-[2rem]')}>
              {title}
            </h1>
            <p
              className={cn(
                'max-w-3xl text-sm leading-6 text-muted-foreground sm:text-[15px]',
                compactOnMobile && 'text-[13px] leading-5 sm:text-[15px] sm:leading-6',
                hideDescriptionOnMobile && 'hidden sm:block'
              )}
            >
              {description}
            </p>
          </div>
          {badges.length > 0 ? (
            <div className={cn('flex flex-wrap gap-2', compactOnMobile && 'gap-1.5 sm:gap-2')}>
              {badges.map((badge, index) => (
                <span
                  key={`${badge.label}-${index}`}
                  className={cn(
                    'rounded-full border px-2.5 py-1 text-[11px] font-medium',
                    compactOnMobile && 'px-2 py-1 text-[10px] sm:px-2.5 sm:text-[11px]',
                    BADGE_TONES[badge.tone ?? 'muted']
                  )}
                >
                  {badge.label}
                </span>
              ))}
            </div>
          ) : null}
        </div>

        {actions ? (
          <div className={cn('flex shrink-0 flex-wrap items-center gap-3 xl:justify-end', compactOnMobile && 'gap-2')}>
            {actions}
          </div>
        ) : null}
      </div>
    </section>
  )
}
