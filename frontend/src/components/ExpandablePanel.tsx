import type { ReactNode } from 'react'
import { ChevronDown } from 'lucide-react'
import { InfoTooltip } from '@/components/InfoTooltip'
import { cn } from '@/lib/utils'

export function ExpandablePanel({
  title,
  description,
  summary,
  open,
  onToggle,
  children,
  actions,
  className,
  contentClassName,
}: {
  title: string
  description?: string
  summary?: ReactNode
  open: boolean
  onToggle: () => void
  children: ReactNode
  actions?: ReactNode
  className?: string
  contentClassName?: string
}) {
  return (
    <section className={cn('rounded-2xl border bg-card/90 p-5 shadow-sm', className)}>
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0 space-y-2">
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={onToggle}
              aria-expanded={open}
              className="inline-flex min-w-0 items-center gap-2 text-left text-sm font-semibold uppercase tracking-wider text-foreground transition-colors hover:text-primary"
            >
              <ChevronDown className={cn('h-4 w-4 shrink-0 transition-transform', open && 'rotate-180')} />
              <span className="truncate">{title}</span>
            </button>
            <div className="shrink-0">
              {description ? <InfoTooltip text={description} className="normal-case tracking-normal" /> : null}
            </div>
          </div>
          {summary ? (
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              {summary}
            </div>
          ) : null}
        </div>

        <div className="flex shrink-0 flex-wrap items-center gap-2">
          {actions}
        </div>
      </div>

      {open ? (
        <div className={cn('mt-5 border-t pt-5', contentClassName)}>
          {children}
        </div>
      ) : null}
    </section>
  )
}
