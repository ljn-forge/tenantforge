import { afterEach, expect, test, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import RolesView from '../src/views/RolesView.vue'
import RoleEditor from '../src/components/RoleEditor.vue'
import { state } from '../src/stores/app'
import type { Me, Module, Role } from '../src/api/types'
import { supportRoleGrants } from '../src/api/supportRoles'
const modules: Module[] = [
  {
    key: 'tickets',
    label: '工单',
    label_en: 'Tickets',
    description: '',
    actions: ['read', 'create', 'update', 'reply', 'assign'],
    fields: [
      ...['title', 'description', 'status', 'priority', 'customer_name', 'resolution'].map(
        (key) => ({ key, type: 'string' as const, label: key, label_en: key }),
      ),
      ...['customer_email', 'internal_notes'].map((key) => ({
        key,
        type: 'string' as const,
        sensitive: true,
        label: key,
        label_en: key,
      })),
    ],
  },
  ...['members', 'departments'].map((key) => ({
    key,
    label: key,
    label_en: key,
    description: '',
    fields: [],
    actions: ['read'],
  })),
]
afterEach(() => {
  state.me = null
  vi.unstubAllGlobals()
})
test('agent template explicitly grants only the two intended sensitive fields and excludes unrelated sensitive extensions', () => {
  const descriptors = structuredClone(modules)
  descriptors[0]!.fields.push({
    key: 'payment_secret',
    label: 'Secret',
    label_en: 'Secret',
    type: 'string',
    sensitive: true,
  })
  const grants = supportRoleGrants('agent', descriptors)
  expect(grants.map((grant) => [grant.action, grant.scope])).toEqual([
    ['read', 'self'],
    ['create', 'self'],
    ['update', 'self'],
    ['reply', 'self'],
  ])
  expect(grants[0]!.read_fields).toContain('customer_email')
  expect(grants[0]!.read_fields).toContain('internal_notes')
  expect(grants[0]!.unmask_fields).toEqual(['customer_email', 'internal_notes'])
  expect(
    grants.every(
      (grant) =>
        !grant.read_fields?.includes('payment_secret') &&
        !grant.write_fields?.includes('payment_secret'),
    ),
  ).toBe(true)
  expect(grants.find((grant) => grant.action === 'reply')!.write_fields).toEqual([
    'description',
    'internal_notes',
  ])
  grants[0]!.read_fields!.push('mutated')
  expect(supportRoleGrants('agent', descriptors)[0]!.read_fields).not.toContain('mutated')
})
test('support role template opens a new reviewable draft without writing or modifying an existing role', async () => {
  state.locale = 'en'
  state.me = {
    permissions: [
      { resource: 'roles', action: 'create', scope: 'tenant' },
      { resource: 'roles', action: 'read', scope: 'tenant' },
    ],
  } as Me
  const existing: Role = {
    id: 'existing',
    name: 'Custom',
    description: '',
    builtin: false,
    grants: [{ resource: 'tickets', action: 'read', scope: 'tenant' }],
  }
  const requests: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, options?: RequestInit) => {
      requests.push(options?.method ?? 'GET')
      return new Response(
        JSON.stringify(
          url.endsWith('/permissions')
            ? { data: { modules, scopes: [] } }
            : { data: [existing], meta: { page: 1, page_size: 20, total: 1 } },
        ),
      )
    }),
  )
  const wrapper = mount(RolesView, { global: { plugins: [ElementPlus] } })
  await flushPromises()
  const button = wrapper
    .findAll('button')
    .find((button) => button.text() === 'New support supervisor role')
  expect(button).toBeDefined()
  await button!.trigger('click')
  await flushPromises()
  const draft = wrapper.findComponent(RoleEditor).props('modelValue')
  expect(
    draft
      .filter((grant) => grant.resource === 'tickets')
      .map((grant) => [grant.action, grant.scope]),
  ).toEqual([
    ['read', 'department'],
    ['create', 'department'],
    ['update', 'department'],
    ['reply', 'department'],
    ['assign', 'department'],
  ])
  expect(
    draft
      .filter((grant) => grant.resource !== 'tickets')
      .map((grant) => [grant.resource, grant.action, grant.scope]),
  ).toEqual([
    ['members', 'read', 'department'],
    ['departments', 'read', 'department'],
  ])
  expect(wrapper.find('.support-template-preview').text()).toContain('customer_email')
  expect(wrapper.find('.support-template-preview').text()).toContain('internal_notes')
  expect(requests.every((method) => method === 'GET')).toBe(true)
  expect(existing.grants).toEqual([{ resource: 'tickets', action: 'read', scope: 'tenant' }])
  wrapper.unmount()
})
