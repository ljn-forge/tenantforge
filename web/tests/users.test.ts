import { afterEach, expect, test, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import ElementPlus from 'element-plus'
import UsersView from '../src/views/UsersView.vue'
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
    routes: [{ path: '/members', component: UsersView }],
  })
  await router.push('/members')
  await router.isReady()
  const wrapper = mount(UsersView, { global: { plugins: [ElementPlus, router] } })
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

test('assign-only membership authority exposes transfer while access inspection also requires role read', async () => {
  state.locale = 'en'
  state.me = {
    platform_admin: false,
    permissions: [{ resource: 'members', action: 'read', scope: 'tenant' }],
  } as Me
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            data: [
              {
                id: 'member',
                display_name: 'Assigned member',
                email: 'member@example.test',
                status: 'active',
                role_ids: [],
                capabilities: { actions: ['read', 'assign'], write_fields: [], transitions: [] },
              },
            ],
            meta: { total: 1, page: 1, page_size: 20 },
          }),
        ),
    ),
  )
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/members', component: UsersView }],
  })
  await router.push('/members')
  await router.isReady()
  const wrapper = mount(UsersView, { global: { plugins: [ElementPlus, router] } })
  await flushPromises()
  const buttons = () => wrapper.findAll('button').map((button) => button.text())
  expect(buttons()).toContain('Transfer')
  expect(buttons()).not.toContain('Edit')
  expect(buttons()).not.toContain('Access')
  state.me.permissions.push({ resource: 'roles', action: 'read', scope: 'tenant' })
  await flushPromises()
  expect(buttons()).toContain('Access')
  wrapper.unmount()
})
