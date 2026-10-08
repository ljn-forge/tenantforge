import { afterEach, expect, test, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import ResourceDetails from '../src/components/resources/ResourceDetails.vue'
import TicketSupport from '../src/components/resources/TicketSupport.vue'
import { state } from '../src/stores/app'
import { resetClient } from '../src/api/client'
import type { Me, Module, ResourceRecord } from '../src/api/types'

const descriptor: Module = {
  key: 'tickets',
  label: '工单',
  label_en: 'Tickets',
  description: '',
  actions: ['read', 'reply', 'assign', 'update'],
  fields: [],
}
function record(kinds?: ('reply' | 'note')[]): ResourceRecord {
  return {
    id: 'ticket',
    tenant_id: 'tenant',
    owner_id: 'user',
    department_id: null,
    version: 1,
    data: { title: 'Broken printer', status: 'in_progress' },
    capabilities: {
      actions: ['read', 'reply', 'assign', 'update'],
      write_fields: ['status', 'resolution'],
      transitions: ['resolved'],
      ...(kinds ? { reply_kinds: kinds } : {}),
    },
    created_at: '',
    updated_at: '',
  }
}
function setup(kinds?: ('reply' | 'note')[]) {
  state.locale = 'en'
  state.tenant = 'tenant'
  state.me = {
    user: { id: 'user', display_name: 'Agent' },
    permissions: [{ resource: 'tickets', action: 'reply', scope: 'tenant', write_fields: ['*'] }],
    modules: [descriptor],
  } as Me
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async (url: string) =>
        new Response(
          JSON.stringify(
            url.endsWith('/auth/csrf')
              ? { data: { csrf_token: 'fixture' } }
              : { data: [], meta: { total: 0, page: 1, page_size: 20 } },
          ),
        ),
    ),
  )
  return mount(ResourceDetails, {
    props: { record: record(kinds), descriptor, busy: false },
    global: { plugins: [ElementPlus] },
  })
}
afterEach(() => {
  vi.unstubAllGlobals()
  state.me = null
  resetClient()
})
test('ticket handling uses explicit reply kinds and does not derive notes from global write grants', async () => {
  const wrapper = setup(['reply'])
  await flushPromises()
  expect(wrapper.find('.ticket-support').exists()).toBe(true)
  expect(wrapper.find('.ticket-composer').text()).toContain('Customer reply')
  expect(wrapper.find('.ticket-composer').text()).not.toContain('Internal note')
  await wrapper.setProps({ record: record() })
  await flushPromises()
  expect(wrapper.find('.ticket-composer').exists()).toBe(false)
  wrapper.unmount()
})
test('resolve opens a required summary dialog and never posts a blank resolution', async () => {
  const wrapper = setup(['reply', 'note'])
  await flushPromises()
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Resolve')!
    .trigger('click')
  await flushPromises()
  expect(wrapper.find('.ticket-resolution').exists()).toBe(true)
  expect(wrapper.find('.ticket-resolution textarea').exists()).toBe(true)
  const submit = wrapper
    .find('.ticket-resolution')
    .findAll('button')
    .find((button) => button.text() === 'Resolve ticket')!
  expect(submit.attributes('disabled')).toBeDefined()
  await wrapper.find('.ticket-resolution textarea').setValue('Printer replaced')
  await wrapper.find('.ticket-resolution form').trigger('submit')
  expect(wrapper.emitted('transition')?.[0]).toEqual([
    record(['reply', 'note']),
    'resolved',
    'Printer replaced',
  ])
  wrapper.unmount()
})
test('reply failure preserves draft and context change discards late history and draft', async () => {
  const wrapper = setup(['reply'])
  await flushPromises()
  const fetcher = vi.mocked(fetch)
  fetcher.mockImplementation(async (url) => {
    if (String(url).endsWith('/auth/csrf'))
      return new Response(JSON.stringify({ data: { csrf_token: 'fixture' } }))
    return new Response(JSON.stringify({ error: { code: 'unavailable', message: 'Try again' } }), {
      status: 503,
    })
  })
  await wrapper.find('.ticket-composer textarea').setValue('Keep this draft')
  await wrapper.find('.ticket-composer').trigger('submit')
  await flushPromises()
  expect(wrapper.text()).toContain('Try again')
  expect((wrapper.find('.ticket-composer textarea').element as HTMLTextAreaElement).value).toBe(
    'Keep this draft',
  )
  let resolve!: (value: Response) => void
  fetcher.mockImplementation(
    () =>
      new Promise<Response>((done) => {
        resolve = done
      }),
  )
  await wrapper
    .find('.ticket-activity')
    .findAll('button')
    .find((button) => button.text() === 'Refresh history')!
    .trigger('click')
  state.me!.permissions = []
  await flushPromises()
  resolve(
    new Response(
      JSON.stringify({
        data: [
          {
            id: 'old',
            kind: 'reply',
            body: 'Sensitive old history',
            actor_name: 'Agent',
            created_at: '',
          },
        ],
        meta: { total: 1, page: 1, page_size: 20 },
      }),
    ),
  )
  await flushPromises()
  expect(wrapper.text()).not.toContain('Sensitive old history')
  expect(wrapper.text()).not.toContain('Keep this draft')
  expect(wrapper.find('.ticket-composer').exists()).toBe(false)
  wrapper.unmount()
})
test('reply submission uses optimistic version once and only publishes a persisted result', async () => {
  const wrapper = setup(['reply', 'note'])
  await flushPromises()
  let resolve!: (response: Response) => void
  const payloads: unknown[] = []
  vi.mocked(fetch).mockImplementation(async (url, options) => {
    if (String(url).endsWith('/auth/csrf'))
      return new Response(JSON.stringify({ data: { csrf_token: 'fixture' } }))
    if (String(url).endsWith('/replies')) {
      payloads.push(JSON.parse(String(options?.body)))
      return new Promise((done) => {
        resolve = done
      })
    }
    return new Response(JSON.stringify({ data: [], meta: { total: 0, page: 1, page_size: 20 } }))
  })
  await wrapper.find('.ticket-composer textarea').setValue('Customer confirmed')
  await wrapper.find('.ticket-composer').trigger('submit')
  await flushPromises()
  await wrapper.find('.ticket-composer').trigger('submit')
  expect(payloads).toEqual([{ version: 1, kind: 'reply', body: 'Customer confirmed' }])
  expect(wrapper.emitted('updated')).toBeUndefined()
  const persisted = { ...record(['reply', 'note']), version: 2 }
  resolve(new Response(JSON.stringify({ data: { record: persisted, event: { kind: 'reply' } } })))
  await flushPromises()
  expect(wrapper.emitted('updated')?.[0]).toEqual([persisted])
  expect((wrapper.find('.ticket-composer textarea').element as HTMLTextAreaElement).value).toBe('')
  wrapper.unmount()
})
test('activity pagination displays authorized server total and system event content is not interpreted as a message', async () => {
  const wrapper = setup(['reply'])
  await flushPromises()
  const requests: string[] = []
  vi.mocked(fetch).mockImplementation(async (url) => {
    requests.push(String(url))
    return new Response(
      JSON.stringify({
        data: [
          {
            id: 'event',
            kind: 'updated',
            actor_name: 'Agent',
            body: 'Unexpected system body',
            created_at: '',
            fields: [],
          },
        ],
        meta: { total: 37, page: 1, page_size: 20 },
      }),
    )
  })
  await wrapper
    .find('.ticket-activity')
    .findAll('button')
    .find((button) => button.text() === 'Refresh history')!
    .trigger('click')
  await flushPromises()
  expect(wrapper.find('.ticket-history-pagination').text()).toContain('37 visible events')
  expect(wrapper.text()).not.toContain('Unexpected system body')
  await wrapper.find('.ticket-history-pagination .btn-next').trigger('click')
  await flushPromises()
  expect(requests.at(-1)).toContain('page=2&page_size=20')
  wrapper.unmount()
})
test('failed conflict refresh leaves submission blocked with the original draft intact', async () => {
  const wrapper = setup(['reply'])
  await flushPromises()
  vi.mocked(fetch).mockImplementation(async (url) => {
    if (String(url).endsWith('/auth/csrf'))
      return new Response(JSON.stringify({ data: { csrf_token: 'fixture' } }))
    return new Response(
      JSON.stringify({
        error: {
          code: String(url).endsWith('/replies') ? 'version_conflict' : 'unavailable',
          message: 'Changed or unavailable',
        },
      }),
      { status: String(url).endsWith('/replies') ? 409 : 503 },
    )
  })
  await wrapper.find('.ticket-composer textarea').setValue('Conflict draft')
  await wrapper.find('.ticket-composer').trigger('submit')
  await flushPromises()
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Reload latest')!
    .trigger('click')
  await flushPromises()
  expect(
    wrapper
      .findComponent(TicketSupport)
      .findAll('button')
      .find((button) => button.text() === 'Add reply')!
      .attributes('disabled'),
  ).toBeDefined()
  expect((wrapper.find('.ticket-composer textarea').element as HTMLTextAreaElement).value).toBe(
    'Conflict draft',
  )
  wrapper.unmount()
})
test('assignment-only permission submits a dedicated action with an active scoped member, no update data', async () => {
  const wrapper = setup()
  await wrapper.setProps({
    record: {
      ...record(),
      capabilities: { actions: ['read', 'assign'], write_fields: [], transitions: [] },
    },
  })
  state.me!.permissions.push(
    { resource: 'members', action: 'read', scope: 'department' },
    { resource: 'departments', action: 'read', scope: 'department' },
  )
  // Start a fresh component under the new authorization context.
  wrapper.unmount()
  const current = {
    ...record(),
    capabilities: { actions: ['read', 'assign'], write_fields: [], transitions: [] },
  }
  const payloads: unknown[] = []
  vi.mocked(fetch).mockImplementation(async (url, options) => {
    if (String(url).endsWith('/auth/csrf'))
      return new Response(JSON.stringify({ data: { csrf_token: 'fixture' } }))
    if (String(url).includes('/members?'))
      return new Response(
        JSON.stringify({
          data: [
            { id: 'm1', user_id: 'user', display_name: 'Active colleague', status: 'active' },
            {
              id: 'm2',
              user_id: 'inactive',
              display_name: 'Suspended colleague',
              status: 'suspended',
            },
          ],
          meta: { total: 2, page: 1, page_size: 100 },
        }),
      )
    if (String(url).endsWith('/assignment')) {
      payloads.push(JSON.parse(String(options?.body)))
      return new Response(JSON.stringify({ data: { ...current, version: 2 } }))
    }
    return new Response(JSON.stringify({ data: [], meta: { total: 0, page: 1, page_size: 20 } }))
  })
  const support = mount(TicketSupport, {
    props: { record: current, descriptor, busy: false },
    global: { plugins: [ElementPlus] },
  })
  await flushPromises()
  await support
    .findAll('button')
    .find((button) => button.text() === 'Assign ticket')!
    .trigger('click')
  await flushPromises()
  expect(support.text()).not.toContain('Suspended colleague')
  await support.find('.ticket-assignment form').trigger('submit')
  await flushPromises()
  expect(payloads).toEqual([{ version: 1, owner_id: 'user' }])
  support.unmount()
})
test('changed row ownership and reply authority clear prior private history before the new history request returns', async () => {
  const wrapper = setup(['reply', 'note'])
  await flushPromises()
  vi.mocked(fetch).mockResolvedValue(
    new Response(
      JSON.stringify({
        data: [
          {
            id: 'private',
            kind: 'note',
            body: 'Old private discussion',
            actor_name: 'Agent',
            created_at: '',
          },
        ],
        meta: { total: 1, page: 1, page_size: 20 },
      }),
    ),
  )
  await wrapper
    .find('.ticket-activity')
    .findAll('button')
    .find((button) => button.text() === 'Refresh history')!
    .trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('Old private discussion')
  await wrapper.find('.ticket-composer textarea').setValue('Old context draft')
  let resolve!: (response: Response) => void
  vi.mocked(fetch).mockImplementation(
    () =>
      new Promise<Response>((done) => {
        resolve = done
      }),
  )
  await wrapper.setProps({ record: { ...record(['reply']), owner_id: 'other', version: 2 } })
  expect(wrapper.text()).not.toContain('Old private discussion')
  expect((wrapper.find('.ticket-composer textarea').element as HTMLTextAreaElement).value).toBe('')
  resolve(new Response(JSON.stringify({ data: [], meta: { total: 0, page: 1, page_size: 20 } })))
  await flushPromises()
  wrapper.unmount()
})
test('a fresh same-version read-only record clears cached notes even when capabilities and legacy data keys are unchanged', async () => {
  const wrapper = setup()
  const readonly = {
    ...record(),
    capabilities: { actions: ['read'], write_fields: [], transitions: [] },
  }
  await wrapper.setProps({ record: readonly })
  await flushPromises()
  vi.mocked(fetch).mockResolvedValue(
    new Response(
      JSON.stringify({
        data: [
          {
            id: 'private',
            kind: 'note',
            body: 'Revoked private discussion',
            actor_name: 'Agent',
            created_at: '',
          },
        ],
        meta: { total: 1, page: 1, page_size: 20 },
      }),
    ),
  )
  await wrapper
    .find('.ticket-activity')
    .findAll('button')
    .find((button) => button.text() === 'Refresh history')!
    .trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('Revoked private discussion')
  let resolve!: (response: Response) => void
  vi.mocked(fetch).mockImplementation(
    () =>
      new Promise<Response>((done) => {
        resolve = done
      }),
  )
  await wrapper.setProps({ record: { ...readonly, data: { ...readonly.data } } })
  expect(wrapper.text()).not.toContain('Revoked private discussion')
  resolve(new Response(JSON.stringify({ data: [], meta: { total: 0, page: 1, page_size: 20 } })))
  await flushPromises()
  wrapper.unmount()
})
