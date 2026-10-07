<script setup lang="ts">
import type { Module, ResourceRecord } from '../../api/types'
import { recordAllows } from '../../api/capabilities'
import { operationLabel } from '../../api/presentation'
defineProps<{ record: ResourceRecord; descriptor: Module | undefined; busy: boolean }>()
const emit = defineEmits<{ operate: [string] }>()
</script>
<template>
  <div v-if="descriptor?.operations?.length" class="record-operations">
    <template v-for="operation in descriptor.operations" :key="operation.key">
      <el-button
        v-if="recordAllows(record, operation.key)"
        :loading="busy"
        type="primary"
        @click="emit('operate', operation.key)"
      >
        {{ operationLabel(operation.key, descriptor) }}
      </el-button>
    </template>
  </div>
</template>
