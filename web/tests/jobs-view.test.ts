import { afterEach, expect, test, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import ElementPlus from 'element-plus'
import ActivityView from '../src/views/ActivityView.vue'
import { state } from '../src/stores/app'
import type { Me } from '../src/api/types'
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
                type: 'export',
                status: 'succeeded',
                attempts: 1,
                max_attempts: 3,
                created_at: '',
                updated_at: '',
              },
              {
                id: 'active',
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
  state.me.permissions = state.me.permissions.filter((grant) => grant.action !== 'update')
  await flushPromises()
  expect(wrapper.findAll('button').filter((button) => button.text() === 'Delete')).toHaveLength(0)
  wrapper.unmount()
})
