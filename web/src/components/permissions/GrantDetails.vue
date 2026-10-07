<script setup lang="ts">
import type { Grant, Module, Department, Scope, Condition } from '../../api/types'
import { validateGrant, parseConditionValue } from '../../api/permissions'
import {
  operationLabel,
  moduleLabel,
  scopeLabel,
  validationLabel,
  grantSummary,
} from '../../api/presentation'
import { state, t } from '../../stores/app'
import FieldAccessMatrix from './FieldAccessMatrix.vue'
const props = defineProps<{
  grant: Grant
  descriptor: Module
  departments: Department[]
  disabled?: boolean
}>()
const emit = defineEmits<{ remove: [] }>()
const scopes: Scope[] = ['self', 'department', 'department_tree', 'selected_departments', 'tenant']
function addCondition(grant: Grant) {
  ;(grant.conditions ??= []).push({
    field: props.descriptor.fields.find((field) => !field.sensitive)?.key ?? '',
    op: 'eq',
    value: '',
  })
}
function updateCondition(grant: Grant, condition: Condition, value: string) {
  condition.value = parseConditionValue(
    props.descriptor.fields.find((item) => item.key === condition.field),
    value,
    condition.op,
  )
}
function conditionText(condition: Condition) {
  return Array.isArray(condition.value) ? condition.value.join(', ') : String(condition.value ?? '')
}
</script>
<template>
  <article data-grant class="grant-card">
    <div class="grant-heading">
      <strong>
        {{ moduleLabel(descriptor) + ' · ' + operationLabel(grant.action, descriptor) }}
      </strong>
      <el-tag effect="plain">{{ grant.action }}</el-tag>
      <el-button v-if="!disabled" text type="danger" @click="emit('remove')">
        {{ t('移除', 'Remove') }}
      </el-button>
    </div>
    <p class="grant-summary">{{ grantSummary(grant, descriptor) }}</p>
    <el-form @submit.prevent label-position="top" :disabled="disabled">
      <el-form-item :label="t('数据范围', 'Record scope')">
        <el-select v-model="grant.scope" data-scope>
          <el-option
            v-for="scope in scopes"
            :key="scope"
            :value="scope"
            :label="scopeLabel(scope)"
          />
        </el-select>
      </el-form-item>
      <el-form-item
        v-if="grant.scope === 'selected_departments'"
        :label="t('指定部门', 'Selected departments')"
      >
        <el-select v-model="grant.department_ids" multiple>
          <el-option
            v-for="department in departments"
            :key="department.id"
            :value="department.id"
            :label="department.name"
          />
        </el-select>
      </el-form-item>
      <FieldAccessMatrix
        v-if="descriptor.fields.length"
        :grant="grant"
        :descriptor="descriptor"
        :disabled="disabled"
      />
      <div
        v-for="(condition, conditionIndex) in grant.conditions"
        :key="conditionIndex"
        class="condition-row"
      >
        <el-select v-model="condition.field" :aria-label="t('条件字段', 'Condition field')">
          <el-option
            v-for="field in descriptor?.fields.filter((field) => !field.sensitive)"
            :key="field.key"
            :label="state.locale === 'zh' ? field.label : field.label_en"
            :value="field.key"
          />
        </el-select>
        <el-select
          v-model="condition.op"
          :aria-label="t('条件运算符', 'Condition operator')"
          @change="condition.value = condition.op === 'in' ? [] : ''"
        >
          <el-option value="eq" :label="t('等于', 'Equals')" />
          <el-option value="in" :label="t('属于集合', 'In list')" />
        </el-select>
        <el-input
          :model-value="conditionText(condition)"
          :aria-label="t('条件值', 'Condition value')"
          :placeholder="
            condition.op === 'in'
              ? t('逗号分隔值', 'Comma-separated values')
              : t('值；布尔类型填 true/false', 'Value; booleans use true/false')
          "
          @update:model-value="(value: string) => updateCondition(grant, condition, value)"
        />
        <el-button
          v-if="!disabled"
          text
          type="danger"
          @click="grant.conditions?.splice(conditionIndex, 1)"
        >
          {{ t('移除', 'Remove') }}
        </el-button>
      </div>
      <el-button
        v-if="descriptor?.fields.some((field) => !field.sensitive) && !disabled"
        text
        @click="addCondition(grant)"
      >
        + {{ t('添加条件', 'Add condition') }}
      </el-button>
    </el-form>
    <ul v-if="validateGrant(grant, descriptor).length" class="validation-errors" role="alert">
      <li v-for="error in validateGrant(grant, descriptor)" :key="error">
        {{ validationLabel(error, descriptor) }}
      </li>
    </ul>
  </article>
</template>
