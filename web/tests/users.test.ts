import { afterEach, expect, test, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import ElementPlus from 'element-plus'
import OrganizationView from '../src/views/OrganizationView.vue'
import { state } from '../src/stores/app'
import type { Me } from '../src/api/types'
afterEach(() => {
  state.me = null
  vi.unstubAllGlobals()
})
test('administrators can open direct user creation with username and password without relying on invitation delivery', async () => {
  state.locale = 'en'
  state.me = {
    platform_admin: true,
    permissions: [
      { resource: 'members', action: 'read', scope: 'tenant' },
      { resource: 'members', action: 'create', scope: 'tenant' },
      { resource: 'members', action: 'assign', scope: 'tenant' },
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
    routes: [{ path: '/members', component: OrganizationView }],
  })
  await router.push('/members')
  await router.isReady()
  const wrapper = mount(OrganizationView, { global: { plugins: [ElementPlus, router] } })
  await flushPromises()
  const add = wrapper.findAll('button').find((button) => button.text().includes('Add user'))
  expect(add).toBeDefined()
  await add!.trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('Username')
  expect(wrapper.text()).toContain('Initial password')
  expect(wrapper.text()).toContain('Display name')
  state.me.platform_admin = false
  await flushPromises()
  expect(
    wrapper.findAll('button').filter((button) => button.text().includes('Add user')),
  ).toHaveLength(0)
  wrapper.unmount()
})
