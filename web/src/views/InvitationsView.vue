<script setup lang="ts">
import { onMounted, ref, reactive } from 'vue'
import { api } from '../api/client'
import { t, allowed } from '../stores/app'
import { dateTime } from '../api/forms'
import { useOrganizationReferences } from '../composables/organizationReferences'
import { useCollection } from '../composables/collection'
import { useRequest, saved } from '../composables/request'
import PageHeader from '../components/PageHeader.vue'
import PageState from '../components/PageState.vue'
import ActionButton from '../components/ActionButton.vue'
import StateBadge from '../components/StateBadge.vue'
import AssignmentFields from '../components/organization/AssignmentFields.vue'
interface Invitation {
  id: string
  email: string
  department_id?: string
  role_ids: string[]
  status: string
  expires_at: string
  created_at: string
}
const list = useCollection<Invitation>(
  () => '/invitations',
  () => allowed('invitations', 'read'),
)
const references = useOrganizationReferences(),
  { departments, roles, departmentName, roleNames } = references
const { run, busy, error } = useRequest(),
  modal = ref(false)
const form = reactive({ email: '', department_id: '', role_ids: [] as string[] })
onMounted(() => run(references.load))
function open() {
  Object.assign(form, { email: '', department_id: '', role_ids: [] })
  modal.value = true
}
async function send() {
  await run(async () => {
    await api('/invitations', {
      method: 'POST',
      body: {
        email: form.email,
        department_id: form.department_id || null,
        role_ids: form.role_ids,
      },
    })
    modal.value = false
    saved()
    await list.load()
  })
}
</script>
<template>
  <PageHeader
    :title="t('组织邀请', 'Invitations')"
    :description="
      t(
        '通过邮件邀请已有或新账号加入组织，邀请凭据不会在这里展示。',
        'Invite existing or new accounts by email. Invitation credentials are never displayed here.',
      )
    "
  >
    <ActionButton
      resource="invitations"
      action="create"
      type="primary"
      :disabled="busy"
      @click="open"
    >
      <span aria-hidden="true">+</span>
      {{ t('邀请成员', 'Invite member') }}
    </ActionButton>
  </PageHeader>
  <PageState
    :denied="!allowed('invitations', 'read')"
    :busy="list.busy.value"
    :error="error || list.error.value"
    @retry="list.load"
  />
  <section v-if="allowed('invitations', 'read')" class="panel">
    <form class="table-toolbar" @submit.prevent="list.search">
      <el-input
        v-model="list.query.value"
        :placeholder="t('搜索邮箱', 'Search email')"
        :aria-label="t('搜索邮箱', 'Search email')"
      />
      <el-button native-type="submit">{{ t('搜索', 'Search') }}</el-button>
      <el-button @click="list.load">{{ t('刷新', 'Refresh') }}</el-button>
    </form>
    <el-table :data="list.items.value" :empty-text="t('暂无邀请', 'No invitations')">
      <el-table-column prop="email" :label="t('邮箱', 'Email')" min-width="240" />
      <el-table-column :label="t('部门', 'Department')" min-width="140">
        <template #default="{ row }">{{ departmentName(row.department_id) }}</template>
      </el-table-column>
      <el-table-column :label="t('角色', 'Roles')" min-width="180">
        <template #default="{ row }">{{ roleNames(row.role_ids ?? []) }}</template>
      </el-table-column>
      <el-table-column :label="t('状态', 'Status')" width="120">
        <template #default="{ row }"><StateBadge :value="row.status" /></template>
      </el-table-column>
      <el-table-column :label="t('到期时间', 'Expires')" min-width="180">
        <template #default="{ row }">{{ dateTime(row.expires_at) }}</template>
      </el-table-column>
    </el-table>
    <div class="pagination">
      <el-pagination
        v-model:current-page="list.page.value"
        :total="list.total.value"
        :page-size="list.pageSize.value"
        layout="prev,pager,next"
        @change="list.load"
      />
    </div>
  </section>
  <el-dialog v-model="modal" :title="t('邀请成员', 'Invite member')" width="520px" destroy-on-close>
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <el-form label-position="top" :disabled="busy" @submit.prevent="send">
      <el-form-item :label="t('邮箱', 'Email')" required>
        <el-input v-model="form.email" type="email" required />
      </el-form-item>
      <AssignmentFields
        :department="form.department_id"
        :role-ids="form.role_ids"
        :departments="departments"
        :roles="roles"
        allow-roles
        @department="form.department_id = $event"
        @roles="form.role_ids = $event"
      />
      <div class="dialog-actions">
        <el-button @click="modal = false">{{ t('取消', 'Cancel') }}</el-button>
        <el-button type="primary" native-type="submit" :loading="busy">
          {{ t('发送邀请', 'Send invitation') }}
        </el-button>
      </div>
    </el-form>
  </el-dialog>
</template>
