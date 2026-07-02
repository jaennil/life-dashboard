import { Children, isValidElement, useEffect, useMemo, useRef, useState, type ReactElement, type ReactNode } from 'react'
import { ChevronDown } from 'lucide-react'
import { cn } from '@/lib/utils'

type OptionProps = {
  value?: string | number
  children?: ReactNode
  disabled?: boolean
}

type SelectChangeEvent = {
  target: { value: string }
  currentTarget: { value: string }
}

type StyledSelectProps = {
  value?: string | number
  onChange?: (event: SelectChangeEvent) => void
  children: ReactNode
  className?: string
  wrapperClassName?: string
  disabled?: boolean
  'aria-label'?: string
  title?: string
  id?: string
  name?: string
}

export function StyledSelect({
  value,
  onChange,
  className,
  wrapperClassName,
  children,
  disabled,
  'aria-label': ariaLabel,
  title,
  id,
  name,
}: StyledSelectProps) {
  const [open, setOpen] = useState(false)
  const [activeIndex, setActiveIndex] = useState(0)
  const ref = useRef<HTMLSpanElement | null>(null)
  const buttonRef = useRef<HTMLButtonElement | null>(null)
  const options = useMemo(
    () => Children.toArray(children)
      .filter(isValidElement)
      .map((child) => {
        const element = child as ReactElement<OptionProps>
        const optionValue = element.props.value ?? String(element.props.children ?? '')
        return {
          value: String(optionValue),
          label: element.props.children,
          disabled: Boolean(element.props.disabled),
        }
      }),
    [children]
  )
  const selectedIndex = options.findIndex(option => option.value === String(value ?? ''))
  const selected = selectedIndex >= 0 ? options[selectedIndex] : options[0]

  useEffect(() => {
    if (!open) return

    function close(event: MouseEvent) {
      if (!ref.current?.contains(event.target as Node)) setOpen(false)
    }

    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        setOpen(false)
        buttonRef.current?.focus()
      }
    }

    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('mousedown', close)
      document.removeEventListener('keydown', closeOnEscape)
    }
  }, [open])

  function selectOption(optionValue: string) {
    onChange?.({ target: { value: optionValue }, currentTarget: { value: optionValue } })
    setOpen(false)
    buttonRef.current?.focus()
  }

  function move(delta: number) {
    if (options.length === 0) return
    let next = activeIndex
    for (let step = 0; step < options.length; step += 1) {
      next = (next + delta + options.length) % options.length
      if (!options[next]?.disabled) {
        setActiveIndex(next)
        return
      }
    }
  }

  return (
    <span ref={ref} className={cn('relative inline-flex min-w-0', wrapperClassName)}>
      <button
        ref={buttonRef}
        id={id}
        name={name}
        type="button"
        disabled={disabled}
        aria-label={ariaLabel}
        title={title}
        aria-haspopup="listbox"
        aria-expanded={open}
        onClick={() => {
          setActiveIndex(selectedIndex >= 0 ? selectedIndex : 0)
          setOpen(current => !current)
        }}
        onKeyDown={(event) => {
          if (event.key === 'ArrowDown') {
            event.preventDefault()
            if (!open) {
              setActiveIndex(selectedIndex >= 0 ? selectedIndex : 0)
              setOpen(true)
              return
            }
            move(1)
          }
          if (event.key === 'ArrowUp') {
            event.preventDefault()
            if (!open) {
              setActiveIndex(selectedIndex >= 0 ? selectedIndex : 0)
              setOpen(true)
              return
            }
            move(-1)
          }
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault()
            if (!open) {
              setActiveIndex(selectedIndex >= 0 ? selectedIndex : 0)
              setOpen(true)
              return
            }
            const option = options[activeIndex]
            if (option && !option.disabled) selectOption(option.value)
          }
        }}
        className={cn(
          'inline-flex h-10 w-full min-w-0 items-center justify-between gap-3 rounded-lg border bg-background py-0 pl-3 pr-9 text-left text-sm text-foreground outline-none transition-colors hover:bg-accent focus:border-primary disabled:cursor-not-allowed disabled:opacity-60',
          className
        )}
      >
        <span className="min-w-0 truncate">{selected?.label}</span>
      </button>
      <ChevronDown className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
      {open ? (
        <div
          role="listbox"
          className="absolute left-0 top-full z-50 mt-1 max-h-72 min-w-full overflow-auto rounded-lg border bg-card p-1 text-sm text-foreground shadow-xl"
        >
          {options.map((option, index) => {
            const selectedOption = option.value === String(value ?? '')
            return (
              <button
                key={`${option.value}-${index}`}
                type="button"
                role="option"
                aria-selected={selectedOption}
                disabled={option.disabled}
                onMouseEnter={() => setActiveIndex(index)}
                onClick={() => selectOption(option.value)}
                className={cn(
                  'flex w-full items-center rounded-md px-3 py-2 text-left outline-none transition-colors disabled:cursor-not-allowed disabled:opacity-50',
                  index === activeIndex ? 'bg-accent text-accent-foreground' : 'hover:bg-accent hover:text-accent-foreground',
                  selectedOption ? 'bg-primary text-primary-foreground hover:bg-primary hover:text-primary-foreground' : ''
                )}
              >
                <span className="min-w-0 truncate">{option.label}</span>
              </button>
            )
          })}
        </div>
      ) : null}
    </span>
  )
}
