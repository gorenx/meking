<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api, ApiError } from '../api/client'
import type { KnowledgeClaimPage, KnowledgeEntityPage, KnowledgeRelationPage } from '../api/types'

const limit = 25
type KnowledgeKind = 'entities' | 'relations' | 'claims'
const knowledgeKindLabels: Record<KnowledgeKind, string> = {
  entities: 'Entity',
  relations: 'Relation',
  claims: 'Claim',
}
const kind = ref<KnowledgeKind>('entities')
const entityPage = ref<KnowledgeEntityPage | null>(null)
const relationPage = ref<KnowledgeRelationPage | null>(null)
const claimPage = ref<KnowledgeClaimPage | null>(null)
const cursorHistory = ref<string[]>([])
const loading = ref(false)
const error = ref('')
let controller: AbortController | null = null

const pageNumber = computed(() => cursorHistory.value.length + 1)
const kindLabel = computed(() => knowledgeKindLabels[kind.value])
const page = computed(() => {
  if (kind.value === 'entities') return entityPage.value
  if (kind.value === 'relations') return relationPage.value
  return claimPage.value
})

async function load(after = ''): Promise<void> {
  const requestedKind = kind.value
  controller?.abort()
  const activeController = new AbortController()
  controller = activeController
  loading.value = true
  error.value = ''
  try {
    if (requestedKind === 'entities') {
      entityPage.value = await api.knowledgeEntities(after, limit, activeController.signal)
    } else if (requestedKind === 'relations') {
      relationPage.value = await api.knowledgeRelations(after, limit, activeController.signal)
    } else {
      claimPage.value = await api.knowledgeClaims(after, limit, activeController.signal)
    }
  } catch (cause) {
    if (activeController.signal.aborted) return
    error.value = cause instanceof ApiError
      ? cause.failure.message
      : `无法读取 ${knowledgeKindLabels[requestedKind]} 列表`
  } finally {
    if (controller === activeController) loading.value = false
  }
}

async function selectKind(nextKind: KnowledgeKind): Promise<void> {
  if (kind.value === nextKind) return
  kind.value = nextKind
  cursorHistory.value = []
  if (nextKind === 'entities') entityPage.value = null
  else if (nextKind === 'relations') relationPage.value = null
  else claimPage.value = null
  await load()
}

async function nextPage(): Promise<void> {
  if (!page.value?.has_more || !page.value.next_after) return
  cursorHistory.value.push(page.value.next_after)
  await load(page.value.next_after)
}

async function previousPage(): Promise<void> {
  if (cursorHistory.value.length === 0) return
  cursorHistory.value.pop()
  await load(cursorHistory.value.at(-1) ?? '')
}

onMounted(() => load())
onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <section class="page knowledge-page">
    <header class="page-header compact">
      <div>
        <p class="eyebrow">PUBLISHED KNOWLEDGE</p>
        <h1>核对<em>活动知识</em></h1>
      </div>
      <div class="publication-context">
        <span>固定读取边界</span>
        <code>{{ page ? `Epoch ${page.epoch_id}` : '尚未加载' }}</code>
      </div>
    </header>

    <div class="knowledge-kind" aria-label="知识对象类型">
      <button :class="{ active: kind === 'entities' }" type="button" @click="selectKind('entities')">Entity</button>
      <button :class="{ active: kind === 'relations' }" type="button" @click="selectKind('relations')">Relation</button>
      <button :class="{ active: kind === 'claims' }" type="button" @click="selectKind('claims')">Claim</button>
    </div>

    <div v-if="error" class="failure-card" role="alert">
      <span>KNOWLEDGE BROWSE</span>
      <strong>{{ error }}</strong>
      <button class="text-button" type="button" @click="load(cursorHistory.at(-1) ?? '')">重新读取</button>
    </div>

    <section class="knowledge-ledger" :aria-busy="loading">
      <header>
        <div>
          <strong>{{ kindLabel }} 账册</strong>
          <span>按稳定 ID 排序 · 仅显示活动版本</span>
        </div>
        <span>第 {{ pageNumber }} 页</span>
      </header>

      <div class="knowledge-table-wrap">
        <table v-if="kind === 'entities'">
          <thead>
            <tr>
              <th>Entity</th>
              <th>类型</th>
              <th>Version</th>
              <th>Degree</th>
              <th>证据</th>
              <th>ID</th>
            </tr>
          </thead>
          <tbody>
              <tr
                v-for="entity in entityPage?.items"
                :key="entity.id"
                class="knowledge-row"
                role="link"
                tabindex="0"
                @click="$router.push({ name: 'knowledge-entity', params: { id: entity.id }, query: { epoch_id: entityPage?.epoch_id } })"
                @keydown.enter="$router.push({ name: 'knowledge-entity', params: { id: entity.id }, query: { epoch_id: entityPage?.epoch_id } })"
              >
              <td>
                <strong>{{ entity.title }}</strong>
                <small>{{ entity.description || entity.aliases.join(' · ') || '没有描述' }}</small>
              </td>
              <td>{{ entity.type || '—' }}</td>
              <td>v{{ entity.version }}</td>
              <td>{{ entity.degree }}</td>
              <td>{{ entity.evidence_count }}</td>
              <td><code>{{ entity.id }}</code></td>
            </tr>
          </tbody>
        </table>

        <table v-else-if="kind === 'relations'">
          <thead>
            <tr>
              <th>方向</th>
              <th>Version</th>
              <th>Weight</th>
              <th>Combined Degree</th>
              <th>证据</th>
              <th>ID</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="relation in relationPage?.items"
              :key="relation.id"
              class="knowledge-row"
              role="link"
              tabindex="0"
              @click="$router.push({ name: 'knowledge-relation', params: { id: relation.id }, query: { epoch_id: relationPage?.epoch_id } })"
              @keydown.enter="$router.push({ name: 'knowledge-relation', params: { id: relation.id }, query: { epoch_id: relationPage?.epoch_id } })"
            >
              <td>
                <strong><code>{{ relation.source_entity_id }}</code> → <code>{{ relation.target_entity_id }}</code></strong>
                <small>{{ relation.description || '没有描述' }}</small>
              </td>
              <td>v{{ relation.version }}</td>
              <td>{{ relation.weight }}</td>
              <td>{{ relation.combined_degree }}</td>
              <td>{{ relation.evidence_count }}</td>
              <td><code>{{ relation.id }}</code></td>
            </tr>
          </tbody>
        </table>

        <table v-else>
          <thead>
            <tr>
              <th>Claim</th>
              <th>Subject</th>
              <th>Version</th>
              <th>证据数</th>
              <th>证据来源</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="claim in claimPage?.items" :key="claim.id">
              <td>
                <strong>{{ claim.type || '未分类' }}</strong>
                <small>{{ claim.id }}</small>
              </td>
              <td><code>{{ claim.subject_id }}</code></td>
              <td>v{{ claim.version }}</td>
              <td>{{ claim.evidence.length }}</td>
              <td>
                <div v-for="(evidence, index) in claim.evidence" :key="`${claim.id}:${index}`" class="knowledge-evidence">
                  <strong>{{ evidence.description || evidence.source_text || '没有来源摘要' }}</strong>
				  <small>
					<code>{{ evidence.text_unit_id }}</code>
					· {{ evidence.subject_text }} → {{ evidence.object_text }}
				  </small>
				</div>
				<small v-if="claim.evidence.length === 0">没有证据</small>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <p v-if="!loading && !page?.items.length" class="empty-state">
        该固定发布中没有活动 {{ kindLabel }}。
      </p>
      <div v-if="loading" class="knowledge-loading">正在读取固定 Knowledge 视图…</div>

      <footer>
        <button class="outline-button" type="button" :disabled="loading || cursorHistory.length === 0" @click="previousPage">上一页</button>
        <span>{{ page?.items.length ?? 0 }} 条 · 每页最多 {{ limit }} 条</span>
        <button class="outline-button" type="button" :disabled="loading || !page?.has_more" @click="nextPage">下一页</button>
      </footer>
    </section>
  </section>
</template>
