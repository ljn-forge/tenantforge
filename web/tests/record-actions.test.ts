import { afterEach, expect, test, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import ElementPlus from 'element-plus'
import ResourcesView from '../src/views/ResourcesView.vue'
import { state } from '../src/stores/app'
import type { Me } from '../src/api/types'
afterEach(() => {
  vi.unstubAllGlobals()
  state.me = null
})
test('read-all update-self exposes edit only on server-authorized rows and denies missing capabilities', async () => {
  state.locale = 'en'
  state.me = {
    permissions: [
      { resource: 'tickets', action: 'read', scope: 'tenant', read_fields: ['*'] },
      { resource: 'tickets', action: 'update', scope: 'self', write_fields: ['title'] },
    ],
    modules: [
      {
        key: 'tickets',
        label: '工单',
        label_en: 'Tickets',
        fields: [{ key: 'title', label: '标题', label_en: 'Title', type: 'string' }],
        actions: ['read', 'update'],
      },
    ],
  } as Me
  const records = [
    {
      id: 'own',
      version: 1,
      data: { title: 'Own ticket' },
      capabilities: { actions: ['read', 'update'], write_fields: ['title'], transitions: [] },
    },
    {
      id: 'other',
      version: 1,
      data: { title: 'Other ticket' },
      capabilities: { actions: ['read'], write_fields: [], transitions: [] },
    },
    { id: 'legacy', version: 1, data: { title: 'Missing authority' } },
  ]
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async () =>
        new Response(JSON.stringify({ data: records, meta: { total: 3, page: 1, page_size: 20 } })),
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
  expect(wrapper.findAll('button').filter((button) => button.text() === 'Edit')).toHaveLength(1)
  wrapper.unmount()
})

test('record edit fields are intersected with row authority even when another scope grants additional fields', async () => {
  state.locale = 'en'
  state.me = {
    permissions: [
      { resource: 'tickets', action: 'read', scope: 'tenant', read_fields: ['*'] },
      {
        resource: 'tickets',
        action: 'update',
        scope: 'tenant',
        write_fields: ['title', 'description'],
      },
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
          { key: 'description', label: '描述', label_en: 'Description', type: 'text' },
        ],
      },
    ],
  } as Me
  const record = {
    id: 'own',
    version: 1,
    data: { title: 'Own ticket', description: 'Restricted' },
    capabilities: { actions: ['read', 'update'], write_fields: ['title'], transitions: [] },
  }
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async (url: string) =>
        new Response(
          JSON.stringify({
            data: url.endsWith('/own') ? record : [record],
            meta: { total: 1, page: 1, page_size: 20 },
          }),
        ),
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
    .find((button) => button.text() === 'Edit')!
    .trigger('click')
  await flushPromises()
  expect(wrapper.findAll('.el-dialog .el-form-item__label').map((label) => label.text())).toEqual([
    'Title',
  ])
  wrapper.unmount()
})
