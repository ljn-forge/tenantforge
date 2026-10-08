<script setup lang="ts">
import { computed, ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, download, clientContext } from '../api/client'
import { recordAllows } from '../api/capabilities'
import {
  jobTypeLabel,
  statusLabel,
  operationLabel,
  jobGuidance,
  resourceLabel,
} from '../api/presentation'
import { state } from '../stores/app'
import StateBadge from '../components/StateBadge.vue'
import type { Job, Audit } from '../api/types'
import ActivityDetails from '../components/activity/ActivityDetails.vue'
import { t, allowed } from '../stores/app'
import { dateTime } from '../api/forms'
import { useCollection } from '../composables/collection'
import { useRequest, saved, confirmDelete } from '../composables/request'
import PageHeader from '../components/PageHeader.vue'
import PageState from '../components/PageState.vue'
import ActionButton from '../components/ActionButton.vue'
import TaskCreateDialog from '../components/activity/TaskCreateDialog.vue'
import { useJobCatalog } from '../composables/jobCatalog'
const route = useRoute()
const router = useRouter()
const jobs = computed(() => route.path === '/jobs')
const resource = computed(() => (jobs.value ? 'jobs' : 'audit'))
const jobStatus = computed(() =>
  jobs.value && typeof route.query.status === 'string' ? route.query.status : '',
)
const jobStatuses = ['pending', 'running', 'succeeded', 'failed', 'cancelled']
async function filterStatus(value: string) {
  await router.replace({ query: { ...route.query, status: value || undefined } })
}
const list = useCollection<Job | Audit>(
  () => `/${resource.value}`,
  () => allowed(resource.value, 'read'),
  (): Record<string, string> => (jobStatus.value ? { status: jobStatus.value } : {}),
)
const { run, busy, error } = useRequest()
const catalog = useJobCatalog(() => jobs.value, list.load)
const detail = ref<Job | Audit | null>(null)
let detailRevision = 0
function closeDetail() {
  detailRevision++
  detail.value = null
}
watch(
  [
    () => state.tenant,
    () => state.me,
    () => JSON.stringify(state.me?.permissions),
    () => clientContext.value,
    () => route.path,
  ],
  closeDetail,
  { flush: 'sync' },
)
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  timer = setInterval(() => {
    if (
      jobs.value &&
      list.items.value.some(
        (item) => 'status' in item && ['pending', 'running'].includes(item.status),
      )
    )
      void list.load()
  }, 5000)
})
onBeforeUnmount(() => {
  clearInterval(timer)
  closeDetail()
})
async function operate(job: Job, action: string) {
  await run(async () => {
    await api(`/jobs/${job.id}/${action}`, { method: 'POST' })
    saved()
    await list.load()
  })
}
async function remove(job: Job) {
  if (!(await confirmDelete(`${jobTypeLabel(job.type, job.definition)} · ${job.id}`))) return
  await run(async () => {
    await api(`/jobs/${job.id}`, { method: 'DELETE' })
    if (detail.value?.id === job.id) detail.value = null
    saved()
    await list.load()
  })
}
async function exportAudit() {
  await run(async () => {
    await api<Job>('/audit/export', {
      method: 'POST',
      body: { columns: ['created_at', 'action', 'resource', 'record_id', 'actor_id', 'changes'] },
    })
    saved()
    await router.push('/jobs')
  })
}
async function inspect(record: Job | Audit) {
  const revision = ++detailRevision,
    epoch = clientContext.value.epoch
  if (jobs.value)
    await run(async () => {
      const result = (await api<Job>(`/jobs/${record.id}`)).data
      if (revision === detailRevision && epoch === clientContext.value.epoch) detail.value = result
    })
  else detail.value = record
}
</script>
<template>
  <PageHeader
    :title="jobs ? t('后台任务', 'Background jobs') : t('审计记录', 'Audit trail')"
    :description="
      jobs
        ? t(
            '查看任务进度，重试失败任务或取消未完成任务。',
            'Track job progress, retry failures and cancel unfinished work.',
          )
        : t(
            '查看组织内已授权操作的审计记录。',
            'Review the authorized operations recorded in your workspace.',
          )
    "
  >
    <ActionButton
      v-if="jobs && catalog.types.value.length"
      resource="jobs"
      action="create"
      :disabled="catalog.loading.value"
      @click="catalog.show"
    >
      {{ t('新建任务', 'New task') }}
    </ActionButton>
    <ActionButton
      v-if="!jobs"
      resource="audit"
      action="export"
      :loading="busy"
      @click="exportAudit"
    >
      {{ t('导出审计', 'Export audit') }}
    </ActionButton>
    <el-button :loading="list.busy.value" @click="list.load">{{ t('刷新', 'Refresh') }}</el-button>
  </PageHeader>
  <div v-if="jobs && catalog.available.value" class="task-catalog-state">
    <el-skeleton v-if="catalog.loading.value" :rows="1" animated />
    <el-alert
      v-else-if="catalog.catalogError.value"
      type="error"
      :closable="false"
      :title="catalog.catalogError.value"
    >
      <el-button @click="catalog.load">{{ t('重试', 'Retry') }}</el-button>
    </el-alert>
    <el-alert
      v-else-if="catalog.loaded.value && !catalog.types.value.length"
      type="info"
      :closable="false"
      :title="
        t('当前组织暂无可发起的额外任务', 'No additional tasks are available in this workspace')
      "
      :description="
        t(
          '如需更多任务，请联系管理员启用相应扩展。',
          'Ask an administrator to enable a task extension when needed.',
        )
      "
    />
  </div>
  <PageState
    :denied="!allowed(resource, 'read')"
    :busy="list.busy.value"
    :error="list.error.value || error"
    @retry="list.load"
  />
  <div v-if="allowed(resource, 'read')" class="panel">
    <form class="table-toolbar" @submit.prevent="list.search">
      <el-input
        v-model="list.query.value"
        :placeholder="t('搜索记录', 'Search records')"
        clearable
      />
      <el-button native-type="submit">{{ t('搜索', 'Search') }}</el-button>
      <el-select
        v-if="jobs"
        :model-value="jobStatus"
        :aria-label="t('任务状态', 'Job status')"
        :placeholder="t('所有状态', 'All statuses')"
        clearable
        class="status-filter"
        @update:model-value="filterStatus($event ?? '')"
      >
        <el-option
          v-for="status in jobStatuses"
          :key="status"
          :value="status"
          :label="statusLabel(status)"
        />
      </el-select>
      <el-tag v-if="jobStatus" closable @close="filterStatus('')">
        {{ t('筛选', 'Filter') }}: {{ statusLabel(jobStatus) }}
      </el-tag>
      <span class="count-label">{{ list.total.value }} {{ t('条记录', 'records') }}</span>
    </form>
    <el-table v-if="jobs" :data="list.items.value" :empty-text="t('暂无任务', 'No jobs')">
      <el-table-column :label="t('任务类型', 'Type')" min-width="220">
        <template #default="{ row }">
          <strong>{{ jobTypeLabel(row.type, row.definition) }}</strong>
          <small v-if="row.status === 'failed'" class="cell-secondary job-error">
            {{ jobGuidance(row) }}
          </small>
        </template>
      </el-table-column>
      <el-table-column :label="t('状态', 'Status')" width="135">
        <template #default="{ row }">
          <StateBadge :value="row.status" />
        </template>
      </el-table-column>
      <el-table-column :label="t('尝试次数', 'Attempts')" width="110">
        <template #default="{ row }">{{ row.attempts }} / {{ row.max_attempts }}</template>
      </el-table-column>
      <el-table-column :label="t('更新时间', 'Updated')" min-width="180">
        <template #default="{ row }">{{ dateTime(row.updated_at) }}</template>
      </el-table-column>
      <el-table-column :label="t('操作', 'Actions')" width="290">
        <template #default="{ row }">
          <el-button link @click="inspect(row)">{{ t('详情', 'View') }}</el-button>
          <el-button
            v-if="
              recordAllows(row, 'read') && row.status === 'succeeded' && row.type.includes('export')
            "
            link
            type="primary"
            :disabled="busy"
            @click="() => run(() => download(`/jobs/${row.id}/download`, `${row.id}.csv`))"
          >
            {{ t('下载', 'Download') }}
          </el-button>
          <ActionButton
            v-if="row.status === 'failed'"
            resource="jobs"
            :record="row"
            action="update"
            link
            type="primary"
            :disabled="busy"
            @click="operate(row, 'retry')"
          >
            {{ t('重试', 'Retry') }}
          </ActionButton>
          <ActionButton
            v-if="['succeeded', 'failed', 'cancelled'].includes(row.status)"
            resource="jobs"
            :record="row"
            action="update"
            link
            type="danger"
            :disabled="busy"
            @click="remove(row)"
          >
            {{ t('删除', 'Delete') }}
          </ActionButton>
          <ActionButton
            v-if="['pending', 'running'].includes(row.status)"
            resource="jobs"
            :record="row"
            action="update"
            link
            type="danger"
            :disabled="busy"
            @click="operate(row, 'cancel')"
          >
            {{ t('取消', 'Cancel') }}
          </ActionButton>
        </template>
      </el-table-column>
    </el-table>
    <el-table v-else :data="list.items.value" :empty-text="t('暂无审计记录', 'No audit entries')">
      <el-table-column :label="t('操作', 'Action')" min-width="240">
        <template #default="{ row }">
          {{
            operationLabel(
              row.action,
              state.me?.modules.find((module) => module.key === row.resource),
            )
          }}
        </template>
      </el-table-column>
      <el-table-column :label="t('资源', 'Resource')" min-width="160">
        <template #default="{ row }">
          {{ resourceLabel(row.resource, state.me?.modules ?? []) }}
        </template>
      </el-table-column>
      <el-table-column :label="t('时间', 'Time')" min-width="180">
        <template #default="{ row }">{{ dateTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column width="95">
        <template #default="{ row }">
          <el-button link @click="inspect(row)">{{ t('详情', 'View') }}</el-button>
        </template>
      </el-table-column>
    </el-table>
    <div class="pagination">
      <el-pagination
        v-model:current-page="list.page.value"
        :total="list.total.value"
        :page-size="list.pageSize.value"
        layout="prev,pager,next"
        @change="list.load"
      />
    </div>
  </div>
  <el-drawer
    :model-value="!!detail"
    :title="t('记录详情', 'Record details')"
    size="min(650px,100%)"
    @close="closeDetail"
  >
    <ActivityDetails v-if="detail" :record="detail" />
  </el-drawer>
  <TaskCreateDialog
    :open="catalog.open.value"
    :types="catalog.types.value"
    :selected-type="catalog.selectedType.value"
    :values="catalog.values.value"
    :errors="catalog.fieldErrors.value"
    :error="catalog.submitError.value"
    :submitting="catalog.submitting.value"
    @choose="catalog.choose"
    @change="catalog.change"
    @close="catalog.close"
    @submit="catalog.submit"
  />
</template>
