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
export interface Module {
  key: string
  label: string
  label_en: string
  description: string
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
export interface Department {
  id: string
  name: string
  parent_id: string | null
}
export interface Member {
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
export interface Role {
  id: string
  name: string
  description: string
  builtin: boolean
  grants: Grant[]
}
export interface ResourceRecord {
  id: string
  tenant_id: string
  owner_id: string
  department_id: string | null
  version: number
  data: Record<string, unknown>
  created_at: string
  updated_at: string
}
export interface Job {
  id: string
  type: string
  status: string
  attempts: number
  max_attempts: number
  error: string
  created_at: string
  updated_at: string
}
export interface FileRecord {
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
