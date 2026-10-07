import { test, expect, type Page } from '@playwright/test'
import * as OTPAuth from 'otpauth'
import { identity, password, signInAccount } from './helpers'
let displayName = ''
let email = ''
async function signIn(page: Page) {
  const user = await signInAccount(page)
  displayName = user.display_name
  email = user.email
}
test('default admin sign-in, initial password change, users and access editor, lifecycle, CSV, files, MFA and responsive layout', async ({
  page,
}) => {
  let csrf = ''
  page.on('request', (request) => {
    const token = request.headers()['x-csrf-token']
    if (token) csrf = token
  })
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  await page.addInitScript(() => localStorage.setItem('tf_locale', 'en'))
  const status = await page.request.get('/api/v1/setup/status')
  expect((await status.json()).data.initialized).toBe(true)
  await signIn(page)
  const me = (await (await page.request.get('/api/v1/auth/me')).json()).data
  const tenant = me.tenants.find((item: { status: string }) => item.status === 'active').id
  const headers = { 'X-Tenant-ID': tenant, Origin: 'http://localhost:5178' }
  const suffix = Date.now().toString()
  await page.goto('/departments')
  await page.getByRole('button', { name: 'Create department' }).click()
  await page.getByLabel('Department name', { exact: true }).fill(`Support ${suffix}`)
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByText(`Support ${suffix}`, { exact: true }).first()).toBeVisible()
  await page.goto('/roles')
  await page.getByRole('button', { name: 'Create role' }).click()
  await page.getByLabel('Name', { exact: true }).fill(`Scoped operator ${suffix}`)
  const editor = page.locator('.role-editor')
  await editor.getByRole('button', { name: 'Tickets · Read', exact: true }).click()
  await editor.getByRole('button', { name: 'Add grant' }).click()
  const grant = page.locator('[data-grant]').first()
  await grant.getByLabel('Record scope', { exact: true }).press('ArrowDown')
  await page.getByRole('option', { name: 'Selected departments', exact: true }).click()
  await expect(grant.getByText('Select at least one department')).toBeVisible()
  await grant.getByLabel('Selected departments', { exact: true }).press('ArrowDown')
  await page.getByRole('option', { name: `Support ${suffix}`, exact: true }).click()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('button', { name: 'Save role' })).toBeEnabled()
  await page.getByRole('button', { name: 'Save role' }).click()
  await expect(page.getByText(`Scoped operator ${suffix}`, { exact: true })).toBeVisible()
  await page.goto('/members')
  await page.getByRole('button', { name: 'Add user' }).click()
  await page.getByLabel('Username', { exact: true }).fill(`operator-${suffix}`)
  await page.getByLabel('Display name', { exact: true }).fill(`Operator ${suffix}`)
  await page.getByLabel('Initial password', { exact: true }).fill('OperatorAcceptance-2026!')
  await page
    .locator('.el-dialog')
    .getByLabel('Email', { exact: true })
    .fill(`operator-${suffix}@example.test`)
  await page.getByLabel('Roles', { exact: true }).press('ArrowDown')
  await page.getByRole('option', { name: `Scoped operator ${suffix}`, exact: true }).click()
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByRole('row').filter({ hasText: `operator-${suffix}` })).toBeVisible()
  const operatorContext = await page
    .context()
    .browser()!
    .newContext({ baseURL: 'http://localhost:5178' })
  const operator = await operatorContext.newPage()
  await operator.addInitScript(() => localStorage.setItem('tf_locale', 'en'))
  await operator.goto('/login')
  await operator.getByLabel('Username or email', { exact: true }).fill(`operator-${suffix}`)
  await operator.getByLabel('Password', { exact: true }).fill('OperatorAcceptance-2026!')
  await operator.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(
    operator.getByText('Change your initial password before entering the administration system.', {
      exact: true,
    }),
  ).toBeVisible()
  await operator.getByLabel('Current password', { exact: true }).fill('OperatorAcceptance-2026!')
  await operator.getByLabel('New password', { exact: true }).fill('OperatorChangedAcceptance-2026!')
  await operator.getByRole('button', { name: 'Update password', exact: true }).click()
  await expect(operator.getByRole('heading', { name: `Welcome, Operator ${suffix}` })).toBeVisible()
  await operator.goto('/resources/tickets')
  await expect(operator.getByText('No visible records', { exact: true })).toBeVisible()
  await expect(operator.getByRole('button', { name: 'New record' })).toHaveCount(0)
  await expect(
    operator.getByRole('navigation').getByRole('link', { name: 'Users', exact: true }),
  ).toHaveCount(0)
  await operatorContext.close()

  await page.goto('/resources/tickets')
  await page.getByRole('button', { name: 'New record' }).click()
  await page.getByLabel('Title', { exact: true }).fill(`Browser ticket ${suffix}`)
  await page.getByLabel('Description', { exact: true }).fill('Persisted acceptance record')
  await page.getByLabel('Customer email', { exact: true }).fill('customer@example.test')
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  const ticketRow = page.getByRole('row').filter({ hasText: `Browser ticket ${suffix}` })
  await expect(ticketRow).toBeVisible()
  await ticketRow.getByRole('button', { name: 'View', exact: true }).click()
  await page.getByRole('button', { name: 'Start work', exact: true }).click()
  await expect(
    page.locator('.el-drawer').getByText('In progress', { exact: true }).first(),
  ).toBeVisible()
  await page.locator('.el-drawer__close-btn').click()
  const resources = await page.request.get('/api/v1/resources/tickets', { headers })
  expect(resources.ok()).toBe(true)
  const persisted = (await resources.json()).data.find(
    (item: { data: { title: string } }) => item.data.title === `Browser ticket ${suffix}`,
  )
  expect(persisted.data.status).toBe('in_progress')
  await ticketRow.getByRole('button', { name: 'Edit', exact: true }).click()
  await page.getByLabel('Description', { exact: true }).fill('My concurrent edit')
  const competing = await page.request.patch(`/api/v1/resources/tickets/${persisted.id}`, {
    headers: { ...headers, 'X-CSRF-Token': csrf },
    data: { version: persisted.version, data: { description: 'Saved by a second actor' } },
  })
  if (!competing.ok()) {
    const failure = await competing.json()
    throw new Error(
      `Competing patch failed ${competing.status()}: ${failure.error?.code} ${failure.error?.message}`,
    )
  }
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Reload latest', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Reload latest', exact: true }).click()
  await expect(page.getByLabel('Description', { exact: true })).toHaveValue(
    'Saved by a second actor',
  )
  await page
    .getByLabel('Description', { exact: true })
    .fill('Accepted after reloading latest version')
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.locator('.el-dialog')).not.toBeVisible()

  await page.locator('input[type=file]').setInputFiles({
    name: 'import.csv',
    mimeType: 'text/csv',
    buffer: Buffer.from(`title,description,priority\nImported ${suffix},CSV acceptance,normal\n`),
  })
  await expect(page.getByText('Imported 1 records', { exact: true })).toBeVisible()
  await expect(page.getByRole('row').filter({ hasText: `Imported ${suffix}` })).toBeVisible()
  await page.getByRole('button', { name: 'Export', exact: true }).click()
  await page.getByRole('button', { name: 'Create export job', exact: true }).click()
  await expect(page).toHaveURL(/\/jobs$/)
  await expect(page.getByRole('button', { name: 'Download', exact: true }).first()).toBeVisible({
    timeout: 20000,
  })
  const downloadEvent = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Download', exact: true }).first().click()
  const exported = await downloadEvent
  expect(exported.suggestedFilename()).toMatch(/\.csv$/)
  const completedJob = page.getByRole('row').filter({ hasText: 'succeeded' }).first()
  const deletedJob = page.waitForResponse(
    (response) =>
      response.request().method() === 'DELETE' &&
      new URL(response.url()).pathname.startsWith('/api/v1/jobs/'),
  )
  await completedJob.getByRole('button', { name: 'Delete', exact: true }).click()
  await page.locator('.el-message-box').getByRole('button', { name: 'Delete', exact: true }).click()
  const deletion = await deletedJob
  expect(deletion.ok()).toBe(true)
  const jobId = new URL(deletion.url()).pathname.split('/').pop()
  const removedJob = await page.request.get(`/api/v1/jobs/${jobId}`, { headers })
  expect(removedJob.status()).toBe(404)
  await page.goto('/files')
  await page.getByRole('button', { name: 'Upload file' }).click()
  await page.locator('.el-dialog input[type=file]').setInputFiles({
    name: `acceptance-${suffix}.txt`,
    mimeType: 'text/plain',
    buffer: Buffer.from('TenantForge persisted upload'),
  })
  await page.getByRole('button', { name: 'Upload', exact: true }).click()
  const fileRow = page.getByRole('row').filter({ hasText: `acceptance-${suffix}.txt` })
  await expect(fileRow).toBeVisible()
  const fileDownload = page.waitForEvent('download')
  await fileRow.getByRole('button', { name: 'Download' }).click()
  expect((await fileDownload).suggestedFilename()).toBe(`acceptance-${suffix}.txt`)
  await page.goto('/audit')
  await expect(page.getByRole('row').filter({ hasText: 'create' }).first()).toBeVisible()
  await page.getByRole('button', { name: 'Export audit' }).click()
  await expect(page).toHaveURL(/\/jobs$/)
  await page.goto('/account')
  await expect(page.getByText('Current session', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Set up two-factor authentication' }).click()
  await page.locator('.el-dialog').getByLabel('Current password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Continue setup' }).click()
  const key = (await page.locator('.enrollment-secret code').textContent())!.trim()
  const totp = new OTPAuth.TOTP({
    secret: OTPAuth.Secret.fromBase32(key),
    algorithm: 'SHA1',
    digits: 6,
    period: 30,
  })
  await page
    .locator('.el-dialog')
    .getByLabel('Authenticator code', { exact: true })
    .fill(totp.generate())
  await page.getByRole('button', { name: 'Verify and enable' }).click()
  await expect(page.locator('.recovery-codes code')).toHaveCount(10)
  const recoveryCode = (await page.locator('.recovery-codes code').first().textContent())!.trim()
  try {
    await page
      .getByRole('checkbox', { name: 'I have securely saved my recovery codes' })
      .press('Space')
    await page.getByRole('button', { name: 'Done', exact: true }).click()
    await expect(page.getByText('Protected with an authenticator')).toBeVisible()
    // Challenge sign-in in an independent browser session, then revoke that session.
    const extra = await page.context().browser()!.newContext({ baseURL: 'http://localhost:5178' })
    const login = await extra.newPage()
    await login.addInitScript(() => localStorage.setItem('tf_locale', 'en'))
    await login.goto('/login')
    await login.getByLabel('Username or email', { exact: true }).fill(identity)
    await login.getByLabel('Password', { exact: true }).fill(password)
    await login.getByRole('button', { name: 'Sign in', exact: true }).click()
    await expect(login.getByLabel('Authenticator code', { exact: true })).toBeVisible()
    await login.getByLabel('Authenticator code', { exact: true }).fill(totp.generate())
    await login.getByRole('button', { name: 'Sign in', exact: true }).click()
    await expect(login.getByRole('heading', { name: `Welcome, ${displayName}` })).toBeVisible()
    await extra.close()
    await expect(page.getByRole('heading', { name: 'Account security activity' })).toBeVisible()
    const refreshedSessions = page.waitForResponse(
      (response) =>
        response.request().method() === 'GET' &&
        new URL(response.url()).pathname === '/api/v1/auth/sessions',
    )
    await page.getByRole('button', { name: 'Refresh', exact: true }).click()
    const listedSessions = (await (await refreshedSessions).json()).data
    const currentSession = listedSessions.find(
      (item: { id: string; current: boolean }) => item.current,
    )
    const otherSession = listedSessions.find(
      (item: { id: string; current: boolean }) => !item.current,
    )
    expect(otherSession).toBeDefined()
    const revokedRequest = page.waitForRequest(
      (request) => request.method() === 'DELETE' && request.url().includes('/auth/sessions/'),
    )
    await page.locator(`button[data-session-id="${otherSession.id}"]`).click()
    await page.getByRole('button', { name: 'Delete', exact: true }).click()
    const revokedId = new URL((await revokedRequest).url()).pathname.split('/').pop()
    expect(revokedId).toBe(otherSession.id)
    expect(revokedId).not.toBe(currentSession.id)
    await expect(page).toHaveURL(/\/account$/)
    await page
      .getByRole('button', { name: 'Disable two-factor authentication', exact: true })
      .click()
    await page.locator('.el-dialog').getByLabel('Current password', { exact: true }).fill(password)
    await page
      .locator('.el-dialog')
      .getByLabel('Or use a one-time recovery code', { exact: true })
      .fill(recoveryCode)
    await page.getByRole('button', { name: 'Disable authentication' }).click()
    await expect(page.getByText('Add an extra layer of protection')).toBeVisible()
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/')
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true)
    const dashboardResponse = await page.request.get('/api/v1/dashboard', { headers })
    if (!dashboardResponse.ok()) {
      const failure = await dashboardResponse.json()
      throw new Error(
        `Dashboard failed ${dashboardResponse.status()}: ${failure.error?.code} ${failure.error?.message}`,
      )
    }
    const dashboardData = (await dashboardResponse.json()).data
    await expect(
      page.locator('.stat-card').filter({ hasText: 'Tickets' }).locator('strong'),
    ).toHaveText(String(dashboardData.counts.tickets))
    await page.setViewportSize({ width: 1440, height: 1000 })
    await expect(page.locator('.shell')).toHaveCount(1)
    await expect(page.getByRole('heading', { name: `Welcome, ${displayName}` })).toHaveCount(1)
    await page.screenshot({ path: 'test-results/dashboard.png' })
    await page.setViewportSize({ width: 390, height: 844 })
    await page.getByRole('button', { name: 'Open navigation', exact: true }).click()
    await expect(page.getByRole('navigation', { name: 'Main navigation' })).toBeVisible()
    await page
      .getByRole('navigation', { name: 'Main navigation' })
      .getByRole('link', { name: 'Overview', exact: true })
      .click()
    await page.getByRole('link', { name: /Account security/ }).click()
    await expect(page.getByRole('heading', { name: 'Account & security' })).toBeVisible()
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true)
    expect(
      await page.evaluate(() =>
        Object.keys(localStorage).every((key) => ['tf_locale', 'tf_theme'].includes(key)),
      ),
    ).toBe(true)
    expect(errors).toEqual([])
  } finally {
    // Restore the isolated fixture even when an assertion after MFA enrollment fails.
    const current = await page.request.get('/api/v1/auth/me')
    if (current.ok() && (await current.json()).data.user.mfa_enabled) {
      await page.request.post('/api/v1/auth/mfa/disable', {
        headers: { ...headers, 'X-CSRF-Token': csrf },
        data: { password, recovery_code: recoveryCode },
      })
    }
  }
})

test('platform tenant provisioning and switching replace data and capabilities with the selected workspace', async ({
  page,
}) => {
  await page.addInitScript(() => localStorage.setItem('tf_locale', 'en'))
  await signIn(page)
  const firstTenant = (await (await page.request.get('/api/v1/auth/me')).json()).data.tenants.find(
    (tenant: { status: string }) => tenant.status === 'active',
  )
  const suffix = Date.now().toString()
  const name = `Isolated workspace ${suffix}`
  await page.goto('/platform/tenants')
  await page.getByRole('button', { name: 'Provision tenant' }).click()
  await page.getByLabel('Workspace name', { exact: true }).fill(name)
  await page.getByLabel('Unique slug', { exact: true }).fill(`browser-switch-${suffix}`)
  await page.getByLabel('Owner email', { exact: true }).fill(email)
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByRole('row').filter({ hasText: name })).toBeVisible()
  await page.locator('.tenant-picker .el-select__wrapper').click()
  await page.getByRole('option', { name, exact: true }).click()
  await expect(page).toHaveURL(/\/$/)
  await expect(page.locator('.topbar-title')).toContainText(name)
  await expect(
    page.locator('.stat-card').filter({ hasText: 'Tickets' }).locator('strong'),
  ).toHaveText('0')
  await page.goto('/resources/tickets')
  await expect(page.getByText('No visible records', { exact: true })).toBeVisible()
  await page.locator('.tenant-picker .el-select__wrapper').click()
  await page.getByRole('option', { name: firstTenant.name, exact: true }).click()
  await expect(page.locator('.topbar-title')).toContainText(firstTenant.name)
  await page.goto('/resources/tickets')
  await expect(page.getByRole('row').filter({ hasText: 'Browser ticket' }).first()).toBeVisible()
})
