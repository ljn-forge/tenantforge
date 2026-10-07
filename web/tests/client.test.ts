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

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

test.each([200, 403])('tenant changes invalidate delayed response bodies (%s)', async (status) => {
  const body = deferred<unknown>()
  const json = vi.fn(() => body.promise)
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: status === 200, status, json }))
  setTenant('first')
  const request = api('/auth/me')
  const outcome = expect(request).rejects.toMatchObject({ code: 'context_changed' })
  await vi.waitFor(() => expect(json).toHaveBeenCalled())
  setTenant('second')
  body.resolve(
    status === 200 ? { data: { user: 'old' } } : { error: { code: 'forbidden', message: 'old' } },
  )
  await outcome
})

test('session reset aborts and invalidates requests even without a selected tenant', async () => {
  const response = deferred<Response>()
  let signal: AbortSignal | undefined
  vi.stubGlobal(
    'fetch',
    vi.fn((_url, options) => {
      signal = options.signal
      return response.promise
    }),
  )
  const request = api('/auth/me')
  const outcome = expect(request).rejects.toMatchObject({ code: 'context_changed' })
  resetClient()
  expect(signal?.aborted).toBe(true)
  response.resolve(new Response(JSON.stringify({ data: { user: 'old session' } })))
  await outcome
})

test('old CSRF bodies cannot publish a token or clear a newer acquisition', async () => {
  const oldBody = deferred<unknown>()
  const newBody = deferred<unknown>()
  const oldJSON = vi.fn(() => oldBody.promise)
  const newJSON = vi.fn(() => newBody.promise)
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce({ ok: true, status: 200, json: oldJSON })
    .mockResolvedValueOnce({ ok: true, status: 200, json: newJSON })
    .mockImplementation(async () => new Response(JSON.stringify({ data: {} })))
  vi.stubGlobal('fetch', fetchMock)
  const old = api('/auth/password', { method: 'POST' })
  const oldOutcome = expect(old).rejects.toMatchObject({ code: 'context_changed' })
  await vi.waitFor(() => expect(oldJSON).toHaveBeenCalled())
  resetClient()
  const first = api('/auth/login', { method: 'POST' })
  await vi.waitFor(() => expect(newJSON).toHaveBeenCalled())
  oldBody.resolve({ data: { csrf_token: 'stale-token' } })
  await oldOutcome
  const second = api('/auth/login', { method: 'POST' })
  expect(fetchMock).toHaveBeenCalledTimes(2)
  newBody.resolve({ data: { csrf_token: 'fresh-token' } })
  await Promise.all([first, second])
  const writes = fetchMock.mock.calls.slice(2)
  expect(writes).toHaveLength(2)
  for (const call of writes)
    expect(new Headers(call[1].headers).get('X-CSRF-Token')).toBe('fresh-token')
})

test('a paginated operation cannot combine pages across tenant contexts', async () => {
  const { allPages } = await import('../src/api/client')
  setTenant('first')
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    json: async () => ({
      data: [{ id: 'old' }],
      get meta() {
        setTenant('second')
        return { total: 2 }
      },
    }),
  })
  vi.stubGlobal('fetch', fetchMock)
  await expect(allPages('/departments')).rejects.toMatchObject({ code: 'context_changed' })
  expect(fetchMock).toHaveBeenCalledTimes(1)
})
