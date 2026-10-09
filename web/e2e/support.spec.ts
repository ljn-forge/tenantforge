import { test, expect, type Page, type BrowserContext } from '@playwright/test'
import { signInAccount } from './helpers'

test('internal support roles, supervisor assignment, scoped conversation, resolution and reopen form a persisted workflow', async ({
  page,
  browser,
}) => {
  const errors: string[] = []
  const watch = (page: Page) => page.on('pageerror', (error) => errors.push(error.message))
  watch(page)
  await page.addInitScript(() => localStorage.setItem('tf_locale', 'en'))
  const admin = await signInAccount(page)
  const me = (await (await page.request.get('/api/v1/auth/me')).json()).data
  const tenant = me.tenants.find((row: { status: string }) => row.status === 'active').id
  const csrf = (await (await page.request.get('/api/v1/auth/csrf')).json()).data.csrf_token
  const headers = { 'X-Tenant-ID': tenant, 'X-CSRF-Token': csrf, Origin: 'http://localhost:5178' }
  async function post(path: string, data: unknown) {
    const response = await page.request.post('/api/v1' + path, { headers, data })
    expect(response.ok(), `${path} HTTP ${response.status()}`).toBe(true)
    return (await response.json()).data
  }
  const suffix = Date.now().toString()
  const department = await post('/departments', { name: `Support desk ${suffix}`, parent_id: null })
  async function template(kind: 'agent' | 'supervisor') {
    const name = `Support ${kind} ${suffix}`
    await page.goto('/roles')
    await page.getByRole('button', { name: `New support ${kind} role`, exact: true }).click()
    await page.getByLabel('Name', { exact: true }).fill(name)
    await expect(page.getByRole('dialog')).toContainText('Customer email')
    await expect(page.getByRole('dialog')).toContainText('Internal notes')
    await page.getByRole('button', { name: 'Save role', exact: true }).click()
    await expect(page.getByRole('row').filter({ hasText: name })).toBeVisible()
    const roles = (await (await page.request.get('/api/v1/roles', { headers })).json()).data
    const role = roles.find((row: { name: string }) => row.name === name)
    expect(role.grants.some((grant: { action: string }) => grant.action === 'reply')).toBe(true)
    expect(role.grants.some((grant: { action: string }) => grant.action === 'delete')).toBe(false)
    return role
  }
  const agentRole = await template('agent')
  const supervisorRole = await template('supervisor')
  const viewerRole = await post('/roles', {
    name: `Support observer ${suffix}`,
    description: 'Reads ordinary ticket fields without contact or private notes',
    grants: [
      {
        resource: 'tickets',
        action: 'read',
        scope: 'department',
        read_fields: ['title', 'description', 'status', 'priority'],
      },
    ],
  })
  const initial = 'SupportFixtureInitial-2026!'
  const changed = 'SupportFixtureChanged-2026!'
  async function employee(label: string, role: string) {
    return post('/members', {
      username: `support-${label}-${suffix}`,
      email: `support-${label}-${suffix}@example.test`,
      display_name: {
        registrar: '登记客服 · 小陈',
        agent: '处理客服 · 李晓月',
        supervisor: '客服主管 · 王经理',
        observer: '只读观察员 · 周同事',
      }[label],
      password: initial,
      department_id: department.id,
      position: label,
      role_ids: [role],
    })
  }
  const registrar = await employee('registrar', agentRole.id)
  const assignee = await employee('agent', agentRole.id)
  const supervisor = await employee('supervisor', supervisorRole.id)
  const observer = await employee('observer', viewerRole.id)
  const contexts: BrowserContext[] = []
  async function employeePage(member: { username: string }) {
    const context = await browser.newContext({ baseURL: 'http://localhost:5178' })
    contexts.push(context)
    const employee = await context.newPage()
    watch(employee)
    await employee.addInitScript(() => localStorage.setItem('tf_locale', 'en'))
    await signInAccount(employee, {
      identity: member.username,
      initialPassword: initial,
      password: changed,
    })
    return employee
  }
  try {
    const registration = await employeePage(registrar)
    await registration.goto('/resources/tickets')
    await registration.getByRole('button', { name: 'New record', exact: true }).click()
    const title = '客户无法下载发票'
    await registration.getByLabel('Title', { exact: true }).fill(title)
    await registration
      .getByLabel('Description', { exact: true })
      .fill('客户修改企业信息后，下载发票时出现错误，需要客服协助排查。')
    await registration.getByLabel('Customer name', { exact: true }).fill('陈女士')
    await registration.getByLabel('Customer email', { exact: true }).fill('customer@example.test')
    const creation = registration.waitForResponse(
      (response) =>
        response.request().method() === 'POST' &&
        new URL(response.url()).pathname === '/api/v1/resources/tickets',
    )
    await registration.getByRole('button', { name: 'Save', exact: true }).click()
    const created = await creation
    expect(created.ok()).toBe(true)
    const ticket = (await created.json()).data
    expect(ticket.owner_id).toBe(registrar.user_id)
    expect(ticket.department_id).toBe(department.id)
    const agent = await employeePage(assignee)
    const denied = await agent.request.get(`/api/v1/resources/tickets/${ticket.id}`, {
      headers: { 'X-Tenant-ID': tenant },
    })
    expect(denied.status()).toBe(404)

    const lead = await employeePage(supervisor)
    await lead.goto('/resources/tickets')
    await lead
      .getByRole('row')
      .filter({ hasText: title })
      .getByRole('button', { name: 'View', exact: true })
      .click()
    await lead.getByRole('button', { name: 'Assign ticket', exact: true }).click()
    const assignment = lead.getByRole('dialog', { name: 'Assign ticket', exact: true })
    await expect(assignment.getByLabel('Assignee', { exact: true })).toBeEnabled()
    await assignment.getByLabel('Assignee', { exact: true }).press('ArrowDown')
    await lead.getByRole('option', { name: assignee.display_name, exact: true }).click()
    await assignment.getByRole('button', { name: 'Assign', exact: true }).click()
    await expect(assignment).not.toBeVisible()
    const oldAccess = await registration.request.get(`/api/v1/resources/tickets/${ticket.id}`, {
      headers: { 'X-Tenant-ID': tenant },
    })
    expect(oldAccess.status()).toBe(404)

    await agent.goto('/resources/tickets?queue=mine')
    await agent
      .getByRole('row')
      .filter({ hasText: title })
      .getByRole('button', { name: 'View', exact: true })
      .click()
    const drawer = agent.locator('.el-drawer')
    await drawer.getByRole('button', { name: 'Start work', exact: true }).click()
    await expect(drawer.getByText('In progress', { exact: true }).first()).toBeVisible()
    const handling = drawer.locator('.ticket-support')
    await handling
      .getByLabel('Reply message', { exact: true })
      .fill('已核对客户账户并复现下载问题，正在检查发票生成记录。')
    await handling.getByRole('button', { name: 'Add reply', exact: true }).click()
    await expect(handling.locator('.ticket-activity')).toContainText('已核对客户账户')
    await handling.getByText('Internal note', { exact: true }).click()
    const privateNote =
      '内部排查：税务信息同步失败，需要重新同步后生成发票。此备注仅供客服团队查看。'
    await handling.getByLabel('Reply message', { exact: true }).fill(privateNote)
    await handling.getByRole('button', { name: 'Add note', exact: true }).click()
    await expect(handling.locator('.ticket-activity')).toContainText(privateNote)

    const viewer = await employeePage(observer)
    await viewer.goto('/resources/tickets')
    await viewer
      .getByRole('row')
      .filter({ hasText: title })
      .getByRole('button', { name: 'View', exact: true })
      .click()
    await expect(viewer.locator('.ticket-activity')).toContainText('已核对客户账户')
    await expect(viewer.locator('.el-drawer')).not.toContainText(privateNote)
    await expect(viewer.locator('.ticket-composer')).toHaveCount(0)
    const ownerHistory = await (
      await page.request.get(`/api/v1/resources/tickets/${ticket.id}/activity`, { headers })
    ).json()
    const viewerHistory = await (
      await viewer.request.get(`/api/v1/resources/tickets/${ticket.id}/activity`, {
        headers: { 'X-Tenant-ID': tenant },
      })
    ).json()
    expect(viewerHistory.data.some((event: { kind: string }) => event.kind === 'note')).toBe(false)
    expect(viewerHistory.meta.total).toBe(ownerHistory.meta.total - 1)
    expect(JSON.stringify(viewerHistory)).not.toContain(privateNote)

    await drawer.getByRole('button', { name: 'Resolve', exact: true }).click()
    const resolution = agent.getByRole('dialog', { name: 'Resolve ticket', exact: true })
    await expect(
      resolution.getByRole('button', { name: 'Resolve ticket', exact: true }),
    ).toBeDisabled()
    const summary = '补齐税务信息并重新生成发票，客户已确认下载成功。'
    await resolution.getByLabel('Resolution summary', { exact: true }).fill(summary)
    await resolution.getByRole('button', { name: 'Resolve ticket', exact: true }).click()
    await expect(resolution).not.toBeVisible()
    await expect(drawer.getByText('Resolved', { exact: true }).first()).toBeVisible()
    await drawer.getByRole('button', { name: 'Close ticket', exact: true }).click()
    await expect(drawer.getByText('Closed', { exact: true }).first()).toBeVisible()
    await expect(handling.locator('.ticket-composer')).toHaveCount(0)
    const persisted = (
      await (await page.request.get(`/api/v1/resources/tickets/${ticket.id}`, { headers })).json()
    ).data
    expect(persisted.owner_id).toBe(assignee.user_id)
    expect(persisted.data.resolution).toBe(summary)
    expect(persisted.data.status).toBe('closed')

    let other = me.tenants.find(
      (row: { id: string; status: string }) => row.id !== tenant && row.status === 'active',
    )
    if (!other)
      other = await post('/platform/tenants', {
        name: `Other support company ${suffix}`,
        slug: `support-company-${suffix}`,
        owner_email: admin.email,
      })
    const crossTenant = await page.request.get(`/api/v1/resources/tickets/${ticket.id}/activity`, {
      headers: { ...headers, 'X-Tenant-ID': other.id },
    })
    expect(crossTenant.status()).toBe(404)

    await drawer.getByRole('button', { name: 'Reopen', exact: true }).click()
    await expect(drawer.getByText('Open', { exact: true }).first()).toBeVisible()
    await drawer.locator('.el-drawer__close-btn').click()
    await expect(drawer).not.toBeVisible()
    await agent.getByRole('button', { name: '中文', exact: true }).click()
    await agent
      .getByRole('row')
      .filter({ hasText: title })
      .getByRole('button', { name: '详情', exact: true })
      .click()
    await expect(drawer.locator('.ticket-activity')).toContainText(privateNote)
    await expect(drawer).toBeInViewport({ ratio: 0.99 })
    await drawer.getByText(privateNote, { exact: true }).scrollIntoViewIfNeeded()
    await expect(agent.locator('.el-message')).toHaveCount(0)
    await agent.screenshot({ path: 'test-results/support-ticket-zh.png', animations: 'disabled' })
    await drawer.locator('.el-drawer__close-btn').click()
    await expect(drawer).not.toBeVisible()
    await agent.screenshot({ path: 'test-results/support-queue-zh.png', animations: 'disabled' })
    await agent.getByRole('button', { name: '切换主题', exact: true }).click()
    await agent.setViewportSize({ width: 390, height: 844 })
    await agent
      .getByRole('row')
      .filter({ hasText: title })
      .getByRole('button', { name: '详情', exact: true })
      .click()
    await expect(drawer.locator('.ticket-activity')).toContainText(privateNote)
    await expect(drawer).toBeInViewport({ ratio: 0.99 })
    await drawer.getByText(privateNote, { exact: true }).scrollIntoViewIfNeeded()
    expect(
      await agent.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true)
    await agent.screenshot({ path: 'test-results/support-mobile-dark.png', animations: 'disabled' })
    expect(errors).toEqual([])
  } finally {
    for (const context of contexts) await context.close()
  }
})
