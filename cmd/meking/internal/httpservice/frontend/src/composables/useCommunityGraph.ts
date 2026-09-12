import { computed, ref } from 'vue'
import { api, ApiError } from '../api/client'
import type { GraphCommunity } from '../api/types'

type ChildPage = { next: number; loaded: number; total: number }

export function useCommunityGraph() {
  const structureID = ref('')
  const communities = ref<GraphCommunity[]>([])
  const search = ref('')
  const selectedID = ref<string>()
  const expanded = ref<string[]>([])
  const childPages = ref<Record<string, ChildPage>>({})
  const page = ref(1)
  const total = ref(0)
  const loadedPageItems = ref(0)
  const loading = ref(false)
  const error = ref('')
  let controller: AbortController | undefined

  const selected = computed(() => communities.value.find((item) => item.id === selectedID.value))
  const canLoadMore = computed(() => loadedPageItems.value < total.value)

  async function load(nextPage = 1, append = false): Promise<void> {
    const signal = begin()
    try {
      const result = await api.communityGraph({ query: search.value.trim() || undefined, page: nextPage, page_size: 50 }, signal)
      const changed = structureID.value !== '' && structureID.value !== result.structure_id
      structureID.value = result.structure_id
      communities.value = append && !changed ? merge(communities.value, result.communities) : result.communities
      loadedPageItems.value = append && !changed
        ? Math.min(result.total, loadedPageItems.value + result.communities.length)
        : result.communities.length
      page.value = result.page
      total.value = result.total
      if (!communities.value.some((item) => item.id === selectedID.value)) selectedID.value = undefined
    } catch (cause) {
      fail(cause, signal)
    } finally {
      finish(signal)
    }
  }

  async function toggleSelected(): Promise<void> {
    const current = selected.value
    if (!current || current.child_count === 0) return
    if (expanded.value.includes(current.id)) {
      collapse(current.id)
      return
    }
    const signal = begin()
    try {
      const result = await api.communityGraph({ parent_id: current.id, page: 1, page_size: 100 }, signal)
      if (structureID.value !== '' && structureID.value !== result.structure_id) {
        clear()
        structureID.value = result.structure_id
        await load()
        return
      }
      communities.value = merge(communities.value, result.communities)
      expanded.value = [...expanded.value, current.id]
      childPages.value = {
        ...childPages.value,
        [current.id]: { next: 2, loaded: result.communities.length, total: result.total },
      }
    } catch (cause) {
      fail(cause, signal)
    } finally {
      finish(signal)
    }
  }

  async function applySearch(): Promise<void> {
    expanded.value = []
    childPages.value = {}
    selectedID.value = undefined
    await load()
  }

  function collapse(id: string): void {
    const removed = new Set<string>()
    let found = true
    while (found) {
      found = false
      for (const item of communities.value) {
        if ((item.parent_id === id || (item.parent_id && removed.has(item.parent_id))) && !removed.has(item.id)) {
          removed.add(item.id)
          found = true
        }
      }
    }
    communities.value = communities.value.filter((item) => !removed.has(item.id))
    expanded.value = expanded.value.filter((item) => item !== id && !removed.has(item))
  }

  function clear(): void {
    communities.value = []
    selectedID.value = undefined
    expanded.value = []
    childPages.value = {}
    total.value = 0
    loadedPageItems.value = 0
  }

  function begin(): AbortSignal {
    controller?.abort()
    controller = new AbortController()
    loading.value = true
    error.value = ''
    return controller.signal
  }

  function finish(signal: AbortSignal): void {
    if (controller?.signal === signal) loading.value = false
  }

  function fail(cause: unknown, signal: AbortSignal): void {
    if (signal.aborted) return
    error.value = cause instanceof ApiError ? cause.failure.message : '无法读取当前 Community Structure'
  }

  return {
    structureID, communities, search, selectedID, selected, expanded, page, total, canLoadMore, loading, error,
    load, toggleSelected, applySearch, clear,
    select: (id: string | undefined) => { selectedID.value = id },
    dispose: () => controller?.abort(),
  }
}

function merge(current: GraphCommunity[], incoming: GraphCommunity[]): GraphCommunity[] {
  const values = new Map(current.map((item) => [item.id, item]))
  for (const item of incoming) values.set(item.id, item)
  return [...values.values()].sort((left, right) => left.level - right.level || left.number - right.number)
}
