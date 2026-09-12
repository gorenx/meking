<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import type { GraphCommunity, GraphEntity, GraphRelation } from '../api/types'

const props = defineProps<{
  mode: 'entities' | 'communities'
  entities: GraphEntity[]
  relations: GraphRelation[]
  communities: GraphCommunity[]
  selected?: string
}>()

const emit = defineEmits<{ select: [value: string | undefined] }>()
const width = 1000
const height = 620
const entityPadding = 16
const entityMinSeparation = 10
interface Point { x: number; y: number }

interface DragState {
  pointerId: number
  entityID: string
  offsetX: number
  offsetY: number
  startX: number
  startY: number
  moved: boolean
}

const entityPositions = ref<Map<string, Point>>(new Map())
const dragging = ref<DragState | null>(null)
const canvas = ref<SVGSVGElement | null>(null)

const entityRadiusByID = computed(() => {
  const values = new Map<string, number>()
  for (const entity of props.entities) {
    values.set(entity.id, Math.max(25, Math.min(48, 25 + entity.degree)))
  }
  return values
})

const hashModulus = 2147483647

function deterministicIndex(value: string): number {
  let hash = 0
  for (let index = 0; index < value.length; index += 1) {
    hash = (hash * 31 + value.charCodeAt(index)) % hashModulus
  }
  return hash
}

function computeEntityPositions() {
  const result = new Map<string, Point>()
  if (props.entities.length === 0) {
    entityPositions.value = result
    return
  }

  const basePoints = props.entities.map((entity, index) => {
    const indexPlusOne = index + 1
    const radius = Math.min(width, height) * 0.39
    const angle = (2 * Math.PI * indexPlusOne / props.entities.length) - Math.PI / 2
    return {
      id: entity.id,
      x: width / 2 + radius * Math.cos(angle),
      y: height / 2 + radius * Math.sin(angle),
      vx: 0,
      vy: 0,
      r: Math.max(25, Math.min(48, 25 + entity.degree)),
    }
  })

  for (let step = 0; step < 160; step += 1) {
    for (let i = 0; i < basePoints.length; i += 1) {
      for (let j = i + 1; j < basePoints.length; j += 1) {
        const left = basePoints[i]
        const right = basePoints[j]
        const dx = left.x - right.x
        const dy = left.y - right.y
        const squaredDistance = dx * dx + dy * dy
        if (squaredDistance === 0) {
          continue
        }
        const distance = Math.sqrt(squaredDistance)
        const required = left.r + right.r + entityMinSeparation
        if (distance >= required) {
          continue
        }
        const push = (required - distance) / distance * 0.25
        const fx = dx * push
        const fy = dy * push
        left.vx += fx
        left.vy += fy
        right.vx -= fx
        right.vy -= fy
      }
    }

    const centerX = width / 2
    const centerY = height / 2
    const damping = 0.9
    basePoints.forEach((point) => {
      point.vx += (centerX - point.x) * 0.0004
      point.vy += (centerY - point.y) * 0.0004
      point.x += point.vx
      point.y += point.vy
      point.vx *= damping
      point.vy *= damping

      const minX = entityPadding + point.r
      const minY = entityPadding + point.r
      const maxX = width - entityPadding - point.r
      const maxY = height - entityPadding - point.r
      point.x = Math.max(minX, Math.min(maxX, point.x))
      point.y = Math.max(minY, Math.min(maxY, point.y))
    })
  }

  for (const point of basePoints) {
    result.set(point.id, { x: point.x, y: point.y })
  }
  entityPositions.value = result
}

function clampEntityPosition(value: Point, radius: number): Point {
  const minX = entityPadding + radius
  const minY = entityPadding + radius
  const maxX = width - entityPadding - radius
  const maxY = height - entityPadding - radius
  return {
    x: Math.max(minX, Math.min(maxX, value.x)),
    y: Math.max(minY, Math.min(maxY, value.y)),
  }
}

function canvasPosition(clientX: number, clientY: number): Point | null {
  if (!canvas.value) {
    return null
  }
  const box = canvas.value.getBoundingClientRect()
  return {
    x: (clientX - box.left) / box.width * width,
    y: (clientY - box.top) / box.height * height,
  }
}

function beginDrag(event: PointerEvent, entityID: string) {
  if (event.button !== 0) {
    return
  }
  const current = entityPositions.value.get(entityID)
  const origin = canvasPosition(event.clientX, event.clientY)
  if (!current || !origin) {
    return
  }
  dragging.value = {
    pointerId: event.pointerId,
    entityID,
    offsetX: origin.x - current.x,
    offsetY: origin.y - current.y,
    startX: current.x,
    startY: current.y,
    moved: false,
  }
}

function moveDraggedEntity(event: PointerEvent) {
  if (!dragging.value) {
    return
  }
  const active = dragging.value
  const position = canvasPosition(event.clientX, event.clientY)
  if (!position) {
    return
  }
  if (!entityPositions.value.has(active.entityID)) {
    return
  }
  const clamped = clampEntityPosition(
    {
      x: position.x - active.offsetX,
      y: position.y - active.offsetY,
    },
    entityRadiusByID.value.get(active.entityID) ?? 30,
  )
  if (!active.moved) {
    active.moved = Math.hypot(
      clamped.x - active.startX,
      clamped.y - active.startY,
    ) > 4
    if (
      active.moved
      && canvas.value
      && typeof canvas.value.hasPointerCapture === 'function'
      && typeof canvas.value.setPointerCapture === 'function'
      && !canvas.value.hasPointerCapture(active.pointerId)
    ) {
      canvas.value.setPointerCapture(active.pointerId)
    }
  }
  entityPositions.value.set(active.entityID, clamped)
}

function finishDrag(event: PointerEvent) {
  if (!dragging.value || event.pointerId !== dragging.value.pointerId) {
    return
  }
  if (
    canvas.value
    && typeof canvas.value.hasPointerCapture === 'function'
    && canvas.value.hasPointerCapture(event.pointerId)
  ) {
    canvas.value.releasePointerCapture(event.pointerId)
  }
  dragging.value = null
}

function handleEntitySelect(entityID: string) {
  emit('select', `entity:${entityID}`)
}

function handleEntityPointerSelect(entityID: string, event: PointerEvent) {
  const active = dragging.value
  if (!active || active.entityID !== entityID || active.pointerId !== event.pointerId) {
    return
  }
  if (!active.moved) {
    emit('select', `entity:${entityID}`)
  }
  if (
    canvas.value
    && typeof canvas.value.hasPointerCapture === 'function'
    && typeof canvas.value.releasePointerCapture === 'function'
    && canvas.value.hasPointerCapture(event.pointerId)
  ) {
    canvas.value.releasePointerCapture(event.pointerId)
  }
  dragging.value = null
}

function handleRelationSelect(relationID: string) {
  emit('select', `relation:${relationID}`)
}

function selectByCanvasTarget(event: Event): void {
  if (!(event.target instanceof Element)) {
    return
  }
  const selected = event.target.closest('[data-select-kind]')
  if (!selected) {
    return
  }
  const kind = selected.getAttribute('data-select-kind')
  const id = selected.getAttribute('data-select-id')
  if (!kind || !id) {
    return
  }
  emit('select', `${kind}:${id}`)
}

function visibleRadius(entityID: string) {
  return entityRadiusByID.value.get(entityID) ?? 30
}

function fallbackPointFor(id: string, communities = false): Point {
  const values = communities ? props.communities : props.entities
  if (values.length === 0) {
    return { x: width / 2, y: height / 2 }
  }
  const index = deterministicIndex(id)
  const angle = (2 * Math.PI * (index % values.length)) / values.length
  const radius = Math.max(90, Math.min(width, height) * 0.35)
  return {
    x: width / 2 + radius * Math.cos(angle),
    y: height / 2 + radius * Math.sin(angle),
  }
}

function visiblePoint(id: string, communities = false): Point {
  const points = communities ? communityPositions.value : entityPositions.value
  return points.get(id) ?? fallbackPointFor(id, communities)
}

watch(
  () => props.entities,
  () => {
    dragging.value = null
    computeEntityPositions()
  },
  { immediate: true, deep: true },
)

onBeforeUnmount(() => {
  dragging.value = null
})

const communityPositions = computed(() => {
  const result = new Map<string, Point>()
  const levels = [...new Set(props.communities.map((item) => item.level))].sort((a, b) => a - b)
  levels.forEach((level, levelIndex) => {
    const items = props.communities.filter((item) => item.level === level)
    items.forEach((item, index) => {
      result.set(item.id, {
        x: width * (index + 1) / (items.length + 1),
        y: 90 + levelIndex * Math.max(130, (height - 170) / Math.max(1, levels.length - 1)),
      })
    })
  })
  return result
})

const visibleRelations = computed(() => props.relations.filter((item) =>
  entityPositions.value.has(item.source_entity_id) && entityPositions.value.has(item.target_entity_id),
))

const visibleCommunityEdges = computed(() => props.communities.filter((item) =>
  item.parent_id && communityPositions.value.has(item.parent_id),
))

function short(value: string, limit = 20): string {
  return value.length > limit ? `${value.slice(0, limit - 1)}…` : value
}
</script>

<template>
  <svg
    class="graph-svg"
    :viewBox="`0 0 ${width} ${height}`"
    role="img"
    :aria-label="mode === 'entities' ? '有向实体关系图' : 'Community 层级图'"
    @click.self="emit('select', undefined)"
    @click.capture="selectByCanvasTarget"
    ref="canvas"
    @pointermove="moveDraggedEntity"
    @pointerup="finishDrag"
    @pointercancel="finishDrag"
  >
    <defs>
      <marker id="graph-arrow" viewBox="0 0 10 10" refX="17" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
        <path d="M 0 0 L 10 5 L 0 10 z" fill="#555" />
      </marker>
    </defs>

    <template v-if="mode === 'entities'">
      <g
        v-for="relation in visibleRelations"
        :key="relation.id"
        class="graph-edge"
        :class="{ selected: selected === `relation:${relation.id}` }"
        role="button"
        tabindex="0"
        :data-select-kind="'relation'"
        :data-select-id="relation.id"
        @click.stop="handleRelationSelect(relation.id)"
        @pointerup="handleRelationSelect(relation.id)"
        @keydown.enter="handleRelationSelect(relation.id)"
      >
        <title>{{ relation.description || relation.id }}</title>
        <line
          class="graph-edge-hit-area"
          :x1="visiblePoint(relation.source_entity_id).x"
          :y1="visiblePoint(relation.source_entity_id).y"
          :x2="visiblePoint(relation.target_entity_id).x"
          :y2="visiblePoint(relation.target_entity_id).y"
          data-select-kind="relation"
          :data-select-id="relation.id"
          @click.stop="handleRelationSelect(relation.id)"
          @pointerup="handleRelationSelect(relation.id)"
        />
        <line
          class="graph-edge-visible"
          :x1="visiblePoint(relation.source_entity_id).x"
          :y1="visiblePoint(relation.source_entity_id).y"
          :x2="visiblePoint(relation.target_entity_id).x"
          :y2="visiblePoint(relation.target_entity_id).y"
          marker-end="url(#graph-arrow)"
          data-select-kind="relation"
          :data-select-id="relation.id"
          @click.stop="handleRelationSelect(relation.id)"
          @pointerup="handleRelationSelect(relation.id)"
        />
      </g>
      <g
        v-for="entity in entities"
        :key="entity.id"
        class="graph-node entity-node"
        :class="{ selected: selected === `entity:${entity.id}` }"
        :transform="`translate(${visiblePoint(entity.id).x} ${visiblePoint(entity.id).y})`"
        role="button"
        tabindex="0"
        :data-select-kind="'entity'"
        :data-select-id="entity.id"
        @pointerdown="beginDrag($event, entity.id)"
        @pointerup="handleEntityPointerSelect(entity.id, $event)"
        @click.stop="handleEntitySelect(entity.id)"
        @keydown.enter="handleEntitySelect(entity.id)"
      >
        <title>{{ entity.title }} · v{{ entity.version }}</title>
        <circle :r="visibleRadius(entity.id)" />
        <text text-anchor="middle" y="4">{{ short(entity.title, 16) }}</text>
      </g>
    </template>

    <template v-else>
      <line
        v-for="community in visibleCommunityEdges"
        :key="`edge:${community.id}`"
        class="community-edge"
        :x1="visiblePoint(community.parent_id!, true).x"
        :y1="visiblePoint(community.parent_id!, true).y"
        :x2="visiblePoint(community.id, true).x"
        :y2="visiblePoint(community.id, true).y"
      />
      <g
        v-for="community in communities"
        :key="community.id"
        class="graph-node community-node"
        :class="{ selected: selected === `community:${community.id}` }"
        :transform="`translate(${visiblePoint(community.id, true).x} ${visiblePoint(community.id, true).y})`"
        role="button"
        tabindex="0"
        :data-select-kind="'community'"
        :data-select-id="community.id"
        @click.stop="emit('select', `community:${community.id}`)"
        @keydown.enter="emit('select', `community:${community.id}`)"
      >
        <title>{{ community.id }} · Level {{ community.level }}</title>
        <rect x="-74" y="-30" width="148" height="60" rx="7" />
        <text text-anchor="middle" y="-3">{{ short(community.id, 20) }}</text>
        <text class="node-note" text-anchor="middle" y="16">L{{ community.level }} · {{ community.entity_count }} entities</text>
      </g>
    </template>
  </svg>
</template>
