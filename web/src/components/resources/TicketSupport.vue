<script setup lang="ts">
import { toRef } from 'vue'
import type { Module, ResourceRecord, TicketEvent } from '../../api/types'
import { useTicketSupport } from '../../composables/ticketSupport'
import { dateTime } from '../../api/forms'
import { statusLabel } from '../../api/presentation'
import { state, t } from '../../stores/app'
const props = defineProps<{
  record: ResourceRecord
  descriptor: Module | undefined
  busy: boolean
}>()
const emit = defineEmits<{ updated: [ResourceRecord]; close: [] }>()
const {
  events,
  total,
  page,
  pageSize,
  loading,
  historyError,
  loadHistory,
  error,
  conflict,
  busy: submitting,
  draft,
  draftIssue,
  kind,
  replyKinds,
  reply,
  reloadConflict,
  canAssign,
  assignmentOpen,
  members,
  departments,
  owner,
  department,
  referencesBusy,
  referencesError,
  openAssignment,
  loadReferences,
  assign,
  ownerName,
  departmentName,
} = useTicketSupport(
  toRef(props, 'record'),
  (record) => emit('updated', record),
  () => emit('close'),
)
function eventLabel(event: TicketEvent) {
  const labels = {
    created: t('登记工单', 'Ticket registered'),
    updated: t('更新工单', 'Ticket updated'),
    assigned: t('调整分配', 'Assignment changed'),
    status_changed: t('状态变更', 'Status changed'),
    reply: t('客户沟通记录', 'Customer reply'),
    note: t('私密内部备注', 'Private internal note'),
  }
  return labels[event.kind] ?? t('工单活动', 'Ticket activity')
}
function fieldLabels(fields: string[]) {
  return fields
    .map((key) => {
      const field = props.descriptor?.fields.find((item) => item.key === key)
      return field ? (state.locale === 'zh' ? field.label : field.label_en) : key
    })
    .join(', ')
}
</script>
<template>
  <section class="ticket-support" :aria-label="t('工单处理', 'Ticket handling')">
    <div class="ticket-owner">
      <div>
        <strong>{{ t('负责人', 'Assignee') }}: {{ ownerName }}</strong>
        <p class="hint">{{ departmentName }}</p>
      </div>
      <el-button v-if="canAssign" :disabled="busy || submitting" @click="openAssignment">
        {{ t('分配工单', 'Assign ticket') }}
      </el-button>
    </div>
    <el-alert v-if="error" type="error" :title="error" :closable="false" />
    <el-alert
      v-if="conflict"
      type="warning"
      :closable="false"
      :title="
        t(
          '工单已被更新。草稿已保留，请载入最新版本再提交。',
          'This ticket changed. Your draft is preserved; reload the latest version before submitting.',
        )
      "
    >
      <el-button :loading="submitting" @click="reloadConflict">
        {{ t('载入最新版本', 'Reload latest') }}
      </el-button>
    </el-alert>
    <section
      class="ticket-activity"
      :aria-label="t('处理历史', 'Activity history')"
      :aria-busy="loading"
    >
      <div class="panel-heading">
        <h3>{{ t('处理历史', 'Activity history') }}</h3>
        <el-button :loading="loading" @click="loadHistory">
          {{ t('刷新历史', 'Refresh history') }}
        </el-button>
      </div>
      <el-alert v-if="historyError" type="error" :title="historyError" :closable="false">
        <el-button @click="loadHistory">{{ t('重试', 'Retry') }}</el-button>
      </el-alert>
      <p v-else-if="!loading && !events.length" class="hint">
        {{
          t(
            '暂无可见处理记录。历史从启用此功能后开始记录。',
            'No visible activity yet. History is recorded from the feature upgrade onward.',
          )
        }}
      </p>
      <ol class="ticket-events">
        <li
          v-for="event in events"
          :key="event.id"
          :class="{ 'ticket-private-note': event.kind === 'note' }"
        >
          <div class="ticket-event-heading">
            <strong>{{ eventLabel(event) }}</strong>
            <time>{{ dateTime(event.created_at) }}</time>
          </div>
          <p class="hint">{{ event.actor_name || t('已离开的同事', 'Former colleague') }}</p>
          <p v-if="event.to_status" class="ticket-event-status">
            {{ event.from_status ? `${statusLabel(event.from_status)} → ` : ''
            }}{{ statusLabel(event.to_status) }}
          </p>
          <p v-if="event.fields?.length" class="hint">
            {{ t('修改字段', 'Updated fields') }}: {{ fieldLabels(event.fields) }}
          </p>
          <p
            v-if="
              event.body &&
              (event.kind === 'reply' ||
                event.kind === 'note' ||
                (event.kind === 'status_changed' && event.to_status === 'resolved'))
            "
            class="ticket-message"
          >
            {{ event.body }}
          </p>
        </li>
      </ol>
      <div v-if="total" class="ticket-history-pagination">
        <span class="hint">{{ total }} {{ t('条可见活动', 'visible events') }}</span>
        <el-pagination
          v-model:current-page="page"
          :total="total"
          :page-size="pageSize"
          layout="prev,pager,next"
          :disabled="loading"
          @change="loadHistory"
        />
      </div>
    </section>
    <el-form
      v-if="replyKinds.length"
      class="ticket-composer"
      label-position="top"
      :disabled="busy || submitting"
      @submit.prevent="reply"
    >
      <el-radio-group v-model="kind" :aria-label="t('记录类型', 'Message kind')">
        <el-radio-button v-if="replyKinds.includes('reply')" value="reply">
          {{ t('客户沟通记录', 'Customer reply') }}
        </el-radio-button>
        <el-radio-button v-if="replyKinds.includes('note')" value="note">
          {{ t('内部备注', 'Internal note') }}
        </el-radio-button>
      </el-radio-group>
      <p class="hint">
        {{
          kind === 'note'
            ? t(
                '私密备注仅向具备内部备注读取和明文权限的同事显示。',
                'Private notes are visible only to colleagues with explicit read and unmask access to internal notes.',
              )
            : t(
                '记录员工与客户的沟通内容，此操作不会发送客户邮件。',
                'Record employee communication with the customer. This action does not send customer email.',
              )
        }}
      </p>
      <el-form-item
        :label="t('沟通内容', 'Reply message')"
        :error="draft && draftIssue ? draftIssue : ''"
        required
      >
        <el-input
          v-model="draft"
          type="textarea"
          :rows="4"
          :aria-label="t('沟通内容', 'Reply message')"
        />
      </el-form-item>
      <div class="ticket-composer-actions">
        <span class="hint">{{ [...draft].length }} / 4000</span>
        <el-button
          native-type="submit"
          type="primary"
          :loading="submitting"
          :disabled="!!draftIssue || conflict || busy"
        >
          {{ kind === 'note' ? t('添加备注', 'Add note') : t('添加沟通记录', 'Add reply') }}
        </el-button>
      </div>
    </el-form>
    <p v-else class="hint">
      {{
        t(
          '当前权限或工单状态不允许添加沟通记录。',
          'Your current access or ticket status does not allow adding messages.',
        )
      }}
    </p>
    <el-dialog
      v-model="assignmentOpen"
      class="ticket-assignment"
      :title="t('分配工单', 'Assign ticket')"
      width="min(520px, calc(100% - 32px))"
      :close-on-click-modal="!submitting"
      destroy-on-close
    >
      <el-alert v-if="error" type="error" :title="error" :closable="false" />
      <el-alert
        v-if="conflict"
        type="warning"
        :closable="false"
        :title="
          t(
            '工单已被更新，载入最新版本后再分配。',
            'This ticket changed. Reload the latest version before assigning.',
          )
        "
      >
        <el-button :loading="submitting" @click="reloadConflict">
          {{ t('载入最新版本', 'Reload latest') }}
        </el-button>
      </el-alert>
      <el-alert v-if="referencesError" type="error" :title="referencesError" :closable="false">
        <el-button @click="loadReferences">{{ t('重试', 'Retry') }}</el-button>
      </el-alert>
      <el-form
        label-position="top"
        :disabled="submitting || referencesBusy || busy"
        @submit.prevent="assign"
      >
        <el-form-item :label="t('负责人', 'Assignee')" required>
          <el-select
            v-model="owner"
            filterable
            :loading="referencesBusy"
            :aria-label="t('负责人', 'Assignee')"
          >
            <el-option
              v-for="member in members"
              :key="member.id"
              :value="member.user_id"
              :label="member.display_name"
            />
          </el-select>
          <p v-if="!referencesBusy && !members.length" class="hint">
            {{
              t(
                '没有可见的启用用户，请检查用户读取权限与数据范围。',
                'No active members are visible. Check member read access and scope.',
              )
            }}
          </p>
        </el-form-item>
        <el-form-item :label="t('工单部门', 'Support department')">
          <el-select v-model="department" :aria-label="t('工单部门', 'Support department')">
            <el-option
              value="__follow"
              :label="t('跟随负责人部门', 'Follow assignee department')"
            />
            <el-option value="__clear" :label="t('不指定部门', 'No department')" />
            <el-option
              v-for="item in departments"
              :key="item.id"
              :value="item.id"
              :label="item.name"
            />
          </el-select>
        </el-form-item>
        <p class="hint">
          {{
            t(
              '仅列出当前范围内的启用用户与可见部门。保存时再次检查分配权限。',
              'Only active members and departments in your current scope are listed. Assignment access is checked again on save.',
            )
          }}
        </p>
        <div class="dialog-actions">
          <el-button @click="assignmentOpen = false">{{ t('取消', 'Cancel') }}</el-button>
          <el-button
            native-type="submit"
            type="primary"
            :loading="submitting"
            :disabled="
              conflict || referencesBusy || busy || !members.some((item) => item.user_id === owner)
            "
          >
            {{ t('分配', 'Assign') }}
          </el-button>
        </div>
      </el-form>
    </el-dialog>
  </section>
</template>
