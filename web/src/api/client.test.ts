import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api, fetchWithAccessToken, TongStockAPIError } from './client'

const storage = new Map<string, string>()

beforeEach(() => {
  storage.clear()
  vi.stubGlobal('window', {
    localStorage: {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
    },
    prompt: vi.fn(),
  })
})

describe('structured API errors', () => {
  it('throws a stable coded error instead of classifying localized text', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({
      error: { code: 'not_found', message: '请求的资源不存在', request_id: 'req-1' },
    }), {
      status: 404,
      headers: { 'Content-Type': 'application/json' },
    })))

    const error = await api.quote('600000').catch((reason) => reason)

    expect(error).toBeInstanceOf(TongStockAPIError)
    expect(error.code).toBe('not_found')
    expect(error.requestId).toBe('req-1')
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('fetchWithAccessToken', () => {
  it('does not add authorization for a successful local request', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response('{}', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    const response = await fetchWithAccessToken('/api/quote')

    expect(response.status).toBe(200)
    const headers = new Headers(fetchMock.mock.calls[0][1]?.headers)
    expect(headers.has('Authorization')).toBe(false)
  })

  it('prompts once on 401, stores the token, and retries with a bearer header', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response('{}', { status: 401 }))
      .mockResolvedValueOnce(new Response('{}', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    vi.mocked(window.prompt).mockReturnValue('secret')

    const response = await fetchWithAccessToken('/api/quote')

    expect(response.status).toBe(200)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    const retryHeaders = new Headers(fetchMock.mock.calls[1][1]?.headers)
    expect(retryHeaders.get('Authorization')).toBe('Bearer secret')
    expect(storage.get('tongstock.access_token')).toBe('secret')
  })
})
