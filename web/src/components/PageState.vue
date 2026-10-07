<script setup lang="ts">
import { t } from '../stores/app'
defineProps<{ denied?: boolean; error?: string; busy?: boolean; empty?: boolean }>()
defineEmits<{ retry: [] }>()
</script>
<template>
  <el-result
    v-if="denied"
    icon="warning"
    :title="t('无访问权限', 'Access denied')"
    :sub-title="
      t('请联系组织管理员确认授权范围。', 'Contact your workspace administrator to review access.')
    "
  />
  <el-alert
    v-else-if="error"
    type="error"
    :title="error"
    :closable="false"
    show-icon
    class="error-banner"
  >
    <el-button text @click="$emit('retry')">{{ t('重试', 'Retry') }}</el-button>
  </el-alert>
  <el-skeleton v-else-if="busy" :rows="5" animated />
  <el-empty
    v-else-if="empty"
    :description="t('暂无可见记录', 'No visible records')"
    :image-size="75"
  />
</template>
