<script setup lang="ts">
import type { Grant, Module, Department } from '../../api/types'
import { state, t } from '../../stores/app'
import { grantSummary } from '../../api/presentation'
const props = defineProps<{ grant: Grant; descriptor: Module; departments: Department[] }>()
function fields(kind: 'read_fields' | 'write_fields' | 'unmask_fields') {
  const values = props.grant[kind] ?? []
  if (kind === 'write_fields' && ['read', 'export'].includes(props.grant.action))
    return t('不允许写入', 'No writes')
  if (!values.length) return t('未授权', 'No access')
  return values
    .map((key) =>
      key === '*'
        ? kind === 'write_fields'
          ? t('全部可变非敏感字段', 'All mutable non-sensitive fields')
          : t('全部非敏感字段', 'All non-sensitive fields')
        : ((state.locale === 'zh'
            ? props.descriptor.fields.find((field) => field.key === key)?.label
            : props.descriptor.fields.find((field) => field.key === key)?.label_en) ?? key),
    )
    .join('、')
}
function departmentNames() {
  return (props.grant.department_ids ?? [])
    .map(
      (id) =>
        props.departments.find((item) => item.id === id)?.name ??
        t('未列出的部门', 'Unlisted department'),
    )
    .join('、')
}
function conditionLabel(field: string) {
  const descriptor = props.descriptor.fields.find((item) => item.key === field)
  return descriptor ? (state.locale === 'zh' ? descriptor.label : descriptor.label_en) : field
}
</script>
<template>
  <div class="grant-preview">
    <strong>{{ grantSummary(grant, descriptor) }}</strong>
    <small v-if="grant.scope === 'selected_departments'">{{ departmentNames() }}</small>
    <dl>
      <dt>{{ t('读取', 'Read') }}</dt>
      <dd>{{ fields('read_fields') }}</dd>
      <dt>{{ t('写入', 'Write') }}</dt>
      <dd>{{ fields('write_fields') }}</dd>
      <dt>{{ t('明文', 'Unmask') }}</dt>
      <dd>{{ fields('unmask_fields') }}</dd>
    </dl>
    <p v-for="(condition, index) in grant.conditions" :key="index" class="hint">
      {{ conditionLabel(condition.field) }}
      {{ condition.op === 'eq' ? t('等于', 'equals') : t('属于', 'in') }}
      {{ Array.isArray(condition.value) ? condition.value.join(', ') : String(condition.value) }}
    </p>
  </div>
</template>
