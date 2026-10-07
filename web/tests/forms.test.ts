import { expect, test, vi } from 'vitest'
import { editableFields, buildResourceData, validateResourceData } from '../src/api/forms'
import { bootstrap, state } from '../src/stores/app'
import type { Module } from '../src/api/types'
const module: Module = {
  key: 'tickets',
  label: '工单',
  label_en: 'Tickets',
  description: '',
  actions: ['create', 'update'],
  fields: [
    { key: 'title', label: '标题', label_en: 'Title', type: 'string', required: true },
    { key: 'secret', label: '秘密', label_en: 'Secret', type: 'string', sensitive: true },
    { key: 'counter', label: '计数', label_en: 'Counter', type: 'number', readonly: true },
  ],
}
test('resource forms never include denied sensitive or immutable fields in mutations', () => {
  const fields = editableFields(
    module,
    [{ resource: 'tickets', action: 'create', scope: 'self', write_fields: ['*'] }],
    'create',
  )
  expect(fields.map((field) => field.key)).toEqual(['title'])
  expect(buildResourceData(fields, { title: 'hello', secret: 'private', counter: 42 })).toEqual({
    title: 'hello',
  })
  expect(validateResourceData(fields, { title: '' })).toEqual({ title: 'Required' })
})
test('initialization failure remains visible and can be retried', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        new Response(
          JSON.stringify({ error: { code: 'unavailable', message: 'Database unavailable' } }),
          { status: 503 },
        ),
      ),
  )
  await bootstrap()
  expect(state.startupError).toBe('Database unavailable')
  vi.unstubAllGlobals()
})
