<script setup lang="ts">
import { statusLabel, statusTone } from '../api/presentation'
defineProps<{ value: unknown }>()
</script>
<template>
  <el-tag
    v-if="value !== undefined && value !== null"
    :type="statusTone(String(value))"
    effect="light"
    size="small"
  >
    {{ statusLabel(String(value)) }}
  </el-tag>
  <span v-else class="muted">—</span>
</template>
