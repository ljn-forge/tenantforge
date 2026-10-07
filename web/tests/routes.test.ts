import { expect, test } from 'vitest'
import { router } from '../src/router'
import { state } from '../src/stores/app'
test('password recovery links sent by the server remain reachable without authentication', async () => {
  state.initialized = true
  state.me = null
  await router.push('/reset?token=fixture-reset-token')
  expect(router.currentRoute.value.path).toBe('/reset')
  expect(router.currentRoute.value.matched).toHaveLength(1)
})
test('invitation links preserve their target across sign-in', async () => {
  state.initialized = true
  state.me = null
  await router.push('/invite?token=fixture-invitation-token')
  expect(router.currentRoute.value.path).toBe('/login')
  expect(router.currentRoute.value.query.redirect).toBe('/invite?token=fixture-invitation-token')
})

test('initial administrator password changes gate every management route', async () => {
  state.initialized = true
  state.me = { user: { must_change_password: true } } as import('../src/api/types').Me
  await router.push('/roles')
  expect(router.currentRoute.value.path).toBe('/account')
  expect(router.currentRoute.value.query.required).toBe('1')
  state.me = null
})
