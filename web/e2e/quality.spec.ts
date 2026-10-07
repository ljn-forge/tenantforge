import { test, expect } from '@playwright/test'
import { signInAccount } from './helpers'
test('matrix presets preserve scope, mixed ownership follows row authority, department clearing persists, and light/dark layouts stay usable', async ({
  page,
}) => {
  async function settleLayout() {
    await page.evaluate(async () => {
      window.scrollTo(0, 0)
      await document.fonts.ready
      await new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
      )
    })
  }
  await page.addInitScript(() => {
    if (!localStorage.getItem('tf_locale')) localStorage.setItem('tf_locale', 'en')
  })
  let csrf = ''
  page.on('request', (request) => {
    const value = request.headers()['x-csrf-token']
    if (value) csrf = value
  })
  await signInAccount(page)
  const me = (await (await page.request.get('/api/v1/auth/me')).json()).data
  const tenant = me.tenants.find((item: { status: string }) => item.status === 'active').id
  const headers = { 'X-Tenant-ID': tenant, Origin: 'http://localhost:5178' }
  const suffix = Date.now().toString()
  await page.goto('/roles')
  await page.getByRole('button', { name: 'Create role' }).click()
  await page.getByLabel('Name', { exact: true }).fill(`Quality owner editor ${suffix}`)
  const matrix = page.locator('.role-editor')
  await matrix.getByRole('button', { name: 'Tickets · Read', exact: true }).click()
  await matrix.getByRole('button', { name: 'Own-work preset', exact: true }).click()
  const preview = page.getByRole('dialog', { name: 'Review added access' })
  await expect(preview).toBeVisible()
  await expect(preview.locator('.preset-preview li')).toHaveCount(3)
  await expect(preview.locator('.preset-preview')).toContainText('Own records')
  await preview.getByRole('button', { name: 'Apply preset', exact: true }).click()
  await expect(matrix.locator('[data-grant]')).toHaveCount(1)
  await matrix.getByLabel('Record scope', { exact: true }).press('ArrowDown')
  await page.getByRole('option', { name: 'Entire workspace', exact: true }).click()
  await matrix.getByRole('button', { name: 'Tickets · Edit', exact: true }).click()
  await expect(matrix.locator('[data-scope]')).toContainText('Own records')
  await page.screenshot({
    animations: 'disabled',
    path: 'test-results/roles-matrix.png',
    fullPage: true,
  })
  await matrix.locator('.permission-detail').scrollIntoViewIfNeeded()
  await page.screenshot({ path: 'test-results/roles-grant.png', animations: 'disabled' })
  const roleResponse = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === '/api/v1/roles',
  )
  await page.getByRole('button', { name: 'Save role', exact: true }).click()
  const role = (await (await roleResponse).json()).data
  const memberResponse = await page.request.post('/api/v1/members', {
    headers: { ...headers, 'X-CSRF-Token': csrf },
    data: {
      username: `quality-${suffix}`,
      email: `quality-${suffix}@example.test`,
      display_name: `Quality operator ${suffix}`,
      password: 'QualityOperatorInitial-2026!',
      role_ids: [role.id],
    },
  })
  expect(memberResponse.ok()).toBe(true)
  const departmentResponse = await page.request.post('/api/v1/departments', {
    headers: { ...headers, 'X-CSRF-Token': csrf },
    data: { name: `Quality team ${suffix}` },
  })
  expect(departmentResponse.ok()).toBe(true)
  const department = (await departmentResponse.json()).data
  const sharedResponse = await page.request.post('/api/v1/resources/tickets', {
    headers: { ...headers, 'X-CSRF-Token': csrf },
    data: { department_id: department.id, data: { title: `Quality ${suffix} team ticket` } },
  })
  expect(sharedResponse.ok()).toBe(true)
  const shared = (await sharedResponse.json()).data
  const operatorContext = await page
    .context()
    .browser()!
    .newContext({ baseURL: 'http://localhost:5178' })
  try {
    const operator = await operatorContext.newPage()
    await operator.addInitScript(() => {
      localStorage.setItem('tf_locale', 'en')
      const errors: string[] = []
      Object.defineProperty(window, 'tf_e2e_resize_errors', { value: errors })
      window.addEventListener('error', (event) => {
        if (event.message.includes('ResizeObserver')) errors.push(event.message)
      })
    })
    await signInAccount(operator, {
      identity: `quality-${suffix}`,
      initialPassword: 'QualityOperatorInitial-2026!',
      password: 'QualityOperatorChanged-2026!',
    })
    await operator.goto('/resources/tickets')
    await operator.getByRole('button', { name: 'New record', exact: true }).click()
    await operator.getByLabel('Title', { exact: true }).fill(`Quality ${suffix} own ticket`)
    await operator.getByRole('button', { name: 'Save', exact: true }).click()
    await operator
      .getByRole('textbox', { name: 'Search records', exact: true })
      .fill(`Quality ${suffix}`)
    await operator.getByRole('button', { name: 'Search', exact: true }).click()
    const ownRow = operator.getByRole('row').filter({ hasText: `Quality ${suffix} own ticket` }),
      otherRow = operator.getByRole('row').filter({ hasText: `Quality ${suffix} team ticket` })
    await expect(ownRow.getByRole('button', { name: 'Edit', exact: true })).toBeVisible()
    await expect(otherRow).toBeVisible()
    await expect(otherRow.getByRole('button', { name: 'Edit', exact: true })).toHaveCount(0)
    await expect(otherRow.getByRole('button', { name: 'Delete', exact: true })).toHaveCount(0)
    expect(await operator.evaluate(() => Reflect.get(window, 'tf_e2e_resize_errors'))).toEqual([])
    await operator.screenshot({
      animations: 'disabled',
      path: 'test-results/resource-authority.png',
    })
  } finally {
    await operatorContext.close()
  }
  await page.goto('/resources/tickets')
  const sharedRow = page.getByRole('row').filter({ hasText: `Quality ${suffix} team ticket` })
  await sharedRow.getByRole('button', { name: 'Edit', exact: true }).click()
  const departmentField = page
    .locator('.el-dialog .el-form-item')
    .filter({ has: page.locator('label').filter({ hasText: /^Department$/ }) })
  await departmentField.locator('.el-select').hover()
  await departmentField.locator('.el-select__clear').click()
  const clearResponse = page.waitForResponse(
    (response) =>
      response.request().method() === 'PATCH' &&
      new URL(response.url()).pathname === `/api/v1/resources/tickets/${shared.id}`,
  )
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  expect((await clearResponse).ok()).toBe(true)
  const persisted = (
    await (await page.request.get(`/api/v1/resources/tickets/${shared.id}`, { headers })).json()
  ).data
  expect(persisted.department_id ?? '').toBe('')
  await page.goto('/')
  await expect(page.getByRole('button', { name: 'Refresh', exact: true })).toBeEnabled()
  await page.getByRole('button', { name: 'Toggle theme', exact: true }).click()
  await expect(page.locator('html')).toHaveClass(/dark/)
  await settleLayout()
  await page.screenshot({ animations: 'disabled', path: 'test-results/dashboard-dark.png' })
  for (const width of [1024, 390]) {
    await page.setViewportSize({ width, height: 844 })
    await settleLayout()
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true)
  }
  await page.screenshot({ animations: 'disabled', path: 'test-results/mobile-dark.png' })
  await page.getByRole('button', { name: 'Toggle theme', exact: true }).click()
  await settleLayout()
  await page.screenshot({ animations: 'disabled', path: 'test-results/mobile-light.png' })
  const navigationToggle = page.getByRole('button', { name: 'Open navigation', exact: true })
  await navigationToggle.focus()
  await expect(navigationToggle).toHaveAttribute('aria-expanded', 'false')
  for (let index = 0; index < 16; index++) {
    await page.keyboard.press('Tab')
    expect(await page.evaluate(() => Boolean(document.activeElement?.closest('.sidebar')))).toBe(
      false,
    )
  }
  await navigationToggle.focus()
  await page.keyboard.press('Enter')
  await expect(navigationToggle).toHaveAttribute('aria-expanded', 'true')
  await expect(page.locator('.sidebar .brand')).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(navigationToggle).toHaveAttribute('aria-expanded', 'false')
  await expect(navigationToggle).toBeFocused()
  await navigationToggle.click()
  await expect(page.getByRole('navigation', { name: 'Main navigation' })).toBeVisible()
  await page
    .getByRole('navigation')
    .getByRole('link', { name: 'Roles & access', exact: true })
    .click()
  await page.getByRole('button', { name: 'Create role', exact: true }).click()
  await page
    .locator('.role-editor')
    .getByRole('button', { name: 'Tickets · Read', exact: true })
    .click()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  )
  await page.getByRole('button', { name: 'Cancel', exact: true }).click()
  await page.setViewportSize({ width: 1440, height: 1000 })
  await settleLayout()
  await page.goto('/')
  await expect(page.getByRole('button', { name: 'Refresh', exact: true })).toBeEnabled()
  await settleLayout()
  await page.screenshot({ animations: 'disabled', path: 'test-results/dashboard.png' })
  await page.getByRole('button', { name: '中文', exact: true }).click()
  await page.goto('/resources/tickets')
  await expect(page.getByRole('heading', { name: '工单', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '新建记录', exact: true }).click()
  await page.getByLabel('优先级', { exact: true }).press('ArrowDown')
  await expect(page.getByRole('option', { name: '紧急', exact: true })).toBeVisible()
  await page.getByRole('option', { name: '紧急', exact: true }).click()
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('.el-dialog .el-form-item__error')).toContainText('必填')
  await page.getByRole('button', { name: '取消', exact: true }).click()
  await page
    .getByRole('row')
    .filter({ hasText: `Quality ${suffix} team ticket` })
    .getByRole('button', { name: '详情', exact: true })
    .click()
  await expect(page.locator('.el-drawer').getByText('待处理', { exact: true })).toBeVisible()
  await page.screenshot({ animations: 'disabled', path: 'test-results/resource-zh.png' })
  await page.locator('.el-drawer__close-btn').click()
  await page.goto('/')
  await expect(page.getByRole('button', { name: '刷新', exact: true })).toBeEnabled()
  await settleLayout()
  await page.screenshot({ animations: 'disabled', path: 'test-results/dashboard-zh.png' })
})
