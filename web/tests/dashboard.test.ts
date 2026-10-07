import { afterEach, expect, test, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import ElementPlus from 'element-plus'
import DashboardView from '../src/views/DashboardView.vue'
import { state } from '../src/stores/app'
import type { Me } from '../src/api/types'
afterEach(() => {
  state.me = null
  vi.unstubAllGlobals()
})
test('registered business extensions receive overview links and real count cards without a builtin allowlist', async () => {
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
    permissions: [{ resource: 'assets', action: 'read', scope: 'tenant', read_fields: ['*'] }],
    modules: [
      {
        key: 'assets',
        label: '资产',
        label_en: 'Assets',
        description: 'Managed assets',
        fields: [],
        actions: ['read', 'create'],
      },
    ],
  } satisfies Me
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async () =>
        new Response(JSON.stringify({ data: { counts: { assets: 3 }, recent_activity: [] } })),
    ),
  )
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: DashboardView },
      { path: '/resources/:module', component: { template: '<div />' } },
      { path: '/account', component: { template: '<div />' } },
    ],
  })
  await router.push('/')
  await router.isReady()
  const wrapper = mount(DashboardView, { global: { plugins: [ElementPlus, router] } })
  await flushPromises()
  expect(wrapper.get('.stat-card').attributes('href')).toBe('/resources/assets')
  expect(wrapper.find('.quick-links a[href="/resources/assets"]').exists()).toBe(true)
  expect(wrapper.get('.stat-card strong').text()).toBe('3')
  wrapper.unmount()
})
