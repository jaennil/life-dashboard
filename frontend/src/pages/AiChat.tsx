import { useState, useRef, useEffect, type ComponentProps } from 'react'
import { Send, Bot, User, Loader2, Trash2 } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { PageHeader } from '@/components/PageHeader'
import { api, type AIHistoryMessage, type AILatestCheckup } from '@/lib/api'
import { cn } from '@/lib/utils'

interface Message {
  id?: string
  role: 'user' | 'assistant'
  content: string
  created_at?: string
  loading?: boolean
}

interface ChatResponse {
  content: string
}

interface CheckupResponse extends ChatResponse {
  period: CheckupPeriod
  period_label: string
  generated_at: string
}

interface SendResult {
  content: string
  isError?: boolean
}

interface CheckupSendResult extends SendResult {
  generatedAt?: string
  periodLabel?: string
}

type CheckupPeriod = 'today' | 'week' | 'month' | 'since_last'

interface CheckupAction {
  period: CheckupPeriod
  label: string
  userMessage: string
}

const SUGGESTIONS = [
  'Сколько я потратил в этом месяце?',
  'На что у меня самые большие траты за 30 дней?',
  'Сколько активности у меня было на этой неделе?',
  'Как прошла последняя тренировка и что улучшить?',
  'Что у меня по Todoist на сегодня?',
  'Какие задачи у меня overdue и что висит давно?',
  'Что по питанию проседает за 7 дней?',
]

const CHECKUP_ACTIONS: CheckupAction[] = [
  { period: 'today', label: 'Сегодня', userMessage: 'Сделай checkup за сегодня' },
  { period: 'week', label: '7 дней', userMessage: 'Сделай checkup за неделю' },
  { period: 'month', label: '30 дней', userMessage: 'Сделай checkup за месяц' },
  { period: 'since_last', label: 'С прошлого', userMessage: 'Сделай checkup с момента последнего отчёта' },
]

const MARKDOWN_COMPONENTS = {
  table: ({ children }: ComponentProps<'table'>) => (
    <div className="my-3 overflow-x-auto">
      <table className="min-w-full border-collapse text-xs sm:text-sm">{children}</table>
    </div>
  ),
  thead: ({ children }: ComponentProps<'thead'>) => (
    <thead className="border-b border-border">{children}</thead>
  ),
  th: ({ children }: ComponentProps<'th'>) => (
    <th className="px-3 py-2 text-left font-medium text-foreground">{children}</th>
  ),
  td: ({ children }: ComponentProps<'td'>) => (
    <td className="border-b border-border/60 px-3 py-2 align-top text-foreground">{children}</td>
  ),
} as const

function looksLikeHTML(value: string) {
  const text = value.trim().toLowerCase()
  return text.startsWith('<!doctype html') || text.startsWith('<html') || text.includes('<body') || text.includes('<title>')
}

function formatChatError(status: number, body: string) {
  const text = body.trim()

  if (status === 403) return 'AI чат сейчас отключён.'
  if (status === 502 || status === 503 || status === 504) {
    return 'AI сервис сейчас недоступен. Попробуй позже.'
  }

  if (!text || looksLikeHTML(text)) {
    return 'Не удалось получить ответ от AI.'
  }

  return `Ошибка: ${text}`
}

const RETRYABLE_CHAT_STATUSES = new Set([502, 503, 504])
const CHAT_RETRY_DELAY_MS = 400
const CHAT_CONTEXT_MESSAGE_LIMIT = 24

function sleep(ms: number) {
  return new Promise(resolve => window.setTimeout(resolve, ms))
}

async function requestChat(message: string, history: Message[]): Promise<SendResult> {
  const payload = JSON.stringify({
    message,
    history: history
      .filter(m => !m.loading)
      .slice(-CHAT_CONTEXT_MESSAGE_LIMIT)
      .map(m => ({ role: m.role, content: m.content })),
  })

  return requestAI('/api/v1/ai/chat', payload)
}

async function requestCheckup(period: CheckupPeriod): Promise<CheckupSendResult> {
  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      const res = await fetch('/api/v1/ai/checkup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ period }),
      })
      const raw = await res.text()

      if (!res.ok) {
        if (attempt === 0 && RETRYABLE_CHAT_STATUSES.has(res.status)) {
          await sleep(CHAT_RETRY_DELAY_MS)
          continue
        }

        return {
          content: formatChatError(res.status, raw),
          isError: true,
        }
      }

      let parsed: CheckupResponse | null = null
      try {
        parsed = JSON.parse(raw) as CheckupResponse
      } catch {
        // keep fallback below
      }

      const content = typeof parsed?.content === 'string' ? parsed.content : raw
      if (!content.trim() || looksLikeHTML(content)) {
        if (attempt === 0) {
          await sleep(CHAT_RETRY_DELAY_MS)
          continue
        }

        return {
          content: 'AI сервис не вернул ответ. Попробуй ещё раз.',
          isError: true,
        }
      }

      return {
        content,
        generatedAt: parsed?.generated_at,
        periodLabel: parsed?.period_label,
      }
    } catch {
      if (attempt === 0) {
        await sleep(CHAT_RETRY_DELAY_MS)
        continue
      }
    }
  }

  return {
    content: 'Не удалось подключиться к AI.',
    isError: true,
  }
}

async function requestAI(url: string, payload: string): Promise<SendResult> {
  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      const res = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: payload,
      })
      const raw = await res.text()

      if (!res.ok) {
        if (attempt === 0 && RETRYABLE_CHAT_STATUSES.has(res.status)) {
          await sleep(CHAT_RETRY_DELAY_MS)
          continue
        }

        return {
          content: formatChatError(res.status, raw),
          isError: true,
        }
      }

      let content = raw
      try {
        const parsed = JSON.parse(raw) as ChatResponse
        if (typeof parsed.content === 'string') {
          content = parsed.content
        }
      } catch {
        // Keep raw body as fallback if proxy/service returns plain text.
      }

      if (!content.trim() || looksLikeHTML(content)) {
        if (attempt === 0) {
          await sleep(CHAT_RETRY_DELAY_MS)
          continue
        }

        return {
          content: 'AI сервис не вернул ответ. Попробуй ещё раз.',
          isError: true,
        }
      }

      return { content }
    } catch {
      if (attempt === 0) {
        await sleep(CHAT_RETRY_DELAY_MS)
        continue
      }
    }
  }

  return {
    content: 'Не удалось подключиться к AI.',
    isError: true,
  }
}

export function AiChat() {
  const [messages, setMessages] = useState<Message[]>([])
  const [input, setInput] = useState('')
  const [loading, setLoading] = useState(false)
  const [historyLoading, setHistoryLoading] = useState(true)
  const [historyError, setHistoryError] = useState('')
  const [latestCheckup, setLatestCheckup] = useState<AILatestCheckup | null>(null)
  const bottomRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  useEffect(() => {
    let active = true

    setHistoryLoading(true)
    setHistoryError('')

    Promise.allSettled([api.getAIHistory(), api.getLatestAICheckup()])
      .then(([historyResult, latestCheckupResult]) => {
        if (!active) return

        if (historyResult.status === 'fulfilled') {
          setMessages(historyResult.value.map(mapHistoryMessage))
        } else {
          setHistoryError('Не удалось загрузить историю чата.')
        }

        if (latestCheckupResult.status === 'fulfilled') {
          setLatestCheckup(latestCheckupResult.value)
        }
      })
      .finally(() => {
        if (!active) return
        setHistoryLoading(false)
      })

    return () => {
      active = false
    }
  }, [])

  async function send(text: string) {
    if (!text.trim() || loading || historyLoading) return
    setInput('')
    setLoading(true)

    const userMsg: Message = { role: 'user', content: text }
    const assistantMsg: Message = { role: 'assistant', content: '', loading: true }
    setMessages(prev => [...prev, userMsg, assistantMsg])

    try {
      const result = await requestChat(text, messages)
      setMessages(prev => [
        ...prev.slice(0, -1),
        { role: 'assistant', content: result.content, loading: false },
      ])
    } finally {
      setLoading(false)
    }
  }

  async function sendCheckup(action: CheckupAction) {
    if (loading || historyLoading) return
    setLoading(true)

    const userMsg: Message = { role: 'user', content: action.userMessage }
    const assistantMsg: Message = { role: 'assistant', content: '', loading: true }
    setMessages(prev => [...prev, userMsg, assistantMsg])

    try {
      const result = await requestCheckup(action.period)
      setMessages(prev => [
        ...prev.slice(0, -1),
        { role: 'assistant', content: result.content, loading: false },
      ])
      if (!result.isError) {
        setLatestCheckup({
          has_report: true,
          period: action.period,
          period_label: result.periodLabel ?? findCheckupLabel(action.period),
          generated_at: result.generatedAt ?? new Date().toISOString(),
        })
      }
    } finally {
      setLoading(false)
    }
  }

  async function clearHistory() {
    if (loading || historyLoading) return

    setHistoryLoading(true)
    setHistoryError('')
    try {
      await api.clearAIHistory()
      setMessages([])
    } catch {
      setHistoryError('Не удалось очистить историю чата.')
    } finally {
      setHistoryLoading(false)
    }
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send(input)
    }
  }

  return (
    <div className="flex min-h-[calc(100dvh-8rem)] flex-col gap-3">
      <PageHeader
        eyebrow="AI"
        title="AI Chat"
        description="Спрашивай про финансы, тренировки, питание, задачи и checkup в одном чате."
        badges={[
          { label: latestCheckup?.has_report ? `Последний checkup: ${latestCheckup.period_label ?? 'есть отчёт'}` : 'Checkup ещё не запускался', tone: latestCheckup?.has_report ? 'primary' : 'muted' },
          { label: messages.length > 0 ? `${messages.length} сообщений в истории` : 'История пока пустая', tone: messages.length > 0 ? 'muted' : 'warning' },
        ]}
        compactOnMobile
        hideDescriptionOnMobile
        actions={(
          <button
            onClick={clearHistory}
            disabled={historyLoading || loading || messages.length === 0}
            className="inline-flex items-center gap-2 rounded-2xl border bg-card/85 px-3 py-2.5 text-sm text-muted-foreground shadow-sm transition-colors hover:bg-accent hover:text-accent-foreground disabled:cursor-not-allowed disabled:opacity-40 sm:px-4 sm:py-3"
          >
            <Trash2 className="h-4 w-4" />
            <span className="hidden sm:inline">Очистить историю</span>
            <span className="sm:hidden">Очистить</span>
          </button>
        )}
      />

      {historyError ? (
        <div className="mb-3 rounded-2xl border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-200">
          {historyError}
        </div>
      ) : null}

      <div className="rounded-2xl border bg-card/80 p-4 shadow-sm">
        <div className="mb-3 flex flex-col gap-1.5 sm:mb-2">
          <div className="flex items-center justify-between gap-3">
            <p className="text-sm font-medium text-foreground">Checkup</p>
            <span className="rounded-full border border-border/80 bg-background/70 px-2 py-1 text-[10px] font-medium text-muted-foreground sm:hidden">
              AI-отчёт
            </span>
          </div>
          <p className="hidden text-xs text-muted-foreground sm:block">Быстрый AI-отчёт по всем сферам за нужный период</p>
          <p className="text-xs leading-5 text-muted-foreground">
            {formatLatestCheckup(latestCheckup)}
          </p>
        </div>
        <div className="grid grid-cols-2 gap-2 sm:flex sm:flex-wrap">
          {CHECKUP_ACTIONS.map(action => (
            <button
              key={action.period}
              onClick={() => sendCheckup(action)}
              disabled={loading || historyLoading}
              className="rounded-xl border bg-background px-3 py-2 text-sm text-foreground transition-colors hover:bg-accent hover:text-accent-foreground disabled:cursor-not-allowed disabled:opacity-50"
            >
              {action.label}
            </button>
          ))}
        </div>
      </div>

      {/* Messages */}
      <div className="flex min-h-0 flex-1 flex-col gap-3 rounded-[24px] border bg-card/90 p-3 shadow-sm sm:p-4">
        {historyLoading ? (
          <div className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            Загружаю историю чата...
          </div>
        ) : messages.length === 0 ? (
          <div className="flex h-full flex-col items-center justify-center gap-5 px-2 text-center">
            <div className="flex items-center justify-center w-14 h-14 rounded-2xl bg-primary/10">
              <Bot className="w-7 h-7 text-primary" />
            </div>
            <div>
              <p className="font-medium text-foreground">Чем могу помочь?</p>
              <p className="text-sm text-muted-foreground mt-1">У меня есть доступ к финансам, активности, тренировкам и питанию</p>
            </div>
            <div className="grid w-full gap-2 sm:flex sm:max-w-lg sm:flex-wrap sm:justify-center">
              {SUGGESTIONS.map(s => (
                <button
                  key={s}
                  onClick={() => send(s)}
                  className="rounded-lg border bg-background px-3 py-2 text-left text-sm transition-colors hover:bg-accent hover:text-accent-foreground"
                >
                  {s}
                </button>
              ))}
            </div>
          </div>
        ) : (
          <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto pr-1">
            {messages.map((msg, i) => (
              <div key={msg.id ?? `${msg.role}-${i}`} className={cn('flex gap-2 sm:gap-3', msg.role === 'user' && 'flex-row-reverse')}>
              <div className={cn(
                'mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full sm:h-8 sm:w-8',
                msg.role === 'user' ? 'bg-primary' : 'bg-muted'
              )}>
                {msg.role === 'user'
                  ? <User className="w-4 h-4 text-primary-foreground" />
                  : <Bot className="w-4 h-4 text-muted-foreground" />}
              </div>
              <div className={cn(
                'rounded-[20px] px-3.5 py-2.5 text-[13px] leading-5 shadow-sm sm:rounded-[22px] sm:px-4 sm:py-3 sm:text-sm sm:leading-6',
                msg.role === 'user'
                  ? 'max-w-[85%] rounded-tr-sm bg-primary text-primary-foreground sm:max-w-[78%]'
                  : 'max-w-[92%] rounded-tl-sm bg-muted text-foreground sm:max-w-[84%] lg:max-w-[78%]'
              )}>
                {msg.loading
                  ? <Loader2 className="w-4 h-4 animate-spin" />
                  : msg.role === 'assistant'
                    ? (
                        <div className="prose prose-sm max-w-none dark:prose-invert prose-p:my-2 prose-li:my-0.5 prose-ul:my-2 prose-ol:my-2">
                          <ReactMarkdown remarkPlugins={[remarkGfm]} components={MARKDOWN_COMPONENTS}>
                            {msg.content}
                          </ReactMarkdown>
                        </div>
                      )
                    : <span className="whitespace-pre-wrap">{msg.content}</span>}
              </div>
              </div>
            ))}
            <div ref={bottomRef} />
          </div>
        )}
      </div>

      {/* Input */}
      <div className="sticky bottom-0 z-10 -mx-1 rounded-[24px] border border-white/5 bg-background/85 px-1 py-2 pb-[calc(0.4rem+env(safe-area-inset-bottom))] backdrop-blur sm:static sm:mx-0 sm:border-0 sm:bg-transparent sm:px-0 sm:py-0 sm:backdrop-blur-none">
        <div className="flex gap-2">
          <textarea
            ref={inputRef}
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Напиши вопрос..."
            rows={1}
            disabled={loading || historyLoading}
            className="flex-1 resize-none rounded-[20px] border bg-card/90 px-4 py-3 text-sm text-foreground placeholder:text-muted-foreground shadow-sm focus:outline-none focus:ring-2 focus:ring-primary/50 disabled:opacity-50"
            style={{ minHeight: '48px', maxHeight: '120px' }}
          />
          <button
            onClick={() => send(input)}
            disabled={!input.trim() || loading || historyLoading}
            className="flex h-12 w-12 shrink-0 items-center justify-center rounded-[20px] bg-primary text-primary-foreground shadow-sm transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-40"
          >
            {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Send className="w-4 h-4" />}
          </button>
        </div>
        <p className="mt-2 px-1 text-[11px] text-muted-foreground">
          Enter — отправить, Shift+Enter — перенос
        </p>
      </div>
    </div>
  )
}

function mapHistoryMessage(message: AIHistoryMessage): Message {
  return {
    id: message.id,
    role: message.role,
    content: message.content,
    created_at: message.created_at,
  }
}

function findCheckupLabel(period: CheckupPeriod) {
  return CHECKUP_ACTIONS.find(action => action.period === period)?.label.toLowerCase() ?? period
}

function formatLatestCheckup(latestCheckup: AILatestCheckup | null) {
  if (!latestCheckup?.has_report || !latestCheckup.generated_at) {
    return 'Последний checkup: ещё не запускался'
  }

  const generatedAt = new Date(latestCheckup.generated_at)
  const timestamp = Number.isNaN(generatedAt.getTime())
    ? latestCheckup.generated_at
    : new Intl.DateTimeFormat('ru-RU', {
        day: '2-digit',
        month: '2-digit',
        year: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      }).format(generatedAt)

  const periodLabel = latestCheckup.period_label?.trim()
  if (periodLabel) {
    return `Последний checkup: ${timestamp} (${periodLabel})`
  }
  return `Последний checkup: ${timestamp}`
}
