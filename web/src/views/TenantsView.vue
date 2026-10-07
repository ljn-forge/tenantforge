<script setup lang="ts">
import { reactive, ref } from 'vue'
import { api } from '../api/client'
import type { Tenant } from '../api/types'
import { state, t, loadMe } from '../stores/app'
import { dateTime, formatValue } from '../api/forms'
import { useCollection } from '../composables/collection'
import { useRequest, saved } from '../composables/request'
import PageHeader from '../components/PageHeader.vue'
import PageState from '../components/PageState.vue'
const list = useCollection<Tenant>(
  () => '/platform/tenants',
  () => !!state.me?.platform_admin,
)
const { run, busy, error } = useRequest()
const modal = ref(false)
const current = ref<Tenant | null>(null)
const form = reactive({
  name: '',
  slug: '',
  owner_email: '',
  status: 'active',
  quotas: {} as Record<string, number | undefined>,
})
const quotaFields = [
  { key: 'records', zh: '业务记录总数', en: 'Resource records' },
  { key: 'members', zh: '成员数', en: 'Members' },
  { key: 'storage_bytes', zh: '文件存储（字节）', en: 'File storage (bytes)' },
  { key: 'jobs', zh: '后台任务数', en: 'Background jobs' },
]
function open(tenant?: Tenant) {
  current.value = tenant ?? null
  Object.assign(form, {
    name: tenant?.name ?? '',
    slug: tenant?.slug ?? '',
    owner_email: '',
    status: tenant?.status ?? 'active',
    quotas: { ...(tenant?.quotas ?? {}) },
  })
  modal.value = true
}
async function save() {
  const quotas = Object.fromEntries(
    Object.entries(form.quotas).filter(([, value]) => typeof value === 'number'),
  )
  await run(async () => {
    await api(`/platform/tenants${current.value ? `/${current.value.id}` : ''}`, {
      method: current.value ? 'PATCH' : 'POST',
      body: current.value
        ? { name: form.name, status: form.status, quotas }
        : { name: form.name, slug: form.slug, owner_email: form.owner_email, quotas },
    })
    modal.value = false
    saved()
    await list.load()
    await loadMe()
  })
}
</script>
<template>
  <PageHeader
    :title="t('平台租户', 'Platform tenants')"
    :description="
      t(
        '创建组织、管理状态与配额。组织业务数据由成员权限独立控制。',
        'Provision workspaces, manage status and quotas. Workspace data access is governed by membership grants.',
      )
    "
  >
    <el-button v-if="state.me?.platform_admin" type="primary" @click="open()">
      + {{ t('创建租户', 'Provision tenant') }}
    </el-button>
  </PageHeader>
  <PageState
    :denied="!state.me?.platform_admin"
    :error="list.error.value"
    :busy="list.busy.value"
    @retry="list.load"
  />
  <div v-if="state.me?.platform_admin" class="panel">
    <form class="table-toolbar" @submit.prevent="list.search">
      <el-input
        v-model="list.query.value"
        :placeholder="t('搜索租户', 'Search tenants')"
        clearable
      />
      <el-button native-type="submit">{{ t('搜索', 'Search') }}</el-button>
      <el-button @click="list.load">{{ t('刷新', 'Refresh') }}</el-button>
    </form>
    <el-table :data="list.items.value" :empty-text="t('暂无租户', 'No tenants')">
      <el-table-column prop="name" :label="t('组织', 'Workspace')" min-width="190" />
      <el-table-column prop="slug" :label="t('标识', 'Slug')" min-width="160" />
      <el-table-column :label="t('状态', 'Status')" width="120">
        <template #default="{ row }">
          <el-tag :type="row.status === 'active' ? 'success' : 'warning'">{{ row.status }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('配额', 'Quotas')" min-width="220">
        <template #default="{ row }">{{ formatValue(row.quotas) }}</template>
      </el-table-column>
      <el-table-column :label="t('创建于', 'Created')" min-width="180">
        <template #default="{ row }">{{ dateTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column width="100">
        <template #default="{ row }">
          <el-button link type="primary" @click="open(row)">{{ t('管理', 'Manage') }}</el-button>
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
    :title="current ? t('管理租户', 'Manage tenant') : t('创建租户', 'Provision tenant')"
    width="560px"
  >
    <el-alert v-if="error" type="error" :title="error" :closable="false" />
    <el-form @submit.prevent="save" label-position="top">
      <el-form-item :label="t('组织名称', 'Workspace name')" required>
        <el-input v-model="form.name" required maxlength="100" />
      </el-form-item>
      <template v-if="!current">
        <el-form-item :label="t('唯一标识', 'Unique slug')" required>
          <el-input v-model="form.slug" required pattern="[a-z0-9-]+" maxlength="80" />
          <small class="hint">
            {{ t('小写英文字母、数字和连字符', 'Lowercase letters, numbers and hyphens') }}
          </small>
        </el-form-item>
        <el-form-item :label="t('所有者邮箱', 'Owner email')" required>
          <el-input v-model="form.owner_email" type="email" required />
        </el-form-item>
      </template>
      <el-form-item v-else :label="t('状态', 'Status')">
        <el-select v-model="form.status">
          <el-option value="active" :label="t('启用', 'Active')" />
          <el-option value="suspended" :label="t('暂停', 'Suspended')" />
        </el-select>
        <small class="hint">
          {{
            t(
              '暂停后，该组织的成员请求与后台任务将拒绝访问。',
              'Suspension blocks member requests and background jobs for this workspace.',
            )
          }}
        </small>
      </el-form-item>
      <div class="field-grid">
        <el-form-item v-for="field in quotaFields" :key="field.key" :label="t(field.zh, field.en)">
          <el-input-number
            v-model="form.quotas[field.key]"
            :min="0"
            :precision="0"
            :max="Number.MAX_SAFE_INTEGER"
            :placeholder="t('服务默认值', 'Service default')"
          />
        </el-form-item>
      </div>
      <p class="hint">
        {{
          t(
            '留空使用服务默认限制。零表示该项配额为零。',
            'Leave blank to use service defaults. Zero sets that quota to zero.',
          )
        }}
      </p>
      <div class="dialog-actions">
        <el-button @click="modal = false">{{ t('取消', 'Cancel') }}</el-button>
        <el-button native-type="submit" type="primary" :loading="busy">
          {{ t('保存', 'Save') }}
        </el-button>
      </div>
    </el-form>
  </el-dialog>
</template>
