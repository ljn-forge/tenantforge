<script setup lang="ts">
import type { Department, Role } from '../../api/types'
import { t } from '../../stores/app'
defineProps<{
  department: string
  roleIds: string[]
  departments: Department[]
  roles: Role[]
  allowRoles: boolean
}>()
const emit = defineEmits<{ department: [string]; roles: [string[]] }>()
</script>
<template>
  <el-form-item :label="t('所属部门', 'Department')">
    <el-select
      :model-value="department"
      clearable
      filterable
      @update:model-value="emit('department', $event ?? '')"
    >
      <el-option v-for="item in departments" :key="item.id" :value="item.id" :label="item.name" />
    </el-select>
  </el-form-item>
  <el-form-item v-if="allowRoles" :label="t('角色', 'Roles')">
    <el-select
      :model-value="roleIds"
      multiple
      filterable
      @update:model-value="emit('roles', $event)"
    >
      <el-option v-for="item in roles" :key="item.id" :value="item.id" :label="item.name" />
    </el-select>
  </el-form-item>
</template>
