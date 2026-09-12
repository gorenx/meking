<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { api, ApiError } from '../api/client'
import type { KnowledgeEntityDetail, KnowledgeRelationDetail } from '../api/types'

const props = defineProps<{
  kind: 'entity' | 'relation'
  id: string
  epochId: number
}>()

const detail = ref<KnowledgeEntityDetail | KnowledgeRelationDetail | null>(null)
const loading = ref(false)
const error = ref('')
let controller: AbortController | null = null

const entityDetail = computed(() => props.kind === 'entity' ? detail.value as KnowledgeEntityDetail | null : null)
const relationDetail = computed(() => props.kind === 'relation' ? detail.value as KnowledgeRelationDetail | null : null)
const claims = computed(() => entityDetail.value?.claims ?? relationDetail.value?.claims ?? [])
const textUnits = computed(() => entityDetail.value?.text_units ?? relationDetail.value?.text_units ?? [])

async function load(): Promise<void> {
  controller?.abort()
  const activeController = new AbortController()
  controller = activeController
  loading.value = true
  error.value = ''
  detail.value = null
  try {
    detail.value = props.kind === 'entity'
      ? await api.knowledgeEntity(props.id, props.epochId, activeController.signal)
      : await api.knowledgeRelation(props.id, props.epochId, activeController.signal)
  } catch (cause) {
    if (activeController.signal.aborted) return
    error.value = cause instanceof ApiError ? cause.failure.message : '无法读取知识对象详情'
  } finally {
    if (controller === activeController) loading.value = false
  }
}

watch(() => [props.kind, props.id, props.epochId], load, { immediate: true })
onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <section class="page knowledge-detail-page">
    <header class="page-header compact">
      <div>
        <p class="eyebrow">KNOWLEDGE OBJECT</p>
        <h1>查看<em>{{ kind === 'entity' ? ' Entity' : ' Relation' }}</em></h1>
      </div>
      <div class="publication-context">
        <span>固定读取边界</span>
        <code>{{ detail ? `Epoch ${detail.epoch_id}` : '尚未加载' }}</code>
      </div>
    </header>

    <RouterLink class="back-button knowledge-back" to="/knowledge">← 返回知识列表</RouterLink>

    <div v-if="error" class="failure-card" role="alert">
      <span>KNOWLEDGE DETAIL</span>
      <strong>{{ error }}</strong>
      <button class="text-button" type="button" @click="load">重新读取</button>
    </div>

    <div v-if="loading" class="knowledge-detail-loading">正在组合固定 Knowledge 与 Corpus 证据…</div>

    <template v-if="entityDetail">
      <article class="knowledge-object-card">
        <p class="eyebrow">ENTITY · v{{ entityDetail.entity.version }}</p>
        <h2>{{ entityDetail.entity.title }}</h2>
        <p>{{ entityDetail.entity.description || '没有描述' }}</p>
        <dl>
          <div><dt>ID</dt><dd>{{ entityDetail.entity.id }}</dd></div>
          <div><dt>类型</dt><dd>{{ entityDetail.entity.type || '—' }}</dd></div>
          <div><dt>Degree</dt><dd>{{ entityDetail.entity.degree }}</dd></div>
          <div><dt>证据</dt><dd>{{ entityDetail.entity.evidence_count }}</dd></div>
        </dl>
      </article>

      <section class="knowledge-association-section">
        <header><h2>关联 Relation</h2><span>{{ entityDetail.relations.length }} 条</span></header>
        <div class="knowledge-association-grid">
          <RouterLink
            v-for="relation in entityDetail.relations"
            :key="relation.id"
            :to="{ name: 'knowledge-relation', params: { id: relation.id }, query: { epoch_id: detail?.epoch_id } }"
          >
            <strong>{{ relation.source_entity_id }} → {{ relation.target_entity_id }}</strong>
            <code>{{ relation.id }}</code>
            <span>v{{ relation.version }} · weight {{ relation.weight }} · {{ relation.evidence_count }} 条证据</span>
            <p>{{ relation.description || '没有描述' }}</p>
          </RouterLink>
        </div>
        <p v-if="entityDetail.relations.length === 0" class="empty-state">该 Entity 没有活动 Relation。</p>
      </section>

      <section class="knowledge-association-section">
        <header><h2>相邻 Entity</h2><span>{{ entityDetail.neighbors.length }} 个</span></header>
        <div class="knowledge-association-grid compact">
          <RouterLink
            v-for="entity in entityDetail.neighbors"
            :key="entity.id"
            :to="{ name: 'knowledge-entity', params: { id: entity.id }, query: { epoch_id: detail?.epoch_id } }"
          >
            <strong>{{ entity.title }}</strong>
            <span>{{ entity.type || '未分类' }} · v{{ entity.version }}</span>
          </RouterLink>
        </div>
      </section>
    </template>

    <template v-if="relationDetail">
      <article class="knowledge-object-card">
        <p class="eyebrow">RELATION · v{{ relationDetail.relation.version }}</p>
        <h2>
          <RouterLink :to="{ name: 'knowledge-entity', params: { id: relationDetail.relation.source_entity_id }, query: { epoch_id: detail?.epoch_id } }">
            {{ relationDetail.relation.source_entity_id }}
          </RouterLink>
          →
          <RouterLink :to="{ name: 'knowledge-entity', params: { id: relationDetail.relation.target_entity_id }, query: { epoch_id: detail?.epoch_id } }">
            {{ relationDetail.relation.target_entity_id }}
          </RouterLink>
        </h2>
        <p>{{ relationDetail.relation.description || '没有描述' }}</p>
        <dl>
          <div><dt>ID</dt><dd>{{ relationDetail.relation.id }}</dd></div>
          <div><dt>Weight</dt><dd>{{ relationDetail.relation.weight }}</dd></div>
          <div><dt>Combined Degree</dt><dd>{{ relationDetail.relation.combined_degree }}</dd></div>
          <div><dt>证据</dt><dd>{{ relationDetail.relation.evidence_count }}</dd></div>
        </dl>
      </article>

      <section class="knowledge-association-section">
        <header><h2>端点 Entity</h2><span>{{ relationDetail.endpoints.length }} 个</span></header>
        <div class="knowledge-association-grid compact">
          <RouterLink
            v-for="entity in relationDetail.endpoints"
            :key="entity.id"
            :to="{ name: 'knowledge-entity', params: { id: entity.id }, query: { epoch_id: detail?.epoch_id } }"
          >
            <strong>{{ entity.title || entity.id }}</strong>
            <span>{{ entity.type || '未分类' }} · v{{ entity.version }}</span>
          </RouterLink>
        </div>
      </section>
    </template>

    <section v-if="detail" class="knowledge-association-section">
      <header><h2>Subject Claim</h2><span>{{ claims.length }} 个</span></header>
      <div class="knowledge-claim-list">
        <article v-for="claim in claims" :key="claim.id">
          <strong>{{ claim.type }} · v{{ claim.version }}</strong>
          <code>{{ claim.id }}</code>
          <p v-for="(evidence, evidenceIndex) in claim.evidence" :key="`${claim.id}:${evidenceIndex}`">
            {{ evidence.description || evidence.source_text }}
          </p>
        </article>
      </div>
      <p v-if="claims.length === 0" class="empty-state">该对象没有活动 Claim。</p>
    </section>

    <section v-if="detail" class="knowledge-association-section">
      <header><h2>关联 TextUnitBody</h2><span>{{ textUnits.length }} 个</span></header>
      <div class="knowledge-text-unit-list">
        <article v-for="(unit, unitIndex) in textUnits" :key="`${unit.text_unit_id}:${unit.text_id}:${unitIndex}`">
          <header>
            <strong>{{ unit.text_title || unit.document_location }}</strong>
            <code>{{ unit.text_unit_id }}</code>
          </header>
          <p>{{ unit.text }}</p>
          <footer>
            <span>Text {{ unit.text_id }}</span>
            <span>Document {{ unit.document_id }}</span>
            <span>{{ unit.document_location }}</span>
          </footer>
        </article>
      </div>
      <p v-if="textUnits.length === 0" class="empty-state">该对象没有关联 TextUnitBody。</p>
    </section>
  </section>
</template>
