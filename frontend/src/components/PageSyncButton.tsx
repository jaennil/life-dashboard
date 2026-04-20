import { RefreshCw } from 'lucide-react'
import { cn } from '@/lib/utils'

export function PageSyncButton({
  label,
  syncCaption,
  syncing,
  disabled = false,
  onClick,
}: {
  label: string
  syncCaption?: string
  syncing: boolean
  disabled?: boolean
  onClick: () => void | Promise<void>
}) {
  const actionLabel = syncing ? 'Синхронизация...' : label

  return (
    <button
      onClick={() => void onClick()}
      disabled={disabled || syncing}
      className={cn(
        'inline-flex min-h-11 items-center gap-3 rounded-2xl border px-4 py-2.5 text-left text-xs font-medium shadow-sm transition-all',
        disabled || syncing
          ? 'cursor-not-allowed border-border bg-card/70 text-muted-foreground/60'
          : 'border-primary/20 bg-card/85 text-primary hover:-translate-y-0.5 hover:border-primary/30 hover:bg-primary/10'
      )}
    >
      <span className={cn(
        'flex h-8 w-8 shrink-0 items-center justify-center rounded-xl',
        disabled || syncing ? 'bg-muted text-muted-foreground/70' : 'bg-primary/12 text-primary'
      )}>
        <RefreshCw className={cn('h-3.5 w-3.5', syncing && 'animate-spin')} />
      </span>
      <span className="flex flex-col items-start gap-0.5 leading-none">
        <span>{actionLabel}</span>
        {syncCaption && (
          <span className="text-[10px] font-normal leading-none text-current opacity-70">
            {syncCaption}
          </span>
        )}
      </span>
    </button>
  )
}
