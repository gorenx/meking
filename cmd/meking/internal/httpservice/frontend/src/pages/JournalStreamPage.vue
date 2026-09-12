<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { api, ApiError } from '../api/client'
import type { JournalEvent } from '../api/types'

const pageSize = 100
const route = useRoute()
const streamID = computed(() => typeof route.params.id === 'string' ? route.params.id : '')
const events = ref<JournalEvent[]>([])
const zoneID = ref('')
const nextAfterSequence = ref('0')
const hasMore = ref(false)
const loading = ref(false)
const error = ref('')
let controller: AbortController | null = null

async function readNext(append: boolean): Promise<void> {
  if (!streamID.value) {
    error.value = 'Stream ID 不能为空'
    return
  }
  controller?.abort()
  const activeController = new AbortController()
  controller = activeController
  loading.value = true
  error.value = ''
  const after = append ? nextAfterSequence.value : '0'
  try {
    const page = await api.journalStream(streamID.value, after, pageSize, activeController.signal)
    zoneID.value = page.zone_id
    events.value = append ? [...events.value, ...page.events] : page.events
    nextAfterSequence.value = page.next_after_sequence
    hasMore.value = page.has_more
  } catch (cause) {
    if (activeController.signal.aborted) return
    error.value = cause instanceof ApiError ? cause.failure.message : '无法读取 Stream 时间线'
  } finally {
    if (controller === activeController) loading.value = false
  }
}

function formatTime(value: string): string {
  const parsed = new Date(value)
  return Number.isNaN(parsed.valueOf()) ? value : parsed.toLocaleString()
}

watch(streamID, () => {
  events.value = []
  zoneID.value = ''
  nextAfterSequence.value = '0'
  hasMore.value = false
  void readNext(false)
}, { immediate: true })
onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <section class="page journal-stream-page">
    <RouterLink class="back-button knowledge-back" to="/journal">← 返回事件日志</RouterLink>
    <header class="page-header compact">
      <div>
        <p class="eyebrow">STREAM TIMELINE</p>
        <h1>追踪<em>持久化事实</em></h1>
      </div>
      <div class="publication-context">
        <span>已读取至 Sequence</span>
        <code>{{ nextAfterSequence }}</code>
      </div>
    </header>

    <section class="stream-identity">
      <span>Stream</span>
      <code>{{ streamID }}</code>
      <small>{{ zoneID ? `Zone ${zoneID}` : '尚未读取 Zone' }}</small>
    </section>

    <div v-if="error" class="failure-card" role="alert">
      <span>STREAM BROWSE</span>
      <strong>{{ error }}</strong>
    </div>

    <ol class="stream-timeline">
      <li v-for="(event, index) in events" :key="event.event_id">
        <div class="stream-marker">{{ event.stream_sequence }}</div>
        <article class="stream-fact">
          <header>
            <div>
              <strong>{{ event.type }}</strong>
              <span>v{{ event.schema_version }} · EventType Sequence {{ event.sequence }}</span>
            </div>
            <time :datetime="event.occurred_at">{{ formatTime(event.occurred_at) }}</time>
          </header>
          <dl>
            <div><dt>Event</dt><dd><code>{{ event.event_id }}</code></dd></div>
            <div><dt>Correlation</dt><dd><code>{{ event.correlation_id || '—' }}</code></dd></div>
            <div><dt>Causation</dt><dd><code>{{ event.causation_id || '—' }}</code></dd></div>
          </dl>
          <strong v-if="index === events.length - 1" class="stream-tail">
            {{ hasMore ? '当前已加载末尾' : '最后持久化事实' }}
          </strong>
        </article>
      </li>
    </ol>

    <p v-if="!loading && events.length === 0" class="empty-state">该 Stream 尚无已提交事件。</p>
    <div v-if="loading" class="stream-loading">正在续读 Stream…</div>

    <footer class="journal-footer">
      <span>Sequence 连续只说明 Stream 事件无缺口，不表示领域工作已经完成。</span>
      <button class="outline-button" type="button" :disabled="loading" @click="readNext(true)">
        {{ hasMore ? '继续读取时间线' : '刷新后续事件' }}
      </button>
    </footer>
  </section>
</template>
