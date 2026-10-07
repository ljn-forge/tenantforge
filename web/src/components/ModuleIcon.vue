<script setup lang="ts">
import { computed } from 'vue'
import {
  House,
  Tickets,
  Collection,
  Setting,
  Bell,
  User,
  OfficeBuilding,
  Key,
  Message,
  FolderOpened,
  Timer,
  DocumentChecked,
  Grid,
  Lock,
  Menu,
} from '@element-plus/icons-vue'
const props = defineProps<{ name: string }>()
const icon = computed(
  () =>
    ({
      overview: House,
      tickets: Tickets,
      dictionaries: Collection,
      settings: Setting,
      notifications: Bell,
      members: User,
      departments: OfficeBuilding,
      roles: Key,
      invitations: Message,
      files: FolderOpened,
      jobs: Timer,
      audit: DocumentChecked,
      tenants: Grid,
      account: Lock,
      menu: Menu,
    })[props.name] ?? Grid,
)
</script>
<template>
  <el-icon aria-hidden="true"><component :is="icon" /></el-icon>
</template>
