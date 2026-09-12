import { readonly, ref, watch } from 'vue'
import { api, ApiError } from '../api/client'
import type { RuntimeInfo } from '../api/types'
import { useZoneSession } from '../zoneSession'

const runtime = ref<RuntimeInfo | null>(null)
const loading = ref(false)
const error = ref('')
const refreshIntervalMilliseconds = 2_000
let refreshTimer: ReturnType<typeof setInterval> | null = null
const { activeZoneID } = useZoneSession()

async function refresh() {
  if (loading.value) return
  if (!activeZoneID.value) {
    runtime.value = null
    error.value = ''
    return
  }
  loading.value = true
  error.value = ''
  try {
    runtime.value = await api.runtime()
  } catch (cause) {
    error.value = cause instanceof ApiError ? cause.failure.message : '无法读取 Zone 状态'
  } finally {
    loading.value = false
  }
}

function startPolling() {
  void refresh()
  if (refreshTimer !== null) return
  refreshTimer = setInterval(() => void refresh(), refreshIntervalMilliseconds)
}

function stopPolling() {
  if (refreshTimer === null) return
  clearInterval(refreshTimer)
  refreshTimer = null
}

watch(activeZoneID, () => {
  runtime.value = null
  void refresh()
})

export function useRuntime() {
  return {
    runtime: readonly(runtime),
    loading: readonly(loading),
    error: readonly(error),
    refresh,
    startPolling,
    stopPolling,
  }
}
