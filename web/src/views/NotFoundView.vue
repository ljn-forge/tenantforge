<script setup lang="ts">
import { t } from '../stores/app'
</script>
<template>
  <el-result
    icon="warning"
    :title="t('页面不存在', 'Page not found')"
    :sub-title="t('此地址不存在或已被移除。', 'This page does not exist or was removed.')"
  >
    <template #extra>
      <router-link to="/">
        <el-button type="primary">{{ t('返回工作台', 'Back to overview') }}</el-button>
      </router-link>
    </template>
  </el-result>
</template>
