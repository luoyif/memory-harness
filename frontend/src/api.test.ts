import { beforeEach, describe, expect, it, vi } from 'vitest'
import { APIClient, connect } from './api'

describe('desktop owner boundary', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    window.go = undefined
  })

  it('stays locked in an ordinary browser and only requests public health', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ status: 'ok' }) })
    vi.stubGlobal('fetch', fetchMock)
    const state = await connect()
    expect(state.mode).toBe('locked')
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(String(fetchMock.mock.calls[0][0])).toContain('/health')
  })

  it('keeps the owner credential in memory and sends CSRF only for mutations', async () => {
    window.go = { main: { DesktopBridge: { Bootstrap: async () => ({
      endpoint: 'http://127.0.0.1:43821', session_id: 'owner-test', token: 'mho_secret',
      csrf_token: 'mhc_secret', expires_at: '2026-08-22T00:00:00Z', version: '2.0.0',
    }) } } }
    const connected = await connect()
    expect(connected.mode).toBe('desktop')
    if (connected.mode !== 'desktop') throw new Error('desktop session expected')
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, text: async () => '{}' })
    vi.stubGlobal('fetch', fetchMock)
    const api = new APIClient(connected.session)
    await api.get('/v1/projects')
    await api.post('/v1/projects', { name: 'test' })
    const getHeaders = fetchMock.mock.calls[0][1].headers as Headers
    const postHeaders = fetchMock.mock.calls[1][1].headers as Headers
    expect(getHeaders.get('X-Memory-Harness-Owner')).toBe('mho_secret')
    expect(getHeaders.has('X-Memory-Harness-CSRF')).toBe(false)
    expect(postHeaders.get('X-Memory-Harness-CSRF')).toBe('mhc_secret')
    expect(localStorage.length).toBe(0)
    expect(sessionStorage.length).toBe(0)
  })
})
