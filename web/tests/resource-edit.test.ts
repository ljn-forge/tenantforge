import { afterEach, expect, test, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import ElementPlus, { ElDialog, ElInput } from 'element-plus'
import ResourcesView from '../src/views/ResourcesView.vue'
import { state } from '../src/stores/app'
import { resetClient } from '../src/api/client'
import type { Me } from '../src/api/types'
afterEach(() => {
  vi.unstubAllGlobals()
  state.me = null
  resetClient()
})
test('editing a title preserves masked and write-only fields until the operator explicitly replaces them', async () => {
  state.locale = 'en'
  state.me = {
    permissions: [
      {
        resource: 'tickets',
        action: 'read',
        scope: 'tenant',
        read_fields: ['title', 'customer_email'],
      },
      {
        resource: 'tickets',
        action: 'update',
        scope: 'tenant',
        write_fields: ['title', 'customer_email', 'description'],
      },
    ],
    modules: [
      {
        key: 'tickets',
        label: '工单',
        label_en: 'Tickets',
        actions: ['read', 'update'],
        fields: [
          { key: 'title', label: '标题', label_en: 'Title', type: 'string', required: true },
          {
            key: 'customer_email',
            label: '邮箱',
            label_en: 'Customer email',
            type: 'string',
            sensitive: true,
          },
          { key: 'description', label: '描述', label_en: 'Description', type: 'text' },
        ],
      },
    ],
  } as Me
  const record = {
    id: 'ticket',
    capabilities: {
      actions: ['read', 'update'],
      write_fields: ['title', 'customer_email', 'description'],
      transitions: [],
    },
    version: 1,
    data: { title: 'Original', customer_email: '***' },
    owner_id: 'owner',
    department_id: null,
  }
  const patches: unknown[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, options?: RequestInit) => {
      if (url.endsWith('/auth/csrf'))
        return new Response(JSON.stringify({ data: { csrf_token: 'fixture' } }))
      if (options?.method === 'PATCH') {
        patches.push(JSON.parse(String(options.body)))
        return new Response(JSON.stringify({ data: record }))
      }
      return new Response(
        JSON.stringify({
          data: url.endsWith('/ticket') ? record : [record],
          meta: { total: 1, page: 1, page_size: 20 },
        }),
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
  async function open() {
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Edit')!
      .trigger('click')
    await flushPromises()
    return wrapper.findComponent(ElDialog)
  }
  let dialog = await open()
  const inputs = dialog.findAllComponents(ElInput)
  expect(inputs[1]!.props('modelValue')).toBe('')
  inputs[0]!.vm.$emit('update:modelValue', 'Changed')
  await flushPromises()
  await dialog.find('form').trigger('submit')
  await flushPromises()
  expect(patches[0]).toEqual({ version: 1, data: { title: 'Changed' } })
  dialog = await open()
  dialog.findAllComponents(ElInput)[1]!.vm.$emit('update:modelValue', 'replacement@example.test')
  await flushPromises()
  await dialog.find('form').trigger('submit')
  await flushPromises()
  expect(patches[1]).toEqual({ version: 1, data: { customer_email: 'replacement@example.test' } })
  wrapper.unmount()
})
test('export column selection uses export grants independently from list read fields and excludes sensitive wildcard access', async () => {
  state.locale = 'en'
  state.me = {
    permissions: [
      { resource: 'tickets', action: 'read', scope: 'tenant', read_fields: ['title'] },
      { resource: 'tickets', action: 'export', scope: 'tenant', read_fields: ['status'] },
    ],
    modules: [
      {
        key: 'tickets',
        label: '工单',
        label_en: 'Tickets',
        actions: ['read', 'export'],
        fields: [
          { key: 'title', label: '标题', label_en: 'Title', type: 'string' },
          { key: 'status', label: '状态', label_en: 'Status', type: 'string' },
          { key: 'secret', label: '秘密', label_en: 'Secret', type: 'string', sensitive: true },
        ],
      },
    ],
  } as Me
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async () =>
        new Response(JSON.stringify({ data: [], meta: { total: 0, page: 1, page_size: 20 } })),
    ),
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
    .find((button) => button.text() === 'Export')!
    .trigger('click')
  await flushPromises()
  expect(wrapper.findAll('.el-checkbox__label').map((label) => label.text())).toEqual(['Status'])
  state.me.permissions[1]!.read_fields = ['*']
  await flushPromises()
  expect(wrapper.findAll('.el-checkbox__label').map((label) => label.text())).toEqual([
    'Title',
    'Status',
  ])
  wrapper.unmount()
})

test('clearing an assigned department sends explicit null while leaving untouched record fields out of PATCH', async () => {
  state.locale = 'en'
  state.me = {
    permissions: [{ resource: 'tickets', action: 'read', scope: 'tenant' }],
    modules: [
      {
        key: 'tickets',
        label: '工单',
        label_en: 'Tickets',
        description: '',
        actions: ['read', 'update', 'assign'],
        fields: [
          { key: 'title', label: '标题', label_en: 'Title', type: 'string', required: true },
        ],
      },
    ],
  } as Me
  const record = {
    id: 'assigned',
    version: 4,
    owner_id: 'owner',
    department_id: 'department',
    data: { title: 'Assigned' },
    capabilities: {
      actions: ['read', 'update', 'assign'],
      write_fields: ['title'],
      transitions: [],
    },
  }
  const patches: unknown[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, options?: RequestInit) => {
      if (url.endsWith('/auth/csrf'))
        return new Response(JSON.stringify({ data: { csrf_token: 'fixture' } }))
      if (options?.method === 'PATCH') {
        patches.push(JSON.parse(String(options.body)))
        return new Response(JSON.stringify({ data: record }))
      }
      return new Response(
        JSON.stringify({
          data: url.endsWith('/assigned') ? record : [record],
          meta: { total: 1, page: 1, page_size: 20 },
        }),
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
    .find((button) => button.text() === 'Edit')!
    .trigger('click')
  await flushPromises()
  const { default: ResourceEditDialog } =
    await import('../src/components/resources/ResourceEditDialog.vue')
  wrapper.findComponent(ResourceEditDialog).vm.$emit('department', '')
  await flushPromises()
  await wrapper.find('.el-dialog form').trigger('submit')
  await flushPromises()
  expect(patches[0]).toEqual({ version: 4, department_id: null, data: {} })
  wrapper.unmount()
})
