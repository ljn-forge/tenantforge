import { afterEach, expect, test, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import ElementPlus from 'element-plus'
import AccountView from '../src/views/AccountView.vue'
import { state } from '../src/stores/app'
import type { Me } from '../src/api/types'
afterEach(() => {
  vi.unstubAllGlobals()
  state.me = null
})
test('required password change explains the gate and waits to load sessions and security events until it is cleared', async () => {
  state.locale = 'en'
  state.me = {
    user: { display_name: 'Administrator', must_change_password: true, created_at: '' },
  } as Me
  const fetch = vi.fn(
    async (_url: string) =>
      new Response(JSON.stringify({ data: [], meta: { total: 0, page: 1, page_size: 20 } })),
  )
  vi.stubGlobal('fetch', fetch)
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/account', component: AccountView }],
  })
  await router.push('/account?required=1')
  await router.isReady()
  const wrapper = mount(AccountView, { global: { plugins: [ElementPlus, router] } })
  await flushPromises()
  expect(wrapper.text()).toContain(
    'Change your initial password before entering the administration system.',
  )
  expect(fetch).not.toHaveBeenCalled()
  expect(wrapper.text()).not.toContain('Sign-in sessions')
  state.me.user.must_change_password = false
  await flushPromises()
  expect(fetch.mock.calls.map((call) => call[0])).toEqual(
    expect.arrayContaining([
      expect.stringContaining('/auth/security-events'),
      expect.stringContaining('/auth/sessions'),
    ]),
  )
  expect(wrapper.text()).toContain('Sign-in sessions')
  wrapper.unmount()
})
