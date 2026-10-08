import { computed, nextTick, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import { api, APIError, allPages, clientContext } from '../api/client'
import type { Department, Member, ResourceRecord, TicketEvent } from '../api/types'
import { recordAllows } from '../api/capabilities'
import { allowed, state, t } from '../stores/app'

export function useTicketSupport(
  record: Ref<ResourceRecord>,
  updated: (record: ResourceRecord) => void,
  close: () => void,
) {
  const events = ref<TicketEvent[]>([])
  const total = ref(0)
  const page = ref(1)
  const pageSize = 20
  const loading = ref(false)
  const historyError = ref('')
  const error = ref('')
  const conflict = ref(false)
  const busy = ref(false)
  const draft = ref('')
  const kind = ref<'reply' | 'note'>('reply')
  const assignmentOpen = ref(false)
  const members = ref<Member[]>([])
  const departments = ref<Department[]>([])
  const owner = ref('')
  // An omitted department follows the target membership; null explicitly clears it.
  const department = ref('__follow')
  const referencesBusy = ref(false)
  const referencesError = ref('')
  let live = true
  let revision = 0
  let historyRevision = 0
  let referenceRevision = 0
  const invalidated = ref(false)
  const context = computed(() =>
    JSON.stringify([
      clientContext.value.epoch,
      state.tenant,
      state.me?.user?.id,
      state.me?.permissions,
    ]),
  )
  const replyKinds = computed(() =>
    invalidated.value
      ? []
      : (record.value.capabilities?.reply_kinds ?? []).filter(
          (item) => item === 'reply' || item === 'note',
        ),
  )
  const canAssign = computed(() => !invalidated.value && recordAllows(record.value, 'assign'))
  const draftIssue = computed(() => {
    if (!draft.value.trim()) return t('请输入内容', 'Enter a message')
    if ([...draft.value].length > 4000 || new TextEncoder().encode(draft.value).length > 8192)
      return t('最多 4000 个字符及 8 KiB 文本', 'Maximum 4,000 characters and 8 KiB of text')
    return ''
  })
  function valid(epoch: number) {
    return live && !invalidated.value && revision === epoch
  }
  function reset() {
    revision++
    historyRevision++
    referenceRevision++
    events.value = []
    total.value = 0
    page.value = 1
    historyError.value = ''
    error.value = ''
    conflict.value = false
    draft.value = ''
    assignmentOpen.value = false
    members.value = []
    departments.value = []
    owner.value = ''
    department.value = '__follow'
    busy.value = false
    loading.value = false
    referencesBusy.value = false
    referencesError.value = ''
  }
  function isContextFailure(failure: unknown) {
    return (
      (failure instanceof APIError && failure.code === 'context_changed') ||
      (failure instanceof DOMException && failure.name === 'AbortError')
    )
  }
  function message(failure: unknown) {
    return failure instanceof Error
      ? failure.message
      : t('请求失败，请重试', 'Request failed. Please try again.')
  }
  async function loadHistory() {
    if (invalidated.value) return
    const epoch = revision
    const request = ++historyRevision
    loading.value = true
    historyError.value = ''
    try {
      const result = await api<TicketEvent[]>(
        `/resources/tickets/${record.value.id}/activity?page=${page.value}&page_size=${pageSize}`,
      )
      if (!valid(epoch) || request !== historyRevision) return
      events.value = Array.isArray(result.data) ? result.data : []
      total.value = result.meta?.total ?? events.value.length
    } catch (failure) {
      if (valid(epoch) && request === historyRevision && !isContextFailure(failure)) {
        events.value = []
        total.value = 0
        historyError.value = message(failure)
        if (failure instanceof APIError && [403, 404].includes(failure.status)) close()
      }
    } finally {
      if (valid(epoch) && request === historyRevision) loading.value = false
    }
  }
  async function refreshRecord() {
    const epoch = revision
    const id = record.value.id
    try {
      const result = await api<ResourceRecord>(`/resources/tickets/${id}`)
      if (!valid(epoch)) return false
      if (!recordAllows(result.data, 'read')) {
        close()
        return false
      }
      const previousHistory = historyRevision
      updated(result.data)
      await nextTick()
      // A parent normally publishes the fresh Record prop, whose watcher clears
      // and reloads history. Also support consumers that have not replaced it yet.
      if (valid(epoch) && historyRevision === previousHistory) {
        events.value = []
        total.value = 0
        await loadHistory()
      }
      return true
    } catch (failure) {
      if (!valid(epoch) || isContextFailure(failure)) return false
      if (failure instanceof APIError && [403, 404].includes(failure.status)) close()
      else error.value = message(failure)
      return false
    }
  }
  async function mutate(operation: () => Promise<ResourceRecord>, success: () => void) {
    if (busy.value || invalidated.value || conflict.value) return
    const epoch = revision
    busy.value = true
    error.value = ''
    try {
      const result = await operation()
      if (!valid(epoch)) return
      if (!recordAllows(result, 'read')) {
        close()
        return
      }
      success()
      page.value = 1
      const previousHistory = historyRevision
      updated(result)
      await nextTick()
      if (valid(epoch) && historyRevision === previousHistory) {
        events.value = []
        total.value = 0
        await loadHistory()
      }
    } catch (failure) {
      if (!valid(epoch) || isContextFailure(failure)) return
      error.value = message(failure)
      if (failure instanceof APIError) {
        if (failure.status === 409) conflict.value = true
        if ([403, 404].includes(failure.status)) {
          await refreshRecord()
        }
      }
    } finally {
      if (valid(epoch)) busy.value = false
    }
  }
  async function reply() {
    if (draftIssue.value || !replyKinds.value.includes(kind.value)) return
    const ticket = record.value
    const body = draft.value
    const replyKind = kind.value
    await mutate(
      async () =>
        (
          await api<{ record: ResourceRecord; event: TicketEvent }>(
            `/resources/tickets/${ticket.id}/replies`,
            {
              method: 'POST',
              body: { version: ticket.version, kind: replyKind, body },
            },
          )
        ).data.record,
      () => {
        draft.value = ''
      },
    )
  }
  async function reloadConflict() {
    if (busy.value || invalidated.value) return
    busy.value = true
    const epoch = revision
    const refreshed = await refreshRecord()
    if (valid(epoch)) {
      if (refreshed) {
        conflict.value = false
      }
      busy.value = false
    }
  }
  async function loadReferences() {
    const epoch = revision
    const request = ++referenceRevision
    referencesBusy.value = true
    referencesError.value = ''
    try {
      const [people, teams] = await Promise.all([
        allowed('members', 'read') ? allPages<Member>('/members') : Promise.resolve([]),
        allowed('departments', 'read') ? allPages<Department>('/departments') : Promise.resolve([]),
      ])
      if (!valid(epoch) || request !== referenceRevision) return
      members.value = people.filter((item) => item.status === 'active')
      departments.value = teams
    } catch (failure) {
      if (valid(epoch) && request === referenceRevision && !isContextFailure(failure))
        referencesError.value = message(failure)
    } finally {
      if (valid(epoch) && request === referenceRevision) referencesBusy.value = false
    }
  }
  async function openAssignment() {
    if (!canAssign.value || busy.value) return
    owner.value = record.value.owner_id
    department.value = '__follow'
    assignmentOpen.value = true
    await loadReferences()
  }
  async function assign() {
    if (
      !canAssign.value ||
      referencesBusy.value ||
      !members.value.some((item) => item.user_id === owner.value)
    )
      return
    const ticket = record.value
    const body = {
      version: ticket.version,
      owner_id: owner.value,
      ...(department.value === '__follow'
        ? {}
        : { department_id: department.value === '__clear' ? null : department.value }),
    }
    await mutate(
      async () =>
        (
          await api<ResourceRecord>(`/resources/tickets/${ticket.id}/assignment`, {
            method: 'POST',
            body,
          })
        ).data,
      () => {
        assignmentOpen.value = false
      },
    )
  }
  const ownerName = computed(() =>
    record.value.owner_id === state.me?.user?.id
      ? state.me.user.display_name || t('本人', 'You')
      : (members.value.find((item) => item.user_id === record.value.owner_id)?.display_name ??
        t('已分配给同事', 'Assigned colleague')),
  )
  const departmentName = computed(
    () =>
      departments.value.find((item) => item.id === record.value.department_id)?.name ??
      (record.value.department_id
        ? t('工单所属部门', 'Ticket department')
        : t('未指定部门', 'No department')),
  )
  watch(
    context,
    () => {
      reset()
      invalidated.value = true
      close()
    },
    { flush: 'sync' },
  )
  // Ownership, state and row capabilities can change the current read predicate
  // even when the account's global grants remain unchanged.
  const recordContext = computed(() =>
    JSON.stringify([
      record.value.id,
      record.value.owner_id,
      record.value.department_id,
      record.value.data.status,
      record.value.capabilities?.actions,
      record.value.capabilities?.write_fields,
      record.value.capabilities?.reply_kinds,
      Object.keys(record.value.data).sort(),
    ]),
  )
  watch(
    [() => record.value, recordContext, () => record.value.version],
    ([, identity], previous) => {
      if (identity !== previous?.[1]) {
        reset()
        invalidated.value = false
      }
      if (!invalidated.value) {
        // A fresh detail read can reflect field revocation without any version,
        // capability or key changes (for example an observer of a legacy ticket).
        events.value = []
        total.value = 0
        historyRevision++
        void loadHistory()
      }
    },
    { immediate: true, flush: 'sync' },
  )
  watch(replyKinds, (kinds) => {
    if (!kinds.includes(kind.value)) {
      draft.value = ''
      kind.value = kinds[0] ?? 'reply'
    }
  })
  onBeforeUnmount(() => {
    live = false
    reset()
  })
  return {
    events,
    total,
    page,
    pageSize,
    loading,
    historyError,
    loadHistory,
    error,
    conflict,
    busy,
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
  }
}
