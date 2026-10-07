import { expect, test } from 'vitest'
import { can, validateGrant, createGrant } from '../src/api/permissions'

test('a read grant never enables an update action', () => {
  expect(can([{ resource: 'tickets', action: 'read', scope: 'tenant' }], 'tickets', 'update')).toBe(
    false,
  )
  expect(can([], 'tickets', 'read')).toBe(false)
})

test('selected department scope requires a department and all scopes are explicit', () => {
  expect(
    validateGrant({
      resource: 'tickets',
      action: 'read',
      scope: 'selected_departments',
      department_ids: [],
    }),
  ).not.toEqual([])
  expect(createGrant('tickets', 'update').scope).toBe('self')
})

test('field unmasking requires explicit readable fields', () => {
  expect(
    validateGrant({
      resource: 'tickets',
      action: 'read',
      scope: 'self',
      read_fields: ['title'],
      unmask_fields: ['customer_email'],
    }),
  ).not.toEqual([])
})

test('condition parser preserves invalid boolean text so validation cannot silently turn it into false', async () => {
  const { parseConditionValue } = await import('../src/api/permissions')
  const field = { key: 'enabled', label: '启用', label_en: 'Enabled', type: 'boolean' as const }
  expect(parseConditionValue(field, 'perhaps', 'eq')).toBe('perhaps')
  expect(parseConditionValue(field, 'true, false', 'in')).toEqual([true, false])
  const numeric = { ...field, type: 'number' as const }
  expect(Number.isNaN(parseConditionValue(numeric, '', 'eq'))).toBe(true)
})

test('sensitive fields cannot be used as record conditions even with explicit read and unmask fields', () => {
  const descriptor = {
    key: 'tickets',
    label: '工单',
    label_en: 'Tickets',
    description: '',
    actions: ['read'],
    fields: [
      {
        key: 'secret',
        label: '秘密',
        label_en: 'Secret',
        type: 'string' as const,
        sensitive: true,
      },
    ],
  }
  expect(
    validateGrant(
      {
        resource: 'tickets',
        action: 'read',
        scope: 'self',
        read_fields: ['secret'],
        unmask_fields: ['secret'],
        conditions: [{ field: 'secret', op: 'eq', value: 'private' }],
      },
      descriptor,
    ),
  ).toContain('Sensitive fields cannot be used in conditions: secret')
})
