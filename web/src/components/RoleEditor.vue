<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Grant, Module, Department, Scope, Condition } from '../api/types'
import { createGrant, validateGrant, parseConditionValue } from '../api/permissions'
import { t, state } from '../stores/app'
const props = defineProps<{
  modelValue: Grant[]
  modules: Module[]
  departments: Department[]
  disabled?: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [Grant[]] }>()
const resource = ref('')
const action = ref('read')
const selected = computed(() => props.modules.find((module) => module.key === resource.value))
const scopes: { value: Scope; zh: string; en: string }[] = [
  { value: 'self', zh: '本人拥有的记录', en: 'Own records' },
  { value: 'department', zh: '所属部门', en: 'Own department' },
  { value: 'department_tree', zh: '所属部门及子部门', en: 'Department and descendants' },
  { value: 'selected_departments', zh: '指定部门', en: 'Selected departments' },
  { value: 'tenant', zh: '整个组织', en: 'Entire workspace' },
]
function descriptor(grant: Grant) {
  return props.modules.find((module) => module.key === grant.resource)
}
function title(module?: Module) {
  return module ? (state.locale === 'zh' ? module.label : module.label_en) : ''
}
function add() {
  if (selected.value?.actions.includes(action.value))
    emit('update:modelValue', [...props.modelValue, createGrant(resource.value, action.value)])
}
function remove(index: number) {
  emit(
    'update:modelValue',
    props.modelValue.filter((_, item) => item !== index),
  )
}
function addCondition(grant: Grant) {
  ;(grant.conditions ??= []).push({
    field: descriptor(grant)?.fields[0]?.key ?? '',
    op: 'eq',
    value: '',
  })
}
function updateCondition(grant: Grant, condition: Condition, value: string) {
  const field = descriptor(grant)?.fields.find((item) => item.key === condition.field)
  condition.value = parseConditionValue(field, value, condition.op)
}
function conditionText(condition: Condition) {
  return Array.isArray(condition.value) ? condition.value.join(', ') : String(condition.value ?? '')
}
</script>
<template>
  <div class="role-editor">
    <div class="toolbar" v-if="!disabled">
      <el-select
        v-model="resource"
        :placeholder="t('选择资源', 'Choose resource')"
        :aria-label="t('资源', 'Resource')"
        @change="action = selected?.actions[0] ?? 'read'"
      >
        <el-option
          v-for="module in modules"
          :key="module.key"
          :label="title(module)"
          :value="module.key"
        />
      </el-select>
      <el-select v-model="action" :aria-label="t('操作', 'Operation')">
        <el-option
          v-for="item in selected?.actions ?? []"
          :key="item"
          :label="item"
          :value="item"
        />
      </el-select>
      <el-button :disabled="!selected" @click="add">{{ t('添加授权', 'Add grant') }}</el-button>
    </div>
    <p class="hint">
      {{
        t(
          '每项操作单独设置数据范围。条件在单项授权内同时满足，多项授权合并生效。敏感字段需显式读取并单独解除遮罩。',
          'Each operation has its own scope. Conditions within a grant use AND; matching grants use OR. Sensitive fields need explicit read access and separate unmask access.',
        )
      }}
    </p>
    <el-empty
      v-if="!modelValue.length"
      :description="t('尚未授权，默认拒绝访问', 'No grants. Access is denied by default. ')"
      :image-size="65"
    />
    <article v-for="(grant, index) in modelValue" :key="index" data-grant class="grant-card">
      <div class="grant-heading">
        <strong>{{ title(descriptor(grant)) || grant.resource }}</strong>
        <el-tag effect="plain">{{ grant.action }}</el-tag>
        <el-button v-if="!disabled" text type="danger" @click="remove(index)">
          {{ t('移除', 'Remove') }}
        </el-button>
      </div>
      <el-form @submit.prevent label-position="top" :disabled="disabled">
        <el-form-item :label="t('数据范围', 'Record scope')">
          <el-select v-model="grant.scope" data-scope>
            <el-option
              v-for="scope in scopes"
              :key="scope.value"
              :value="scope.value"
              :label="t(scope.zh, scope.en)"
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
        <div v-if="descriptor(grant)?.fields.length" class="field-grid">
          <el-form-item :label="t('可读取字段', 'Readable fields')">
            <el-select v-model="grant.read_fields" multiple>
              <el-option value="*" :label="t('所有非敏感字段', 'All non-sensitive fields')" />
              <el-option
                v-for="field in descriptor(grant)?.fields"
                :key="field.key"
                :value="field.key"
                :label="
                  (state.locale === 'zh' ? field.label : field.label_en) +
                  (field.sensitive ? ' ◈' : '')
                "
              />
            </el-select>
          </el-form-item>
          <el-form-item :label="t('可写入字段', 'Writable fields')">
            <el-select v-model="grant.write_fields" multiple>
              <el-option
                value="*"
                :label="t('所有可变非敏感字段', 'All mutable non-sensitive fields')"
              />
              <el-option
                v-for="field in descriptor(grant)?.fields.filter((item) => !item.readonly)"
                :key="field.key"
                :value="field.key"
                :label="state.locale === 'zh' ? field.label : field.label_en"
              />
            </el-select>
          </el-form-item>
          <el-form-item :label="t('解除敏感字段遮罩', 'Unmask sensitive fields')">
            <el-select v-model="grant.unmask_fields" multiple>
              <el-option
                v-for="field in descriptor(grant)?.fields.filter((item) => item.sensitive)"
                :key="field.key"
                :value="field.key"
                :label="state.locale === 'zh' ? field.label : field.label_en"
                :disabled="!grant.read_fields?.includes(field.key)"
              />
            </el-select>
          </el-form-item>
        </div>
        <div
          v-for="(condition, conditionIndex) in grant.conditions"
          :key="conditionIndex"
          class="condition-row"
        >
          <el-select v-model="condition.field" :aria-label="t('条件字段', 'Condition field')">
            <el-option
              v-for="field in descriptor(grant)?.fields"
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
          v-if="descriptor(grant)?.fields.length && !disabled"
          text
          @click="addCondition(grant)"
        >
          + {{ t('添加条件', 'Add condition') }}
        </el-button>
      </el-form>
      <ul
        v-if="validateGrant(grant, descriptor(grant)).length"
        class="validation-errors"
        role="alert"
      >
        <li v-for="error in validateGrant(grant, descriptor(grant))" :key="error">{{ error }}</li>
      </ul>
    </article>
  </div>
</template>
