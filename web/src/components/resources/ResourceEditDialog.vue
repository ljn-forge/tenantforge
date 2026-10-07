<script setup lang="ts">
import type { ResourceRecord, Field, Member, Department } from '../../api/types'
import { t } from '../../stores/app'
import FieldInput from '../FieldInput.vue'
defineProps<{
  modelValue: boolean
  current: ResourceRecord | null
  fields: Field[]
  values: Record<string, unknown>
  errors: Record<string, string>
  error: string
  busy: boolean
  conflict: boolean
  canAssign: boolean
  owner: string
  department: string
  members: Member[]
  departments: Department[]
}>()
const emit = defineEmits<{
  'update:modelValue': [boolean]
  save: []
  reload: []
  field: [string, unknown]
  owner: [string]
  department: [string]
}>()
</script>
<template>
  <el-dialog
    :model-value="modelValue"
    @update:model-value="emit('update:modelValue', $event)"
    :title="current ? t('编辑记录', 'Edit record') : t('新建记录', 'New record')"
    width="640px"
    :close-on-click-modal="!busy"
    destroy-on-close
  >
    <el-alert v-if="error" type="error" :title="error" :closable="false" />
    <el-alert
      v-if="conflict"
      type="warning"
      :title="
        t(
          '记录已被其他人更新。重新载入后再修改。',
          'This record has changed. Reload the latest version before editing.',
        )
      "
      :closable="false"
    >
      <el-button @click="emit('reload')">{{ t('载入最新版本', 'Reload latest') }}</el-button>
    </el-alert>
    <el-form @submit.prevent="emit('save')" label-position="top" :disabled="busy">
      <FieldInput
        v-for="field in fields"
        :key="field.key"
        :field="field"
        :model-value="values[field.key]"
        @update:model-value="emit('field', field.key, $event)"
        :error="errors[field.key]"
      />
      <el-empty
        v-if="!fields.length"
        :description="t('当前授权没有可写字段', 'No writable fields in your current grants')"
        :image-size="60"
      />
      <template v-if="canAssign">
        <div class="field-grid">
          <el-form-item :label="t('负责人', 'Owner')">
            <el-select
              v-if="members.length"
              :model-value="owner"
              @update:model-value="emit('owner', $event ?? '')"
              clearable
              filterable
            >
              <el-option
                v-for="member in members.filter((item) => item.status === 'active')"
                :key="member.id"
                :value="member.user_id"
                :label="member.display_name + ' · ' + member.email"
              />
            </el-select>
            <el-input
              v-else
              :model-value="owner"
              @update:model-value="emit('owner', $event ?? '')"
              :placeholder="t('用户 ID（留空为本人）', 'User ID (leave blank for yourself)')"
            />
          </el-form-item>
          <el-form-item :label="t('归属部门', 'Department')">
            <el-select
              v-if="departments.length"
              :model-value="department"
              @update:model-value="emit('department', $event ?? '')"
              clearable
            >
              <el-option
                v-for="item in departments"
                :key="item.id"
                :value="item.id"
                :label="item.name"
              />
            </el-select>
            <el-input
              v-else
              :model-value="department"
              @update:model-value="emit('department', $event ?? '')"
              :placeholder="t('部门 ID（可留空）', 'Department ID (optional)')"
            />
          </el-form-item>
        </div>
      </template>
      <p class="hint">
        {{
          t(
            '仅显示授权可写字段。保存时组织、数据范围与字段权限将再次校验。',
            'Only permitted writable fields are shown. Your workspace, scope and field access are checked again on save.',
          )
        }}
      </p>
      <p v-if="current" class="hint">
        {{
          t(
            '只保存修改过的字段。敏感字段留空可保留原值，输入内容将替换原值。',
            'Only changed fields are saved. Leave sensitive fields untouched to preserve their values; entering a value replaces them.',
          )
        }}
      </p>
      <div class="dialog-actions">
        <el-button @click="emit('update:modelValue', false)">{{ t('取消', 'Cancel') }}</el-button>
        <el-button
          native-type="submit"
          type="primary"
          :loading="busy"
          :disabled="conflict || (!fields.length && !canAssign)"
        >
          {{ t('保存', 'Save') }}
        </el-button>
      </div>
    </el-form>
  </el-dialog>
</template>
