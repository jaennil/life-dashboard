import { useCallback, useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
import { AlertCircle, Bell, BellOff, CheckCircle2, Clock3, Loader2, PenLine, Send } from 'lucide-react'
import { PageHeader } from '@/components/PageHeader'
import { api, type InputJob, type QuickInputResponse } from '@/lib/api'

const EXAMPLES = [
  'Лимонад с витаминами 330 мл',
  'Подтягивания 8 раз, 3 подхода',
  'Закончить тренировку',
]

const DOMAIN_LABELS: Record<string, string> = {
  food: 'Питание',
  workout: 'Тренировка',
  task: 'Задача',
  note: 'Заметка',
  weight: 'Вес',
  unknown: 'Не определено',
}

export function QuickInput() {
  const [text, setText] = useState('')
  const [result, setResult] = useState<QuickInputResponse | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [jobs, setJobs] = useState<InputJob[]>([])
  const [pushAvailable, setPushAvailable] = useState(false)
  const [pushEnabled, setPushEnabled] = useState(false)
  const [pushBusy, setPushBusy] = useState(false)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  const loadJobs = useCallback(async () => {
    try {
      setJobs(await api.getInputJobs())
    } catch {
      // Submission errors remain actionable; a transient history refresh is not.
    }
  }, [])

  useEffect(() => {
    void loadJobs()
    if (!('serviceWorker' in navigator) || !('PushManager' in window)) return
    void Promise.all([api.getPushConfig(), navigator.serviceWorker.ready]).then(async ([config, registration]) => {
      setPushAvailable(config.enabled)
      setPushEnabled(Boolean(await registration.pushManager.getSubscription()))
    }).catch(() => undefined)
  }, [loadJobs])

  useEffect(() => {
    if (!jobs.some(job => job.status === 'queued' || job.status === 'processing')) return
    const timer = window.setInterval(() => void loadJobs(), 3000)
    return () => window.clearInterval(timer)
  }, [jobs, loadJobs])

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
      void loadJobs()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Не удалось обработать текст.')
    } finally {
      setLoading(false)
      inputRef.current?.focus()
    }
  }

  async function togglePush() {
    if (!pushAvailable || pushBusy) return
    setPushBusy(true)
    setError('')
    try {
      const registration = await navigator.serviceWorker.ready
      const existing = await registration.pushManager.getSubscription()
      if (existing) {
        await api.deletePushSubscription(existing.endpoint)
        await existing.unsubscribe()
        setPushEnabled(false)
        return
      }
      const config = await api.getPushConfig()
      const subscription = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(config.public_key),
      })
      await api.savePushSubscription(subscription.toJSON())
      setPushEnabled(true)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Не удалось включить уведомления.')
    } finally {
      setPushBusy(false)
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
        {pushAvailable ? (
          <button
            type="button"
            onClick={() => void togglePush()}
            disabled={pushBusy}
            className="flex items-center justify-between gap-3 rounded-2xl border bg-card/70 px-4 py-3 text-left text-sm disabled:opacity-60"
          >
            <span className="flex items-center gap-2 font-medium">
              {pushEnabled ? <Bell className="h-4 w-4 text-emerald-400" /> : <BellOff className="h-4 w-4 text-muted-foreground" />}
              Уведомления о результате
            </span>
            <span className="text-xs text-muted-foreground">
              {pushBusy ? 'Подождите…' : pushEnabled ? 'Включены' : 'Включить'}
            </span>
          </button>
        ) : null}

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
                {result.status === 'queued' ? 'В очереди' : 'Готово'}
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

        {jobs.length > 0 ? (
          <section className="rounded-2xl border bg-card/70 p-4 sm:p-5">
            <h2 className="text-sm font-semibold text-foreground">Последние записи</h2>
            <div className="mt-3 divide-y divide-border/70">
              {jobs.map(job => (
                <div key={job.id} className="py-3 first:pt-0 last:pb-0">
                  <div className="flex items-start gap-3">
                    <JobStatusIcon status={job.status} />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center justify-between gap-3">
                        <p className="truncate text-sm font-medium text-foreground">{job.text}</p>
                        <time className="shrink-0 text-xs text-muted-foreground">
                          {new Date(job.created_at).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })}
                        </time>
                      </div>
                      <p className="mt-1 whitespace-pre-wrap text-xs leading-5 text-muted-foreground">
                        {job.result ? resultWithoutTranscript(job.result.display) : jobStatusText(job)}
                      </p>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </section>
        ) : null}

        <p className="rounded-2xl border bg-card/60 px-4 py-3 text-xs leading-5 text-muted-foreground">
          Питание записывается сразу. Фразы о тренировке добавляются в открытую тренировку до команды «Закончить тренировку».
        </p>
      </div>
    </div>
  )
}

function JobStatusIcon({ status }: { status: InputJob['status'] }) {
  if (status === 'succeeded') return <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-400" />
  if (status === 'failed') return <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-rose-400" />
  if (status === 'processing') return <Loader2 className="mt-0.5 h-4 w-4 shrink-0 animate-spin text-primary" />
  return <Clock3 className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
}

function jobStatusText(job: InputJob) {
  if (job.status === 'processing') return `Обрабатывается${job.attempts > 1 ? `, попытка ${job.attempts}` : ''}…`
  if (job.status === 'failed') return job.last_error || 'Не удалось обработать.'
  return job.attempts > 0 ? 'Ожидает повторной попытки…' : 'Ожидает обработки…'
}

function resultWithoutTranscript(display: string) {
  const newline = display.indexOf('\n')
  if (newline < 0) return display

  const firstLine = display.slice(0, newline)
  if (firstLine.startsWith('Услышал: ') || firstLine.startsWith('Введено: ')) {
    return display.slice(newline + 1)
  }
  return display
}

function urlBase64ToUint8Array(value: string) {
  const padding = '='.repeat((4 - value.length % 4) % 4)
  const base64 = (value + padding).replace(/-/g, '+').replace(/_/g, '/')
  const bytes = atob(base64)
  return Uint8Array.from(bytes, character => character.charCodeAt(0))
}
