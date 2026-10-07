<script setup lang="ts">
import type { ResourceRecord, Module } from '../../api/types'
import { state, t } from '../../stores/app'
import { formatValue, dateTime } from '../../api/forms'
import { transitionLabel } from '../../api/presentation'
import StateBadge from '../StateBadge.vue'
import ResourceOperations from './ResourceOperations.vue'
defineProps<{ record: ResourceRecord | null; descriptor: Module | undefined; busy: boolean }>()
const emit = defineEmits<{
  close: []
  transition: [ResourceRecord, string]
  operate: [ResourceRecord, string]
}>()
</script>
<template>
  <el-drawer
    :model-value="!!record"
    :title="t('记录详情', 'Record details')"
    size="min(640px, 100%)"
    @close="emit('close')"
  >
    <template v-if="record">
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
          @click="emit('transition', record!, status)"
        >
          {{ transitionLabel(status) }}
        </el-button>
      </div>
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
    </template>
  </el-drawer>
</template>
