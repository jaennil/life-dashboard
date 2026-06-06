import { cn } from '@/lib/utils'

export function InfoTooltip({ text, className }: { text: string; className?: string }) {
  return (
    <span className={cn('group relative inline-flex shrink-0', className)}>
      <span
        className="inline-flex h-5 w-5 items-center justify-center rounded-full border border-border/80 bg-background/70 text-[11px] font-semibold text-muted-foreground transition-colors group-hover:border-primary/40 group-hover:text-primary"
        aria-label={text}
        role="img"
      >
        ?
      </span>
      <span className="pointer-events-none absolute left-1/2 top-full z-50 mt-2 hidden w-64 -translate-x-1/2 rounded-lg border bg-card px-3 py-2 text-xs font-normal leading-5 text-muted-foreground shadow-xl group-hover:block">
        {text}
      </span>
    </span>
  )
}
