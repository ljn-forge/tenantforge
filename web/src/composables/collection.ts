import { ref, watch, onBeforeUnmount } from 'vue'
import { api } from '../api/client'
import { useRequest } from './request'
export function useCollection<T>(
  path: () => string,
  enabled: () => boolean = () => true,
  filters: () => Record<string, string> = () => ({}),
  context: () => unknown = () => '',
) {
  const items = ref<T[]>([])
  const total = ref(0)
  const page = ref(1)
  const pageSize = ref(20)
  const query = ref('')
  const { run, busy, error } = useRequest()
  let live = true
  let revision = 0
  let queued = false
  async function load() {
    if (busy.value) {
      queued = true
      revision++
      return
    }
    const ownRevision = ++revision
    if (!enabled()) {
      items.value = []
      total.value = 0
      return
    }
    await run(async () => {
      const params = new URLSearchParams({
        page: String(page.value),
        page_size: String(pageSize.value),
      })
      if (query.value) params.set('q', query.value)
      for (const [key, value] of Object.entries(filters())) if (value) params.set(key, value)
      const result = await api<T[]>(`${path()}?${params}`)
      if (live && revision === ownRevision) {
        items.value = Array.isArray(result.data) ? result.data : []
        total.value = result.meta?.total ?? items.value.length
      }
    })
    if (queued && live) {
      queued = false
      await load()
    }
  }
  watch(
    [path, enabled, () => JSON.stringify(filters()), context],
    () => {
      items.value = []
      total.value = 0
      error.value = ''
      page.value = 1
      void load()
    },
    { immediate: true },
  )
  onBeforeUnmount(() => {
    live = false
    revision++
  })
  function search() {
    page.value = 1
    return load()
  }
  return { items, total, page, pageSize, query, load, search, busy, error }
}
