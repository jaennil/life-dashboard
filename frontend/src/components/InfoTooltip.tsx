import { useEffect, useId, useRef, useState } from 'react'
import { cn } from '@/lib/utils'

export function InfoTooltip({ text, className }: { text: string; className?: string }) {
  const [open, setOpen] = useState(false)
  const id = useId()
  const rootRef = useRef<HTMLSpanElement | null>(null)

  useEffect(() => {
    if (!open) return

    function close(event: MouseEvent) {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false)
    }

    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === 'Escape') setOpen(false)
    }

    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('mousedown', close)
      document.removeEventListener('keydown', closeOnEscape)
    }
  }, [open])

  return (
    <span ref={rootRef} className={cn('group relative inline-flex shrink-0', className)}>
      <button
        type="button"
        onClick={() => setOpen(current => !current)}
        aria-expanded={open}
        aria-describedby={id}
        className="inline-flex h-5 w-5 items-center justify-center rounded-full border border-border/80 bg-background/70 text-[11px] font-semibold text-muted-foreground transition-colors group-hover:border-primary/40 group-hover:text-primary"
        aria-label={text}
      >
        ?
      </button>
      <span
        id={id}
        role="tooltip"
        className={cn(
          'absolute left-1/2 top-full z-50 mt-2 w-64 -translate-x-1/2 rounded-lg border bg-card px-3 py-2 text-xs font-normal leading-5 text-muted-foreground shadow-xl',
          open ? 'block' : 'hidden group-hover:block group-focus-within:block',
        )}
      >
        {text}
      </span>
    </span>
  )
}
