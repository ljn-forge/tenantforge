import { test, expect } from '@playwright/test'
import process from 'node:process'
import { signInAccount } from './helpers'

test('public asset extension launches a scoped task and displays bilingual typed results in the built UI', async ({
  page,
}) => {
  test.skip(
    process.env.TF_E2E_ASSETS !== '1',
    'Use browser-check.py --built --assets to install the public extension.',
  )
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('console', (message) => {
    if (message.type() === 'warning' && /Failed to resolve component/.test(message.text()))
      errors.push(message.text())
  })
  await page.addInitScript(() => localStorage.setItem('tf_locale', 'en'))
  await signInAccount(page)
  await page.goto('/resources/assets')
  await page.getByRole('button', { name: 'New record', exact: true }).click()
  const name = `Browser inventory ${Date.now()}`
  await page.getByLabel('Name', { exact: true }).fill(name)
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  const asset = page.getByRole('row').filter({ hasText: name })
  await expect(asset).toBeVisible()
  await asset.getByRole('button', { name: 'View', exact: true }).click()
  await page.getByRole('button', { name: 'Approve activation', exact: true }).click()
  await expect(page.locator('.el-drawer').getByText('true', { exact: true }).first()).toBeVisible()
  await page.locator('.el-drawer__close-btn').click()
  await page.goto('/jobs')
  await page.getByRole('button', { name: 'New task', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Start task', exact: true })
  await expect(dialog).toContainText('Asset inventory')
  const creation = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/jobs',
  )
  await dialog.getByRole('button', { name: 'Start task', exact: true }).click()
  const response = await creation
  expect(response.ok()).toBe(true)
  const job = (await response.json()).data
  expect(job.type).toBe('assets_inventory')
  await expect(dialog).not.toBeVisible()
  const message = page.locator('.el-message').last()
  await expect(message).toBeVisible()
  expect(await message.evaluate((element) => getComputedStyle(element).position)).toBe('fixed')
  const row = page.getByRole('row').filter({ hasText: 'Asset inventory' }).first()
  await expect(row).toContainText('Succeeded', { timeout: 20000 })
  await row.getByRole('button', { name: 'View', exact: true }).click()
  const drawer = page.locator('.el-drawer')
  await expect(drawer.getByText('Visible asset count', { exact: true })).toBeVisible()
  await expect(drawer).toBeInViewport({ ratio: 0.99 })
  const tenant = await page.evaluate(
    () => JSON.parse(sessionStorage.getItem('tf_workspace') ?? '{}').tenant_id,
  )
  const detail = await page.request.get(`/api/v1/jobs/${job.id}`, {
    headers: { 'X-Tenant-ID': tenant },
  })
  expect(detail.ok()).toBe(true)
  const persisted = (await detail.json()).data
  expect(persisted.result.count).toBeGreaterThan(0)
  expect(persisted.definition.result_fields[0].label_en).toBe('Visible asset count')
  expect(persisted).not.toHaveProperty('payload')
  await page.screenshot({ path: 'test-results/extension-task-en.png', animations: 'disabled' })
  await drawer.locator('.el-drawer__close-btn').click()
  await expect(drawer).not.toBeVisible()
  await page.getByRole('button', { name: '中文', exact: true }).click()
  await page
    .getByRole('row')
    .filter({ hasText: '资产盘点' })
    .first()
    .getByRole('button', { name: '详情', exact: true })
    .click()
  await expect(drawer.getByText('可见资产数', { exact: true })).toBeVisible()
  await expect(drawer).toBeInViewport({ ratio: 0.99 })
  await page.screenshot({ path: 'test-results/extension-task-zh.png', animations: 'disabled' })
  await drawer.locator('.el-drawer__close-btn').click()
  await page.getByRole('button', { name: '新建任务', exact: true }).click()
  await expect(page.getByRole('dialog', { name: '发起任务', exact: true })).toBeVisible()
  await page.screenshot({
    path: 'test-results/extension-task-create-zh.png',
    animations: 'disabled',
  })
  await page.getByRole('dialog').getByRole('button', { name: '取消', exact: true }).click()
  await page.getByRole('button', { name: '切换主题', exact: true }).click()
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole('button', { name: '新建任务', exact: true }).click()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  )
  await page.screenshot({
    path: 'test-results/extension-task-mobile-dark.png',
    animations: 'disabled',
  })
  await page.getByRole('dialog').getByRole('button', { name: '取消', exact: true }).click()
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page
    .getByRole('row')
    .filter({ hasText: '资产盘点' })
    .first()
    .getByRole('button', { name: '删除', exact: true })
    .click()
  const confirmation = page.locator('.el-message-box')
  await expect(confirmation).toBeVisible()
  expect(await confirmation.evaluate((element) => getComputedStyle(element).position)).toBe(
    'relative',
  )
  expect(
    await page
      .locator('.el-overlay-message-box')
      .evaluate((element) => getComputedStyle(element).position),
  ).toBe('fixed')
  await confirmation.getByRole('button', { name: '取消', exact: true }).click()
  expect(errors).toEqual([])
})
