<script setup lang="ts">
import { computed } from 'vue'
import type { Field } from '../../api/types'
import { state, t } from '../../stores/app'
import { statusLabel } from '../../api/presentation'
const props = defineProps<{ fields: Field[]; values: Record<string, string> }>()
const emit = defineEmits<{ change: [string, string]; clear: [] }>()
const active = computed(() => props.fields.filter((field) => props.values[field.key]))
</script>
<template>
  <el-select
    v-for="field in fields"
    :key="field.key"
    :model-value="values[field.key] ?? ''"
    :aria-label="`${t('筛选', 'Filter')} ${state.locale === 'zh' ? field.label : field.label_en}`"
    :placeholder="state.locale === 'zh' ? field.label : field.label_en"
    clearable
    class="status-filter"
    @update:model-value="emit('change', field.key, $event ?? '')"
  >
    <el-option
      v-for="option in field.options"
      :key="option"
      :value="option"
      :label="statusLabel(option)"
    />
  </el-select>
  <div v-if="active.length" class="active-filters">
    <el-tag
      v-for="field in active"
      :key="field.key"
      closable
      @close="emit('change', field.key, '')"
    >
      {{ state.locale === 'zh' ? field.label : field.label_en }}:
      {{ statusLabel(values[field.key]!) }}
    </el-tag>
    <el-button text size="small" @click="emit('clear')">
      {{ t('清除筛选', 'Clear filters') }}
    </el-button>
  </div>
</template>
