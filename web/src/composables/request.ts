import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { APIError } from '../api/client'
import { t } from '../stores/app'

export function useRequest() {
  const busy = ref(false)
  const error = ref('')
  async function run<T>(operation: () => Promise<T>): Promise<T | undefined> {
    if (busy.value) return undefined
    busy.value = true
    error.value = ''
    try {
      return await operation()
    } catch (failure) {
      if (
        (failure instanceof DOMException && failure.name === 'AbortError') ||
        (failure instanceof APIError && failure.code === 'context_changed')
      )
        return undefined
      error.value =
        failure instanceof Error
          ? failure.message
          : t('请求失败，请重试', 'Request failed. Please try again.')
      ElMessage.error(error.value)
      return undefined
    } finally {
      busy.value = false
    }
  }
  return { busy, error, run }
}
export async function confirmDelete(label: string): Promise<boolean> {
  try {
    await ElMessageBox.confirm(
      t(`确认删除“${label}”？`, `Delete “${label}”?`),
      t('确认删除', 'Confirm deletion'),
      {
        type: 'warning',
        confirmButtonText: t('删除', 'Delete'),
        cancelButtonText: t('取消', 'Cancel'),
      },
    )
    return true
  } catch {
    return false
  }
}
export function saved() {
  ElMessage.success(t('已保存', 'Saved'))
}
