import { computed, ref, watch, onBeforeUnmount } from 'vue'
import { api, APIError, clientContext } from '../api/client'
import type { Job, JobType } from '../api/types'
import { buildResourceData, validateResourceData } from '../api/forms'
import { state, allowed, t } from '../stores/app'
import { saved } from './request'

export function useJobCatalog(enabled: () => boolean, afterCreate: () => Promise<void>) {
  const types = ref<JobType[]>([])
  const loading = ref(false)
  const catalogError = ref('')
  const loaded = ref(false)
  const open = ref(false)
  const selectedType = ref('')
  const values = ref<Record<string, unknown>>({})
  const fieldErrors = ref<Record<string, string>>({})
  const submitError = ref('')
  const submitting = ref(false)
  const selected = computed(() => types.value.find((item) => item.type === selectedType.value))
  const available = computed(
    () =>
      enabled() &&
      allowed('jobs', 'create') &&
      Boolean(state.me?.user?.id) &&
      Boolean(state.tenant) &&
      clientContext.value.tenant === state.tenant,
  )
  let revision = 0
  let loadID = 0
  let live = true
  function current(token: number, epoch: number) {
    return live && token === revision && epoch === clientContext.value.epoch && available.value
  }
  function clearDraft() {
    open.value = false
    selectedType.value = ''
    values.value = {}
    fieldErrors.value = {}
    submitError.value = ''
    submitting.value = false
  }
  function reset() {
    revision++
    loadID++
    clearDraft()
    types.value = []
    loading.value = false
    catalogError.value = ''
    loaded.value = false
  }
  function failureMessage(failure: unknown) {
    return failure instanceof Error
      ? failure.message
      : t('请求失败，请重试', 'Request failed. Please try again.')
  }
  function changed(failure: unknown) {
    return (
      (failure instanceof APIError && failure.code === 'context_changed') ||
      (failure instanceof DOMException && failure.name === 'AbortError')
    )
  }
  async function load() {
    if (!available.value) return
    const token = revision,
      epoch = clientContext.value.epoch,
      request = ++loadID
    loading.value = true
    catalogError.value = ''
    try {
      const result = await api<JobType[]>('/job-types')
      if (!current(token, epoch) || request !== loadID) return
      types.value = Array.isArray(result.data) ? result.data : []
      loaded.value = true
    } catch (failure) {
      if (current(token, epoch) && request === loadID && !changed(failure))
        catalogError.value = failureMessage(failure)
    } finally {
      if (current(token, epoch) && request === loadID) loading.value = false
    }
  }
  function choose(type: string) {
    selectedType.value = type
    values.value = Object.fromEntries(
      (selected.value?.input_fields ?? [])
        .filter((field) => field.type === 'boolean')
        .map((field) => [field.key, false]),
    )
    fieldErrors.value = {}
    submitError.value = ''
  }
  function show() {
    if (!available.value || !types.value.length) return
    choose(types.value[0]!.type)
    open.value = true
  }
  function close() {
    if (!submitting.value) clearDraft()
  }
  function change(field: string, value: unknown) {
    values.value = { ...values.value, [field]: value }
    delete fieldErrors.value[field]
  }
  async function submit() {
    if (!available.value || !open.value || !selected.value || submitting.value) return
    const definition = selected.value
    fieldErrors.value = validateResourceData(definition.input_fields, values.value)
    submitError.value = ''
    if (Object.keys(fieldErrors.value).length) return
    const token = revision,
      epoch = clientContext.value.epoch
    submitting.value = true
    try {
      await api<Job>('/jobs', {
        method: 'POST',
        body: {
          type: definition.type,
          payload: buildResourceData(definition.input_fields, values.value),
        },
      })
      if (!current(token, epoch)) return
      clearDraft()
      saved()
      await afterCreate()
    } catch (failure) {
      if (!current(token, epoch) || changed(failure)) return
      if (failure instanceof APIError && failure.fields) {
        fieldErrors.value = Object.fromEntries(
          definition.input_fields
            .filter(
              (field) => failure.fields?.[field.key] || failure.fields?.[`payload.${field.key}`],
            )
            .map((field) => [
              field.key,
              failure.fields![field.key] ?? failure.fields![`payload.${field.key}`]!,
            ]),
        )
      }
      submitError.value = failureMessage(failure)
    } finally {
      if (current(token, epoch)) submitting.value = false
    }
  }
  watch(
    [
      available,
      () => state.tenant,
      () => state.me,
      () => JSON.stringify(state.me?.permissions),
      () => clientContext.value,
      enabled,
    ],
    () => {
      reset()
      if (available.value) void load()
    },
    { immediate: true, flush: 'sync' },
  )
  onBeforeUnmount(() => {
    live = false
    reset()
  })
  return {
    types,
    loading,
    loaded,
    catalogError,
    available,
    open,
    selectedType,
    selected,
    values,
    fieldErrors,
    submitError,
    submitting,
    load,
    choose,
    show,
    close,
    change,
    submit,
  }
}
