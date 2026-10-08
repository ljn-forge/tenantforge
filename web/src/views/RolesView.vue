<script setup lang="ts">
import { onMounted, ref, computed, reactive } from 'vue'
import { api, allPages } from '../api/client'
import type { Role, Module, Department, Grant } from '../api/types'
import { validateGrant } from '../api/permissions'
import { supportRoleGrants, type SupportRoleKind } from '../api/supportRoles'
import { t, allowed, loadMe } from '../stores/app'
import { useCollection } from '../composables/collection'
import { useRequest, saved, confirmDelete } from '../composables/request'
import PageHeader from '../components/PageHeader.vue'
import PageState from '../components/PageState.vue'
import RoleEditor from '../components/RoleEditor.vue'
import ActionButton from '../components/ActionButton.vue'
const list = useCollection<Role>(
  () => '/roles',
  () => allowed('roles', 'read'),
)
const { run, busy, error } = useRequest()
const modules = ref<Module[]>([])
const departments = ref<Department[]>([])
const modal = ref(false)
const current = ref<Role | null>(null)
const readonly = ref(false)
const form = reactive({ name: '', description: '', grants: [] as Grant[] })
const supportTemplate = ref<SupportRoleKind | null>(null)
const templateReady = computed(() => supportRoleGrants('supervisor', modules.value).length > 0)
const copyOpen = ref(false)
const copyId = ref('')
const copyName = ref('')
const issues = computed(() =>
  form.grants.flatMap((grant) =>
    validateGrant(
      grant,
      modules.value.find((item) => item.key === grant.resource),
    ),
  ),
)
onMounted(async () => {
  await run(async () => {
    const result = await api<{ modules: Module[]; scopes: string[] }>('/permissions')
    modules.value = result.data.modules
    if (allowed('departments', 'read'))
      departments.value = await allPages<Department>('/departments')
  })
})
function open(role?: Role, view = false) {
  supportTemplate.value = null
  current.value = role ?? null
  readonly.value = view
  form.name = role?.name ?? ''
  form.description = role?.description ?? ''
  form.grants = role ? JSON.parse(JSON.stringify(role.grants)) : []
  modal.value = true
}
function openSupportTemplate(kind: SupportRoleKind) {
  const grants = supportRoleGrants(kind, modules.value)
  if (!grants.length || !allowed('roles', 'create')) return
  open()
  supportTemplate.value = kind
  form.name =
    kind === 'agent' ? t('客服处理员', 'Support agent') : t('客服主管', 'Support supervisor')
  form.description =
    kind === 'agent'
      ? t('处理本人拥有的客服工单', 'Handle support tickets you own')
      : t('分配与处理所属部门的客服工单', 'Assign and handle support tickets in your department')
  form.grants = grants
}
async function save() {
  if (!form.name.trim() || issues.value.length) return
  await run(async () => {
    await api(`/roles${current.value ? `/${current.value.id}` : ''}`, {
      method: current.value ? 'PATCH' : 'POST',
      body: form,
    })
    modal.value = false
    saved()
    await list.load()
    await loadMe()
  })
}
async function remove(role: Role) {
  if (await confirmDelete(role.name))
    await run(async () => {
      await api(`/roles/${role.id}`, { method: 'DELETE' })
      saved()
      await list.load()
      await loadMe()
    })
}
function startCopy(role: Role) {
  copyId.value = role.id
  copyName.value = `${role.name} ${t('副本', 'copy')}`
  copyOpen.value = true
}
async function copy() {
  await run(async () => {
    await api(`/roles/${copyId.value}/copy`, { method: 'POST', body: { name: copyName.value } })
    copyOpen.value = false
    saved()
    await list.load()
  })
}
</script>
<template>
  <PageHeader
    :title="t('角色与授权', 'Roles & access')"
    :description="
      t(
        '通过角色定义操作、数据范围、字段访问与条件。',
        'Define operations, record scopes, field access and conditions through roles.',
      )
    "
  >
    <ActionButton
      v-if="templateReady"
      resource="roles"
      action="create"
      :disabled="busy"
      @click="openSupportTemplate('agent')"
    >
      {{ t('新建客服处理员角色', 'New support agent role') }}
    </ActionButton>
    <ActionButton
      v-if="templateReady"
      resource="roles"
      action="create"
      :disabled="busy"
      @click="openSupportTemplate('supervisor')"
    >
      {{ t('新建客服主管角色', 'New support supervisor role') }}
    </ActionButton>
    <ActionButton resource="roles" action="create" type="primary" :disabled="busy" @click="open()">
      <span aria-hidden="true">+</span>
      {{ t('创建角色', 'Create role') }}
    </ActionButton>
  </PageHeader>
  <PageState
    :denied="!allowed('roles', 'read')"
    :error="list.error.value"
    :busy="list.busy.value"
    @retry="list.load"
  />
  <div v-if="allowed('roles', 'read')" class="panel">
    <form class="table-toolbar" @submit.prevent="list.search">
      <el-input v-model="list.query.value" :placeholder="t('搜索角色', 'Search roles')" />
      <el-button native-type="submit">{{ t('搜索', 'Search') }}</el-button>
      <el-button @click="list.load">{{ t('刷新', 'Refresh') }}</el-button>
    </form>
    <el-table :data="list.items.value" :empty-text="t('暂无角色', 'No roles')">
      <el-table-column prop="name" :label="t('角色名称', 'Role')" min-width="180">
        <template #default="{ row }">
          <strong>{{ row.name }}</strong>
          <el-tag v-if="row.builtin" size="small" class="inline-tag">
            {{ t('内置', 'Built-in') }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="description" :label="t('说明', 'Description')" min-width="200" />
      <el-table-column :label="t('授权数', 'Grants')" width="100">
        <template #default="{ row }">{{ row.grants?.length ?? 0 }}</template>
      </el-table-column>
      <el-table-column :label="t('操作', 'Actions')" width="230">
        <template #default="{ row }">
          <el-button link @click="open(row, true)">{{ t('查看', 'View') }}</el-button>
          <ActionButton
            v-if="!row.builtin"
            resource="roles"
            :record="row"
            action="update"
            link
            type="primary"
            @click="open(row)"
          >
            {{ t('编辑', 'Edit') }}
          </ActionButton>
          <ActionButton resource="roles" action="create" link @click="startCopy(row)">
            {{ t('复制', 'Copy') }}
          </ActionButton>
          <ActionButton
            v-if="!row.builtin"
            resource="roles"
            :record="row"
            action="delete"
            link
            type="danger"
            @click="remove(row)"
          >
            {{ t('删除', 'Delete') }}
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
  </div>
  <el-dialog
    v-model="modal"
    :title="
      readonly
        ? t('角色详情', 'Role details')
        : current
          ? t('编辑角色', 'Edit role')
          : t('创建角色', 'Create role')
    "
    width="min(1184px, calc(100% - 32px))"
    :close-on-click-modal="!busy"
    destroy-on-close
  >
    <el-alert v-if="error" type="error" :title="error" :closable="false" />
    <div>
      <el-form @submit.prevent="save" label-position="top" :disabled="readonly || busy">
        <div class="field-grid">
          <el-form-item :label="t('名称', 'Name')" required>
            <el-input v-model="form.name" required maxlength="100" />
          </el-form-item>
          <el-form-item :label="t('说明', 'Description')">
            <el-input v-model="form.description" maxlength="1000" />
          </el-form-item>
        </div>
      </el-form>
      <el-alert
        v-if="supportTemplate && !current"
        class="support-template-preview"
        type="warning"
        :closable="false"
        :title="t('检查客服模板的敏感字段访问', 'Review sensitive access in this support template')"
      >
        <p>
          {{
            t(
              '此新角色草稿明确授予 customer_email（客户邮箱）与 internal_notes（内部备注）的读取和明文权限，以及创建/更新写入权限。回复操作仅写入 description / internal_notes；私密备注只有具备明确读取和明文授权的同事可见。',
              'This new role draft explicitly grants read and unmask access to customer_email and internal_notes, plus create/update writes. Reply writes are limited to description / internal_notes; private notes require explicit read and unmask access.',
            )
          }}
        </p>
        <p>
          {{
            supportTemplate === 'agent'
              ? t('工单权限范围为本人拥有的记录。', 'Ticket access is limited to own records.')
              : t(
                  '工单权限范围为所属部门，并包含所属部门用户与部门读取，用于分配。',
                  'Ticket access is limited to the own department, with department-scoped member and department reads for assignment.',
                )
          }}
        </p>
        <p>
          {{
            t(
              '请核对下方权限矩阵后手动保存。模板不会自动修改已有角色。',
              'Review the permission matrix below and save manually. Templates do not modify existing roles automatically.',
            )
          }}
        </p>
      </el-alert>
      <RoleEditor
        v-model="form.grants"
        :modules="modules"
        :departments="departments"
        :disabled="readonly || busy"
      />
      <p class="hint">
        {{
          t(
            '不能授予超出自身可委派权限的访问范围。服务端校验每条授权。',
            'You can delegate only authority you hold. Each grant is verified by the server.',
          )
        }}
      </p>
      <div class="dialog-actions">
        <el-button @click="modal = false">
          {{ readonly ? t('关闭', 'Close') : t('取消', 'Cancel') }}
        </el-button>
        <el-button
          v-if="!readonly"
          @click="save"
          type="primary"
          :disabled="!!issues.length || !form.name.trim()"
          :loading="busy"
        >
          {{ t('保存角色', 'Save role') }}
        </el-button>
      </div>
    </div>
  </el-dialog>
  <el-dialog v-model="copyOpen" :title="t('复制角色', 'Copy role')" width="420px">
    <el-form @submit.prevent="copy" label-position="top">
      <el-form-item :label="t('新角色名称', 'New role name')">
        <el-input v-model="copyName" required maxlength="100" />
      </el-form-item>
      <el-button native-type="submit" type="primary" :loading="busy">
        {{ t('复制', 'Copy') }}
      </el-button>
    </el-form>
  </el-dialog>
</template>
