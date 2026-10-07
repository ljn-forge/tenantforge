<script setup lang="ts">
import type { ResourceRecord, Field } from '../../api/types'
import { state, t } from '../../stores/app'
import { formatValue, dateTime } from '../../api/forms'
import ActionButton from '../ActionButton.vue'
import StateBadge from '../StateBadge.vue'
const props = defineProps<{ records: ResourceRecord[]; fields: Field[]; resource: string }>()
function editable(record: ResourceRecord) {
  return (
    record.capabilities?.actions.includes('update') &&
    (record.capabilities.write_fields.some(
      (field) => props.resource !== 'tickets' || field !== 'status',
    ) ||
      record.capabilities.actions.includes('assign'))
  )
}
const emit = defineEmits<{
  view: [ResourceRecord]
  edit: [ResourceRecord]
  remove: [ResourceRecord]
}>()
</script>
<template>
  <el-table
    :data="records"
    stripe
    row-key="id"
    :empty-text="t('暂无可见记录', 'No visible records')"
  >
    <el-table-column
      v-for="field in fields.slice(0, 6)"
      :key="field.key"
      :label="state.locale === 'zh' ? field.label : field.label_en"
      width="160"
    >
      <template #default="{ row }">
        <StateBadge
          v-if="['status', 'priority'].includes(field.key)"
          :value="row.data[field.key]"
        />
        <span v-else :class="{ 'sensitive-value': field.sensitive }">
          {{ formatValue(row.data[field.key]) }}
        </span>
      </template>
    </el-table-column>
    <el-table-column :label="t('最后更新', 'Updated')" width="175">
      <template #default="{ row }">{{ dateTime(row.updated_at) }}</template>
    </el-table-column>
    <el-table-column :label="t('操作', 'Actions')" width="190" fixed="right">
      <template #default="{ row }">
        <el-button link @click="emit('view', row)">{{ t('详情', 'View') }}</el-button>
        <ActionButton
          v-if="editable(row)"
          :resource="resource"
          :record="row"
          action="update"
          link
          type="primary"
          @click="emit('edit', row)"
        >
          {{ t('编辑', 'Edit') }}
        </ActionButton>
        <ActionButton
          :resource="resource"
          :record="row"
          action="delete"
          link
          type="danger"
          @click="emit('remove', row)"
        >
          {{ t('删除', 'Delete') }}
        </ActionButton>
      </template>
    </el-table-column>
  </el-table>
</template>
