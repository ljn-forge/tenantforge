import { afterEach, expect, test, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import ElementPlus from 'element-plus'
import ActivityView from '../src/views/ActivityView.vue'
import ActivityDetails from '../src/components/activity/ActivityDetails.vue'
import { state } from '../src/stores/app'
import type { Me, Job } from '../src/api/types'
afterEach(() => {
  state.me = null
  vi.unstubAllGlobals()
})
test('job deletion is offered only for terminal jobs with job update authority', async () => {
  state.locale = 'en'
  state.me = {
    permissions: [
      { resource: 'jobs', action: 'read', scope: 'tenant' },
      { resource: 'jobs', action: 'update', scope: 'tenant' },
    ],
  } as Me
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            data: [
              {
                id: 'done',
                capabilities: { actions: ['read', 'update'], write_fields: [], transitions: [] },
                type: 'export',
                status: 'succeeded',
                attempts: 1,
                max_attempts: 3,
                created_at: '',
                updated_at: '',
              },
              {
                id: 'active',
                capabilities: { actions: ['read', 'update'], write_fields: [], transitions: [] },
                type: 'export',
                status: 'running',
                attempts: 1,
                max_attempts: 3,
                created_at: '',
                updated_at: '',
              },
            ],
            meta: { total: 2, page: 1, page_size: 20 },
          }),
        ),
    ),
  )
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/jobs', component: ActivityView }],
  })
  await router.push('/jobs')
  await router.isReady()
  const wrapper = mount(ActivityView, { global: { plugins: [ElementPlus, router] } })
  await flushPromises()
  expect(wrapper.findAll('button').filter((button) => button.text() === 'Delete')).toHaveLength(1)
  // An updated server row authority, not the union of unrelated grants, removes controls.
  await wrapper.setProps({})
  const fetch = globalThis.fetch as ReturnType<typeof vi.fn>
  fetch.mockResolvedValue(
    new Response(
      JSON.stringify({
        data: [
          {
            id: 'done',
            type: 'export',
            status: 'succeeded',
            capabilities: { actions: ['read'], write_fields: [], transitions: [] },
          },
        ],
        meta: { total: 1, page: 1, page_size: 20 },
      }),
    ),
  )
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Refresh')!
    .trigger('click')
  await flushPromises()
  expect(wrapper.findAll('button').filter((button) => button.text() === 'Delete')).toHaveLength(0)
  wrapper.unmount()
})

test('failed job details use safe diagnostic guidance instead of raw error strings', async () => {
  state.locale = 'zh'
  state.me = { permissions: [{ resource: 'jobs', action: 'read', scope: 'tenant' }] } as Me
  const job = {
    id: 'failed-job',
    type: 'export',
    status: 'failed',
    error_code: 'permission_revoked',
    error_stage: 'authorize',
    error: 'private provider credential',
    capabilities: { actions: ['read'], write_fields: [], transitions: [] },
    attempts: 1,
    max_attempts: 3,
  }
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async (url: string) =>
        new Response(
          JSON.stringify({
            data: url.endsWith('/failed-job') ? job : [job],
            meta: { total: 1, page: 1, page_size: 20 },
          }),
        ),
    ),
  )
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/jobs', component: ActivityView }],
  })
  await router.push('/jobs')
  await router.isReady()
  const wrapper = mount(ActivityView, { global: { plugins: [ElementPlus, router] } })
  await flushPromises()
  await wrapper
    .findAll('button')
    .find((button) => button.text() === '详情')!
    .trigger('click')
  await flushPromises()
  expect(wrapper.find('.el-drawer').text()).not.toContain('private provider credential')
  expect(wrapper.find('.el-drawer').text()).toContain('当前权限不再允许此任务')
  expect(wrapper.find('.el-drawer').text()).toContain('权限检查')
  wrapper.unmount()
})

test('registered results show bilingual typed fields while undeclared values stay hidden', async () => {
  state.locale = 'zh'
  state.me = { permissions: [{ resource: 'jobs', action: 'read', scope: 'tenant' }] } as Me
  const job = {
    id: 'typed',
    type: 'inventory',
    status: 'succeeded',
    definition: {
      label: '资产盘点',
      label_en: 'Asset inventory',
      description: '授权资产统计',
      result_fields: [
        { key: 'count', label: '资产数量', label_en: 'Asset count', type: 'number' },
        { key: 'missing', label: '不可见', label_en: 'Hidden', type: 'number' },
      ],
    },
    result: { count: 7, undeclared: 'private provider value' },
    attempts: 1,
    max_attempts: 3,
  }
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async (url: string) =>
        new Response(
          JSON.stringify({
            data: url.endsWith('/typed') ? job : [job],
            meta: { total: 1, page: 1, page_size: 20 },
          }),
        ),
    ),
  )
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/jobs', component: ActivityView }],
  })
  await router.push('/jobs')
  await router.isReady()
  const wrapper = mount(ActivityView, { global: { plugins: [ElementPlus, router] } })
  await flushPromises()
  await wrapper
    .findAll('button')
    .find((button) => button.text() === '详情')!
    .trigger('click')
  await flushPromises()
  expect(wrapper.find('.el-drawer').text()).toContain('资产盘点')
  expect(wrapper.find('.typed-job-results').text()).toContain('资产数量')
  expect(wrapper.find('.typed-job-results').text()).toContain('7')
  expect(wrapper.find('.el-drawer').text()).not.toContain('private provider value')
  expect(wrapper.find('.typed-job-results').text()).not.toContain('不可见')
  wrapper.unmount()
})

test('legacy task result descriptors without localized labels retain visible field names', () => {
  state.locale = 'en'
  const wrapper = mount(ActivityDetails, {
    props: {
      record: {
        id: 'legacy',
        type: 'legacy',
        status: 'succeeded',
        error: '',
        attempts: 1,
        max_attempts: 3,
        created_at: '',
        updated_at: '',
        result: { count: 3 },
        definition: {
          label: '旧任务',
          label_en: 'Legacy task',
          result_fields: [{ key: 'count', label: '', label_en: '', type: 'number' }],
        },
      } as Job,
    },
    global: { plugins: [ElementPlus] },
  })
  expect(wrapper.find('.typed-job-results').text()).toContain('count')
  wrapper.unmount()
})
