import { RefreshCw } from 'lucide-react'
import { cn } from '@/lib/utils'

export function PageSyncButton({
  label,
  syncing,
  disabled = false,
  onClick,
}: {
  label: string
  syncing: boolean
  disabled?: boolean
  onClick: () => void | Promise<void>
}) {
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
      {syncing ? 'Синхронизация...' : label}
    </button>
  )
}
