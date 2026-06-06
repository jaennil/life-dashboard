import type { ReactNode } from 'react'
import { ChevronDown, ChevronUp } from 'lucide-react'
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
          <div className="space-y-1">
            <h2 className="inline-flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-foreground">
              {title}
              {description ? <InfoTooltip text={description} className="normal-case tracking-normal" /> : null}
            </h2>
          </div>
          {summary ? (
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              {summary}
            </div>
          ) : null}
        </div>

        <div className="flex shrink-0 flex-wrap items-center gap-2">
          {actions}
          <button
            type="button"
            onClick={onToggle}
            className="inline-flex items-center gap-2 rounded-xl border bg-background/70 px-3 py-2 text-sm font-medium text-foreground transition-colors hover:bg-accent"
          >
            {open ? (
              <>
                <ChevronUp className="h-4 w-4" />
                Скрыть
              </>
            ) : (
              <>
                <ChevronDown className="h-4 w-4" />
                Показать
              </>
            )}
          </button>
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
