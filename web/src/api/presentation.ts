import { t, state } from '../stores/app'
import type { Grant, Module, Job, JobDefinition } from './types'
const operations: Record<string, [string, string]> = {
  read: ['查看', 'Read'],
  create: ['新建', 'Create'],
  update: ['编辑', 'Edit'],
  delete: ['删除', 'Delete'],
  assign: ['分配', 'Assign'],
  reply: ['记录沟通与备注', 'Reply'],
  import: ['导入', 'Import'],
  export: ['导出', 'Export'],
  copy: ['复制', 'Copy'],
  transfer: ['转移记录', 'Transfer'],
  retry: ['重试', 'Retry'],
  cancel: ['取消', 'Cancel'],
  download: ['下载', 'Download'],
  approve: ['批准', 'Approve'],
  succeeded: ['任务完成', 'Job completed'],
  failed: ['任务失败', 'Job failed'],
  login: ['登录', 'Signed in'],
  session_revoked: ['撤销会话', 'Session revoked'],
  password_changed: ['修改密码', 'Password changed'],
  mfa_enabled: ['启用双重验证', 'Two-factor authentication enabled'],
  mfa_disabled: ['关闭双重验证', 'Two-factor authentication disabled'],
  mfa_enrollment_started: ['开始设置双重验证', 'Two-factor setup started'],
  password_reset: ['重置密码', 'Password reset'],
  transition: ['更新状态', 'Status changed'],
  oidc_linked: ['关联单点登录', 'Single sign-on linked'],
  oidc_login: ['单点登录', 'Single sign-on sign-in'],
  provision: ['创建组织', 'Workspace provisioned'],
}
const statuses: Record<string, [string, string]> = {
  open: ['待处理', 'Open'],
  in_progress: ['处理中', 'In progress'],
  resolved: ['已解决', 'Resolved'],
  closed: ['已关闭', 'Closed'],
  pending: ['等待中', 'Pending'],
  running: ['执行中', 'Running'],
  succeeded: ['已完成', 'Succeeded'],
  failed: ['失败', 'Failed'],
  cancelled: ['已取消', 'Cancelled'],
  active: ['启用', 'Active'],
  suspended: ['暂停', 'Suspended'],
  accepted: ['已接受', 'Accepted'],
  expired: ['已过期', 'Expired'],
  normal: ['普通', 'Normal'],
  low: ['低', 'Low'],
  high: ['高', 'High'],
  urgent: ['紧急', 'Urgent'],
  draft: ['草稿', 'Draft'],
  published: ['已发布', 'Published'],
  approved: ['已批准', 'Approved'],
  rejected: ['已拒绝', 'Rejected'],
}
const scopes: Record<string, [string, string]> = {
  self: ['本人拥有的记录', 'Own records'],
  department: ['所属部门', 'Own department'],
  department_tree: ['所属部门及下级', 'Department and descendants'],
  selected_departments: ['指定部门', 'Selected departments'],
  tenant: ['整个组织', 'Entire workspace'],
}
const controlResources: Record<string, [string, string]> = {
  jobs: ['后台任务', 'Jobs'],
  files: ['文件', 'Files'],
  members: ['用户', 'Users'],
  roles: ['角色', 'Roles'],
  departments: ['部门', 'Departments'],
  audit: ['审计记录', 'Audit trail'],
  invitations: ['邀请', 'Invitations'],
  tenants: ['组织', 'Workspaces'],
}
function label(map: Record<string, [string, string]>, key: string) {
  const pair = map[key]
  return pair ? t(pair[0], pair[1]) : key.replaceAll('_', ' ')
}
export function operationLabel(key: string, module?: Module) {
  const operation = module?.operations?.find((item) => item.key === key)
  return operation
    ? state.locale === 'zh'
      ? operation.label
      : operation.label_en
    : label(operations, key)
}
export function statusLabel(key: string) {
  return label(statuses, key)
}
export function scopeLabel(key: string) {
  return label(scopes, key)
}
export function moduleLabel(module: Module) {
  return controlResources[module.key]
    ? label(controlResources, module.key)
    : state.locale === 'zh'
      ? module.label
      : module.label_en
}
export function moduleDescription(module: Module) {
  if (state.locale === 'en') return module.description_en ?? module.description
  const builtins: Record<string, string> = {
    tickets: '处理工单，跟进团队工作',
    dictionaries: '维护组织的业务字典',
    settings: '管理组织配置',
    notifications: '发布通知，与团队保持同步',
  }
  return module.description_en ? module.description : (builtins[module.key] ?? module.description)
}
export function statusTone(key: string): 'success' | 'danger' | 'warning' | 'info' | 'primary' {
  return ['succeeded', 'resolved', 'active', 'accepted', 'approved', 'published'].includes(key)
    ? 'success'
    : ['failed', 'urgent', 'rejected'].includes(key)
      ? 'danger'
      : ['pending', 'running', 'in_progress', 'high'].includes(key)
        ? 'warning'
        : key === 'open'
          ? 'primary'
          : 'info'
}
export function transitionLabel(key: string) {
  const pairs: Record<string, [string, string]> = {
    in_progress: ['开始处理', 'Start work'],
    resolved: ['标记解决', 'Resolve'],
    closed: ['关闭工单', 'Close ticket'],
    open: ['重新打开', 'Reopen'],
  }
  return label(pairs, key)
}
export function grantSummary(grant: Grant, module?: Module) {
  return `${module ? moduleLabel(module) : grant.resource} · ${operationLabel(grant.action, module)} · ${scopeLabel(grant.scope)}${grant.conditions?.length ? t(` · ${grant.conditions.length} 个条件`, ` · ${grant.conditions.length} conditions`) : ''}`
}
export function activitySentence(entry: Record<string, unknown>, modules: Module[]) {
  const key = String(entry.resource ?? '')
  const module = modules.find((item) => item.key === key)
  const resource = resourceLabel(key, modules)
  const action = String(entry.action ?? '')
  const sentences: Record<string, [string, string]> = {
    create: [`新建了${resource}记录`, `Created a record in ${resource}`],
    update: [`修改了${resource}记录`, `Updated a record in ${resource}`],
    delete: [`删除了${resource}记录`, `Deleted a record from ${resource}`],
    assign: [`调整了${resource}归属`, `Changed ownership in ${resource}`],
    import: [`导入了${resource}记录`, `Imported records into ${resource}`],
    export: [`发起了${resource}导出`, `Started an export from ${resource}`],
    copy: [`复制了${resource}记录`, `Copied a record in ${resource}`],
    transfer: [`转移了${resource}记录`, `Transferred records in ${resource}`],
    transition: [`更新了${resource}状态`, `Changed a status in ${resource}`],
    succeeded: [`${resource}已完成`, `${resource} completed`],
    failed: [`${resource}执行失败`, `${resource} failed`],
  }
  return sentences[action]
    ? t(...sentences[action]!)
    : `${operationLabel(action, module)} · ${resource}`
}
export function jobStageLabel(stage: string | undefined) {
  const stages: Record<string, [string, string]> = {
    claim: ['领取任务', 'Claiming work'],
    authorize: ['权限检查', 'Access checks'],
    prepare: ['准备数据', 'Preparing data'],
    reserve: ['预留配额', 'Reserving quota'],
    storage: ['保存结果', 'Saving results'],
    publish: ['发布结果', 'Publishing results'],
    handler: ['执行任务', 'Executing work'],
    cleanup: ['清理文件', 'Cleaning files'],
    deliver: ['交付结果', 'Delivering results'],
    housekeeping: ['维护数据', 'Maintaining data'],
  }
  return stage && stages[stage] ? t(...stages[stage]!) : t('执行阶段', 'Execution')
}
export function jobGuidance(job: Job) {
  if (job.status !== 'failed') return ''
  const code = job.error_code ?? ''
  const reasons: Record<string, [string, string]> = {
    permission_revoked: [
      '当前权限不再允许此任务。请联系管理员后重新发起。',
      'Access changed. Contact an administrator before starting a new job.',
    ],
    quota_exceeded: [
      '已达到组织配额。释放空间或调整配额后重试。',
      'Workspace quota reached. Release space or adjust the quota before retrying.',
    ],
    handler_failed: [
      '任务未能完成。重试仍失败时，请向管理员提供任务编号。',
      'The job could not finish. Retry; if it fails again, share its job ID with an administrator.',
    ],
    validation_failed: [
      '任务输入无效，请检查数据后重新发起。',
      'Check the input data before starting a new job.',
    ],
    publish_conflict: [
      '结果状态已变化，请刷新后重新发起。',
      'The result changed. Refresh before starting a new job.',
    ],
    timeout: ['任务超时，请稍后重试。', 'The job timed out. Retry later.'],
    storage_unavailable: ['存储暂时不可用，请稍后重试。', 'Storage is unavailable. Retry later.'],
    provider_unavailable: [
      '外部服务暂时不可用，请稍后重试。',
      'The external service is unavailable. Retry later.',
    ],
    database_unavailable: [
      '数据服务暂时不可用，请稍后重试。',
      'The data service is unavailable. Retry later.',
    ],
    cancelled: ['任务已取消。', 'The job was cancelled.'],
  }
  return code && reasons[code]
    ? t(...reasons[code]!)
    : t(
        '任务执行失败。可重试；如反复失败，请联系管理员。',
        'The job failed. Retry, or contact an administrator if it keeps failing.',
      )
}
export function validationLabel(issue: string, module?: Module) {
  if (state.locale !== 'zh') return issue
  const exact: Record<string, string> = {
    'Resource and action are required': '请选择资源与操作',
    'Select at least one department': '至少选择一个部门',
    'Invalid condition': '条件字段或运算符无效',
    'The in operator requires an array': '集合条件必须填写一组值',
    'Invalid scope': '数据范围无效',
    'Unknown operation': '未知操作',
  }
  if (exact[issue]) return exact[issue]
  const rules: Record<string, string> = {
    'Unknown readable field: ': '未知可读字段：',
    'Unknown writable field: ': '未知可写字段：',
    'Read-only field cannot be writable: ': '只读字段不可写入：',
    'Unknown sensitive field: ': '未知敏感字段：',
    'Unknown condition field: ': '未知条件字段：',
    'Invalid condition value: ': '条件值格式不正确：',
    'Sensitive fields cannot be used in conditions: ': '敏感字段不可用于条件：',
  }
  for (const [prefix, text] of Object.entries(rules))
    if (issue.startsWith(prefix)) {
      const field = issue.slice(prefix.length)
      return text + (module?.fields.find((item) => item.key === field)?.label ?? field)
    }
  if (issue.startsWith('Explicit read access is required to unmask ')) {
    const key = issue.slice('Explicit read access is required to unmask '.length)
    return `查看${module?.fields.find((item) => item.key === key)?.label ?? key}明文前需先明确授权读取`
  }
  return issue
}
export function jobTypeLabel(type: string, definition?: JobDefinition) {
  if (definition) return state.locale === 'zh' ? definition.label : definition.label_en
  const builtins: Record<string, [string, string]> = {
    export: ['数据导出', 'Data export'],
    audit_export: ['审计导出', 'Audit export'],
    resource_export: ['资源导出', 'Resource export'],
  }
  return label(builtins, type)
}
export function fieldValidationLabel(issue: string) {
  const values: Record<string, [string, string]> = {
    Required: ['必填', 'Required'],
    'Enter a number': ['请输入有效数字', 'Enter a number'],
    'Choose a valid option': ['请选择有效选项', 'Choose a valid option'],
  }
  return label(values, issue)
}

export function resourceLabel(key: string, modules: Module[]) {
  if (controlResources[key]) return label(controlResources, key)
  const module = modules.find((item) => item.key === key)
  return module ? moduleLabel(module) : key.replaceAll('_', ' ')
}
