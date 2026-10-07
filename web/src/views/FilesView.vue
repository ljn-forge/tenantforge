<script setup lang="ts">
import { ref, reactive } from 'vue'
import { api, download } from '../api/client'
import type { FileRecord } from '../api/types'
import { t, allowed } from '../stores/app'
import { dateTime } from '../api/forms'
import { useCollection } from '../composables/collection'
import { useRequest, saved, confirmDelete } from '../composables/request'
import PageHeader from '../components/PageHeader.vue'
import PageState from '../components/PageState.vue'
import ActionButton from '../components/ActionButton.vue'
const list = useCollection<FileRecord>(
  () => '/files',
  () => allowed('files', 'read'),
)
const { run, busy, error } = useRequest()
const modal = ref(false)
const selected = ref<File | null>(null)
const form = reactive({ owner_id: '', department_id: '' })
const fileInput = ref<HTMLInputElement>()
function size(bytes: number) {
  return bytes < 1024
    ? `${bytes} B`
    : bytes < 1024 * 1024
      ? `${(bytes / 1024).toFixed(1)} KiB`
      : `${(bytes / 1024 / 1024).toFixed(1)} MiB`
}
function choose(event: Event) {
  selected.value = (event.target as HTMLInputElement).files?.[0] ?? null
}
async function upload() {
  if (!selected.value) return
  await run(async () => {
    const body = new FormData()
    body.append('file', selected.value!)
    if (allowed('members', 'assign')) {
      if (form.owner_id) body.append('owner_id', form.owner_id)
      if (form.department_id) body.append('department_id', form.department_id)
    }
    await api('/files', { method: 'POST', body })
    modal.value = false
    selected.value = null
    form.owner_id = ''
    form.department_id = ''
    saved()
    await list.load()
  })
}
async function remove(file: FileRecord) {
  if (await confirmDelete(file.name))
    await run(async () => {
      await api(`/files/${file.id}`, { method: 'DELETE' })
      saved()
      await list.load()
    })
}
</script>
<template>
  <PageHeader
    :title="t('组织文件', 'Workspace files')"
    :description="
      t(
        '上传、下载与访问范围由当前组织权限控制。',
        'Upload and download files within your workspace access scope.',
      )
    "
  >
    <ActionButton resource="files" action="create" type="primary" @click="modal = true">
      + {{ t('上传文件', 'Upload file') }}
    </ActionButton>
  </PageHeader>
  <PageState
    :denied="!allowed('files', 'read')"
    :error="list.error.value || error"
    :busy="list.busy.value"
    @retry="list.load"
  />
  <div v-if="allowed('files', 'read')" class="panel">
    <form class="table-toolbar" @submit.prevent="list.search">
      <el-input
        v-model="list.query.value"
        :placeholder="t('搜索文件名', 'Search file names')"
        clearable
      />
      <el-button native-type="submit">{{ t('搜索', 'Search') }}</el-button>
      <el-button @click="list.load">{{ t('刷新', 'Refresh') }}</el-button>
      <span class="count-label">{{ list.total.value }} {{ t('个文件', 'files') }}</span>
    </form>
    <el-table :data="list.items.value" :empty-text="t('暂无可见文件', 'No visible files')">
      <el-table-column :label="t('文件名', 'Name')" min-width="240">
        <template #default="{ row }">
          <strong>{{ row.name }}</strong>
          <small class="cell-secondary">{{ row.content_type }}</small>
        </template>
      </el-table-column>
      <el-table-column :label="t('大小', 'Size')" width="130">
        <template #default="{ row }">{{ size(row.size) }}</template>
      </el-table-column>
      <el-table-column :label="t('上传时间', 'Uploaded')" min-width="180">
        <template #default="{ row }">{{ dateTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column :label="t('操作', 'Actions')" width="170">
        <template #default="{ row }">
          <el-button
            link
            type="primary"
            :disabled="busy"
            @click="() => run(() => download(`/files/${row.id}/download`, row.name))"
          >
            {{ t('下载', 'Download') }}
          </el-button>
          <ActionButton
            resource="files"
            action="delete"
            link
            type="danger"
            :disabled="busy"
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
    :title="t('上传文件', 'Upload file')"
    width="530px"
    :close-on-click-modal="!busy"
    destroy-on-close
  >
    <el-alert v-if="error" type="error" :title="error" :closable="false" />
    <el-form @submit.prevent="upload" label-position="top">
      <el-form-item :label="t('选择文件', 'Choose file')" required>
        <input ref="fileInput" type="file" class="file-input" required @change="choose" />
        <small v-if="selected" class="hint">{{ selected.name }} · {{ size(selected.size) }}</small>
      </el-form-item>
      <template v-if="allowed('members', 'assign')">
        <el-form-item :label="t('负责人用户 ID（可选）', 'Owner user ID (optional)')">
          <el-input v-model="form.owner_id" />
        </el-form-item>
        <el-form-item :label="t('部门 ID（可选）', 'Department ID (optional)')">
          <el-input v-model="form.department_id" />
        </el-form-item>
      </template>
      <p class="hint">
        {{
          t(
            '文件大小上限由组织配额与服务配置决定。',
            'File limits are determined by workspace quotas and service configuration.',
          )
        }}
      </p>
      <el-button type="primary" native-type="submit" :disabled="!selected" :loading="busy">
        {{ t('上传', 'Upload') }}
      </el-button>
    </el-form>
  </el-dialog>
</template>
