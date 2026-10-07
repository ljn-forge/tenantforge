import type { Field, Grant, Module } from './types'
import { state } from '../stores/app'
export function editableFields(module: Module, grants: Grant[], action: string): Field[] {
  const matching = grants.filter(
    (grant) => grant.resource === module.key && grant.action === action,
  )
  return module.fields.filter(
    (field) =>
      !field.readonly &&
      matching.some(
        (grant) =>
          grant.write_fields?.includes(field.key) ||
          (!field.sensitive && grant.write_fields?.includes('*')),
      ),
  )
}
export function buildResourceData(
  fields: Field[],
  values: Record<string, unknown>,
): Record<string, unknown> {
  return Object.fromEntries(
    fields
      .filter((field) => values[field.key] !== undefined)
      .map((field) => [field.key, values[field.key]]),
  )
}
export function validateResourceData(
  fields: Field[],
  values: Record<string, unknown>,
): Record<string, string> {
  const errors: Record<string, string> = {}
  for (const field of fields) {
    const value = values[field.key]
    if (
      field.required &&
      (value === undefined || value === null || (typeof value === 'string' && !value.trim()))
    ) {
      errors[field.key] = 'Required'
      continue
    }
    if (value === undefined || value === null || value === '') continue
    if (field.type === 'number' && (typeof value !== 'number' || !Number.isFinite(value)))
      errors[field.key] = 'Enter a number'
    if (field.type === 'enum' && !field.options?.includes(String(value)))
      errors[field.key] = 'Choose a valid option'
  }
  return errors
}
export function formatValue(value: unknown): string {
  return value === undefined || value === null
    ? '—'
    : typeof value === 'object'
      ? JSON.stringify(value)
      : String(value)
}
export function dateTime(value: string): string {
  return value ? new Date(value).toLocaleString(state.locale === 'zh' ? 'zh-CN' : 'en-US') : '—'
}
