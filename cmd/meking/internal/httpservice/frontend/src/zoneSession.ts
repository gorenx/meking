import { readonly, ref } from 'vue'

const storageKey = 'meking.active-zone'

function storedZoneID(): string {
  try {
    return globalThis.localStorage?.getItem(storageKey) ?? ''
  } catch {
    return ''
  }
}

const activeZoneID = ref(storedZoneID())

export function selectZone(id: string) {
  activeZoneID.value = id
  try {
    globalThis.localStorage?.setItem(storageKey, id)
  } catch {
    // The in-memory selection remains usable when storage is unavailable.
  }
}

export function clearZone() {
  activeZoneID.value = ''
  try {
    globalThis.localStorage?.removeItem(storageKey)
  } catch {
    // The in-memory selection is already cleared.
  }
}

export function currentZoneID(): string {
  return activeZoneID.value
}

export function zoneAPIPath(path: string): string {
  const id = currentZoneID()
  if (!id) throw new Error('请先选择一个 Zone')
  return `/api/v1/zones/${encodeURIComponent(id)}${path}`
}

export function useZoneSession() {
  return { activeZoneID: readonly(activeZoneID), selectZone, clearZone }
}
