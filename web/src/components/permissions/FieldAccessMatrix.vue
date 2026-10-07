<script setup lang="ts">
import type { Grant, Module } from '../../api/types'
import { state, t } from '../../stores/app'
const props = defineProps<{ grant: Grant; descriptor: Module; disabled?: boolean }>()
type FieldKind = 'read_fields' | 'write_fields' | 'unmask_fields'
function includes(kind: FieldKind, key: string) {
  const field = props.descriptor.fields.find((item) => item.key === key)!
  return (
    props.grant[kind]?.includes(key) ||
    (!field.sensitive &&
      kind !== 'unmask_fields' &&
      props.grant[kind]?.includes('*') &&
      !(kind === 'write_fields' && field.readonly)) ||
    false
  )
}
function toggle(kind: FieldKind, key: string, checked: boolean) {
  let existing = props.grant[kind] ?? []
  if (existing.includes('*'))
    existing = props.descriptor.fields
      .filter((field) => !field.sensitive && (kind !== 'write_fields' || !field.readonly))
      .map((field) => field.key)
      .concat(existing.filter((item) => item !== '*'))
  props.grant[kind] = checked
    ? [...new Set([...existing, key])]
    : existing.filter((item) => item !== key)
  if (kind === 'read_fields' && !checked)
    props.grant.unmask_fields = props.grant.unmask_fields?.filter((item) => item !== key)
}
</script>
<template>
  <el-table
    :data="descriptor.fields"
    size="small"
    class="field-access-matrix"
    :empty-text="t('此资源无字段规则', 'No field rules for this resource')"
  >
    <el-table-column :label="t('字段', 'Field')" min-width="160">
      <template #default="{ row }">
        <span>{{ state.locale === 'zh' ? row.label : row.label_en }}</span>
        <el-tag v-if="row.sensitive" size="small" type="warning">
          {{ t('敏感', 'Sensitive') }}
        </el-tag>
        <small v-if="row.readonly" class="cell-secondary">{{ t('只读', 'Read-only') }}</small>
      </template>
    </el-table-column>
    <el-table-column
      v-for="kind in ['read_fields', 'write_fields', 'unmask_fields'] as const"
      :key="kind"
      :label="
        kind === 'read_fields'
          ? t('读取', 'Read')
          : kind === 'write_fields'
            ? t('写入', 'Write')
            : t('明文', 'Unmask')
      "
      width="100"
      align="center"
    >
      <template #default="{ row }">
        <el-checkbox
          :model-value="includes(kind, row.key)"
          :aria-label="`${state.locale === 'zh' ? row.label : row.label_en} · ${kind === 'read_fields' ? t('读取', 'Read') : kind === 'write_fields' ? t('写入', 'Write') : t('明文', 'Unmask')}`"
          :disabled="
            disabled ||
            (kind === 'write_fields' &&
              (row.readonly || ['read', 'export'].includes(grant.action))) ||
            (kind === 'unmask_fields' && (!row.sensitive || !grant.read_fields?.includes(row.key)))
          "
          @update:model-value="toggle(kind, row.key, $event === true)"
        />
      </template>
    </el-table-column>
  </el-table>
</template>
