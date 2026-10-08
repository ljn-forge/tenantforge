<script setup lang="ts">
import { computed } from 'vue'
import type { JobType } from '../../api/types'
import { state, t } from '../../stores/app'
import FieldInput from '../FieldInput.vue'
const props = defineProps<{
  open: boolean
  types: JobType[]
  selectedType: string
  values: Record<string, unknown>
  errors: Record<string, string>
  error: string
  submitting: boolean
}>()
const emit = defineEmits<{ close: []; choose: [string]; change: [string, unknown]; submit: [] }>()
const selected = computed(() => props.types.find((item) => item.type === props.selectedType))
</script>
<template>
  <el-dialog
    :model-value="open"
    :title="t('发起任务', 'Start task')"
    width="min(620px,100%)"
    :close-on-click-modal="!submitting"
    :close-on-press-escape="!submitting"
    :show-close="!submitting"
    @close="emit('close')"
  >
    <el-alert v-if="error" type="error" :title="error" :closable="false" />
    <el-form label-position="top" @submit.prevent="emit('submit')">
      <el-form-item :label="t('任务类型', 'Task type')">
        <el-select
          :model-value="selectedType"
          :disabled="submitting"
          @update:model-value="emit('choose', $event)"
        >
          <el-option
            v-for="type in types"
            :key="type.type"
            :value="type.type"
            :label="state.locale === 'zh' ? type.label : type.label_en"
          />
        </el-select>
      </el-form-item>
      <p v-if="selected" class="hint">
        {{
          state.locale === 'zh'
            ? selected.description
            : (selected.description_en ?? selected.description)
        }}
      </p>
      <div class="task-inputs">
        <FieldInput
          v-for="field in selected?.input_fields"
          :key="field.key"
          :field="field"
          :model-value="values[field.key]"
          :error="errors[field.key]"
          :disabled="submitting"
          @update:model-value="emit('change', field.key, $event)"
        />
      </div>
      <p v-if="selected && !selected.input_fields.length" class="hint">
        {{ t('此任务无需额外配置，可直接发起。', 'This task needs no additional configuration.') }}
      </p>
      <div class="dialog-actions">
        <el-button :disabled="submitting" @click="emit('close')">
          {{ t('取消', 'Cancel') }}
        </el-button>
        <el-button type="primary" native-type="submit" :loading="submitting" :disabled="!selected">
          {{ t('发起任务', 'Start task') }}
        </el-button>
      </div>
    </el-form>
  </el-dialog>
</template>
