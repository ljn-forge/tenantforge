import type { AuthorizedRecord, Field, Module, ResourceRecord } from './types'
export function recordAllows(record: AuthorizedRecord | null | undefined, action: string): boolean {
  return record?.capabilities?.actions.includes(action) === true
}
export function recordWritableFields(module: Module, record: ResourceRecord): Field[] {
  if (!recordAllows(record, 'update')) return []
  return module.fields.filter(
    (field) => !field.readonly && record.capabilities?.write_fields.includes(field.key),
  )
}
