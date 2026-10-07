<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Grant, Module, Department } from '../api/types'
import { createGrant } from '../api/permissions'
import { t } from '../stores/app'
import { moduleLabel, resourceLabel, operationLabel, grantSummary } from '../api/presentation'
import GrantDetails from './permissions/GrantDetails.vue'
import GrantPreview from './permissions/GrantPreview.vue'
import { validateGrant } from '../api/permissions'
import { validationLabel } from '../api/presentation'
const props = defineProps<{
  modelValue: Grant[]
  modules: Module[]
  departments: Department[]
  disabled?: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [Grant[]] }>()
const selected = ref({
  resource: props.modelValue[0]?.resource ?? props.modules[0]?.key ?? '',
  action: props.modelValue[0]?.action ?? 'read',
})
watch(
  () => props.modules,
  (modules) => {
    if (!modules.some((module) => module.key === selected.value.resource))
      selected.value = { resource: modules[0]?.key ?? '', action: modules[0]?.actions[0] ?? 'read' }
  },
)
const descriptor = computed(() =>
  props.modules.find((module) => module.key === selected.value.resource),
)
const actions = computed(() => [...new Set(props.modules.flatMap((module) => module.actions))])
const grants = computed(() =>
  props.modelValue
    .map((grant, index) => ({ grant, index }))
    .filter(
      (item) =>
        item.grant.resource === selected.value.resource &&
        item.grant.action === selected.value.action,
    ),
)
const preview = ref<Grant[] | null>(null)
const resultingPreview = computed(() => [
  ...props.modelValue
    .filter((grant) => grant.resource === descriptor.value?.key)
    .map((grant) => ({ grant, added: false })),
  ...(preview.value ?? []).map((grant) => ({ grant, added: true })),
])
const invalidGrants = computed(() =>
  props.modelValue.flatMap((grant) => {
    const module = props.modules.find((item) => item.key === grant.resource)
    return validateGrant(grant, module).map((issue) => ({
      grant,
      issue: validationLabel(issue, module),
    }))
  }),
)
function count(resource: string, action: string) {
  return props.modelValue.filter((grant) => grant.resource === resource && grant.action === action)
    .length
}
function select(resource: string, action: string) {
  selected.value = { resource, action }
}
function add() {
  const grant = createGrant(selected.value.resource, selected.value.action)
  if (['read', 'export'].includes(grant.action)) grant.write_fields = []
  emit('update:modelValue', [...props.modelValue, grant])
}
function remove(index: number) {
  emit(
    'update:modelValue',
    props.modelValue.filter((_, item) => item !== index),
  )
}
function preset(kind: 'read' | 'work') {
  if (!descriptor.value) return
  const allowed = kind === 'read' ? ['read'] : ['read', 'create', 'update']
  preview.value = allowed
    .filter(
      (action) =>
        descriptor.value!.actions.includes(action) && !count(descriptor.value!.key, action),
    )
    .map((action) => ({
      ...createGrant(descriptor.value!.key, action),
      write_fields: action === 'read' ? [] : ['*'],
    }))
}
function applyPreset() {
  if (preview.value) emit('update:modelValue', [...props.modelValue, ...preview.value])
  preview.value = null
}
</script>
<template>
  <div class="role-editor">
    <div class="permission-intro">
      <div>
        <h3>{{ t('权限矩阵', 'Permission matrix') }}</h3>
        <p>
          {{
            t(
              '先选择资源和操作，再设定范围与字段。每个单元格可以包含多条独立授权。',
              'Choose a resource and operation, then refine its scope and fields. Each cell can contain independent grants.',
            )
          }}
        </p>
      </div>
      <el-tag>{{ modelValue.length }} {{ t('条授权', 'grants') }}</el-tag>
    </div>
    <el-alert
      v-if="invalidGrants.length"
      type="warning"
      :closable="false"
      :title="
        t(
          '还有授权需要完善，选择下方问题继续编辑。',
          'Some grants need attention. Select an issue below to continue editing.',
        )
      "
    >
      <div class="grant-issues">
        <el-button
          v-for="(issue, index) in invalidGrants"
          :key="index"
          text
          @click="select(issue.grant.resource, issue.grant.action)"
        >
          {{ issue.issue }}
        </el-button>
      </div>
    </el-alert>
    <el-table
      :data="modules"
      border
      class="permission-matrix"
      row-key="key"
      :empty-text="t('暂无资源', 'No resources')"
    >
      <el-table-column :label="t('资源', 'Resource')" min-width="160" fixed>
        <template #default="{ row }">
          <strong>{{ resourceLabel(row.key, modules) }}</strong>
        </template>
      </el-table-column>
      <el-table-column
        v-for="action in actions"
        :key="action"
        :label="operationLabel(action)"
        min-width="104"
        align="center"
      >
        <template #default="{ row }">
          <el-button
            v-if="row.actions.includes(action)"
            :type="
              selected.resource === row.key && selected.action === action
                ? 'primary'
                : count(row.key, action)
                  ? 'success'
                  : 'default'
            "
            :plain="!(selected.resource === row.key && selected.action === action)"
            size="small"
            :aria-label="`${moduleLabel(row)} · ${operationLabel(action, row)}`"
            :aria-pressed="selected.resource === row.key && selected.action === action"
            @click="select(row.key, action)"
          >
            {{
              count(row.key, action)
                ? t(`${count(row.key, action)} 条`, `${count(row.key, action)} grants`)
                : t('未授权', 'No access')
            }}
          </el-button>
          <span v-else class="muted" aria-hidden="true">—</span>
        </template>
      </el-table-column>
    </el-table>
    <section v-if="descriptor" class="permission-detail" aria-live="polite">
      <div class="panel-heading">
        <div>
          <span class="eyebrow">{{ t('当前选择', 'SELECTED ACCESS') }}</span>
          <h3>
            {{ moduleLabel(descriptor) }}
            <span class="muted">/</span>
            {{ operationLabel(selected.action, descriptor) }}
          </h3>
        </div>
        <el-button v-if="!disabled" @click="add">
          <span aria-hidden="true">+</span>
          {{ grants.length ? t('增加独立范围', 'Add another scope') : t('添加授权', 'Add grant') }}
        </el-button>
      </div>
      <div v-if="!disabled" class="preset-toolbar">
        <span class="hint">
          {{
            t(
              '快速开始；只添加缺少的操作，已有规则保留。',
              'Start safely. Presets add missing operations and preserve existing rules.',
            )
          }}
        </span>
        <el-button size="small" @click="preset('read')">
          {{ t('只读预设', 'Read-only preset') }}
        </el-button>
        <el-button size="small" @click="preset('work')">
          {{ t('本人工作预设', 'Own-work preset') }}
        </el-button>
      </div>
      <el-empty
        v-if="!grants.length"
        :description="
          t(
            '此操作默认拒绝。添加授权后再选择数据范围。',
            'This operation is denied. Add a grant to define its record scope.',
          )
        "
        :image-size="48"
      />
      <p v-if="grants.length > 1" class="hint">
        {{
          t(
            '以下范围以“或”合并生效，各授权的条件以“且”同时满足。',
            'The scopes below are combined with OR. Conditions inside each grant use AND.',
          )
        }}
      </p>
      <GrantDetails
        v-for="item in grants"
        :key="item.index"
        :grant="item.grant"
        :descriptor="descriptor"
        :departments="departments"
        :disabled="disabled"
        @remove="remove(item.index)"
      />
    </section>
    <el-dialog
      :model-value="preview !== null"
      :title="t('确认新增权限', 'Review added access')"
      width="640px"
      @update:model-value="preview = null"
    >
      <p>
        {{
          t(
            '以下预设仅影响当前资源；现有授权保持不变，不包含敏感字段明文权限。',
            'This preset affects only the selected resource. Existing grants remain unchanged; no sensitive unmask access is added.',
          )
        }}
      </p>
      <el-empty
        v-if="!preview?.length"
        :description="
          t(
            '这些操作已有授权，无需新增。',
            'These operations already have grants. Nothing will be added.',
          )
        "
        :image-size="48"
      />
      <p class="hint">
        {{
          t(
            '当前资源最终权限如下。以下范围以“或”合并生效，各授权条件需同时满足。其他资源保持不变。',
            'Resulting access for this resource is shown below. Grant scopes combine with OR; conditions inside each grant use AND. Other resources remain unchanged.',
          )
        }}
      </p>
      <ul v-if="resultingPreview.length && descriptor" class="preset-preview">
        <li v-for="(entry, index) in resultingPreview" :key="index">
          <el-tag :type="entry.added ? 'success' : 'info'" size="small">
            {{ entry.added ? t('新增', 'Added') : t('保留', 'Preserved') }}
          </el-tag>
          <GrantPreview :grant="entry.grant" :descriptor="descriptor" :departments="departments" />
        </li>
      </ul>
      <template #footer>
        <el-button @click="preview = null">{{ t('取消', 'Cancel') }}</el-button>
        <el-button type="primary" :disabled="!preview?.length" @click="applyPreset">
          {{ t('应用预设', 'Apply preset') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>
