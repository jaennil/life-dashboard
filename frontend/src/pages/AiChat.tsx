import { useState, useRef, useEffect } from 'react'
import { Send, Bot, User, Loader2 } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import { cn } from '@/lib/utils'

interface Message {
  role: 'user' | 'assistant'
  content: string
  loading?: boolean
}

interface ChatResponse {
  content: string
}

interface SendResult {
  content: string
  isError?: boolean
}

const SUGGESTIONS = [
  'Сколько я потратил в этом месяце?',
  'На что больше всего трачу деньги?',
  'Когда последний раз тренировался?',
  'Сколько километров пробежал на этой неделе?',
  'Проанализируй мои финансы за месяц',
]

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

function sleep(ms: number) {
  return new Promise(resolve => window.setTimeout(resolve, ms))
}

async function requestChat(message: string, history: Message[]): Promise<SendResult> {
  const payload = JSON.stringify({
    message,
    history: history
      .filter(m => !m.loading)
      .map(m => ({ role: m.role, content: m.content })),
  })

  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      const res = await fetch('/api/v1/ai/chat', {
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
  const bottomRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  async function send(text: string) {
    if (!text.trim() || loading) return
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

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send(input)
    }
  }

  return (
    <div className="flex flex-col h-[calc(100vh-48px)]">
      <div className="mb-4">
        <h1 className="text-2xl font-bold text-foreground">AI Chat</h1>
        <p className="text-sm text-muted-foreground mt-1">Задавай вопросы о своих данных</p>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto rounded-xl border bg-card p-4 flex flex-col gap-4 min-h-0">
        {messages.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full gap-6 text-center">
            <div className="flex items-center justify-center w-14 h-14 rounded-2xl bg-primary/10">
              <Bot className="w-7 h-7 text-primary" />
            </div>
            <div>
              <p className="font-medium text-foreground">Чем могу помочь?</p>
              <p className="text-sm text-muted-foreground mt-1">У меня есть доступ к твоим финансам, активностям и тренировкам</p>
            </div>
            <div className="flex flex-wrap gap-2 justify-center max-w-lg">
              {SUGGESTIONS.map(s => (
                <button
                  key={s}
                  onClick={() => send(s)}
                  className="px-3 py-2 text-sm rounded-lg border bg-background hover:bg-accent hover:text-accent-foreground transition-colors text-left"
                >
                  {s}
                </button>
              ))}
            </div>
          </div>
        ) : (
          messages.map((msg, i) => (
            <div key={i} className={cn('flex gap-3', msg.role === 'user' && 'flex-row-reverse')}>
              <div className={cn(
                'flex items-center justify-center w-8 h-8 rounded-full shrink-0 mt-0.5',
                msg.role === 'user' ? 'bg-primary' : 'bg-muted'
              )}>
                {msg.role === 'user'
                  ? <User className="w-4 h-4 text-primary-foreground" />
                  : <Bot className="w-4 h-4 text-muted-foreground" />}
              </div>
              <div className={cn(
                'max-w-[75%] rounded-2xl px-4 py-2.5 text-sm',
                msg.role === 'user'
                  ? 'bg-primary text-primary-foreground rounded-tr-sm'
                  : 'bg-muted text-foreground rounded-tl-sm'
              )}>
                {msg.loading
                  ? <Loader2 className="w-4 h-4 animate-spin" />
                  : msg.role === 'assistant'
                    ? <div className="prose prose-sm dark:prose-invert max-w-none"><ReactMarkdown>{msg.content}</ReactMarkdown></div>
                    : <span className="whitespace-pre-wrap">{msg.content}</span>}
              </div>
            </div>
          ))
        )}
        <div ref={bottomRef} />
      </div>

      {/* Input */}
      <div className="mt-3 flex gap-2">
        <textarea
          ref={inputRef}
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Напиши вопрос... (Enter — отправить, Shift+Enter — перенос)"
          rows={1}
          disabled={loading}
          className="flex-1 resize-none rounded-xl border bg-card px-4 py-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-primary/50 disabled:opacity-50"
          style={{ minHeight: '48px', maxHeight: '120px' }}
        />
        <button
          onClick={() => send(input)}
          disabled={!input.trim() || loading}
          className="flex items-center justify-center w-12 h-12 rounded-xl bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-40 disabled:cursor-not-allowed transition-colors shrink-0"
        >
          {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Send className="w-4 h-4" />}
        </button>
      </div>
    </div>
  )
}
