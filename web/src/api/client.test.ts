import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { fetchWithAccessToken } from './client'

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
