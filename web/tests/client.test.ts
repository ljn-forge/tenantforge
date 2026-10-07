import { beforeEach, expect, test, vi } from 'vitest'
import { api, setTenant, resetClient, APIError } from '../src/api/client'

beforeEach(() => {
  resetClient()
  vi.unstubAllGlobals()
})

test('mutations obtain CSRF then carry tenant context and cookies', async () => {
  const mock = vi
    .fn()
    .mockResolvedValueOnce(new Response(JSON.stringify({ data: { csrf_token: 'csrf-test' } })))
    .mockResolvedValueOnce(new Response(JSON.stringify({ data: { id: 'record' } })))
  vi.stubGlobal('fetch', mock)
  setTenant('tenant-a')
  await api('/resources/tickets', { method: 'POST', body: { data: { title: 'hello' } } })
  expect(mock).toHaveBeenCalledTimes(2)
  const options = mock.mock.calls[1]![1] as RequestInit
  expect(options.credentials).toBe('include')
  expect(new Headers(options.headers).get('X-Tenant-ID')).toBe('tenant-a')
  expect(new Headers(options.headers).get('X-CSRF-Token')).toBe('csrf-test')
})

test('GET failures preserve server error code without leaking a raw body', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: { code: 'forbidden', message: 'Access denied' } }), {
        status: 403,
      }),
    ),
  )
  await expect(api('/roles')).rejects.toMatchObject({
    status: 403,
    code: 'forbidden',
    message: 'Access denied',
  })
})

test('malformed error response gets a safe generic error', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(new Response('<html>private diagnostics</html>', { status: 502 })),
  )
  await expect(api('/roles')).rejects.toBeInstanceOf(APIError)
  await expect(api('/roles')).rejects.not.toHaveProperty(
    'message',
    expect.stringContaining('private diagnostics'),
  )
})

test('tenant switch changes only tenant context and does not persist tokens', async () => {
  const mock = vi.fn().mockImplementation(async () => new Response(JSON.stringify({ data: [] })))
  vi.stubGlobal('fetch', mock)
  setTenant('a')
  await api('/members')
  setTenant('b')
  await api('/members')
  expect(new Headers(mock.mock.calls[1]![1].headers).get('X-Tenant-ID')).toBe('b')
  expect(localStorage.getItem('tf_session')).toBeNull()
})

test('reference pickers consume every authorized page instead of truncating after the first hundred records', async () => {
  const { allPages } = await import('../src/api/client')
  const requests: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      requests.push(url)
      const second = url.includes('page=2')
      return new Response(
        JSON.stringify({
          data: [{ id: second ? 'second' : 'first' }],
          meta: { page: second ? 2 : 1, page_size: 1, total: 2 },
        }),
      )
    }),
  )
  expect(await allPages<{ id: string }>('/departments')).toEqual([
    { id: 'first' },
    { id: 'second' },
  ])
  expect(requests).toHaveLength(2)
})
