<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, ApiError } from '../api/client'
import type { ReportDetail, ReportPage } from '../api/types'

const reportPageSize = 10
const route = useRoute()
const router = useRouter()
const page = ref<ReportPage | null>(null)
const detail = ref<ReportDetail | null>(null)
const loading = ref(false)
const error = ref('')
const selectedID = computed(() => typeof route.params.id === 'string' ? route.params.id : '')
const selectedEpochID = computed(() => Number(route.query.epoch_id))
const canReadPreviousPage = computed(() => (page.value?.page ?? 1) > 1)
const canReadNextPage = computed(() => {
  if (!page.value) return false
  return page.value.page * page.value.page_size < page.value.total
})

async function loadPage(nextPage = 1) {
  loading.value = true
  error.value = ''
  try {
    page.value = await api.reports(nextPage, reportPageSize)
  } catch (cause) {
    error.value = cause instanceof ApiError ? cause.failure.message : '无法读取社区报告'
  } finally {
    loading.value = false
  }
}

async function loadDetail(id: string, epochID: number) {
  if (!id) {
    detail.value = null
    return
  }
  if (!Number.isInteger(epochID) || epochID <= 0) {
    detail.value = null
    error.value = '报告详情缺少固定 Epoch'
    return
  }
  loading.value = true
  error.value = ''
  try {
    detail.value = await api.report(id, epochID)
  } catch (cause) {
    error.value = cause instanceof ApiError ? cause.failure.message : '无法读取报告详情'
  } finally {
    loading.value = false
  }
}

function select(id: string) {
  router.push({ name: 'reports', params: { id }, query: { epoch_id: page.value?.epoch_id } })
}

onMounted(loadPage)
watch(
  () => [selectedID.value, selectedEpochID.value] as const,
  ([id, epochID]) => loadDetail(id, epochID),
  { immediate: true },
)
</script>

<template>
  <section class="page reports-page">
    <header class="page-header compact">
      <div>
        <p class="eyebrow">COMMUNITY REPORTS</p>
        <h1>图的<em>结构化叙事</em></h1>
      </div>
      <div class="report-total">
        <strong>{{ page?.total ?? '—' }}</strong>
        <span>份已发布报告 · {{ page?.epoch_id ? `Epoch ${page.epoch_id}` : '尚未加载' }}</span>
      </div>
    </header>

    <div v-if="error" class="failure-card"><span>读取失败</span><strong>{{ error }}</strong></div>

    <div class="reports-layout">
      <div class="report-index" :class="{ hidden: detail }">
        <button
          v-for="report in page?.reports"
          :key="report.id"
          type="button"
          :class="{ active: report.id === selectedID }"
          @click="select(report.id)"
        >
          <span class="report-number">{{ String(report.community_number).padStart(2, '0') }}</span>
          <span class="report-copy">
            <strong>{{ report.title }}</strong>
            <small>{{ report.summary }}</small>
          </span>
          <span class="report-rank">{{ report.rank.toFixed(1) }}</span>
        </button>
        <p v-if="!loading && !page?.reports.length" class="empty-state">当前 ReportSet 没有可见社区报告。</p>
        <footer class="report-pagination">
          <button class="outline-button" type="button" :disabled="loading || !canReadPreviousPage" @click="loadPage((page?.page ?? 1) - 1)">上一页</button>
          <span>第 {{ page?.page ?? 1 }} 页 · {{ page?.total ?? 0 }} 份</span>
          <button class="outline-button" type="button" :disabled="loading || !canReadNextPage" @click="loadPage((page?.page ?? 0) + 1)">下一页</button>
        </footer>
      </div>

      <article v-if="detail" class="report-detail">
        <button class="back-button" type="button" @click="router.push('/reports')">← 返回报告列表</button>
        <div class="report-detail-heading">
          <div>
            <p>COMMUNITY {{ detail.community_number }} · LEVEL {{ detail.level }}</p>
            <h2>{{ detail.title }}</h2>
          </div>
          <span>{{ detail.rank.toFixed(1) }}</span>
        </div>
        <p class="report-summary">{{ detail.summary }}</p>
        <div class="report-content">{{ detail.full_content }}</div>

        <section v-if="detail.findings.length" class="finding-grid">
          <div class="section-heading finding-heading">
            <p>关键发现</p>
            <span>{{ detail.findings.length }} 项</span>
          </div>
          <article v-for="(finding, index) in detail.findings" :key="finding.summary">
            <span>{{ String(index + 1).padStart(2, '0') }}</span>
            <h3>{{ finding.summary }}</h3>
            <p>{{ finding.explanation }}</p>
          </article>
        </section>

        <details class="source-closure">
          <summary>报告来源</summary>
          <dl>
            <div><dt>Entities</dt><dd>{{ detail.sources.entities.length }}</dd></div>
            <div><dt>Relations</dt><dd>{{ detail.sources.relations.length }}</dd></div>
            <div><dt>Claims</dt><dd>{{ detail.sources.claims.length }}</dd></div>
            <div><dt>TextUnits</dt><dd>{{ detail.sources.text_unit_ids.length }}</dd></div>
          </dl>
          <section class="report-source-group">
            <h3>Entity</h3>
            <div class="report-source-grid">
              <RouterLink
                v-for="source in detail.sources.entities"
                :key="`${source.id}:${source.version}`"
                :to="{ path: `/knowledge/entities/${encodeURIComponent(source.id)}`, query: { epoch_id: detail.epoch_id } }"
              >
                <code>{{ source.id }}</code><span>v{{ source.version }}</span>
              </RouterLink>
            </div>
            <p v-if="detail.sources.entities.length === 0" class="muted">没有 Entity 来源。</p>
          </section>
          <section class="report-source-group">
            <h3>Relation</h3>
            <div class="report-source-grid">
              <RouterLink
                v-for="source in detail.sources.relations"
                :key="`${source.id}:${source.version}`"
                :to="{ path: `/knowledge/relations/${encodeURIComponent(source.id)}`, query: { epoch_id: detail.epoch_id } }"
              >
                <code>{{ source.id }}</code><span>v{{ source.version }}</span>
              </RouterLink>
            </div>
            <p v-if="detail.sources.relations.length === 0" class="muted">没有 Relation 来源。</p>
          </section>
          <section class="report-source-group">
            <h3>Claim Statement</h3>
            <div class="report-source-grid">
              <article v-for="source in detail.sources.claims" :key="`${source.id}:${source.version}:${source.evidence_index}`">
                <code>{{ source.id }}</code>
                <span>v{{ source.version }} · Statement {{ source.evidence_index }}</span>
              </article>
            </div>
            <p v-if="detail.sources.claims.length === 0" class="muted">没有 Claim 来源。</p>
          </section>
          <section class="report-source-group">
            <h3>TextUnitBody</h3>
            <div class="report-text-unit-ids">
              <code v-for="textUnitID in detail.sources.text_unit_ids" :key="textUnitID">{{ textUnitID }}</code>
            </div>
            <p v-if="detail.sources.text_unit_ids.length === 0" class="muted">没有 TextUnitBody 来源。</p>
          </section>
        </details>
      </article>

      <div v-else-if="!loading" class="report-placeholder">
        <span>↗</span>
        <p>选择一份社区报告，查看完整内容与证据闭包。</p>
      </div>
    </div>
  </section>
</template>
