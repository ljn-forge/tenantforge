<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, download } from '../api/client'
import type { Job } from '../api/types'
import { t, allowed } from '../stores/app'
import { formatValue, dateTime } from '../api/forms'
import { useCollection } from '../composables/collection'
import { useRequest, saved, confirmDelete } from '../composables/request'
import PageHeader from '../components/PageHeader.vue'
import PageState from '../components/PageState.vue'
import ActionButton from '../components/ActionButton.vue'
interface Audit {
  id: string
  actor_id: string
  action: string
  resource: string
  record_id: string
  request_id: string
  created_at: string
  changes: Record<string, unknown>
}
const route = useRoute()
const router = useRouter()
const jobs = computed(() => route.path === '/jobs')
const resource = computed(() => (jobs.value ? 'jobs' : 'audit'))
const list = useCollection<Job | Audit>(
  () => `/${resource.value}`,
  () => allowed(resource.value, 'read'),
)
const { run, busy, error } = useRequest()
const detail = ref<Job | Audit | null>(null)
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
onBeforeUnmount(() => clearInterval(timer))
async function operate(job: Job, action: string) {
  await run(async () => {
    await api(`/jobs/${job.id}/${action}`, { method: 'POST' })
    saved()
    await list.load()
  })
}
async function remove(job: Job) {
  if (!(await confirmDelete(`${job.type} · ${job.id}`))) return
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
  if (jobs.value)
    await run(async () => {
      detail.value = (await api<Job>(`/jobs/${record.id}`)).data
    })
  else detail.value = record
}
function statusType(status: string) {
  return status === 'succeeded'
    ? 'success'
    : status === 'failed'
      ? 'danger'
      : status === 'running'
        ? 'warning'
        : 'info'
}
</script>
<template>
  <PageHeader
    :title="jobs ? t('后台任务', 'Background jobs') : t('审计记录', 'Audit trail')"
    :description="
      jobs
        ? t(
            '查看异步导出状态，重试失败任务或取消未完成任务。',
            'Track asynchronous exports, retry failures and cancel unfinished work.',
          )
        : t(
            '查看组织内已授权操作的审计记录。',
            'Review the authorized operations recorded in your workspace.',
          )
    "
  >
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
      <span class="count-label">{{ list.total.value }} {{ t('条记录', 'records') }}</span>
    </form>
    <el-table v-if="jobs" :data="list.items.value" :empty-text="t('暂无任务', 'No jobs')">
      <el-table-column prop="type" :label="t('任务类型', 'Type')" min-width="180" />
      <el-table-column :label="t('状态', 'Status')" width="135">
        <template #default="{ row }">
          <el-tag :type="statusType(row.status)">{{ row.status }}</el-tag>
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
            v-if="row.status === 'succeeded' && row.type.includes('export')"
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
      <el-table-column prop="action" :label="t('操作', 'Action')" min-width="160" />
      <el-table-column prop="resource" :label="t('资源', 'Resource')" min-width="150" />
      <el-table-column
        prop="actor_id"
        :label="t('操作者', 'Actor')"
        min-width="190"
        show-overflow-tooltip
      />
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
    @close="detail = null"
  >
    <el-descriptions v-if="detail" :column="1" border>
      <el-descriptions-item v-for="(value, key) in detail" :key="key" :label="String(key)">
        <pre v-if="typeof value === 'object'" class="json-detail">{{
          JSON.stringify(value, null, 2)
        }}</pre>
        <span v-else>{{ formatValue(value) }}</span>
      </el-descriptions-item>
    </el-descriptions>
  </el-drawer>
</template>
