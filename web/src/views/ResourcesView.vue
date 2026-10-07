<script setup lang="ts">
import { useResources } from '../composables/resources'
import { state, allowed, t } from '../stores/app'
import { moduleDescription } from '../api/presentation'
import PageHeader from '../components/PageHeader.vue'
import PageState from '../components/PageState.vue'
import ActionButton from '../components/ActionButton.vue'
import ResourceFilters from '../components/resources/ResourceFilters.vue'
import ResourceTable from '../components/resources/ResourceTable.vue'
import ResourceEditDialog from '../components/resources/ResourceEditDialog.vue'
import ResourceDetails from '../components/resources/ResourceDetails.vue'
const {
  filterFields,
  filters,
  setFilter,
  clearFilters,
  key,
  module,
  title,
  list,
  busy,
  error,
  modal,
  current,
  detail,
  values,
  errors,
  conflict,
  owner,
  department,
  members,
  departments,
  importInput,
  exportOpen,
  columns,
  fields,
  visibleFields,
  exportFields,
  canAssign,
  edit,
  view,
  save,
  remove,
  transition,
  importCSV,
  importCount,
  openExport,
  exportCSV,
  updateValue,
  operate,
} = useResources()
</script>
<template>
  <PageHeader
    :title="title"
    :description="module ? moduleDescription(module) : undefined"
    :eyebrow="t('业务管理', 'WORKSPACE')"
  >
    <ActionButton :resource="key" action="import" :disabled="busy" @click="importInput?.click()">
      {{ t('导入 CSV', 'Import CSV') }}
    </ActionButton>
    <ActionButton :resource="key" action="export" :disabled="busy" @click="openExport">
      {{ t('导出', 'Export') }}
    </ActionButton>
    <ActionButton :resource="key" action="create" type="primary" :disabled="busy" @click="edit()">
      <span aria-hidden="true">+</span>
      {{ t('新建记录', 'New record') }}
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
        <ResourceFilters
          :fields="filterFields"
          :values="filters"
          @change="setFilter"
          @clear="clearFilters"
        />
        <span class="count-label">{{ list.total.value }} {{ t('条记录', 'records') }}</span>
      </form>
      <ResourceTable
        :records="list.items.value"
        :fields="visibleFields"
        :resource="key"
        @view="view"
        @edit="edit"
        @remove="remove"
      />
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
  <ResourceEditDialog
    v-model="modal"
    :current="current"
    :fields="fields"
    :values="values"
    :errors="errors"
    :error="error"
    :busy="busy"
    :conflict="conflict"
    :can-assign="canAssign"
    :owner="owner"
    :department="department"
    :members="members"
    :departments="departments"
    @save="save"
    @reload="current && edit(current)"
    @field="updateValue"
    @owner="owner = $event"
    @department="department = $event"
  />
  <ResourceDetails
    :record="detail"
    :descriptor="module"
    :busy="busy"
    @close="detail = null"
    @transition="transition"
    @operate="operate"
  />
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
