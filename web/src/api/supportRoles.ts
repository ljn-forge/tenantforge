import type { Grant, Module } from './types'

export type SupportRoleKind = 'agent' | 'supervisor'
export function supportRoleGrants(kind: SupportRoleKind, modules: Module[]): Grant[] {
  const tickets = modules.find((module) => module.key === 'tickets')
  if (
    !tickets ||
    !['read', 'create', 'update', 'reply'].every((action) => tickets.actions.includes(action))
  )
    return []
  const scope = kind === 'agent' ? 'self' : 'department'
  const templateFields = tickets.fields.filter(
    (field) => !field.sensitive || ['customer_email', 'internal_notes'].includes(field.key),
  )
  const readable = templateFields.map((field) => field.key)
  const writable = templateFields.filter((field) => !field.readonly).map((field) => field.key)
  const sensitive = ['customer_email', 'internal_notes'].filter((key) =>
    tickets.fields.some((field) => field.key === key && field.sensitive),
  )
  const grants: Grant[] = [
    { resource: 'tickets', action: 'read', scope, read_fields: readable, unmask_fields: sensitive },
    { resource: 'tickets', action: 'create', scope, write_fields: [...writable] },
    { resource: 'tickets', action: 'update', scope, write_fields: [...writable] },
    {
      resource: 'tickets',
      action: 'reply',
      scope,
      write_fields: ['description', 'internal_notes'].filter((key) => writable.includes(key)),
    },
  ]
  if (kind === 'supervisor') {
    if (!tickets.actions.includes('assign')) return []
    grants.push({ resource: 'tickets', action: 'assign', scope })
    for (const resource of ['members', 'departments']) {
      if (!modules.some((module) => module.key === resource && module.actions.includes('read')))
        return []
      grants.push({ resource, action: 'read', scope, read_fields: ['*'] })
    }
  }
  return grants
}
