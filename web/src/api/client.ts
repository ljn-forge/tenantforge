import type { Envelope } from './types'

let tenant = ''
let csrf = ''
let csrfRequest: Promise<string> | null = null
let generation = 0
const controllers = new Set<AbortController>()

export class APIError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public fields?: Record<string, string>,
  ) {
    super(message)
    this.name = 'APIError'
  }
}
function invalidateContext() {
  generation++
  controllers.forEach((controller) => controller.abort())
  controllers.clear()
}
function assertContext(epoch: number) {
  if (epoch !== generation)
    throw new APIError(409, 'context_changed', 'Session or tenant context changed')
}
export function setTenant(id: string) {
  if (tenant !== id) {
    tenant = id
    invalidateContext()
    csrfRequest = null
  }
}
export function resetClient() {
  tenant = ''
  invalidateContext()
  csrf = ''
  csrfRequest = null
}

async function responseError(response: Response, epoch: number): Promise<APIError> {
  try {
    const body = await response.json()
    assertContext(epoch)
    return new APIError(
      response.status,
      typeof body.error?.code === 'string' ? body.error.code : 'request_failed',
      typeof body.error?.message === 'string' ? body.error.message : 'Request failed',
      body.error?.fields,
    )
  } catch {
    assertContext(epoch)
    return new APIError(response.status, 'request_failed', `Request failed (${response.status})`)
  }
}
async function csrfToken(): Promise<string> {
  if (csrf) return csrf
  if (csrfRequest) return csrfRequest
  const epoch = generation
  const controller = new AbortController()
  controllers.add(controller)
  const acquisition = (async () => {
    try {
      const response = await fetch('/api/v1/auth/csrf', {
        credentials: 'include',
        signal: controller.signal,
      })
      assertContext(epoch)
      if (!response.ok) throw await responseError(response, epoch)
      const body = await response.json()
      assertContext(epoch)
      if (typeof body.data?.csrf_token !== 'string' || !body.data.csrf_token)
        throw new APIError(502, 'invalid_response', 'Invalid CSRF response')
      csrf = body.data.csrf_token
      return csrf
    } finally {
      controllers.delete(controller)
    }
  })()
  const pending = acquisition.finally(() => {
    if (csrfRequest === pending) csrfRequest = null
  })
  csrfRequest = pending
  return pending
}
export interface APIOptions {
  method?: string
  body?: unknown
  signal?: AbortSignal
}
export async function api<T>(path: string, options: APIOptions = {}): Promise<Envelope<T>> {
  const method = options.method ?? 'GET'
  const epoch = generation
  const selectedTenant = tenant
  const headers = new Headers({ Accept: 'application/json' })
  if (selectedTenant) headers.set('X-Tenant-ID', selectedTenant)
  if (!['GET', 'HEAD'].includes(method)) headers.set('X-CSRF-Token', await csrfToken())
  assertContext(epoch)
  const form = options.body instanceof FormData
  if (options.body !== undefined && !form) headers.set('Content-Type', 'application/json')
  const controller = new AbortController()
  const abort = () => controller.abort()
  options.signal?.addEventListener('abort', abort, { once: true })
  if (options.signal?.aborted) controller.abort()
  controllers.add(controller)
  try {
    const response = await fetch(`/api/v1${path}`, {
      method,
      headers,
      credentials: 'include',
      body:
        options.body === undefined
          ? undefined
          : form
            ? (options.body as FormData)
            : JSON.stringify(options.body),
      signal: controller.signal,
    })
    assertContext(epoch)
    if (!response.ok) throw await responseError(response, epoch)
    if (response.status === 204) return { data: undefined as T }
    const result = await response.json()
    assertContext(epoch)
    if (!Object.hasOwn(result, 'data'))
      throw new APIError(502, 'invalid_response', 'Invalid response')
    return result as Envelope<T>
  } finally {
    controllers.delete(controller)
    options.signal?.removeEventListener('abort', abort)
  }
}

export async function download(path: string, filename: string) {
  const epoch = generation
  const headers = new Headers()
  if (tenant) headers.set('X-Tenant-ID', tenant)
  const controller = new AbortController()
  controllers.add(controller)
  try {
    const response = await fetch(`/api/v1${path}`, {
      credentials: 'include',
      headers,
      signal: controller.signal,
    })
    assertContext(epoch)
    if (!response.ok) throw await responseError(response, epoch)
    const blob = await response.blob()
    assertContext(epoch)
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = filename
    link.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  } finally {
    controllers.delete(controller)
  }
}

export async function allPages<T>(path: string): Promise<T[]> {
  const epoch = generation
  const records: T[] = []
  for (let page = 1; ; page++) {
    assertContext(epoch)
    const separator = path.includes('?') ? '&' : '?'
    const result = await api<T[]>(`${path}${separator}page=${page}&page_size=100`)
    assertContext(epoch)
    const data = Array.isArray(result.data) ? result.data : []
    records.push(...data)
    const finished = !data.length || !result.meta || records.length >= result.meta.total
    assertContext(epoch)
    if (finished) return records
  }
}
