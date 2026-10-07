import { afterEach, expect, test, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import ElementPlus, { ElDrawer } from 'element-plus'
import ResourcesView from '../src/views/ResourcesView.vue'
import { state } from '../src/stores/app'
import type { Me, ResourceRecord } from '../src/api/types'
afterEach(() => {
  vi.unstubAllGlobals()
  state.me = null
})
test('closing ticket details while a workflow refresh is pending never reopens the old drawer', async () => {
  const record: ResourceRecord = {
    id: 'record',
    capabilities: { actions: ['read', 'update'], write_fields: ['title'], transitions: [] },
    tenant_id: 'tenant',
    owner_id: 'user',
    department_id: null,
    version: 1,
    data: { title: 'Ticket', status: 'open' },
    created_at: '',
    updated_at: '',
  }
  state.locale = 'en'
  state.me = {
    user: {
      id: 'user',
      display_name: 'User',
      email: 'user@example.test',
      mfa_enabled: false,
      created_at: '',
    },
    tenants: [],
    platform_admin: false,
    permissions: [
      { resource: 'tickets', action: 'read', scope: 'tenant', read_fields: ['*'] },
      { resource: 'tickets', action: 'update', scope: 'tenant', write_fields: ['title'] },
    ],
    modules: [
      {
        key: 'tickets',
        label: '工单',
        label_en: 'Tickets',
        description: '',
        actions: ['read', 'update'],
        fields: [
          { key: 'title', label: '标题', label_en: 'Title', type: 'string' },
          {
            key: 'status',
            label: '状态',
            label_en: 'Status',
            type: 'enum',
            options: ['open', 'in_progress'],
          },
        ],
      },
    ],
  } satisfies Me
  let details = 0
  let resolveRefresh!: (response: Response) => void
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url.endsWith('/auth/csrf'))
        return new Response(JSON.stringify({ data: { csrf_token: 'fixture' } }))
      if (url.endsWith('/transition')) {
        record.data.status = 'in_progress'
        record.version++
        return new Response(JSON.stringify({ data: record }))
      }
      if (url.endsWith('/record')) {
        details++
        if (details === 3)
          return new Promise<Response>((resolve) => {
            resolveRefresh = resolve
          })
        return new Response(JSON.stringify({ data: record }))
      }
      return new Response(
        JSON.stringify({ data: [record], meta: { total: 1, page: 1, page_size: 20 } }),
      )
    }),
  )
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/resources/:module', component: ResourcesView }],
  })
  await router.push('/resources/tickets')
  await router.isReady()
  const wrapper = mount(ResourcesView, { global: { plugins: [ElementPlus, router] } })
  await flushPromises()
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'View')!
    .trigger('click')
  await flushPromises()
  const drawer = wrapper.findComponent(ElDrawer)
  expect(drawer.props('modelValue')).toBe(true)
  expect(drawer.find('.ticket-transitions').exists()).toBe(false)
  record.capabilities.write_fields = ['status']
  record.capabilities.transitions = ['in_progress']
  const drawerRecord = wrapper.findComponent(ElDrawer)
  // Fresh capability data arrives from the server when detail is reopened.
  drawerRecord.vm.$emit('close')
  await flushPromises()
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'View')!
    .trigger('click')
  await flushPromises()
  expect(drawer.find('.ticket-transitions').exists()).toBe(true)
  await drawer
    .findAll('button')
    .find((button) => button.text() === 'Start work')!
    .trigger('click')
  await flushPromises()
  drawer.vm.$emit('close')
  await flushPromises()
  resolveRefresh(new Response(JSON.stringify({ data: record })))
  await flushPromises()
  expect(drawer.props('modelValue')).toBe(false)
  wrapper.unmount()
})

test('closing details while a custom operation refresh is pending keeps the drawer closed', async () => {
  const record: ResourceRecord = {
    id: 'asset',
    tenant_id: 'tenant',
    owner_id: 'user',
    department_id: null,
    version: 1,
    data: { name: 'Asset' },
    capabilities: { actions: ['read', 'approve'], write_fields: [], transitions: [] },
    created_at: '',
    updated_at: '',
  }
  state.locale = 'en'
  state.me = {
    permissions: [{ resource: 'assets', action: 'read', scope: 'tenant' }],
    modules: [
      {
        key: 'assets',
        label: '资产',
        label_en: 'Assets',
        description: '',
        actions: ['read', 'approve'],
        operations: [{ key: 'approve', label: '批准', label_en: 'Approve' }],
        fields: [{ key: 'name', label: '名称', label_en: 'Name', type: 'string' }],
      },
    ],
  } as Me
  let details = 0
  let resolveRefresh!: (value: Response) => void
  const operations: unknown[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, options?: RequestInit) => {
      if (url.endsWith('/auth/csrf'))
        return new Response(JSON.stringify({ data: { csrf_token: 'fixture' } }))
      if (url.endsWith('/actions/approve')) {
        operations.push(JSON.parse(String(options?.body)))
        return new Response(JSON.stringify({ data: record }))
      }
      if (url.endsWith('/asset')) {
        details++
        if (details === 2)
          return new Promise<Response>((resolve) => {
            resolveRefresh = resolve
          })
        return new Response(JSON.stringify({ data: record }))
      }
      return new Response(
        JSON.stringify({ data: [record], meta: { total: 1, page: 1, page_size: 20 } }),
      )
    }),
  )
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/resources/:module', component: ResourcesView }],
  })
  await router.push('/resources/assets')
  await router.isReady()
  const wrapper = mount(ResourcesView, { global: { plugins: [ElementPlus, router] } })
  await flushPromises()
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'View')!
    .trigger('click')
  await flushPromises()
  const drawer = wrapper.findComponent(ElDrawer)
  await drawer
    .findAll('button')
    .find((button) => button.text() === 'Approve')!
    .trigger('click')
  await flushPromises()
  expect(operations).toEqual([{ version: 1, data: {} }])
  drawer.vm.$emit('close')
  await flushPromises()
  resolveRefresh(new Response(JSON.stringify({ data: record })))
  await flushPromises()
  expect(drawer.props('modelValue')).toBe(false)
  wrapper.unmount()
})
