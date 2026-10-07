import { ref } from 'vue'
import { allPages } from '../api/client'
import type { Department, Role } from '../api/types'
import { allowed } from '../stores/app'
export function useOrganizationReferences() {
  const departments = ref<Department[]>([])
  const roles = ref<Role[]>([])
  async function load() {
    await Promise.all([
      allowed('departments', 'read')
        ? allPages<Department>('/departments').then((result) => {
            departments.value = result
          })
        : Promise.resolve(),
      allowed('roles', 'read')
        ? allPages<Role>('/roles').then((result) => {
            roles.value = result
          })
        : Promise.resolve(),
    ])
  }
  function departmentName(id: string | null | undefined) {
    return departments.value.find((item) => item.id === id)?.name ?? (id ? '—' : '—')
  }
  function roleNames(ids: string[]) {
    return ids.map((id) => roles.value.find((item) => item.id === id)?.name ?? id).join(', ')
  }
  return { departments, roles, load, departmentName, roleNames }
}
