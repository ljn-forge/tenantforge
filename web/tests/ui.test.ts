import { afterEach, expect, test, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import RoleEditor from '../src/components/RoleEditor.vue'
import ActionButton from '../src/components/ActionButton.vue'
import { state } from '../src/stores/app'
import { validateGrant } from '../src/api/permissions'
import type { Grant, Me, Module } from '../src/api/types'
const module: Module = {
  key: 'tickets',
  label: '工单',
  label_en: 'Tickets',
  description: '',
  actions: ['read', 'update'],
  fields: [
    { key: 'title', label: '标题', label_en: 'Title', type: 'string' },
    { key: 'status', label: '状态', label_en: 'Status', type: 'enum', options: ['open', 'closed'] },
    {
      key: 'secret',
      label: '秘密',
      label_en: 'Secret',
      type: 'string',
      sensitive: true,
      readonly: true,
    },
  ],
}
afterEach(() => {
  state.me = null
  vi.restoreAllMocks()
})
test('descriptor-aware grants reject unknown fields, readonly writes and wrong typed conditions', () => {
  const base: Grant = { resource: 'tickets', action: 'read', scope: 'self' }
  expect(validateGrant({ ...base, read_fields: ['invented'] }, module)).toContain(
    'Unknown readable field: invented',
  )
  expect(validateGrant({ ...base, write_fields: ['secret'] }, module)).toContain(
    'Read-only field cannot be writable: secret',
  )
  expect(
    validateGrant({ ...base, conditions: [{ field: 'status', op: 'eq', value: 'bogus' }] }, module)
      .length,
  ).toBeGreaterThan(0)
})
test('action button disappears for an unauthorized operation while read remains allowed', () => {
  state.me = { permissions: [{ resource: 'tickets', action: 'read', scope: 'tenant' }] } as Me
  const wrapper = mount(ActionButton, {
    props: { resource: 'tickets', action: 'update' },
    slots: { default: 'Edit' },
    global: { plugins: [ElementPlus] },
  })
  expect(wrapper.find('button').exists()).toBe(false)
  state.me.permissions.push({ resource: 'tickets', action: 'update', scope: 'self' })
  return wrapper.vm.$nextTick().then(() => {
    expect(wrapper.find('button').exists()).toBe(true)
  })
})
test('role editor renders operation scopes independently and requires selected departments', () => {
  const grants: Grant[] = [
    { resource: 'tickets', action: 'read', scope: 'tenant' },
    { resource: 'tickets', action: 'update', scope: 'selected_departments', department_ids: [] },
  ]
  const wrapper = mount(RoleEditor, {
    props: { modelValue: grants, modules: [module], departments: [] },
    global: { plugins: [ElementPlus] },
  })
  expect(wrapper.findAll('[data-grant]').length).toBe(2)
  expect(wrapper.text()).toContain('Select at least one department')
  expect(wrapper.findAll('[data-scope]').length).toBe(2)
})

test('administrator login submits its actual Element Plus form when the native submit button is clicked', async () => {
  const { createRouter, createMemoryHistory } = await import('vue-router')
  const { default: AuthView } = await import('../src/views/AuthView.vue')
  const { flushPromises } = await import('@vue/test-utils')
  state.initialized = true
  state.locale = 'en'
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/login', component: AuthView },
      { path: '/', component: { template: '<div>home</div>' } },
      { path: '/reset-password', component: AuthView },
    ],
  })
  await router.push('/login')
  await router.isReady()
  const calls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (path: string) => {
      calls.push(path)
      return new Response(
        JSON.stringify({
          data: path.endsWith('/csrf')
            ? { csrf_token: 'fixture-csrf' }
            : path.endsWith('/me')
              ? {
                  user: {
                    id: 'u',
                    email: 'test@example.test',
                    display_name: 'Test',
                    mfa_enabled: false,
                    created_at: '',
                  },
                  tenants: [],
                  permissions: [],
                  modules: [],
                  platform_admin: true,
                }
              : {},
        }),
        { status: 200 },
      )
    }),
  )
  const wrapper = mount(AuthView, {
    attachTo: document.body,
    global: { plugins: [ElementPlus, router] },
  })
  const inputs = wrapper.findAll('input')
  await inputs[0]!.setValue('admin')
  await inputs[1]!.setValue('Valid-fixture-password')
  ;(wrapper.get('button[type=submit]').element as HTMLButtonElement).click()
  await flushPromises()
  expect(wrapper.findAll('form')).toHaveLength(1)
  expect(calls).toContain('/api/v1/auth/login')
  wrapper.unmount()
  vi.unstubAllGlobals()
  state.initialized = true
})
