export type Scope = 'self' | 'department' | 'department_tree' | 'selected_departments' | 'tenant'
export interface Condition {
  field: string
  op: 'eq' | 'in'
  value: unknown
}
export interface Grant {
  resource: string
  action: string
  scope: Scope
  department_ids?: string[]
  read_fields?: string[]
  write_fields?: string[]
  unmask_fields?: string[]
  conditions?: Condition[]
}
export interface User {
  username?: string
  must_change_password?: boolean
  id: string
  email: string
  display_name: string
  mfa_enabled: boolean
  created_at: string
}
export interface Tenant {
  id: string
  name: string
  slug: string
  status: string
  quotas?: Record<string, number>
  created_at: string
}
export interface Field {
  key: string
  label: string
  label_en: string
  type: 'string' | 'text' | 'number' | 'boolean' | 'enum' | 'date'
  required?: boolean
  sensitive?: boolean
  readonly?: boolean
  options?: string[]
}
export interface Operation {
  key: string
  label: string
  label_en: string
}
export interface Capabilities {
  actions: string[]
  write_fields: string[]
  transitions: string[]
}
export interface AuthorizedRecord {
  capabilities?: Capabilities
}
export interface Module {
  key: string
  label: string
  label_en: string
  description: string
  description_en?: string
  operations?: Operation[]
  fields: Field[]
  actions: string[]
}
export interface Me {
  user: User
  tenants: Tenant[]
  permissions: Grant[]
  modules: Module[]
  platform_admin: boolean
}
export interface Department extends AuthorizedRecord {
  id: string
  name: string
  parent_id: string | null
}
export interface Member extends AuthorizedRecord {
  username?: string
  id: string
  user_id: string
  email: string
  display_name: string
  department_id: string | null
  position: string
  role_ids: string[]
  status: string
}
export interface Role extends AuthorizedRecord {
  id: string
  name: string
  description: string
  builtin: boolean
  grants: Grant[]
}
export interface ResourceRecord {
  capabilities: Capabilities
  id: string
  tenant_id: string
  owner_id: string
  department_id: string | null
  version: number
  data: Record<string, unknown>
  created_at: string
  updated_at: string
}
export interface Audit {
  id: string
  actor_id: string
  action: string
  resource: string
  record_id: string
  request_id: string
  created_at: string
  changes: Record<string, unknown>
}
export interface Job extends AuthorizedRecord {
  id: string
  type: string
  status: string
  attempts: number
  max_attempts: number
  error: string
  error_code?: string
  error_stage?: string
  result?: Record<string, unknown>
  created_at: string
  updated_at: string
}
export interface FileRecord extends AuthorizedRecord {
  id: string
  name: string
  size: number
  content_type: string
  owner_id: string
  department_id: string | null
  created_at: string
}
export interface Envelope<T> {
  data: T
  meta?: { page: number; page_size: number; total: number }
}
