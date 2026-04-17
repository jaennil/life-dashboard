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
        'inline-flex items-center gap-2 rounded-lg border px-3 py-2 text-xs font-medium transition-colors',
        disabled || syncing
          ? 'cursor-not-allowed border-border text-muted-foreground/60'
          : 'border-primary/30 text-primary hover:bg-primary/10'
      )}
    >
      <RefreshCw className={cn('h-3.5 w-3.5', syncing && 'animate-spin')} />
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
