<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, ApiError } from '../api/client'
import type { ApiFailure, ZoneDefinition } from '../api/types'
import { clearZone, currentZoneID, selectZone } from '../zoneSession'

const router = useRouter()
const zones = ref<ZoneDefinition[]>([])
const loading = ref(false)
const creating = ref(false)
const parentZoneID = ref('')
const failure = ref<ApiFailure>()

const roots = computed(() => zones.value.filter((zone) => zone.role === 'root'))

function showFailure(cause: unknown, fallback: string) {
  failure.value = cause instanceof ApiError
    ? cause.failure
    : { code: 'client_failure', message: cause instanceof Error ? cause.message : fallback }
}

async function loadZones() {
  loading.value = true
  failure.value = undefined
  try {
    const result = await api.zones()
    zones.value = result.zones
    const active = currentZoneID()
    if (active && !zones.value.some((zone) => zone.id === active)) clearZone()
  } catch (cause) {
    showFailure(cause, '无法读取 Zone')
  } finally {
    loading.value = false
  }
}

async function create(parentID?: string) {
  if (creating.value) return
  creating.value = true
  failure.value = undefined
  try {
    const created = await api.createZone(parentID)
    zones.value = [...zones.value, created]
    parentZoneID.value = ''
    selectZone(created.id)
    await router.push('/documents')
  } catch (cause) {
    showFailure(cause, '无法创建 Zone')
  } finally {
    creating.value = false
  }
}

async function openZone(zone: ZoneDefinition) {
  selectZone(zone.id)
  await router.push('/documents')
}

function formatTime(value: string): string {
  const parsed = new Date(value)
  return Number.isNaN(parsed.valueOf()) ? value : parsed.toLocaleString()
}

onMounted(loadZones)
</script>

<template>
  <section class="page zones-page">
    <header class="page-header compact">
      <div>
        <p class="eyebrow">KNOWLEDGE BOUNDARIES</p>
        <h1>选择<em>知识空间</em></h1>
      </div>
      <div class="publication-context">
        <span>当前结构</span>
        <code>Root → Child</code>
      </div>
    </header>

    <section class="zone-creation-rail" aria-label="创建 Zone">
      <article>
        <span class="rail-marker">ROOT</span>
        <div>
          <h2>创建独立 Zone</h2>
          <p>Root 拥有自己的文档选择和知识发布，也可以接收一级 Child 的稳定结果。</p>
        </div>
        <button class="submit-button" type="button" :disabled="creating" @click="create()">
          创建 Root Zone
        </button>
      </article>
      <article>
        <span class="rail-marker">CHILD</span>
        <div>
          <h2>创建 Child Zone</h2>
          <p>Child 只能直属一个 Root；它先独立构建，再由 Root 明确接收。</p>
        </div>
        <label>
          <span>Parent Root</span>
          <select v-model="parentZoneID" :disabled="creating || roots.length === 0">
            <option value="">选择 Root Zone</option>
            <option v-for="root in roots" :key="root.id" :value="root.id">{{ root.id }}</option>
          </select>
        </label>
        <button
          class="outline-button"
          type="button"
          :disabled="creating || !parentZoneID"
          @click="create(parentZoneID)"
        >
          创建 Child Zone
        </button>
      </article>
    </section>

    <div v-if="failure" class="failure-card" role="alert">
      <span>{{ failure.code }}</span>
      <strong>{{ failure.message }}</strong>
    </div>

    <section class="zone-catalog" aria-labelledby="zone-catalog-heading">
      <header>
        <div>
          <p class="eyebrow">ZONE CATALOG</p>
          <h2 id="zone-catalog-heading">可用知识空间</h2>
        </div>
        <button class="text-button" type="button" :disabled="loading" @click="loadZones">刷新</button>
      </header>
      <div v-if="zones.length" class="zone-grid">
        <article v-for="zone in zones" :key="zone.id" :class="['zone-card', zone.role]">
          <header>
            <span>{{ zone.role }}</span>
            <time :datetime="zone.created_at">{{ formatTime(zone.created_at) }}</time>
          </header>
          <code>{{ zone.id }}</code>
          <p v-if="zone.parent_zone_id">Parent <code>{{ zone.parent_zone_id }}</code></p>
          <p v-else>独立 Root，可创建一级 Child。</p>
          <button class="outline-button" type="button" @click="openZone(zone)">查看 Zone 文档</button>
        </article>
      </div>
      <p v-else-if="!loading" class="empty-state">还没有 Zone。先创建一个 Root Zone。</p>
      <div v-if="loading" class="journal-loading">正在读取 Zone…</div>
    </section>
  </section>
</template>
