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
export async function loadMe() {
  try {
    const result = await api<Me>('/auth/me')
    state.me = result.data
    const selected = state.me.tenants.find(
      (tenant) => tenant.id === state.tenant && tenant.status === 'active',
    )
    if (!selected) {
      state.tenant = state.me.tenants.find((tenant) => tenant.status === 'active')?.id ?? ''
      setTenant(state.tenant)
      if (state.tenant) state.me = (await api<Me>('/auth/me')).data
    }
  } catch (error) {
    if (error instanceof APIError && error.status === 401) {
      state.me = null
      state.tenant = ''
      resetClient()
    } else throw error
  }
}
export async function switchTenant(id: string) {
  state.loading = true
  state.me = state.me ? { ...state.me, permissions: [], modules: [] } : null
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
