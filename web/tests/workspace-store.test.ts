import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { loadMe, logout, state, switchTenant } from '../src/stores/app'
import { resetClient } from '../src/api/client'
import type { Me } from '../src/api/types'
const workspaceKey = 'tf_workspace'
function identity(user = 'user-a', secondStatus = 'active'): Me {
  return {
    user: {
      id: user,
      display_name: user,
      email: `${user}@example.test`,
      mfa_enabled: false,
      created_at: '',
    },
    tenants: [
      { id: 'first', name: 'First', slug: 'first', status: 'active', created_at: '' },
      ...(secondStatus
        ? [{ id: 'second', name: 'Second', slug: 'second', status: secondStatus, created_at: '' }]
        : []),
    ],
    permissions: [],
    modules: [],
    platform_admin: false,
  }
}
function mockIdentity(me: Me) {
  const selected: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (path: string, options: RequestInit) => {
      if (path.endsWith('/auth/csrf'))
        return new Response(JSON.stringify({ data: { csrf_token: 'fixture' } }))
      if (path.endsWith('/auth/logout'))
        return new Response(JSON.stringify({ data: { logged_out: true } }))
      const tenant = new Headers(options.headers).get('X-Tenant-ID') ?? ''
      selected.push(tenant)
      return new Response(
        JSON.stringify({
          data: {
            ...me,
            permissions: tenant ? [{ resource: tenant, action: 'read', scope: 'tenant' }] : [],
          },
        }),
      )
    }),
  )
  return selected
}
beforeEach(() => {
  resetClient()
  state.me = null
  state.tenant = ''
  sessionStorage.clear()
})
afterEach(() => {
  resetClient()
  state.me = null
  state.tenant = ''
  sessionStorage.clear()
  vi.unstubAllGlobals()
})

test('hard reload restores the selected workspace after verifying current server memberships', async () => {
  const selected = mockIdentity(identity())
  await loadMe()
  await switchTenant('second')
  resetClient()
  state.me = null
  state.tenant = ''
  selected.length = 0
  await loadMe()
  expect(selected).toEqual(['', 'second'])
  expect(state.tenant).toBe('second')
  expect(state.me).toMatchObject({ permissions: [{ resource: 'second' }] })
})

test.each(['', 'suspended'])(
  'revoked or suspended saved workspace falls back without sending its tenant header (%s)',
  async (status) => {
    sessionStorage.setItem(workspaceKey, JSON.stringify({ user_id: 'user-a', tenant_id: 'second' }))
    const selected = mockIdentity(identity('user-a', status))
    await loadMe()
    expect(selected).toEqual(['', 'first'])
    expect(state.tenant).toBe('first')
    expect(JSON.parse(sessionStorage.getItem(workspaceKey)!)).toEqual({
      user_id: 'user-a',
      tenant_id: 'first',
    })
  },
)

test('a different authenticated user never inherits another account workspace preference', async () => {
  sessionStorage.setItem(workspaceKey, JSON.stringify({ user_id: 'user-a', tenant_id: 'second' }))
  const selected = mockIdentity(identity('user-b'))
  await loadMe()
  expect(selected).toEqual(['', 'first'])
  expect(state.me?.user.id).toBe('user-b')
  expect(JSON.parse(sessionStorage.getItem(workspaceKey)!)).toEqual({
    user_id: 'user-b',
    tenant_id: 'first',
  })
})

test('logout clears the tab preference together with client session context', async () => {
  mockIdentity(identity())
  await loadMe()
  await switchTenant('second')
  await logout()
  expect(sessionStorage.getItem(workspaceKey)).toBeNull()
  expect(state.tenant).toBe('')
  expect(state.me).toBeNull()
})
