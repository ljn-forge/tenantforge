import { reactive, computed } from 'vue'
import { api, APIError, setTenant, resetClient } from '../api/client'
import { can } from '../api/permissions'
import type { Me } from '../api/types'

export const state = reactive({
  me: null as Me | null,
  tenant: '',
  loading: false,
  initialized: true,
  oidc: false,
  version: '',
  startupError: '',
  startupBusy: false,
  locale: localStorage.getItem('tf_locale') === 'en' ? 'en' : 'zh',
  dark: localStorage.getItem('tf_theme') === 'dark',
})
export const currentTenant = computed(() =>
  state.me?.tenants.find((tenant) => tenant.id === state.tenant),
)
export function t(zh: string, en: string) {
  return state.locale === 'zh' ? zh : en
}
export function allowed(resource: string, action: string) {
  return can(state.me?.permissions ?? [], resource, action)
}
export function setLocale() {
  state.locale = state.locale === 'zh' ? 'en' : 'zh'
  localStorage.setItem('tf_locale', state.locale)
  document.documentElement.lang = state.locale === 'zh' ? 'zh-CN' : 'en'
}
export function toggleTheme() {
  state.dark = !state.dark
  localStorage.setItem('tf_theme', state.dark ? 'dark' : 'light')
  document.documentElement.classList.toggle('dark', state.dark)
}
const workspaceKey = 'tf_workspace'
function savedWorkspace(userID: string): string {
  try {
    const saved = JSON.parse(sessionStorage.getItem(workspaceKey) ?? 'null')
    return saved?.user_id === userID && typeof saved.tenant_id === 'string' ? saved.tenant_id : ''
  } catch {
    return ''
  }
}
function rememberWorkspace(userID: string, tenantID: string) {
  try {
    if (tenantID)
      sessionStorage.setItem(workspaceKey, JSON.stringify({ user_id: userID, tenant_id: tenantID }))
    else sessionStorage.removeItem(workspaceKey)
  } catch {
    // A disabled preference store must not prevent cookie-based authentication.
  }
}
export async function loadMe() {
  try {
    const previousUser = state.me?.user.id
    const result = await api<Me>('/auth/me')
    state.me = result.data
    const activeTenants = result.data.tenants.filter((tenant) => tenant.status === 'active')
    const current = !previousUser || previousUser === result.data.user.id ? state.tenant : ''
    const preferred = current || savedWorkspace(result.data.user.id)
    const selected = activeTenants.find((tenant) => tenant.id === preferred) ?? activeTenants[0]
    const tenantID = selected?.id ?? ''
    if (state.tenant !== tenantID) {
      state.tenant = tenantID
      setTenant(state.tenant)
      if (state.tenant) state.me = (await api<Me>('/auth/me')).data
    }
    rememberWorkspace(state.me.user.id, state.tenant)
  } catch (error) {
    if (error instanceof APIError && error.status === 401) {
      state.me = null
      state.tenant = ''
      resetClient()
      rememberWorkspace('', '')
    } else throw error
  }
}
export async function switchTenant(id: string) {
  const identity = state.me
  if (!identity?.tenants.some((tenant) => tenant.id === id && tenant.status === 'active'))
    throw new APIError(
      403,
      'workspace_unavailable',
      t('此组织当前不可用', 'This workspace is unavailable'),
    )
  // Persist the user-bound selection before asynchronous authorization so a reload
  // retains the intent. loadMe still verifies fresh memberships before restoring it.
  rememberWorkspace(identity.user.id, id)
  state.loading = true
  state.me = { ...identity, permissions: [], modules: [] }
  state.tenant = id
  setTenant(id)
  try {
    await loadMe()
  } finally {
    state.loading = false
  }
}
export async function initialize() {
  const result = await api<{ initialized: boolean; version: string; oidc_enabled: boolean }>(
    '/setup/status',
  )
  state.initialized = result.data.initialized
  state.oidc = result.data.oidc_enabled
  state.version = result.data.version
  if (state.initialized) await loadMe()
}
export async function loggedIn() {
  resetClient()
  state.tenant = ''
  await loadMe()
}
export async function logout() {
  await api('/auth/logout', { method: 'POST' })
  state.me = null
  state.tenant = ''
  resetClient()
  rememberWorkspace('', '')
}
export async function bootstrap() {
  state.startupBusy = true
  state.startupError = ''
  try {
    await initialize()
  } catch (error) {
    state.startupError = error instanceof Error ? error.message : 'Unable to connect'
  } finally {
    state.startupBusy = false
  }
}
