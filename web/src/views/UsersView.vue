<script setup lang="ts">
import { onMounted, ref, reactive } from 'vue'
import { api } from '../api/client'
import type { Member, Grant, Module } from '../api/types'
import { recordAllows } from '../api/capabilities'
import { state, t, allowed, loadMe } from '../stores/app'
import { useOrganizationReferences } from '../composables/organizationReferences'
import { useCollection } from '../composables/collection'
import { useRequest, saved } from '../composables/request'
import PageHeader from '../components/PageHeader.vue'
import PageState from '../components/PageState.vue'
import ActionButton from '../components/ActionButton.vue'
import StateBadge from '../components/StateBadge.vue'
import RoleEditor from '../components/RoleEditor.vue'
import UserForm, { type UserDraft } from '../components/organization/UserForm.vue'
import TransferDialog from '../components/organization/TransferDialog.vue'
const list = useCollection<Member>(
  () => '/members',
  () => allowed('members', 'read'),
)
const references = useOrganizationReferences()
const { departments, roles, departmentName, roleNames } = references
const { run, busy, error } = useRequest()
const current = ref<Member | null>(null),
  modal = ref(false),
  transferFrom = ref<Member | null>(null)
const effective = ref<Grant[] | null>(null),
  modules = ref<Module[]>([])
const form = reactive<UserDraft>({
  username: '',
  display_name: '',
  email: '',
  password: '',
  department_id: '',
  position: '',
  role_ids: [],
  status: 'active',
})
onMounted(() => run(references.load))
async function open(record?: Member) {
  await run(async () => {
    await references.load()
    current.value = record ?? null
    Object.assign(
      form,
      {
        username: '',
        display_name: '',
        email: '',
        password: '',
        department_id: '',
        position: '',
        role_ids: [],
        status: 'active',
      },
      record ?? {},
    )
    form.department_id ||= ''
    modal.value = true
  })
}
async function save() {
  await run(async () => {
    const body = current.value
      ? {
          department_id: form.department_id || null,
          position: form.position,
          status: form.status,
          ...(recordAllows(current.value, 'assign') ? { role_ids: form.role_ids } : {}),
        }
      : {
          username: form.username,
          email: form.email,
          display_name: form.display_name,
          password: form.password,
          department_id: form.department_id || null,
          position: form.position,
          role_ids: form.role_ids,
        }
    await api(`/members${current.value ? `/${current.value.id}` : ''}`, {
      method: current.value ? 'PATCH' : 'POST',
      body,
    })
    modal.value = false
    form.password = ''
    saved()
    await list.load()
    await loadMe()
  })
}
async function inspect(record: Member) {
  await run(async () => {
    const [grants, registry] = await Promise.all([
      api<Grant[]>(`/members/${record.id}/permissions`),
      api<{ modules: Module[] }>('/permissions'),
    ])
    effective.value = grants.data
    modules.value = registry.data.modules
  })
}
</script>
<template>
  <PageHeader
    :title="t('用户管理', 'Users')"
    :description="
      t(
        '管理团队账号、岗位、角色及当前访问状态。',
        'Manage team accounts, positions, roles and current access.',
      )
    "
  >
    <ActionButton
      v-if="state.me?.platform_admin && allowed('members', 'assign')"
      resource="members"
      action="create"
      type="primary"
      :disabled="busy"
      @click="open()"
    >
      <span aria-hidden="true">+</span>
      {{ t('添加用户', 'Add user') }}
    </ActionButton>
    <router-link v-if="allowed('invitations', 'create')" to="/invitations">
      <el-button>{{ t('邀请成员', 'Invite member') }}</el-button>
    </router-link>
  </PageHeader>
  <PageState
    :denied="!allowed('members', 'read')"
    :busy="list.busy.value"
    :error="error || list.error.value"
    @retry="list.load"
  />
  <section v-if="allowed('members', 'read')" class="panel">
    <form class="table-toolbar" @submit.prevent="list.search">
      <el-input
        v-model="list.query.value"
        :placeholder="t('搜索用户', 'Search users')"
        :aria-label="t('搜索用户', 'Search users')"
        clearable
      />
      <el-button native-type="submit">{{ t('搜索', 'Search') }}</el-button>
      <el-button @click="list.load">{{ t('刷新', 'Refresh') }}</el-button>
      <span class="count-label">{{ list.total.value }} {{ t('位用户', 'users') }}</span>
    </form>
    <el-table :data="list.items.value" row-key="id" :empty-text="t('暂无用户', 'No users')">
      <el-table-column :label="t('用户', 'User')" min-width="240">
        <template #default="{ row }">
          <div class="person-cell">
            <span class="avatar">{{ row.display_name.charAt(0) }}</span>
            <div>
              <strong>{{ row.display_name }}</strong>
              <small class="cell-secondary">{{ row.username }} · {{ row.email }}</small>
            </div>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="t('部门 / 岗位', 'Department / position')" min-width="180">
        <template #default="{ row }">
          {{ departmentName(row.department_id) }}
          <small class="cell-secondary">{{ row.position || '—' }}</small>
        </template>
      </el-table-column>
      <el-table-column :label="t('角色', 'Roles')" min-width="160">
        <template #default="{ row }">{{ roleNames(row.role_ids ?? []) }}</template>
      </el-table-column>
      <el-table-column :label="t('状态', 'Status')" width="108">
        <template #default="{ row }"><StateBadge :value="row.status" /></template>
      </el-table-column>
      <el-table-column :label="t('操作', 'Actions')" width="220">
        <template #default="{ row }">
          <ActionButton
            resource="members"
            :record="row"
            action="update"
            link
            type="primary"
            @click="open(row)"
          >
            {{ t('编辑', 'Edit') }}
          </ActionButton>
          <ActionButton
            v-if="allowed('roles', 'read')"
            resource="members"
            :record="row"
            action="read"
            link
            @click="inspect(row)"
          >
            {{ t('权限', 'Access') }}
          </ActionButton>
          <ActionButton
            resource="members"
            :record="row"
            action="assign"
            link
            @click="transferFrom = row"
          >
            {{ t('转移记录', 'Transfer') }}
          </ActionButton>
        </template>
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
  <el-dialog
    v-model="modal"
    :title="current ? t('编辑用户', 'Edit user') : t('添加用户', 'Add user')"
    width="600px"
    destroy-on-close
    @closed="form.password = ''"
  >
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <UserForm
      :form="form"
      :creating="!current"
      :allow-roles="current ? recordAllows(current, 'assign') : allowed('members', 'assign')"
      :departments="departments"
      :roles="roles"
      :busy="busy"
      @save="save"
      @cancel="modal = false"
    />
  </el-dialog>
  <TransferDialog :member="transferFrom" @close="transferFrom = null" @saved="list.load" />
  <el-drawer
    :model-value="effective !== null"
    :title="t('当前有效授权', 'Effective access')"
    size="min(1000px,100%)"
    @close="effective = null"
  >
    <RoleEditor
      v-if="effective"
      :model-value="effective"
      :modules="modules"
      :departments="departments"
      disabled
    />
  </el-drawer>
</template>
