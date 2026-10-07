<script setup lang="ts">
import type { AuthorizedRecord } from '../api/types'
import { recordAllows } from '../api/capabilities'
import { allowed } from '../stores/app'
defineProps<{ resource: string; action: string; record?: AuthorizedRecord }>()
</script>
<template>
  <el-button
    v-if="record ? recordAllows(record, action) : allowed(resource, action)"
    v-bind="$attrs"
  >
    <slot />
  </el-button>
</template>
