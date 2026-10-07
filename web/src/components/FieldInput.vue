<script setup lang="ts">
import type { Field } from '../api/types'
import { state } from '../stores/app'
defineProps<{ field: Field; modelValue: unknown; error?: string; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [unknown] }>()
function update(value: unknown) {
  emit('update:modelValue', value)
}
</script>
<template>
  <el-form-item
    :label="state.locale === 'zh' ? field.label : field.label_en"
    :required="field.required"
    :error="error"
  >
    <el-input-number
      v-if="field.type === 'number'"
      :model-value="typeof modelValue === 'number' ? modelValue : undefined"
      :disabled="disabled"
      @update:model-value="update"
    />
    <el-switch
      v-else-if="field.type === 'boolean'"
      :model-value="!!modelValue"
      :disabled="disabled"
      @update:model-value="update"
    />
    <el-select
      v-else-if="field.type === 'enum'"
      :model-value="modelValue"
      :disabled="disabled"
      @update:model-value="update"
    >
      <el-option v-for="option in field.options" :key="option" :value="option" :label="option" />
    </el-select>
    <el-date-picker
      v-else-if="field.type === 'date'"
      :model-value="modelValue"
      type="date"
      value-format="YYYY-MM-DD"
      :disabled="disabled"
      @update:model-value="update"
    />
    <el-input
      v-else
      :model-value="String(modelValue ?? '')"
      :type="field.type === 'text' ? 'textarea' : 'text'"
      :rows="4"
      :maxlength="field.type === 'text' ? 10000 : 1000"
      :disabled="disabled"
      @update:model-value="update"
    />
  </el-form-item>
</template>
