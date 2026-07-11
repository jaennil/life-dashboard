import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/sentry', () => ({
  captureAPIFailure: vi.fn(),
}))

import {
  APIError,
  collectionQuery,
  dateRangeQuery,
  jsonHeaders,
  parseJSON,
  postJSON,
  postNoContent,
  request,
} from './api-client'

function response(body: BodyInit | null = null, init: ResponseInit = {}) {
  return new Response(body, { status: 200, ...init })
}

describe('API client', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('merges JSON headers from every HeadersInit representation', () => {
    const headers = jsonHeaders(new Headers([
      ['Content-Type', 'application/problem+json'],
      ['X-Request-ID', 'test-id'],
    ]))

    expect(headers.get('Content-Type')).toBe('application/problem+json')
    expect(headers.get('X-Request-ID')).toBe('test-id')
    expect(jsonHeaders([['X-Test', 'yes']]).get('Content-Type')).toBe('application/json')
  })

  it('builds JSON POST requests without allowing the method to drift', async () => {
    const fetchMock = vi.fn().mockResolvedValue(response('{"ok":true}'))
    vi.stubGlobal('fetch', fetchMock)

    await expect(postJSON<{ ok: boolean }>('/example', { value: 42 }, {
      method: 'DELETE',
      headers: { 'X-Test': 'yes' },
    })).resolves.toEqual({ ok: true })

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/v1/example')
    expect(init.method).toBe('POST')
    expect(init.body).toBe('{"value":42}')
    expect(new Headers(init.headers).get('Content-Type')).toBe('application/json')
    expect(new Headers(init.headers).get('X-Test')).toBe('yes')
  })

  it('does not add a content type to an empty POST', async () => {
    const fetchMock = vi.fn().mockResolvedValue(response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)

    await postNoContent('/empty')

    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(init).toMatchObject({ method: 'POST' })
    expect(init.headers).toBeUndefined()
    expect(init.body).toBeUndefined()
  })

  it('parses JSON, empty bodies, and no-content responses', async () => {
    await expect(parseJSON<{ value: number }>(response('{"value":7}'))).resolves.toEqual({ value: 7 })
    await expect(parseJSON<void>(response(''))).resolves.toBeUndefined()
    await expect(parseJSON<void>(response(null, { status: 204 }))).resolves.toBeUndefined()
  })

  it('returns structured errors for unsuccessful responses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response('invalid input', {
      status: 400,
      statusText: 'Bad Request',
    })))

    const error = await request('/broken', { method: 'PATCH' }).catch(value => value)

    expect(error).toBeInstanceOf(APIError)
    expect(error).toMatchObject({
      message: 'invalid input',
      path: '/broken',
      method: 'PATCH',
      statusCode: 400,
    })
  })
})

describe('API query builders', () => {
  it('omits empty date bounds', () => {
    expect(dateRangeQuery()).toBe('')
    expect(dateRangeQuery({ from: '2026-07-01', to: '' })).toBe('?from=2026-07-01')
  })

  it('serializes collection filters and preserves zero values', () => {
    expect(collectionQuery({
      page: 0,
      page_size: 30,
      search: '',
      order: 'desc',
      category: 'Food & drink',
    })).toBe('?page=0&page_size=30&order=desc&category=Food+%26+drink')
  })
})
