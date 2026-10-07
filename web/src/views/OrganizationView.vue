<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api, allPages } from '../api/client'
import type { Department, Member, Role, Grant, Module } from '../api/types'
import { t, allowed, state, loadMe } from '../stores/app'
import { dateTime } from '../api/forms'
import { departmentForest } from '../api/organization'
import { useCollection } from '../composables/collection'
import { useRequest, saved, confirmDelete } from '../composables/request'
import PageHeader from '../components/PageHeader.vue'
import PageState from '../components/PageState.vue'
import ActionButton from '../components/ActionButton.vue'
import RoleEditor from '../components/RoleEditor.vue'
interface Invitation {
  id: string
  email: string
  department_id?: string
  role_ids: string[]
  status: string
  expires_at: string
  created_at: string
}
const route = useRoute()
const resource = computed(() => route.path.slice(1))
const departmentPage = computed(() => resource.value === 'departments')
const invitationPage = computed(() => resource.value === 'invitations')
const list = useCollection<Department | Member | Invitation>(
  () => `/${resource.value}`,
  () => allowed(resource.value, 'read'),
)
const { run, busy, error } = useRequest()
const departments = ref<Department[]>([])
const roles = ref<Role[]>([])
const modules = ref<Module[]>([])
const members = ref<Member[]>([])
const modal = ref(false)
const current = ref<Department | Member | null>(null)
const form = reactive({
  username: '',
  display_name: '',
  password: '',
  name: '',
  parent_id: '',
  email: '',
  department_id: '',
  position: '',
  role_ids: [] as string[],
  status: 'active',
})
const transferOpen = ref(false)
const transferFrom = ref<Member | null>(null)
const transferTo = ref('')
const effective = ref<Grant[] | null>(null)
const newUser = computed(() => !departmentPage.value && !invitationPage.value && !current.value)
const title = computed(() =>
  departmentPage.value
    ? t('部门与岗位', 'Organization')
    : invitationPage.value
      ? t('组织邀请', 'Invitations')
      : t('用户管理', 'Users'),
)
const tree = computed(() => departmentForest(departments.value, list.query.value))
async function refresh() {
  await run(async () => {
    await list.load()
    await dependencies()
  })
}
async function dependencies() {
  await Promise.all([
    allowed('departments', 'read')
      ? allPages<Department>('/departments').then((result) => {
          departments.value = result
        })
      : Promise.resolve(),
    allowed('roles', 'read')
      ? allPages<Role>('/roles').then((result) => {
          roles.value = result
        })
      : Promise.resolve(),
  ])
}
onMounted(() => run(dependencies))
function departmentName(id: string | null) {
  return departments.value.find((item) => item.id === id)?.name ?? id ?? '—'
}
function roleNames(ids: string[]) {
  return ids.map((id) => roles.value.find((item) => item.id === id)?.name ?? id).join(', ')
}
async function open(record?: Department | Member) {
  await run(async () => {
    await dependencies()
    current.value = record ?? null
    Object.assign(form, {
      username: '',
      display_name: '',
      password: '',
      name: '',
      parent_id: '',
      email: '',
      department_id: '',
      position: '',
      role_ids: [],
      status: 'active',
    })
    if (record) Object.assign(form, record)
    form.parent_id ||= ''
    form.department_id ||= ''
    modal.value = true
  })
}
async function save() {
  await run(async () => {
    let body: Record<string, unknown>
    if (departmentPage.value) body = { name: form.name, parent_id: form.parent_id || null }
    else if (invitationPage.value)
      body = {
        email: form.email,
        department_id: form.department_id || null,
        role_ids: form.role_ids,
      }
    else {
      body = { department_id: form.department_id || null, position: form.position }
      if (current.value)
        Object.assign(body, {
          status: form.status,
          ...(allowed('members', 'assign') ? { role_ids: form.role_ids } : {}),
        })
      else
        Object.assign(body, {
          username: form.username,
          email: form.email,
          display_name: form.display_name,
          password: form.password,
          role_ids: form.role_ids,
        })
    }
    await api(`/${resource.value}${current.value ? `/${current.value.id}` : ''}`, {
      method: current.value ? 'PATCH' : 'POST',
      body,
    })
    modal.value = false
    form.password = ''
    saved()
    await list.load()
    await dependencies()
    await loadMe()
  })
}
async function remove(record: Department) {
  if (await confirmDelete(record.name))
    await run(async () => {
      await api(`/departments/${record.id}`, { method: 'DELETE' })
      saved()
      await list.load()
      await dependencies()
    })
}
async function inspect(record: Member) {
  await run(async () => {
    const [permissionResult, registry] = await Promise.all([
      api<Grant[]>(`/members/${record.id}/permissions`),
      api<{ modules: Module[] }>('/permissions'),
    ])
    effective.value = permissionResult.data
    modules.value = registry.data.modules
  })
}
async function startTransfer(record: Member) {
  await run(async () => {
    members.value = await allPages<Member>('/members')
    transferFrom.value = record
    transferTo.value = ''
    transferOpen.value = true
  })
}
async function transfer() {
  await run(async () => {
    await api(`/members/${transferFrom.value!.id}/transfer`, {
      method: 'POST',
      body: { owner_id: transferTo.value },
    })
    transferOpen.value = false
    saved()
    await list.load()
  })
}
</script>
<template>
  <PageHeader
    :title="title"
    :description="
      departmentPage
        ? t(
            '维护部门层级，成员岗位在成员资料中设置。',
            'Manage the department tree. Positions are set in member profiles.',
          )
        : invitationPage
          ? t('邀请通过邮件送达，不在页面展示邀请凭据。', 'Invitations are delivered by email.')
          : t(
              '管理成员的部门、岗位、角色与账号状态。',
              'Manage department, position, roles and membership status.',
            )
    "
  >
    <ActionButton
      v-if="
        departmentPage ||
        invitationPage ||
        (state.me?.platform_admin && allowed('members', 'assign'))
      "
      :resource="resource"
      action="create"
      :disabled="busy"
      type="primary"
      @click="open()"
    >
      +
      {{
        departmentPage
          ? t('创建部门', 'Create department')
          : invitationPage
            ? t('邀请成员', 'Invite member')
            : t('添加用户', 'Add user')
      }}
    </ActionButton>
    <router-link
      v-if="!departmentPage && !invitationPage && allowed('invitations', 'create')"
      to="/invitations"
    >
      <el-button type="primary">+ {{ t('邀请成员', 'Invite member') }}</el-button>
    </router-link>
  </PageHeader>
  <PageState
    :denied="!allowed(resource, 'read')"
    :busy="list.busy.value"
    :error="list.error.value"
    @retry="list.load"
  />
  <div v-if="allowed(resource, 'read')" class="panel">
    <form class="table-toolbar" @submit.prevent="list.search">
      <el-input v-model="list.query.value" :placeholder="t('搜索', 'Search')" clearable />
      <el-button native-type="submit">{{ t('搜索', 'Search') }}</el-button>
      <el-button @click="refresh">{{ t('刷新', 'Refresh') }}</el-button>
      <span class="count-label">{{ list.total.value }} {{ t('条记录', 'records') }}</span>
    </form>
    <el-tree
      v-if="departmentPage"
      :data="tree"
      node-key="id"
      default-expand-all
      :expand-on-click-node="false"
      :props="{ label: 'name', children: 'children' }"
      :empty-text="t('暂无部门', 'No departments')"
    >
      <template #default="{ data }">
        <div class="department-row">
          <strong>{{ data.name }}</strong>
          <div>
            <ActionButton
              resource="departments"
              action="update"
              link
              type="primary"
              @click="open(data)"
            >
              {{ t('编辑', 'Edit') }}
            </ActionButton>
            <ActionButton
              resource="departments"
              action="delete"
              link
              type="danger"
              @click="remove(data)"
            >
              {{ t('删除', 'Delete') }}
            </ActionButton>
          </div>
        </div>
      </template>
    </el-tree>
    <el-table
      v-else-if="invitationPage"
      :data="list.items.value"
      :empty-text="t('暂无邀请', 'No invitations')"
    >
      <el-table-column prop="email" :label="t('邮箱', 'Email')" min-width="240" />
      <el-table-column :label="t('部门', 'Department')" min-width="160">
        <template #default="{ row }">{{ departmentName(row.department_id) }}</template>
      </el-table-column>
      <el-table-column :label="t('角色', 'Roles')" min-width="180">
        <template #default="{ row }">{{ roleNames(row.role_ids ?? []) }}</template>
      </el-table-column>
      <el-table-column prop="status" :label="t('状态', 'Status')" width="110" />
      <el-table-column :label="t('到期时间', 'Expires')" min-width="180">
        <template #default="{ row }">{{ dateTime(row.expires_at) }}</template>
      </el-table-column>
    </el-table>
    <el-table v-else :data="list.items.value" :empty-text="t('暂无成员', 'No members')">
      <el-table-column :label="t('成员', 'Member')" min-width="230">
        <template #default="{ row }">
          <div class="person-cell">
            <span class="avatar">{{ row.display_name?.charAt(0) }}</span>
            <div>
              <strong>{{ row.display_name }}</strong>
              <small>{{ row.username ? row.username + ' · ' : '' }}{{ row.email }}</small>
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
      <el-table-column :label="t('角色', 'Roles')" min-width="180">
        <template #default="{ row }">{{ roleNames(row.role_ids ?? []) }}</template>
      </el-table-column>
      <el-table-column :label="t('状态', 'Status')" width="110">
        <template #default="{ row }">
          <el-tag :type="row.status === 'active' ? 'success' : 'info'">{{ row.status }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('操作', 'Actions')" width="230">
        <template #default="{ row }">
          <ActionButton resource="members" action="update" link type="primary" @click="open(row)">
            {{ t('编辑', 'Edit') }}
          </ActionButton>
          <el-button link @click="inspect(row)">{{ t('权限', 'Access') }}</el-button>
          <ActionButton resource="members" action="assign" link @click="startTransfer(row)">
            {{ t('转移记录', 'Transfer') }}
          </ActionButton>
        </template>
      </el-table-column>
    </el-table>
    <div v-if="!departmentPage" class="pagination">
      <el-pagination
        v-model:current-page="list.page.value"
        :total="list.total.value"
        :page-size="list.pageSize.value"
        layout="prev,pager,next"
        @change="list.load"
      />
    </div>
  </div>
  <el-dialog
    v-model="modal"
    :title="
      current
        ? t('编辑', 'Edit')
        : departmentPage
          ? t('创建部门', 'Create department')
          : invitationPage
            ? t('邀请成员', 'Invite member')
            : t('添加用户', 'Add user')
    "
    width="560px"
    destroy-on-close
  >
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <el-form @submit.prevent="save" label-position="top" :disabled="busy">
      <template v-if="departmentPage">
        <el-form-item :label="t('部门名称', 'Department name')" required>
          <el-input v-model="form.name" required maxlength="100" />
        </el-form-item>
        <el-form-item :label="t('上级部门', 'Parent department')">
          <el-select v-model="form.parent_id" clearable>
            <el-option
              v-for="department in departments.filter((item) => item.id !== current?.id)"
              :key="department.id"
              :label="department.name"
              :value="department.id"
            />
          </el-select>
        </el-form-item>
        <p class="hint">
          {{
            t(
              '不能形成循环。已有成员、子部门或记录引用的部门不可删除。',
              'Cycles are rejected. Departments referenced by members, child departments or records cannot be deleted.',
            )
          }}
        </p>
      </template>
      <template v-else>
        <template v-if="newUser">
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
                t(
                  '3–40位，以小写字母开头，可使用数字、下划线、点和连字符。',
                  '3–40 characters, starting with a lowercase letter; numbers, underscore, dot and hyphen are allowed.',
                )
              }}
            </small>
          </el-form-item>
          <el-form-item :label="t('姓名', 'Display name')" required>
            <el-input v-model="form.display_name" required maxlength="100" autocomplete="off" />
          </el-form-item>
          <el-form-item :label="t('初始密码', 'Initial password')" required>
            <el-input
              v-model="form.password"
              type="password"
              show-password
              required
              minlength="12"
              autocomplete="new-password"
            />
            <small class="hint">
              {{
                t(
                  '至少12位，请通过安全渠道告知用户。',
                  'At least 12 characters. Share it with the user through a secure channel.',
                )
              }}
            </small>
          </el-form-item>
        </template>
        <el-form-item v-if="invitationPage || newUser" :label="t('邮箱', 'Email')" required>
          <el-input v-model="form.email" type="email" required />
        </el-form-item>
        <el-form-item :label="t('所属部门', 'Department')">
          <el-select v-model="form.department_id" clearable>
            <el-option
              v-for="department in departments"
              :key="department.id"
              :label="department.name"
              :value="department.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item v-if="!invitationPage" :label="t('岗位', 'Position')">
          <el-input v-model="form.position" maxlength="100" />
        </el-form-item>
        <el-form-item
          v-if="invitationPage || allowed('members', 'assign')"
          :label="t('角色', 'Roles')"
        >
          <el-select v-model="form.role_ids" multiple>
            <el-option v-for="role in roles" :key="role.id" :value="role.id" :label="role.name" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="!invitationPage && !newUser" :label="t('状态', 'Status')">
          <el-select v-model="form.status">
            <el-option value="active" :label="t('启用', 'Active')" />
            <el-option value="suspended" :label="t('暂停', 'Suspended')" />
          </el-select>
        </el-form-item>
        <p v-if="!invitationPage" class="hint">
          {{
            t(
              '暂停成员前可先转移其记录。不能移除组织的最后一名所有者。',
              'Transfer owned records before suspension. The final workspace owner must remain active.',
            )
          }}
        </p>
      </template>
      <div class="dialog-actions">
        <el-button @click="modal = false">{{ t('取消', 'Cancel') }}</el-button>
        <el-button type="primary" native-type="submit" :loading="busy">
          {{ invitationPage ? t('发送邀请', 'Send invitation') : t('保存', 'Save') }}
        </el-button>
      </div>
    </el-form>
  </el-dialog>
  <el-dialog
    v-model="transferOpen"
    :title="t('转移成员记录', 'Transfer owned records')"
    width="500px"
  >
    <p>{{ transferFrom?.display_name }} → {{ t('选择新的负责人', 'Choose a new owner') }}</p>
    <el-alert
      type="warning"
      :title="
        t(
          '记录将转移给所选成员，请确认接收方。',
          'Owned records will move to the selected member. Verify the recipient.',
        )
      "
      :closable="false"
    />
    <el-select v-model="transferTo" filterable class="full-width">
      <el-option
        v-for="member in members.filter(
          (item) => item.user_id !== transferFrom?.user_id && item.status === 'active',
        )"
        :key="member.id"
        :value="member.user_id"
        :label="member.display_name + ' · ' + member.email"
      />
    </el-select>
    <template #footer>
      <el-button type="primary" :loading="busy" :disabled="!transferTo" @click="transfer">
        {{ t('确认转移', 'Transfer records') }}
      </el-button>
    </template>
  </el-dialog>
  <el-drawer
    :model-value="effective !== null"
    :title="t('成员有效授权', 'Effective member access')"
    size="min(850px,100%)"
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
