import { useState } from 'react'
import { Check, RefreshCw, X, XCircle } from 'lucide-react'
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
  const [result, setResult] = useState<{ tone: 'success' | 'error'; message: string } | null>(null)
  const actionLabel = syncing ? 'Синхронизация...' : label

  async function handleClick() {
    setResult(null)
    try {
      await onClick()
      setResult({ tone: 'success', message: 'Данные синхронизированы' })
    } catch (error) {
      setResult({
        tone: 'error',
        message: error instanceof Error ? error.message : 'Синхронизация завершилась с ошибкой',
      })
    }
  }

  return (
    <div className="relative">
      <button
        onClick={() => void handleClick()}
        disabled={disabled || syncing}
        className={cn(
          'inline-flex min-h-10 items-center gap-2 rounded-lg border px-3 py-2 text-left text-xs font-medium transition-colors',
          disabled || syncing
            ? 'cursor-not-allowed border-border bg-card/70 text-muted-foreground/60'
            : 'border-primary/20 bg-card text-primary hover:border-primary/40 hover:bg-primary/10'
        )}
      >
        <RefreshCw className={cn('h-4 w-4 shrink-0', syncing && 'animate-spin')} />
        <span className="flex flex-col items-start gap-0.5 leading-none">
          <span>{actionLabel}</span>
          {syncCaption && (
            <span className="text-[10px] font-normal leading-none text-current opacity-70">
              {syncCaption}
            </span>
          )}
        </span>
      </button>
      {result ? (
        <div
          role={result.tone === 'error' ? 'alert' : 'status'}
          className={cn(
            'absolute right-0 top-full z-50 mt-2 flex w-72 items-start gap-2 rounded-lg border bg-card px-3 py-2 text-xs shadow-xl',
            result.tone === 'success' ? 'border-emerald-500/30 text-emerald-300' : 'border-rose-500/30 text-rose-300',
          )}
        >
          {result.tone === 'success' ? <Check className="mt-0.5 h-4 w-4 shrink-0" /> : <XCircle className="mt-0.5 h-4 w-4 shrink-0" />}
          <span className="min-w-0 flex-1 break-words leading-5">{result.message}</span>
          <button type="button" onClick={() => setResult(null)} className="shrink-0 text-muted-foreground hover:text-foreground" aria-label="Скрыть статус синхронизации">
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      ) : null}
    </div>
  )
}
