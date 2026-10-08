import { computed, reactive, ref, watch, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, APIError, allPages, clientContext } from '../api/client'
import type { ResourceRecord, Department, Member, Job } from '../api/types'
import { editableFields, buildResourceData, validateResourceData } from '../api/forms'
import { recordAllows, recordWritableFields } from '../api/capabilities'
import { state, allowed, t } from '../stores/app'
import { useCollection } from '../composables/collection'
import { useRequest, saved, confirmDelete } from '../composables/request'

export function useResources() {
  const route = useRoute()
  const router = useRouter()
  const key = computed(() => String(route.params.module))
  const context = computed(() =>
    JSON.stringify([
      clientContext.value.epoch,
      state.tenant,
      state.me?.user?.id,
      state.me?.permissions,
      key.value,
    ]),
  )
  let revision = 0
  let detailRevision = 0
  let live = true
  function valid(epoch: number) {
    return live && epoch === revision
  }
  const module = computed(() => state.me?.modules.find((item) => item.key === key.value))
  const title = computed(() =>
    module.value ? (state.locale === 'zh' ? module.value.label : module.value.label_en) : key.value,
  )
  const filterFields = computed(
    () =>
      module.value?.fields.filter(
        (field) =>
          field.type === 'enum' &&
          !field.sensitive &&
          state.me?.permissions.some(
            (grant) =>
              grant.resource === key.value &&
              grant.action === 'read' &&
              (grant.read_fields?.includes('*') || grant.read_fields?.includes(field.key)),
          ),
      ) ?? [],
  )
  const filters = computed(() =>
    Object.fromEntries(
      filterFields.value.flatMap((field) =>
        typeof route.query[field.key] === 'string'
          ? [[field.key, String(route.query[field.key])]]
          : [],
      ),
    ),
  )
  const queue = computed(() =>
    key.value === 'tickets' && typeof route.query.queue === 'string' ? route.query.queue : '',
  )
  const listFilters = computed(() => ({
    ...filters.value,
    ...(queue.value ? { queue: queue.value } : {}),
  }))
  async function setQueue(value: string) {
    await router.replace({ query: { ...route.query, queue: value || undefined } })
  }
  async function setFilter(key: string, value: string) {
    await router.replace({ query: { ...route.query, [key]: value || undefined } })
  }
  async function clearFilters() {
    const query = { ...route.query }
    filterFields.value.forEach((field) => delete query[field.key])
    await router.replace({ query })
  }
  const list = useCollection<ResourceRecord>(
    () => `/resources/${key.value}`,
    () => allowed(key.value, 'read'),
    () => listFilters.value,
    () => context.value,
  )
  const { run, busy, error } = useRequest()
  const modal = ref(false)
  const current = ref<ResourceRecord | null>(null)
  const detail = ref<ResourceRecord | null>(null)
  const values = reactive<Record<string, unknown>>({})
  const dirtyFields = reactive(new Set<string>())
  const errors = ref<Record<string, string>>({})
  const conflict = ref(false)
  const detailConflict = ref(false)
  const owner = ref('')
  const department = ref('')
  const members = ref<Member[]>([])
  const departments = ref<Department[]>([])
  const importInput = ref<HTMLInputElement>()
  const exportOpen = ref(false)
  const columns = ref<string[]>([])
  const fields = computed(() =>
    module.value
      ? (current.value
          ? recordWritableFields(module.value, current.value)
          : editableFields(module.value, state.me?.permissions ?? [], 'create')
        ).filter((field) => !(key.value === 'tickets' && field.key === 'status'))
      : [],
  )
  const canAssign = computed(() =>
    current.value ? recordAllows(current.value, 'assign') : allowed(key.value, 'assign'),
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
    const epoch = revision
    await Promise.all([
      allowed('members', 'read')
        ? allPages<Member>('/members').then((result) => {
            if (valid(epoch)) members.value = result.filter((item) => item.status === 'active')
          })
        : Promise.resolve(),
      allowed('departments', 'read')
        ? allPages<Department>('/departments').then((result) => {
            if (valid(epoch)) departments.value = result
          })
        : Promise.resolve(),
    ])
  }
  async function edit(record?: ResourceRecord) {
    const epoch = revision
    await run(async () => {
      const result = record
        ? (await api<ResourceRecord>(`/resources/${key.value}/${record.id}`)).data
        : null
      if (!valid(epoch)) return
      current.value = result
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
      if (canAssign.value) await assignments()
      if (valid(epoch)) modal.value = true
    })
  }
  function updateValue(field: string, value: unknown) {
    values[field] = value
    dirtyFields.add(field)
  }
  async function view(record: ResourceRecord) {
    const epoch = revision
    const request = ++detailRevision
    await run(async () => {
      const result = (await api<ResourceRecord>(`/resources/${key.value}/${record.id}`)).data
      if (valid(epoch) && request === detailRevision) {
        detail.value = result
        detailConflict.value = false
      }
    })
  }
  async function save() {
    const epoch = revision
    const submittedFields = current.value
      ? fields.value.filter((field) => dirtyFields.has(field.key))
      : fields.value
    errors.value = validateResourceData(submittedFields, values)
    if (Object.keys(errors.value).length) return
    await run(async () => {
      const body: Record<string, unknown> = { data: buildResourceData(submittedFields, values) }
      if (current.value) body.version = current.value.version
      if (canAssign.value) {
        if (owner.value && owner.value !== current.value?.owner_id) body.owner_id = owner.value
        if (department.value !== (current.value?.department_id ?? ''))
          body.department_id = department.value || null
      }
      try {
        await api(`/resources/${key.value}${current.value ? `/${current.value.id}` : ''}`, {
          method: current.value ? 'PATCH' : 'POST',
          body,
        })
        if (!valid(epoch)) return
        modal.value = false
        saved()
        await list.load()
      } catch (failure) {
        if (valid(epoch) && failure instanceof APIError) {
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
  async function transition(record: ResourceRecord, status: string, resolution?: string) {
    if (
      !record.capabilities?.transitions.includes(status) ||
      (status === 'resolved' && !resolution?.trim())
    )
      return
    const epoch = revision
    await run(async () => {
      try {
        await api(`/resources/tickets/${record.id}/transition`, {
          method: 'POST',
          body: {
            version: record.version,
            status,
            ...(status === 'resolved' ? { resolution } : {}),
          },
        })
        if (!valid(epoch)) return
        saved()
        await list.load()
        await refreshDetail(record)
      } catch (failure) {
        if (valid(epoch) && failure instanceof APIError && failure.status === 409)
          detailConflict.value = true
        throw failure
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

  async function refreshDetail(record: ResourceRecord) {
    if (detail.value?.id !== record.id) return
    const epoch = revision
    const request = detailRevision
    try {
      const refreshed = (await api<ResourceRecord>(`/resources/${key.value}/${record.id}`)).data
      if (!valid(epoch) || request !== detailRevision || detail.value?.id !== record.id) return
      if (!recordAllows(refreshed, 'read')) closeDetail()
      else {
        detail.value = refreshed
        detailConflict.value = false
      }
    } catch (failure) {
      if (
        valid(epoch) &&
        request === detailRevision &&
        failure instanceof APIError &&
        [403, 404].includes(failure.status)
      )
        closeDetail()
      else throw failure
    }
  }
  function closeDetail() {
    detailRevision++
    detail.value = null
    detailConflict.value = false
    error.value = ''
  }
  async function updatedDetail(record: ResourceRecord) {
    if (detail.value?.id !== record.id) return
    if (recordAllows(record, 'read')) detail.value = record
    else closeDetail()
    await list.load()
  }
  async function reloadDetail() {
    if (detail.value) await run(() => refreshDetail(detail.value!))
  }
  async function operate(record: ResourceRecord, action: string) {
    if (!recordAllows(record, action)) return
    await run(async () => {
      await api(`/resources/${key.value}/${record.id}/actions/${action}`, {
        method: 'POST',
        body: { version: record.version, data: {} },
      })
      saved()
      await list.load()
      await refreshDetail(record)
    })
  }
  watch(
    context,
    () => {
      revision++
      closeDetail()
      modal.value = false
      current.value = null
      Object.keys(values).forEach((item) => delete values[item])
      dirtyFields.clear()
      members.value = []
      departments.value = []
      errors.value = {}
      conflict.value = false
      owner.value = ''
      department.value = ''
      exportOpen.value = false
      columns.value = []
    },
    { flush: 'sync' },
  )
  onBeforeUnmount(() => {
    live = false
    revision++
    detailRevision++
  })

  return {
    filterFields,
    filters,
    queue,
    setQueue,
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
    detailConflict,
    closeDetail,
    updatedDetail,
    reloadDetail,
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
  }
}
