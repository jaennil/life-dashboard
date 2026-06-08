import { captureAPIFailure } from '@/lib/sentry'

const BASE = '/api/v1'

export interface DateRangeParams {
  from?: string
  to?: string
}

export interface CollectionParams extends DateRangeParams {
  page?: number
  page_size?: number
  search?: string
  sort?: string
  order?: 'asc' | 'desc'
  type?: string
  category?: string
  split?: string
  payee?: string
}

export class APIError extends Error {
  path: string
  method: string
  statusCode?: number

  constructor(message: string, path: string, method: string, statusCode?: number) {
    super(message)
    this.name = 'APIError'
    this.path = path
    this.method = method
    this.statusCode = statusCode
  }
}

export function jsonHeaders(headers?: HeadersInit): HeadersInit {
  return {
    'Content-Type': 'application/json',
    ...(headers ?? {}),
  }
}

async function readErrorText(res: Response): Promise<string> {
  const text = (await res.text()).trim()
  return text || `${res.status} ${res.statusText}`
}

export async function request(path: string, init?: RequestInit): Promise<Response> {
  const method = (init?.method ?? 'GET').toUpperCase()

  try {
    const res = await fetch(BASE + path, init)
    if (!res.ok) {
      const message = await readErrorText(res)
      const error = new APIError(message, path, method, res.status)
      if (res.status >= 500) {
        captureAPIFailure({
          path,
          method,
          statusCode: res.status,
          responseBody: message,
          error,
        })
      }
      throw error
    }
    return res
  } catch (error) {
    if (error instanceof APIError) {
      throw error
    }

    captureAPIFailure({
      path,
      method,
      error,
    })
    throw error instanceof Error ? error : new Error(`request failed: ${method} ${path}`)
  }
}

export async function parseJSON<T>(res: Response): Promise<T> {
  if (res.status === 204) {
    return undefined as T
  }
  const text = await res.text()
  if (!text) {
    return undefined as T
  }
  return JSON.parse(text) as T
}

export async function get<T>(path: string): Promise<T> {
  const res = await request(path)
  return parseJSON<T>(res)
}

export async function postJSON<T>(path: string, body?: unknown, init?: RequestInit): Promise<T> {
  const res = await request(path, {
    method: 'POST',
    headers: jsonHeaders(init?.headers),
    ...init,
    body: body === undefined ? init?.body : JSON.stringify(body),
  })
  return parseJSON<T>(res)
}

export async function patchJSON<T>(path: string, body?: unknown): Promise<T> {
  const res = await request(path, {
    method: 'PATCH',
    headers: jsonHeaders(),
    body: JSON.stringify(body),
  })
  return parseJSON<T>(res)
}

export async function postNoContent(path: string, body?: unknown, init?: RequestInit): Promise<void> {
  await request(path, {
    method: 'POST',
    headers: body === undefined && !init?.headers ? undefined : jsonHeaders(init?.headers),
    ...init,
    body: body === undefined ? init?.body : JSON.stringify(body),
  })
}

export async function deleteNoContent(path: string): Promise<void> {
  await request(path, { method: 'DELETE' })
}

export function dateRangeQuery(params: DateRangeParams = {}) {
  const p = new URLSearchParams()
  if (params.from) p.set('from', params.from)
  if (params.to) p.set('to', params.to)
  const qs = p.toString()
  return qs ? '?' + qs : ''
}

export function collectionQuery(params: CollectionParams = {}) {
  const p = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== '') p.set(key, String(value))
  })
  const qs = p.toString()
  return qs ? '?' + qs : ''
}
