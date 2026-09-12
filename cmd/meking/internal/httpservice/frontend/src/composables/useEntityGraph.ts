import { computed, ref } from 'vue'
import { api, ApiError } from '../api/client'
import type { EntityGraph, EntityNeighborhood } from '../api/types'

const emptyGraph = (): EntityGraph => ({
  entities: [],
  relations: [],
  matched_entities: 0,
  matched_relations: 0,
  truncated: false,
})

export function useEntityGraph() {
  const graph = ref<EntityGraph>(emptyGraph())
  const search = ref('')
  const entityType = ref('')
  const selectedEntityID = ref<string>()
  const selectedRelationID = ref<string>()
  const loading = ref(false)
  const error = ref('')
  let controller: AbortController | undefined

  const selectedEntity = computed(() => graph.value.entities.find((item) => item.id === selectedEntityID.value))
  const selectedRelation = computed(() => graph.value.relations.find((item) => item.id === selectedRelationID.value))
  const entityTypes = computed(() => [...new Set(graph.value.entities.map((item) => item.type).filter(Boolean))].sort())

  async function load(): Promise<void> {
    const signal = begin()
    try {
      const result = await api.entityGraph({
        query: search.value.trim() || undefined,
        entity_type: entityType.value || undefined,
        limit: 100,
      }, signal)
      graph.value = result
      clearMissingSelection()
    } catch (cause) {
      fail(cause, signal)
    } finally {
      finish(signal)
    }
  }

  async function expandSelected(): Promise<void> {
    if (!selectedEntityID.value) return
    const signal = begin()
    try {
      const result = await api.entityNeighborhood(selectedEntityID.value, 100, signal)
      graph.value = graphFromNeighborhood(result)
    } catch (cause) {
      fail(cause, signal)
    } finally {
      finish(signal)
    }
  }

  async function reset(): Promise<void> {
    search.value = ''
    entityType.value = ''
    selectedEntityID.value = undefined
    selectedRelationID.value = undefined
    await load()
  }

  function select(value: string | undefined): void {
    if (value?.startsWith('entity:')) {
      selectedEntityID.value = value.slice(7)
      selectedRelationID.value = undefined
    } else if (value?.startsWith('relation:')) {
      selectedRelationID.value = value.slice(9)
      selectedEntityID.value = undefined
    } else {
      selectedEntityID.value = undefined
      selectedRelationID.value = undefined
    }
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
    error.value = cause instanceof ApiError ? cause.failure.message : '无法读取当前 Knowledge 实体图'
  }

  function clearMissingSelection(): void {
    if (!graph.value.entities.some((item) => item.id === selectedEntityID.value)) selectedEntityID.value = undefined
    if (!graph.value.relations.some((item) => item.id === selectedRelationID.value)) selectedRelationID.value = undefined
  }

  return {
    graph, search, entityType, selectedEntityID, selectedRelationID,
    selectedEntity, selectedRelation, entityTypes, loading, error,
    load, expandSelected, reset, select, dispose: () => controller?.abort(),
  }
}

function graphFromNeighborhood(value: EntityNeighborhood): EntityGraph {
  return {
    ...value,
    matched_entities: value.entities.length,
  }
}
