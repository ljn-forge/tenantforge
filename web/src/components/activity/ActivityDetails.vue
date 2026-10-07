<script setup lang="ts">
import { computed } from 'vue'
import type { Audit, Job } from '../../api/types'
import { state, t } from '../../stores/app'
import { dateTime } from '../../api/forms'
import {
  jobTypeLabel,
  jobStageLabel,
  jobGuidance,
  operationLabel,
  resourceLabel,
} from '../../api/presentation'
import StateBadge from '../StateBadge.vue'
const props = defineProps<{ record: Job | Audit }>()
const job = computed(() => ('status' in props.record ? props.record : null))
const audit = computed(() => ('action' in props.record ? props.record : null))
const auditOperation = computed(() => {
  const entry = audit.value
  return entry
    ? operationLabel(
        entry.action,
        state.me?.modules.find((item) => item.key === entry.resource),
      )
    : ''
})
</script>
<template>
  <div v-if="job">
    <div class="panel-heading">
      <h2>{{ jobTypeLabel(job.type) }}</h2>
      <StateBadge :value="job.status" />
    </div>
    <el-alert
      v-if="job.status === 'failed'"
      type="warning"
      :closable="false"
      :title="jobGuidance(job)"
    />
    <el-descriptions :column="1" border>
      <el-descriptions-item :label="t('尝试次数', 'Attempts')">
        {{ job.attempts }} / {{ job.max_attempts }}
      </el-descriptions-item>
      <el-descriptions-item v-if="job.error_stage" :label="t('失败阶段', 'Failure stage')">
        {{ jobStageLabel(job.error_stage) }}
      </el-descriptions-item>
      <el-descriptions-item :label="t('创建时间', 'Created')">
        {{ dateTime(job.created_at) }}
      </el-descriptions-item>
      <el-descriptions-item :label="t('更新时间', 'Updated')">
        {{ dateTime(job.updated_at) }}
      </el-descriptions-item>
    </el-descriptions>
  </div>
  <div v-else-if="audit">
    <h2>
      {{ auditOperation }}
      · {{ resourceLabel(audit.resource, state.me?.modules ?? []) }}
    </h2>
    <p class="hint">{{ dateTime(audit.created_at) }}</p>
  </div>
  <el-collapse>
    <el-collapse-item :title="t('技术详情', 'Technical details')" name="metadata">
      <el-descriptions :column="1" border>
        <el-descriptions-item :label="t('记录编号', 'Record ID')">
          {{ record.id }}
        </el-descriptions-item>
        <template v-if="audit">
          <el-descriptions-item :label="t('操作者编号', 'Actor ID')">
            {{ audit.actor_id }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('目标记录编号', 'Target record ID')">
            {{ audit.record_id }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('请求编号', 'Request ID')">
            {{ audit.request_id }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('变更', 'Changes')">
            <pre class="json-detail">{{ JSON.stringify(audit.changes, null, 2) }}</pre>
          </el-descriptions-item>
        </template>
        <el-descriptions-item
          v-if="job?.result && Object.keys(job.result).length"
          :label="t('任务结果', 'Job result')"
        >
          <pre class="json-detail">{{ JSON.stringify(job.result, null, 2) }}</pre>
        </el-descriptions-item>
      </el-descriptions>
    </el-collapse-item>
  </el-collapse>
</template>
