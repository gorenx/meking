<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api, ApiError } from '../api/client'
import type { JournalEvent } from '../api/types'

const pageSize = 50
const events = ref<JournalEvent[]>([])
const zoneID = ref('')
const nextOffset = ref('0')
const hasMore = ref(false)
const loading = ref(false)
const error = ref('')
let controller: AbortController | null = null

async function load(append = false): Promise<void> {
  controller?.abort()
  const activeController = new AbortController()
  controller = activeController
  loading.value = true
  error.value = ''
  const offset = append ? nextOffset.value : '0'
  try {
    const page = await api.journalEvents(offset, pageSize, activeController.signal)
    zoneID.value = page.zone_id
    events.value = append ? [...events.value, ...page.events] : page.events
    nextOffset.value = page.next_offset
    hasMore.value = page.has_more
  } catch (cause) {
    if (activeController.signal.aborted) return
    error.value = cause instanceof ApiError ? cause.failure.message : '无法读取 Journal 事件'
  } finally {
    if (controller === activeController) loading.value = false
  }
}

function formatTime(value: string): string {
  const parsed = new Date(value)
  return Number.isNaN(parsed.valueOf()) ? value : parsed.toLocaleString()
}

function streamPath(streamID: string): string {
  return `/journal/streams/${streamID.split('/').map(encodeURIComponent).join('/')}`
}

onMounted(() => load())
onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <section class="page journal-page">
    <header class="page-header compact">
      <div>
        <p class="eyebrow">COMMITTED EVENTS</p>
        <h1>检查<em>事件日志</em></h1>
      </div>
      <div class="publication-context">
        <span>Zone</span>
        <code>{{ zoneID || '尚未读取' }}</code>
      </div>
    </header>

    <div class="journal-toolbar">
      <div>
        <strong>{{ events.length }} 个已加载事件</strong>
        <span>下一页 Offset {{ nextOffset }}</span>
      </div>
      <button class="outline-button" type="button" :disabled="loading" @click="load(false)">从头刷新</button>
    </div>

    <div v-if="error" class="failure-card" role="alert">
      <span>JOURNAL BROWSE</span>
      <strong>{{ error }}</strong>
    </div>

    <section class="journal-ledger" aria-label="Journal 事件列表">
      <article v-for="event in events" :key="event.event_id" class="journal-event">
        <header>
          <span>{{ event.type }} SEQUENCE {{ event.sequence }}</span>
          <time :datetime="event.occurred_at">{{ formatTime(event.occurred_at) }}</time>
        </header>
        <div class="journal-event-title">
          <strong>{{ event.type }}</strong>
          <span>v{{ event.schema_version }}</span>
        </div>
        <dl>
          <div><dt>Event</dt><dd><code>{{ event.event_id }}</code></dd></div>
          <div>
            <dt>Stream</dt>
            <dd>
              <a :href="streamPath(event.stream_id)"><code>{{ event.stream_id }}</code></a>
              <span>#{{ event.stream_sequence }}</span>
            </dd>
          </div>
          <div><dt>Correlation</dt><dd><code>{{ event.correlation_id || '—' }}</code></dd></div>
          <div><dt>Causation</dt><dd><code>{{ event.causation_id || '—' }}</code></dd></div>
        </dl>
      </article>
      <p v-if="!loading && events.length === 0" class="empty-state">Journal 中还没有已提交事件。</p>
      <div v-if="loading" class="journal-loading">正在读取已提交事件…</div>
    </section>

    <footer class="journal-footer">
      <span>页面 Offset 只用于浏览；事件 Sequence 在各 Zone 的 EventType 内独立递增。</span>
      <button class="outline-button" type="button" :disabled="loading || !hasMore" @click="load(true)">加载更多</button>
    </footer>
  </section>
</template>
