<script setup lang="ts">
import { ref, watch } from 'vue'
import type { ResourceRecord, Module } from '../../api/types'
import { state, t } from '../../stores/app'
import { formatValue, dateTime } from '../../api/forms'
import { transitionLabel } from '../../api/presentation'
import StateBadge from '../StateBadge.vue'
import ResourceOperations from './ResourceOperations.vue'
import TicketSupport from './TicketSupport.vue'
const props = defineProps<{
  record: ResourceRecord | null
  descriptor: Module | undefined
  busy: boolean
  error?: string
  conflict?: boolean
}>()
const emit = defineEmits<{
  close: []
  transition: [ResourceRecord, string, string?]
  operate: [ResourceRecord, string]
  updated: [ResourceRecord]
  reload: []
}>()
const resolving = ref(false)
const resolution = ref('')
function transition(status: string) {
  if (!props.record || props.busy) return
  if (status === 'resolved') {
    resolution.value = ''
    resolving.value = true
  } else emit('transition', props.record, status)
}
function resolve() {
  if (!props.record || !resolution.value.trim() || props.busy || props.conflict) return
  emit('transition', props.record, 'resolved', resolution.value)
}
watch(
  () => props.record?.id,
  () => {
    resolving.value = false
    resolution.value = ''
  },
)
watch(
  () => props.record?.version,
  (version, old) => {
    if (version !== old && props.record?.data.status === 'resolved') {
      resolving.value = false
      resolution.value = ''
    }
  },
)
</script>
<template>
  <el-drawer
    :model-value="!!record"
    :title="t('记录详情', 'Record details')"
    size="min(640px, 100%)"
    @close="emit('close')"
  >
    <template v-if="record">
      <h3 v-if="descriptor?.key === 'tickets'" class="ticket-title">
        {{ record.data.title ?? t('客户工单', 'Customer ticket') }}
      </h3>
      <el-alert v-if="error && !resolving" type="error" :title="error" :closable="false" />
      <el-alert
        v-if="conflict && !resolving"
        type="warning"
        :closable="false"
        :title="
          t(
            '工单已被更新，请载入最新版本后再操作。',
            'This ticket changed. Reload the latest version before continuing.',
          )
        "
      >
        <el-button :loading="busy" @click="emit('reload')">
          {{ t('载入最新版本', 'Reload latest') }}
        </el-button>
      </el-alert>
      <el-descriptions :column="1" border>
        <el-descriptions-item
          v-for="field in descriptor?.fields.filter((item) =>
            Object.hasOwn(record!.data, item.key),
          )"
          :key="field.key"
          :label="state.locale === 'zh' ? field.label : field.label_en"
        >
          <StateBadge
            v-if="['status', 'priority'].includes(field.key)"
            :value="record.data[field.key]"
          />
          <span v-else>{{ formatValue(record.data[field.key]) }}</span>
        </el-descriptions-item>
      </el-descriptions>
      <div
        v-if="descriptor?.key === 'tickets' && record.capabilities?.transitions.length"
        class="ticket-transitions"
      >
        <p>{{ t('工单流转', 'Ticket workflow') }}</p>
        <el-button
          v-for="status in record.capabilities?.transitions ?? []"
          :key="status"
          :loading="busy"
          :disabled="conflict"
          @click="transition(status)"
        >
          {{ transitionLabel(status) }}
        </el-button>
      </div>
      <TicketSupport
        v-if="descriptor?.key === 'tickets'"
        :key="record.id"
        :record="record"
        :descriptor="descriptor"
        :busy="busy"
        @updated="emit('updated', $event)"
        @close="emit('close')"
      />
      <ResourceOperations
        :record="record"
        :descriptor="descriptor"
        :busy="busy"
        @operate="emit('operate', record, $event)"
      />
      <el-collapse class="record-metadata">
        <el-collapse-item :title="t('记录信息与标识', 'Record metadata')">
          <el-descriptions :column="1">
            <el-descriptions-item :label="t('版本', 'Version')">
              {{ record.version }}
            </el-descriptions-item>
            <el-descriptions-item :label="t('负责人', 'Owner')">
              {{ record.owner_id }}
            </el-descriptions-item>
            <el-descriptions-item :label="t('部门 ID', 'Department ID')">
              {{ record.department_id ?? '—' }}
            </el-descriptions-item>
            <el-descriptions-item :label="t('创建于', 'Created')">
              {{ dateTime(record.created_at) }}
            </el-descriptions-item>
          </el-descriptions>
        </el-collapse-item>
      </el-collapse>
      <p class="hint">
        {{
          t(
            '敏感字段可能已被遮罩，未授权字段不返回。',
            'Sensitive fields may be masked. Fields outside your grants are omitted.',
          )
        }}
      </p>
      <el-dialog
        v-model="resolving"
        class="ticket-resolution"
        :title="t('解决工单', 'Resolve ticket')"
        width="min(520px, calc(100% - 32px))"
        :close-on-click-modal="!busy"
        destroy-on-close
      >
        <el-alert v-if="error" type="error" :title="error" :closable="false" />
        <el-alert
          v-if="conflict"
          type="warning"
          :closable="false"
          :title="
            t(
              '工单已被更新。总结已保留，请载入最新版本。',
              'This ticket changed. Your summary is preserved; reload the latest version.',
            )
          "
        >
          <el-button :loading="busy" @click="emit('reload')">
            {{ t('载入最新版本', 'Reload latest') }}
          </el-button>
        </el-alert>
        <el-form label-position="top" :disabled="busy" @submit.prevent="resolve">
          <el-form-item :label="t('解决总结', 'Resolution summary')" required>
            <el-input
              v-model="resolution"
              type="textarea"
              :rows="4"
              :aria-label="t('解决总结', 'Resolution summary')"
            />
          </el-form-item>
          <p class="hint">
            {{
              t(
                '说明问题如何解决，每次标记解决都需要新的总结。',
                'Describe how the issue was resolved. Each resolution needs a new summary.',
              )
            }}
          </p>
          <div class="dialog-actions">
            <el-button @click="resolving = false">{{ t('取消', 'Cancel') }}</el-button>
            <el-button
              native-type="submit"
              type="primary"
              :loading="busy"
              :disabled="!resolution.trim() || conflict"
            >
              {{ t('解决工单', 'Resolve ticket') }}
            </el-button>
          </div>
        </el-form>
      </el-dialog>
    </template>
  </el-drawer>
</template>
