import { afterEach, expect, test, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import ElementPlus from 'element-plus'
import ResourcesView from '../src/views/ResourcesView.vue'
import ResourceDetails from '../src/components/resources/ResourceDetails.vue'
import { state } from '../src/stores/app'
import { resetClient } from '../src/api/client'
import type { Me, ResourceRecord } from '../src/api/types'
const record: ResourceRecord = {
  id: 'ticket',
  tenant_id: 'tenant',
  owner_id: 'user',
  department_id: null,
  version: 1,
  data: { title: 'Ticket', status: 'open' },
  capabilities: {
    actions: ['read', 'update'],
    write_fields: ['status', 'resolution'],
    transitions: ['resolved'],
  },
  created_at: '',
  updated_at: '',
}
async function setup(url = '/resources/tickets') {
  state.locale = 'en'
  state.tenant = 'tenant'
  state.me = {
    user: { id: 'user' },
    permissions: [{ resource: 'tickets', action: 'read', scope: 'tenant', read_fields: ['*'] }],
    modules: [
      {
        key: 'tickets',
        label: '工单',
        label_en: 'Tickets',
        description: '',
        actions: ['read', 'update'],
        fields: [
          {
            key: 'status',
            label: '状态',
            label_en: 'Status',
            type: 'enum',
            options: ['open', 'in_progress', 'resolved', 'closed'],
          },
        ],
      },
    ],
  } as Me
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/resources/:module', component: ResourcesView }],
  })
  await router.push(url)
  await router.isReady()
  const wrapper = mount(ResourcesView, { global: { plugins: [ElementPlus, router] } })
  await flushPromises()
  return { wrapper, router }
}
function respond(url: string) {
  return new Response(
    JSON.stringify(
      url.endsWith('/ticket')
        ? { data: record }
        : url.includes('/activity?')
          ? { data: [], meta: { total: 0, page: 1, page_size: 20 } }
          : { data: [record], meta: { total: 1, page: 1, page_size: 20 } },
    ),
  )
}
afterEach(() => {
  vi.unstubAllGlobals()
  state.me = null
  resetClient()
})
test('ticket queues combine with existing filters and synchronize the resource route', async () => {
  const urls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      urls.push(url)
      return respond(url)
    }),
  )
  const { wrapper, router } = await setup('/resources/tickets?queue=mine&status=open')
  expect(new URL(urls[0]!, 'http://test').searchParams.get('queue')).toBe('mine')
  expect(new URL(urls[0]!, 'http://test').searchParams.get('status')).toBe('open')
  expect(wrapper.find('.ticket-queues').exists()).toBe(true)
  await wrapper
    .find('.ticket-queues')
    .findAll('button')
    .find((button) => button.text() === 'Resolved')!
    .trigger('click')
  await flushPromises()
  expect(router.currentRoute.value.query).toEqual({ queue: 'resolved', status: 'open' })
  wrapper.unmount()
})
test('permission changes close details and pending detail requests cannot republish old records', async () => {
  let resolve!: (value: Response) => void
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) =>
      url.endsWith('/ticket')
        ? new Promise<Response>((done) => {
            resolve = done
          })
        : respond(url),
    ),
  )
  const { wrapper } = await setup()
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'View')!
    .trigger('click')
  state.me!.permissions = []
  await flushPromises()
  resolve(respond('/ticket'))
  await flushPromises()
  expect(wrapper.findComponent(ResourceDetails).props('record')).toBeNull()
  wrapper.unmount()
})
