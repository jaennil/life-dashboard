export interface AIMessage {
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

export interface CheckupSendResult extends SendResult {
  generatedAt?: string
  periodLabel?: string
}

export interface AIStreamEvent {
  type: 'delta' | 'done' | 'error' | 'status'
  content?: string
  period?: CheckupPeriod
  period_label?: string
  generated_at?: string
  stage?: 'planning' | 'loading' | 'generating'
  tool?: string
  section?: string
}

export type CheckupPeriod = 'today' | 'yesterday' | 'week' | 'month' | 'since_last'

export interface CheckupAction {
  period: CheckupPeriod
  label: string
  userMessage: string
}

export const CHECKUP_ACTIONS: CheckupAction[] = [
  { period: 'today', label: 'Сегодня', userMessage: 'Сделай checkup за сегодня' },
  { period: 'yesterday', label: 'Вчера', userMessage: 'Сделай checkup за вчера' },
  { period: 'week', label: '7 дней', userMessage: 'Сделай checkup за неделю' },
  { period: 'month', label: '30 дней', userMessage: 'Сделай checkup за месяц' },
  { period: 'since_last', label: 'С прошлого', userMessage: 'Сделай checkup с момента последнего отчёта' },
]

const RETRYABLE_CHAT_STATUSES = new Set([502, 503, 504])
const CHAT_RETRY_DELAY_MS = 400
const CHAT_CONTEXT_MESSAGE_LIMIT = 24

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

function sleep(ms: number) {
  return new Promise(resolve => window.setTimeout(resolve, ms))
}

export interface AIStreamHandlers {
  onDelta: (content: string) => void
  onStatus?: (event: AIStreamEvent) => void
}

export type StreamStatusState = 'pending' | 'active' | 'done'

export interface StreamStatusItem {
  key: string
  label: string
  state: StreamStatusState
}

const CHECKUP_STATUS_TEMPLATE: StreamStatusItem[] = [
  { key: 'Готовлю данные для checkup', label: 'Готовлю данные для checkup', state: 'pending' },
  { key: 'Загружаю финансы', label: 'Загружаю финансы', state: 'pending' },
  { key: 'Загружаю задачи', label: 'Загружаю задачи', state: 'pending' },
  { key: 'Загружаю здоровье', label: 'Загружаю здоровье', state: 'pending' },
  { key: 'Загружаю активности', label: 'Загружаю активности', state: 'pending' },
  { key: 'Загружаю тренировки', label: 'Загружаю тренировки', state: 'pending' },
  { key: 'Загружаю питание', label: 'Загружаю питание', state: 'pending' },
  { key: 'Загружаю привычки', label: 'Загружаю привычки', state: 'pending' },
  { key: 'Загружаю заметки', label: 'Загружаю заметки', state: 'pending' },
  { key: 'Загружаю календарь', label: 'Загружаю календарь', state: 'pending' },
  { key: 'Собираю итоговый отчёт', label: 'Собираю итоговый отчёт', state: 'pending' },
]

export function buildCheckupStatusItems() {
  return CHECKUP_STATUS_TEMPLATE.map<StreamStatusItem>((item, index) => ({
    ...item,
    state: index === 0 ? 'active' : 'pending',
  }))
}

export function activateStreamStatus(items: StreamStatusItem[], label: string) {
  const normalized = label.trim()
  if (!normalized) return items

  const currentActive = items.find(item => item.state === 'active')?.key
  if (currentActive === normalized) {
    return items
  }

  let found = false
  const next = items.map(item => {
    if (item.state === 'active') {
      return { ...item, state: 'done' as const }
    }
    if (item.key === normalized) {
      found = true
      return { ...item, label: normalized, state: 'active' as const }
    }
    return item
  })

  if (found) {
    return next
  }

  return [...next, { key: normalized, label: normalized, state: 'active' as const }]
}

export async function requestChatStream(message: string, history: AIMessage[], handlers: AIStreamHandlers): Promise<SendResult> {
  const payload = JSON.stringify({
    message,
    history: history
      .filter(m => !m.loading)
      .slice(-CHAT_CONTEXT_MESSAGE_LIMIT)
      .map(m => ({ role: m.role, content: m.content })),
  })

  return requestAIStream('/api/v1/ai/chat', payload, handlers)
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

export async function requestCheckupStream(period: CheckupPeriod, handlers: AIStreamHandlers): Promise<CheckupSendResult> {
  return requestAIStream('/api/v1/ai/checkup', JSON.stringify({ period }), handlers)
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

async function requestAIStream(url: string, payload: string, handlers: AIStreamHandlers): Promise<CheckupSendResult> {
  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      const res = await fetch(`${url}?stream=1`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: payload,
      })
      const raw = await (async () => {
        if (res.ok || !res.body) return ''
        return res.text()
      })()

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

      if (!res.body) {
        return url.includes('/checkup')
          ? requestCheckup(JSON.parse(payload).period as CheckupPeriod)
          : requestAI(url, payload)
      }

      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''
      let content = ''
      let finalEventType: AIStreamEvent['type'] | null = null
      let finalEventContent = ''
      let finalEventPeriodLabel: string | undefined
      let finalEventGeneratedAt: string | undefined

      const handleLine = (line: string) => {
        if (!line.trim()) return
        let event: AIStreamEvent
        try {
          event = JSON.parse(line) as AIStreamEvent
        } catch {
          return
        }

        if (event.type === 'delta' && event.content) {
          content += event.content
          handlers.onDelta(content)
          return
        }

        if (event.type === 'status') {
          handlers.onStatus?.(event)
          return
        }

        if (event.type === 'done') {
          if (event.content && event.content !== content) {
            content = event.content
            handlers.onDelta(content)
          }
          finalEventType = 'done'
          finalEventContent = event.content || content
          finalEventPeriodLabel = event.period_label
          finalEventGeneratedAt = event.generated_at
          return
        }

        if (event.type === 'error') {
          finalEventType = 'error'
          finalEventContent = event.content || ''
        }
      }

      while (true) {
        const { value, done } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })

        let newlineIndex = buffer.indexOf('\n')
        while (newlineIndex >= 0) {
          const line = buffer.slice(0, newlineIndex).trim()
          buffer = buffer.slice(newlineIndex + 1)
          handleLine(line)
          newlineIndex = buffer.indexOf('\n')
        }
      }

      buffer += decoder.decode()
      if (buffer.trim()) {
        handleLine(buffer.trim())
      }

      if (finalEventType === 'error') {
        return {
          content: finalEventContent || 'AI сервис сейчас недоступен. Попробуй позже.',
          isError: true,
        }
      }

      if (finalEventType === 'done') {
        return {
          content: finalEventContent || content,
          periodLabel: finalEventPeriodLabel,
          generatedAt: finalEventGeneratedAt,
        }
      }

      if (content.trim()) {
        return { content }
      }

      if (attempt === 0) {
        await sleep(CHAT_RETRY_DELAY_MS)
        continue
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
