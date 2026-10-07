<script setup lang="ts">
import type { Department, Role } from '../../api/types'
import { t } from '../../stores/app'
import AssignmentFields from './AssignmentFields.vue'
export interface UserDraft {
  username: string
  display_name: string
  email: string
  password: string
  department_id: string
  position: string
  role_ids: string[]
  status: string
}
defineProps<{
  form: UserDraft
  creating: boolean
  allowRoles: boolean
  departments: Department[]
  roles: Role[]
  busy: boolean
}>()
const emit = defineEmits<{ save: []; cancel: [] }>()
</script>
<template>
  <el-form label-position="top" :disabled="busy" @submit.prevent="emit('save')">
    <template v-if="creating">
      <div class="field-grid">
        <el-form-item :label="t('用户名', 'Username')" required>
          <el-input
            v-model="form.username"
            required
            pattern="[a-z][a-z0-9_.-]{2,39}"
            maxlength="40"
            autocomplete="off"
          />
          <small class="hint">
            {{
              t('3–40位，以小写字母开头。', '3–40 characters, starting with a lowercase letter.')
            }}
          </small>
        </el-form-item>
        <el-form-item :label="t('姓名', 'Display name')" required>
          <el-input v-model="form.display_name" required maxlength="100" />
        </el-form-item>
      </div>
      <el-form-item :label="t('邮箱', 'Email')" required>
        <el-input v-model="form.email" type="email" required />
      </el-form-item>
      <el-form-item :label="t('初始密码', 'Initial password')" required>
        <el-input
          v-model="form.password"
          type="password"
          required
          minlength="12"
          show-password
          autocomplete="new-password"
        />
        <small class="hint">
          {{
            t(
              '至少12位；用户首次登录必须修改。请通过安全渠道告知。',
              'At least 12 characters. The user must change it at first sign-in; share it securely.',
            )
          }}
        </small>
      </el-form-item>
    </template>
    <AssignmentFields
      :department="form.department_id"
      :role-ids="form.role_ids"
      :departments="departments"
      :roles="roles"
      :allow-roles="allowRoles"
      @department="form.department_id = $event"
      @roles="form.role_ids = $event"
    />
    <el-form-item :label="t('岗位', 'Position')">
      <el-input v-model="form.position" maxlength="100" />
    </el-form-item>
    <el-form-item v-if="!creating" :label="t('状态', 'Status')">
      <el-select v-model="form.status">
        <el-option value="active" :label="t('启用', 'Active')" />
        <el-option value="suspended" :label="t('暂停', 'Suspended')" />
      </el-select>
    </el-form-item>
    <p class="hint">
      {{
        t(
          '暂停前可先转移用户负责的记录，组织须保留至少一名所有者。',
          'Transfer owned records before suspension. A workspace must retain an active owner.',
        )
      }}
    </p>
    <div class="dialog-actions">
      <el-button @click="emit('cancel')">{{ t('取消', 'Cancel') }}</el-button>
      <el-button type="primary" native-type="submit" :loading="busy">
        {{ t('保存', 'Save') }}
      </el-button>
    </div>
  </el-form>
</template>
