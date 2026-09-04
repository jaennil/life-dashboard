import { useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
import { AlertCircle, CheckCircle2, Loader2, PenLine, Send } from 'lucide-react'
import { PageHeader } from '@/components/PageHeader'
import { api, type QuickInputResponse } from '@/lib/api'

const EXAMPLES = [
  'Лимонад с витаминами 330 мл',
  'Подтягивания 8 раз, 3 подхода',
  'Закончить тренировку',
]

const DOMAIN_LABELS: Record<string, string> = {
  food: 'Питание',
  workout: 'Тренировка',
  note: 'Заметка',
  weight: 'Вес',
  unknown: 'Не определено',
}

export function QuickInput() {
  const [text, setText] = useState('')
  const [result, setResult] = useState<QuickInputResponse | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  async function submit() {
    const value = text.trim()
    if (!value || loading) return

    setLoading(true)
    setError('')
    setResult(null)
    try {
      const response = await api.submitQuickInput(value)
      setResult(response)
      setText('')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Не удалось обработать текст.')
    } finally {
      setLoading(false)
      inputRef.current?.focus()
    }
  }

  function handleSubmit(event: FormEvent) {
    event.preventDefault()
    void submit()
  }

  function handleKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
      event.preventDefault()
      void submit()
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        eyebrow="Quick capture"
        title="Ввод"
        description="Записывай питание и тренировку текстом — обработка такая же, как у голосовой команды."
        badges={[{ label: 'Текст или голос', tone: 'primary' }]}
      />

      <div className="mx-auto grid w-full max-w-3xl gap-5">
        <form onSubmit={handleSubmit} className="rounded-2xl border bg-card/90 p-4 shadow-sm sm:p-6">
          <label htmlFor="quick-input" className="flex items-center gap-2 text-sm font-semibold text-foreground">
            <PenLine className="h-4 w-4 text-primary" />
            Что записать?
          </label>
          <textarea
            ref={inputRef}
            id="quick-input"
            value={text}
            onChange={event => setText(event.target.value)}
            onKeyDown={handleKeyDown}
            rows={5}
            autoFocus
            placeholder="Например: лимонад с витаминами 330 мл"
            className="mt-3 w-full resize-y rounded-2xl border bg-background px-4 py-3 text-base text-foreground outline-none transition-colors placeholder:text-muted-foreground focus:border-primary/60 focus:ring-2 focus:ring-primary/15"
          />
          <div className="mt-3 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-xs text-muted-foreground">Ctrl/⌘ + Enter — отправить</p>
            <button
              type="submit"
              disabled={!text.trim() || loading}
              className="inline-flex min-h-11 items-center justify-center gap-2 rounded-2xl bg-primary px-5 py-2.5 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
              {loading ? 'Обрабатываю…' : 'Записать'}
            </button>
          </div>
        </form>

        <div className="flex flex-wrap gap-2">
          {EXAMPLES.map(example => (
            <button
              key={example}
              type="button"
              onClick={() => {
                setText(example)
                inputRef.current?.focus()
              }}
              className="rounded-full border bg-card px-3 py-1.5 text-xs text-muted-foreground transition-colors hover:border-primary/30 hover:text-foreground"
            >
              {example}
            </button>
          ))}
        </div>

        <div aria-live="polite">
          {result ? (
            <div className="rounded-2xl border border-emerald-500/25 bg-emerald-500/10 p-4 sm:p-5">
              <div className="flex items-center gap-2 text-sm font-semibold text-emerald-400">
                <CheckCircle2 className="h-4 w-4" />
                Готово
                {result.domain ? (
                  <span className="ml-auto rounded-full border border-emerald-500/25 px-2.5 py-1 text-xs font-normal">
                    {DOMAIN_LABELS[result.domain] ?? result.domain}
                  </span>
                ) : null}
              </div>
              <p className="mt-3 whitespace-pre-wrap text-sm leading-6 text-foreground">{result.display}</p>
            </div>
          ) : null}

          {error ? (
            <div className="rounded-2xl border border-rose-500/25 bg-rose-500/10 p-4 text-sm text-rose-300">
              <div className="flex items-start gap-2">
                <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
                <span>{error}</span>
              </div>
            </div>
          ) : null}
        </div>

        <p className="rounded-2xl border bg-card/60 px-4 py-3 text-xs leading-5 text-muted-foreground">
          Питание записывается сразу. Фразы о тренировке добавляются в открытую тренировку до команды «Закончить тренировку».
        </p>
      </div>
    </div>
  )
}
