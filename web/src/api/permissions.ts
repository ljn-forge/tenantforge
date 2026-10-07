import type { Grant, Module, Field } from './types'

export function can(grants: Grant[], resource: string, action: string): boolean {
  return grants.some((grant) => grant.resource === resource && grant.action === action)
}
export function createGrant(resource: string, action: string): Grant {
  return {
    resource,
    action,
    scope: 'self',
    read_fields: ['*'],
    write_fields: ['*'],
    unmask_fields: [],
    conditions: [],
    department_ids: [],
  }
}
export function validateGrant(grant: Grant, module?: Module): string[] {
  const errors: string[] = []
  if (!grant.resource || !grant.action) errors.push('Resource and action are required')
  if (grant.scope === 'selected_departments' && !grant.department_ids?.length)
    errors.push('Select at least one department')
  for (const field of grant.unmask_fields ?? []) {
    if (!grant.read_fields?.includes(field))
      errors.push(`Explicit read access is required to unmask ${field}`)
  }
  for (const condition of grant.conditions ?? []) {
    if (!condition.field || !['eq', 'in'].includes(condition.op)) errors.push('Invalid condition')
    if (condition.op === 'in' && !Array.isArray(condition.value))
      errors.push('The in operator requires an array')
  }
  if (
    !['self', 'department', 'department_tree', 'selected_departments', 'tenant'].includes(
      grant.scope,
    )
  )
    errors.push('Invalid scope')
  if (module) {
    if (!module.actions.includes(grant.action)) errors.push('Unknown operation')
    for (const field of grant.read_fields ?? [])
      if (field !== '*' && !module.fields.some((item) => item.key === field))
        errors.push(`Unknown readable field: ${field}`)
    for (const field of grant.write_fields ?? []) {
      const descriptor = module.fields.find((item) => item.key === field)
      if (field !== '*' && !descriptor) errors.push(`Unknown writable field: ${field}`)
      if (descriptor?.readonly) errors.push(`Read-only field cannot be writable: ${field}`)
    }
    for (const field of grant.unmask_fields ?? [])
      if (!module.fields.some((item) => item.key === field && item.sensitive))
        errors.push(`Unknown sensitive field: ${field}`)
    for (const condition of grant.conditions ?? []) {
      const field = module.fields.find((item) => item.key === condition.field)
      if (!field) {
        errors.push(`Unknown condition field: ${condition.field}`)
        continue
      }
      if (field.sensitive)
        errors.push(`Sensitive fields cannot be used in conditions: ${field.key}`)
      const values =
        condition.op === 'in' && Array.isArray(condition.value)
          ? condition.value
          : [condition.value]
      if (values.some((value) => !validFieldValue(field, value)))
        errors.push(`Invalid condition value: ${field.key}`)
    }
  }
  return errors
}
function validFieldValue(field: Field, value: unknown): boolean {
  if (field.type === 'number') return typeof value === 'number' && Number.isFinite(value)
  if (field.type === 'boolean') return typeof value === 'boolean'
  if (field.type === 'enum') return typeof value === 'string' && !!field.options?.includes(value)
  if (field.type === 'date')
    return (
      typeof value === 'string' &&
      /^\d{4}-\d{2}-\d{2}$/.test(value) &&
      !Number.isNaN(Date.parse(value))
    )
  return typeof value === 'string'
}
export function parseConditionValue(
  field: Field | undefined,
  text: string,
  operator: 'eq' | 'in',
): unknown {
  const parse = (value: string): unknown => {
    if (field?.type === 'number') return value.trim() === '' ? NaN : Number(value)
    if (field?.type === 'boolean')
      return value === 'true' ? true : value === 'false' ? false : value
    return value
  }
  return operator === 'in' ? text.split(',').map((value) => parse(value.trim())) : parse(text)
}
