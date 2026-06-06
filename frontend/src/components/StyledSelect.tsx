import type { SelectHTMLAttributes } from 'react'
import { ChevronDown } from 'lucide-react'
import { cn } from '@/lib/utils'

type StyledSelectProps = SelectHTMLAttributes<HTMLSelectElement> & {
  wrapperClassName?: string
}

export function StyledSelect({ className, wrapperClassName, children, ...props }: StyledSelectProps) {
  return (
    <span className={cn('relative inline-flex min-w-0', wrapperClassName)}>
      <select
        {...props}
        className={cn(
          'h-10 w-full appearance-none rounded-lg border bg-background py-0 pl-3 pr-9 text-sm text-foreground outline-none transition-colors hover:bg-accent focus:border-primary disabled:cursor-not-allowed disabled:opacity-60',
          className
        )}
      >
        {children}
      </select>
      <ChevronDown className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
    </span>
  )
}
