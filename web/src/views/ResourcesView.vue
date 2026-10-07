<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, APIError, allPages } from '../api/client'
import type { ResourceRecord, Department, Member, Job } from '../api/types'
import {
  editableFields,
  buildResourceData,
  validateResourceData,
  formatValue,
  dateTime,
} from '../api/forms'
import { state, allowed, t } from '../stores/app'
import { useCollection } from '../composables/collection'
import { useRequest, saved, confirmDelete } from '../composables/request'
import PageHeader from '../components/PageHeader.vue'
import PageState from '../components/PageState.vue'
import FieldInput from '../components/FieldInput.vue'
import ActionButton from '../components/ActionButton.vue'
const route = useRoute()
const router = useRouter()
const key = computed(() => String(route.params.module))
const module = computed(() => state.me?.modules.find((item) => item.key === key.value))
const title = computed(() =>
  module.value ? (state.locale === 'zh' ? module.value.label : module.value.label_en) : key.value,
)
const list = useCollection<ResourceRecord>(
  () => `/resources/${key.value}`,
  () => allowed(key.value, 'read'),
)
const { run, busy, error } = useRequest()
const modal = ref(false)
const current = ref<ResourceRecord | null>(null)
const detail = ref<ResourceRecord | null>(null)
const values = reactive<Record<string, unknown>>({})
const dirtyFields = reactive(new Set<string>())
const errors = ref<Record<string, string>>({})
const conflict = ref(false)
const owner = ref('')
const department = ref('')
const members = ref<Member[]>([])
const departments = ref<Department[]>([])
const importInput = ref<HTMLInputElement>()
const exportOpen = ref(false)
const columns = ref<string[]>([])
const fields = computed(() =>
  module.value
    ? editableFields(
        module.value,
        state.me?.permissions ?? [],
        current.value ? 'update' : 'create',
      ).filter((field) => !(key.value === 'tickets' && field.key === 'status'))
    : [],
)
const statusWritable = computed(() =>
  module.value
    ? editableFields(module.value, state.me?.permissions ?? [], 'update').some(
        (field) => field.key === 'status',
      )
    : false,
)
const visibleFields = computed(
  () =>
    module.value?.fields.filter((field) =>
      list.items.value.some((item) => Object.hasOwn(item.data, field.key)),
    ) ?? [],
)
const exportFields = computed(
  () =>
    module.value?.fields.filter((field) =>
      state.me?.permissions.some(
        (grant) =>
          grant.resource === key.value &&
          grant.action === 'export' &&
          (grant.read_fields?.includes(field.key) ||
            (!field.sensitive && grant.read_fields?.includes('*'))),
      ),
    ) ?? [],
)
async function assignments() {
  await Promise.all([
    allowed('members', 'read')
      ? allPages<Member>('/members').then((result) => {
          members.value = result
        })
      : Promise.resolve(),
    allowed('departments', 'read')
      ? allPages<Department>('/departments').then((result) => {
          departments.value = result
        })
      : Promise.resolve(),
  ])
}
async function edit(record?: ResourceRecord) {
  await run(async () => {
    current.value = record
      ? (await api<ResourceRecord>(`/resources/${key.value}/${record.id}`)).data
      : null
    Object.keys(values).forEach((item) => delete values[item])
    dirtyFields.clear()
    Object.assign(values, current.value?.data ?? {})
    fields.value.forEach((field) => {
      if (current.value && field.sensitive) values[field.key] = ''
      else if (values[field.key] === undefined)
        values[field.key] =
          field.type === 'boolean'
            ? false
            : field.type === 'enum'
              ? field.options?.[0]
              : field.type === 'number'
                ? undefined
                : ''
    })
    owner.value = current.value?.owner_id ?? ''
    department.value = current.value?.department_id ?? ''
    errors.value = {}
    conflict.value = false
    if (allowed(key.value, 'assign')) await assignments()
    modal.value = true
  })
}
function updateValue(field: string, value: unknown) {
  values[field] = value
  dirtyFields.add(field)
}
async function view(record: ResourceRecord) {
  await run(async () => {
    detail.value = (await api<ResourceRecord>(`/resources/${key.value}/${record.id}`)).data
  })
}
async function save() {
  const submittedFields = current.value
    ? fields.value.filter((field) => dirtyFields.has(field.key))
    : fields.value
  errors.value = validateResourceData(submittedFields, values)
  if (Object.keys(errors.value).length) return
  await run(async () => {
    const body: Record<string, unknown> = { data: buildResourceData(submittedFields, values) }
    if (current.value) body.version = current.value.version
    if (allowed(key.value, 'assign')) {
      if (owner.value && owner.value !== current.value?.owner_id) body.owner_id = owner.value
      if (department.value !== (current.value?.department_id ?? ''))
        body.department_id = department.value || null
    }
    try {
      await api(`/resources/${key.value}${current.value ? `/${current.value.id}` : ''}`, {
        method: current.value ? 'PATCH' : 'POST',
        body,
      })
      modal.value = false
      saved()
      await list.load()
    } catch (failure) {
      if (failure instanceof APIError) {
        if (failure.status === 409) conflict.value = true
        if (failure.fields) errors.value = failure.fields
      }
      throw failure
    }
  })
}
async function remove(record: ResourceRecord) {
  if (await confirmDelete(String(record.data.title ?? record.data.name ?? record.id)))
    await run(async () => {
      await api(`/resources/${key.value}/${record.id}?version=${record.version}`, {
        method: 'DELETE',
      })
      saved()
      await list.load()
    })
}
const edges: Record<string, string[]> = {
  open: ['in_progress'],
  in_progress: ['resolved'],
  resolved: ['closed', 'in_progress'],
  closed: ['open'],
}
async function transition(record: ResourceRecord, status: string) {
  await run(async () => {
    await api(`/resources/tickets/${record.id}/transition`, {
      method: 'POST',
      body: { version: record.version, status },
    })
    saved()
    await list.load()
    if (detail.value?.id === record.id) {
      const refreshed = (await api<ResourceRecord>(`/resources/tickets/${record.id}`)).data
      if (detail.value?.id === record.id) detail.value = refreshed
    }
  })
}
async function importCSV(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  await run(async () => {
    if (file.size > 1024 * 1024)
      throw new Error(t('CSV 文件不得超过 1 MiB', 'CSV must be no larger than 1 MiB'))
    const body = new FormData()
    body.append('file', file)
    const result = await api<{ inserted: number }>(`/resources/${key.value}/import`, {
      method: 'POST',
      body,
    })
    saved()
    await list.load()
    detail.value = null
    importCount.value = result.data.inserted
  })
  input.value = ''
}
const importCount = ref<number | null>(null)
function openExport() {
  columns.value = exportFields.value.map((field) => field.key)
  exportOpen.value = true
}
async function exportCSV() {
  await run(async () => {
    await api<Job>(`/resources/${key.value}/export`, {
      method: 'POST',
      body: { columns: columns.value },
    })
    exportOpen.value = false
    await router.push('/jobs')
  })
}
</script>
<template>
  <PageHeader
    :title="title"
    :description="module?.description"
    :eyebrow="t('业务管理', 'WORKSPACE')"
  >
    <ActionButton :resource="key" action="import" :disabled="busy" @click="importInput?.click()">
      {{ t('导入 CSV', 'Import CSV') }}
    </ActionButton>
    <ActionButton :resource="key" action="export" :disabled="busy" @click="openExport">
      {{ t('导出', 'Export') }}
    </ActionButton>
    <ActionButton :resource="key" action="create" type="primary" :disabled="busy" @click="edit()">
      + {{ t('新建记录', 'New record') }}
    </ActionButton>
    <input ref="importInput" hidden type="file" accept=".csv,text/csv" @change="importCSV" />
  </PageHeader>
  <el-alert
    v-if="importCount !== null"
    type="success"
    :title="t(`已导入 ${importCount} 条记录`, `Imported ${importCount} records`)"
    show-icon
    class="error-banner"
  />
  <PageState
    :denied="!allowed(key, 'read') || !module"
    :error="list.error.value"
    :busy="list.busy.value"
    @retry="list.load"
  />
  <template v-if="allowed(key, 'read') && module">
    <div class="panel">
      <form class="table-toolbar" @submit.prevent="list.search">
        <el-input
          v-model="list.query.value"
          :placeholder="t('搜索记录', 'Search records')"
          clearable
          :aria-label="t('搜索记录', 'Search records')"
        />
        <el-button native-type="submit" :loading="list.busy.value">
          {{ t('搜索', 'Search') }}
        </el-button>
        <el-button @click="list.load">{{ t('刷新', 'Refresh') }}</el-button>
        <span class="count-label">{{ list.total.value }} {{ t('条记录', 'records') }}</span>
      </form>
      <el-table
        :data="list.items.value"
        stripe
        row-key="id"
        :empty-text="t('暂无可见记录', 'No visible records')"
      >
        <el-table-column
          v-for="field in visibleFields.slice(0, 6)"
          :key="field.key"
          :label="state.locale === 'zh' ? field.label : field.label_en"
          min-width="160"
        >
          <template #default="{ row }">
            <span :class="{ 'sensitive-value': field.sensitive }">
              {{ formatValue(row.data[field.key]) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column :label="t('最后更新', 'Updated')" min-width="175">
          <template #default="{ row }">{{ dateTime(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column :label="t('操作', 'Actions')" width="190" fixed="right">
          <template #default="{ row }">
            <el-button link @click="view(row)">{{ t('详情', 'View') }}</el-button>
            <ActionButton :resource="key" action="update" link type="primary" @click="edit(row)">
              {{ t('编辑', 'Edit') }}
            </ActionButton>
            <ActionButton :resource="key" action="delete" link type="danger" @click="remove(row)">
              {{ t('删除', 'Delete') }}
            </ActionButton>
          </template>
        </el-table-column>
      </el-table>
      <div class="pagination">
        <el-pagination
          v-model:current-page="list.page.value"
          v-model:page-size="list.pageSize.value"
          :page-sizes="[20, 50, 100]"
          :total="list.total.value"
          layout="prev, pager, next, sizes"
          @change="list.load"
        />
      </div>
    </div>
  </template>
  <el-dialog
    v-model="modal"
    :title="current ? t('编辑记录', 'Edit record') : t('新建记录', 'New record')"
    width="640px"
    :close-on-click-modal="!busy"
    destroy-on-close
  >
    <el-alert v-if="error" type="error" :title="error" :closable="false" />
    <el-alert
      v-if="conflict"
      type="warning"
      :title="
        t(
          '记录已被其他人更新。重新载入后再修改。',
          'This record has changed. Reload the latest version before editing.',
        )
      "
      :closable="false"
    >
      <el-button @click="edit(current!)">{{ t('载入最新版本', 'Reload latest') }}</el-button>
    </el-alert>
    <el-form @submit.prevent="save" label-position="top" :disabled="busy">
      <FieldInput
        v-for="field in fields"
        :key="field.key"
        :field="field"
        :model-value="values[field.key]"
        @update:model-value="updateValue(field.key, $event)"
        :error="errors[field.key]"
      />
      <el-empty
        v-if="!fields.length"
        :description="t('当前授权没有可写字段', 'No writable fields in your current grants')"
        :image-size="60"
      />
      <template v-if="allowed(key, 'assign')">
        <div class="field-grid">
          <el-form-item :label="t('负责人', 'Owner')">
            <el-select v-if="members.length" v-model="owner" clearable filterable>
              <el-option
                v-for="member in members.filter((item) => item.status === 'active')"
                :key="member.id"
                :value="member.user_id"
                :label="member.display_name + ' · ' + member.email"
              />
            </el-select>
            <el-input
              v-else
              v-model="owner"
              :placeholder="t('用户 ID（留空为本人）', 'User ID (leave blank for yourself)')"
            />
          </el-form-item>
          <el-form-item :label="t('归属部门', 'Department')">
            <el-select v-if="departments.length" v-model="department" clearable>
              <el-option
                v-for="item in departments"
                :key="item.id"
                :value="item.id"
                :label="item.name"
              />
            </el-select>
            <el-input
              v-else
              v-model="department"
              :placeholder="t('部门 ID（可留空）', 'Department ID (optional)')"
            />
          </el-form-item>
        </div>
      </template>
      <p class="hint">
        {{
          t(
            '仅显示授权可写字段。保存时组织、数据范围与字段权限将再次校验。',
            'Only permitted writable fields are shown. Your workspace, scope and field access are checked again on save.',
          )
        }}
      </p>
      <p v-if="current" class="hint">
        {{
          t(
            '只保存修改过的字段。敏感字段留空可保留原值，输入内容将替换原值。',
            'Only changed fields are saved. Leave sensitive fields untouched to preserve their values; entering a value replaces them.',
          )
        }}
      </p>
      <div class="dialog-actions">
        <el-button @click="modal = false">{{ t('取消', 'Cancel') }}</el-button>
        <el-button
          native-type="submit"
          type="primary"
          :loading="busy"
          :disabled="conflict || !fields.length"
        >
          {{ t('保存', 'Save') }}
        </el-button>
      </div>
    </el-form>
  </el-dialog>
  <el-drawer
    :model-value="!!detail"
    :title="t('记录详情', 'Record details')"
    size="min(640px, 100%)"
    @close="detail = null"
  >
    <template v-if="detail">
      <el-descriptions :column="1" border>
        <el-descriptions-item
          v-for="field in module?.fields.filter((item) => Object.hasOwn(detail!.data, item.key))"
          :key="field.key"
          :label="state.locale === 'zh' ? field.label : field.label_en"
        >
          {{ formatValue(detail.data[field.key]) }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('版本', 'Version')">
          {{ detail.version }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('负责人 ID', 'Owner ID')">
          {{ detail.owner_id }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('部门 ID', 'Department ID')">
          {{ detail.department_id ?? '—' }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('创建于', 'Created')">
          {{ dateTime(detail.created_at) }}
        </el-descriptions-item>
      </el-descriptions>
      <div v-if="key === 'tickets' && statusWritable" class="ticket-transitions">
        <p>{{ t('工单流转', 'Ticket workflow') }}</p>
        <el-button
          v-for="status in edges[String(detail.data.status)] ?? []"
          :key="status"
          :loading="busy"
          @click="transition(detail!, status)"
        >
          {{ status }}
        </el-button>
      </div>
      <p class="hint">
        {{
          t(
            '敏感字段可能已被遮罩，未授权字段不返回。',
            'Sensitive fields may be masked. Fields outside your grants are omitted.',
          )
        }}
      </p>
    </template>
  </el-drawer>
  <el-dialog v-model="exportOpen" :title="t('创建导出任务', 'Create export job')" width="520px">
    <p>
      {{
        t(
          '选择列。后台任务与下载时都会重新验证当前权限。',
          'Choose columns. Access is checked when the job runs and again when downloading.',
        )
      }}
    </p>
    <el-checkbox-group v-model="columns">
      <el-checkbox v-for="field in exportFields" :key="field.key" :value="field.key">
        {{ state.locale === 'zh' ? field.label : field.label_en }}
      </el-checkbox>
    </el-checkbox-group>
    <template #footer>
      <el-button type="primary" :loading="busy" :disabled="!columns.length" @click="exportCSV">
        {{ t('创建导出任务', 'Create export job') }}
      </el-button>
    </template>
  </el-dialog>
</template>
