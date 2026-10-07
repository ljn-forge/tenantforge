<script setup lang="ts">
import { computed, onMounted, ref, reactive } from 'vue'
import { api, allPages } from '../api/client'
import type { Department } from '../api/types'
import { t, allowed } from '../stores/app'
import { departmentForest } from '../api/organization'
import { useCollection } from '../composables/collection'
import { useRequest, saved, confirmDelete } from '../composables/request'
import PageHeader from '../components/PageHeader.vue'
import PageState from '../components/PageState.vue'
import ActionButton from '../components/ActionButton.vue'
const list = useCollection<Department>(
  () => '/departments',
  () => allowed('departments', 'read'),
)
const { run, busy, error } = useRequest()
const departments = ref<Department[]>([])
const tree = computed(() => departmentForest(departments.value, list.query.value))
const current = ref<Department | null>(null),
  modal = ref(false)
const form = reactive({ name: '', parent_id: '' })
async function load() {
  if (allowed('departments', 'read')) departments.value = await allPages<Department>('/departments')
}
onMounted(() => run(load))
function open(record?: Department) {
  current.value = record ?? null
  form.name = record?.name ?? ''
  form.parent_id = record?.parent_id ?? ''
  modal.value = true
}
async function save() {
  await run(async () => {
    await api(`/departments${current.value ? `/${current.value.id}` : ''}`, {
      method: current.value ? 'PATCH' : 'POST',
      body: { name: form.name, parent_id: form.parent_id || null },
    })
    modal.value = false
    saved()
    await load()
    await list.load()
  })
}
async function remove(record: Department) {
  if (await confirmDelete(record.name))
    await run(async () => {
      await api(`/departments/${record.id}`, { method: 'DELETE' })
      saved()
      await load()
      await list.load()
    })
}
</script>
<template>
  <PageHeader
    :title="t('部门与岗位', 'Organization')"
    :description="
      t(
        '清晰的部门层级，让数据范围和职责一致。岗位在用户资料中设置。',
        'A clear department tree keeps access scopes aligned with responsibilities. Positions are set in user profiles.',
      )
    "
  >
    <ActionButton
      resource="departments"
      action="create"
      type="primary"
      :disabled="busy"
      @click="open()"
    >
      <span aria-hidden="true">+</span>
      {{ t('创建部门', 'Create department') }}
    </ActionButton>
  </PageHeader>
  <PageState
    :denied="!allowed('departments', 'read')"
    :error="error || list.error.value"
    :busy="busy || list.busy.value"
    @retry="() => run(load)"
  />
  <section v-if="allowed('departments', 'read')" class="panel">
    <form class="table-toolbar" @submit.prevent="list.search">
      <el-input
        v-model="list.query.value"
        :placeholder="t('搜索部门', 'Search departments')"
        :aria-label="t('搜索部门', 'Search departments')"
        clearable
      />
      <el-button native-type="submit">{{ t('搜索', 'Search') }}</el-button>
      <el-button @click="() => run(load)">{{ t('刷新', 'Refresh') }}</el-button>
      <span class="count-label">{{ departments.length }} {{ t('个部门', 'departments') }}</span>
    </form>
    <el-table
      :data="tree"
      row-key="id"
      default-expand-all
      :tree-props="{ children: 'children' }"
      :empty-text="t('暂无部门', 'No departments')"
    >
      <el-table-column prop="name" :label="t('部门名称', 'Department')" min-width="260" />
      <el-table-column :label="t('操作', 'Actions')" width="180">
        <template #default="{ row }">
          <ActionButton
            resource="departments"
            :record="row"
            action="update"
            link
            type="primary"
            @click="open(row)"
          >
            {{ t('编辑', 'Edit') }}
          </ActionButton>
          <ActionButton
            resource="departments"
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
  </section>
  <el-dialog
    v-model="modal"
    :title="current ? t('编辑部门', 'Edit department') : t('创建部门', 'Create department')"
    width="520px"
    destroy-on-close
  >
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <el-form label-position="top" :disabled="busy" @submit.prevent="save">
      <el-form-item :label="t('部门名称', 'Department name')" required>
        <el-input v-model="form.name" required maxlength="100" />
      </el-form-item>
      <el-form-item :label="t('上级部门', 'Parent department')">
        <el-select v-model="form.parent_id" clearable>
          <el-option
            v-for="item in departments.filter((item) => item.id !== current?.id)"
            :key="item.id"
            :value="item.id"
            :label="item.name"
          />
        </el-select>
      </el-form-item>
      <p class="hint">
        {{
          t(
            '不允许循环层级。有成员、子部门或记录引用的部门不可删除。',
            'Circular hierarchies are rejected. Referenced departments cannot be deleted.',
          )
        }}
      </p>
      <div class="dialog-actions">
        <el-button @click="modal = false">{{ t('取消', 'Cancel') }}</el-button>
        <el-button type="primary" native-type="submit" :loading="busy">
          {{ t('保存', 'Save') }}
        </el-button>
      </div>
    </el-form>
  </el-dialog>
</template>
