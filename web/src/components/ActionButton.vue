<script setup lang="ts">
import { allowed } from '../stores/app'
defineProps<{ resource: string; action: string }>()
</script>
<template>
  <el-button v-if="allowed(resource, action)" v-bind="$attrs"><slot /></el-button>
</template>
