import { expect, type Page } from '@playwright/test'
import process from 'node:process'
export const identity = process.env.TF_E2E_USERNAME ?? 'admin'
export const password = process.env.TF_E2E_PASSWORD ?? 'BrowserAcceptance-Only-2026!'
const initialPassword = process.env.TF_E2E_INITIAL_PASSWORD ?? 'Admin@123456'
export async function signInAccount(
  page: Page,
  credentials = { identity, initialPassword, password },
) {
  await page.goto('/login')
  await expect(page.getByRole('heading', { name: 'Administrator sign in' })).toBeVisible()
  await page.getByLabel('Username or email', { exact: true }).fill(credentials.identity)
  async function submit(value: string) {
    const response = page.waitForResponse(
      (response) =>
        response.request().method() === 'POST' &&
        new URL(response.url()).pathname === '/api/v1/auth/login',
    )
    await page.getByLabel('Password', { exact: true }).fill(value)
    await page.getByRole('button', { name: 'Sign in', exact: true }).click()
    return response
  }
  let used = credentials.initialPassword
  let response = await submit(used)
  if (response.status() === 401) {
    used = credentials.password
    response = await submit(used)
  }
  expect(response.ok()).toBe(true)
  const user = (await response.json()).data as {
    display_name: string
    email: string
    must_change_password: boolean
  }
  if (user.must_change_password) {
    await expect(
      page.getByText('Change your initial password before entering the administration system.', {
        exact: true,
      }),
    ).toBeVisible()
    await page.getByLabel('Current password', { exact: true }).fill(used)
    await page.getByLabel('New password', { exact: true }).fill(credentials.password)
    await page.getByRole('button', { name: 'Update password', exact: true }).click()
  }
  await expect(page.getByRole('heading', { name: `Welcome, ${user.display_name}` })).toBeVisible()
  return user
}
