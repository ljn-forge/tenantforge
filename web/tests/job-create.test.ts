import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import ElementPlus, { ElDialog } from 'element-plus'
import ActivityView from '../src/views/ActivityView.vue'
import { state } from '../src/stores/app'
import { resetClient, setTenant } from '../src/api/client'
import type { Me } from '../src/api/types'
const definition = {
  type: 'inventory',
  label: '资产盘点',
  label_en: 'Asset inventory',
  description: '统计授权资产',
  description_en: 'Count authorized assets',
  resource: 'assets',
  action: 'read',
  input_fields: [{ key: 'name', label: '名称', label_en: 'Name', type: 'string', required: true }],
  result_fields: [{ key: 'count', label: '资产数量', label_en: 'Asset count', type: 'number' }],
}
function response(data: unknown) {
  return new Response(JSON.stringify({ data, meta: { total: 0, page: 1, page_size: 20 } }))
}
let wrapper: ReturnType<typeof mount> | undefined
beforeEach(() => {
  resetClient()
  state.locale = 'en'
  state.tenant = 'tenant-a'
  setTenant('tenant-a')
  state.me = {
    user: { id: 'user' },
    permissions: [
      { resource: 'jobs', action: 'read', scope: 'tenant' },
      { resource: 'jobs', action: 'create', scope: 'tenant' },
    ],
  } as Me
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  state.me = null
  state.tenant = ''
  resetClient()
  vi.unstubAllGlobals()
})
async function render() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/jobs', component: ActivityView }],
  })
  await router.push('/jobs')
  await router.isReady()
  wrapper = mount(ActivityView, { global: { plugins: [ElementPlus, router] } })
  await flushPromises()
  return wrapper
}
function button(view: ReturnType<typeof mount>, name: string) {
  return view.findAll('button').find((item) => item.text() === name)
}

test('registered task form validates then submits its declared payload once', async () => {
  const writes: unknown[] = []
  let finish!: (value: Response) => void
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, options: RequestInit) => {
      if (url.endsWith('/job-types')) return response([definition])
      if (url.endsWith('/auth/csrf')) return response({ csrf_token: 'fixture' })
      if (options.method === 'POST') {
        writes.push(JSON.parse(String(options.body)))
        return new Promise<Response>((resolve) => {
          finish = resolve
        })
      }
      return response([])
    }),
  )
  const view = await render()
  expect(button(view, 'New task')).toBeDefined()
  await button(view, 'New task')!.trigger('click')
  await flushPromises()
  const dialog = view.findComponent(ElDialog)
  await dialog.find('form').trigger('submit')
  await flushPromises()
  expect(writes).toEqual([])
  await vi.waitFor(() => expect(dialog.text()).toContain('Required'))
  await dialog.find('.task-inputs input').setValue('Inventory run')
  await dialog.find('form').trigger('submit')
  await flushPromises()
  await dialog.find('form').trigger('submit')
  expect(writes).toEqual([{ type: 'inventory', payload: { name: 'Inventory run' } }])
  finish(response({ id: 'job', type: 'inventory', status: 'pending' }))
  await flushPromises()
  expect(dialog.props('modelValue')).toBe(false)
})

test('actors without job creation authority never request or see the catalog', async () => {
  state.me!.permissions = state.me!.permissions.filter((grant) => grant.action !== 'create')
  const fetch = vi.fn(async (_url: string) => response([]))
  vi.stubGlobal('fetch', fetch)
  const view = await render()
  expect(button(view, 'New task')).toBeUndefined()
  expect(fetch.mock.calls.some((call) => String(call[0]).includes('/job-types'))).toBe(false)
})

test('late catalog from a previous tenant cannot populate current task choices', async () => {
  let finish!: (value: Response) => void
  let catalogs = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) =>
      url.endsWith('/job-types')
        ? ++catalogs === 1
          ? new Promise<Response>((resolve) => {
              finish = resolve
            })
          : response([])
        : response([]),
    ),
  )
  const view = await render()
  state.tenant = 'tenant-b'
  setTenant('tenant-b')
  await flushPromises()
  finish(response([definition]))
  await flushPromises()
  expect(button(view, 'New task')).toBeUndefined()
  expect(view.text()).toContain('No additional tasks')
})

test('session epoch reset clears an already opened task draft even when user and tenant strings have not changed', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => response(url.endsWith('/job-types') ? [definition] : [])),
  )
  const view = await render()
  await button(view, 'New task')!.trigger('click')
  await flushPromises()
  expect(view.findComponent(ElDialog).props('modelValue')).toBe(true)
  resetClient()
  await flushPromises()
  expect(view.findComponent(ElDialog).props('modelValue')).toBe(false)
  expect(button(view, 'New task')).toBeUndefined()
})

test('server field errors remain inline and an empty catalog explains availability', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, options: RequestInit) => {
      if (url.endsWith('/job-types')) return response([definition])
      if (url.endsWith('/auth/csrf')) return response({ csrf_token: 'fixture' })
      if (options.method === 'POST')
        return new Response(
          JSON.stringify({
            error: {
              code: 'validation',
              message: 'Review task input',
              fields: { name: 'Use a supported value' },
            },
          }),
          { status: 400 },
        )
      return response([])
    }),
  )
  const view = await render()
  await button(view, 'New task')!.trigger('click')
  await flushPromises()
  const dialog = view.findComponent(ElDialog)
  await dialog.find('.task-inputs input').setValue('invalid')
  await dialog.find('form').trigger('submit')
  await flushPromises()
  await vi.waitFor(() => expect(dialog.text()).toContain('Use a supported value'))
  expect(dialog.props('modelValue')).toBe(true)
})

test('permission revocation clears drafts and suppresses late submission completion', async () => {
  let finish!: (value: Response) => void
  let lists = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, options: RequestInit) => {
      if (url.endsWith('/job-types')) return response([definition])
      if (url.endsWith('/auth/csrf')) return response({ csrf_token: 'fixture' })
      if (options.method === 'POST')
        return new Promise<Response>((resolve) => {
          finish = resolve
        })
      lists++
      return response([])
    }),
  )
  const view = await render()
  await button(view, 'New task')!.trigger('click')
  await flushPromises()
  const dialog = view.findComponent(ElDialog)
  await dialog.find('.task-inputs input').setValue('private draft')
  await dialog.find('form').trigger('submit')
  await flushPromises()
  state.me!.permissions = state.me!.permissions.filter((grant) => grant.action !== 'create')
  await flushPromises()
  expect(dialog.props('modelValue')).toBe(false)
  expect(button(view, 'New task')).toBeUndefined()
  finish(response({ id: 'old-job' }))
  await flushPromises()
  expect(lists).toBe(1)
})

test('empty input definitions submit an empty payload and catalog failures offer a retry', async () => {
  const writes: unknown[] = []
  let attempts = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, options: RequestInit) => {
      if (url.endsWith('/job-types')) {
        if (++attempts === 1)
          return new Response(
            JSON.stringify({ error: { code: 'unavailable', message: 'Catalog unavailable' } }),
            { status: 503 },
          )
        return response([{ ...definition, input_fields: [] }])
      }
      if (url.endsWith('/auth/csrf')) return response({ csrf_token: 'fixture' })
      if (options.method === 'POST') writes.push(JSON.parse(String(options.body)))
      return response([])
    }),
  )
  const view = await render()
  expect(view.text()).toContain('Catalog unavailable')
  await button(view, 'Retry')!.trigger('click')
  await flushPromises()
  await button(view, 'New task')!.trigger('click')
  await flushPromises()
  const dialog = view.findComponent(ElDialog)
  expect(dialog.text()).toContain('needs no additional configuration')
  await dialog.find('form').trigger('submit')
  await flushPromises()
  expect(writes).toEqual([{ type: 'inventory', payload: {} }])
})
