<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import GraphCanvas from '../components/GraphCanvas.vue'
import { useCommunityGraph } from '../composables/useCommunityGraph'
import { useEntityGraph } from '../composables/useEntityGraph'

type Mode = 'entities' | 'communities'

const mode = ref<Mode>('entities')
const entities = reactive(useEntityGraph())
const communities = reactive(useCommunityGraph())
const loading = computed(() => mode.value === 'entities' ? entities.loading : communities.loading)
const error = computed(() => mode.value === 'entities' ? entities.error : communities.error)
const selectedCanvasID = computed(() => {
  if (mode.value === 'communities') return communities.selectedID ? `community:${communities.selectedID}` : undefined
  if (entities.selectedRelationID) return `relation:${entities.selectedRelationID}`
  return entities.selectedEntityID ? `entity:${entities.selectedEntityID}` : undefined
})
const relationSource = computed(() => entities.graph.entities.find((item) => item.id === entities.selectedRelation?.source_entity_id))
const relationTarget = computed(() => entities.graph.entities.find((item) => item.id === entities.selectedRelation?.target_entity_id))

async function switchMode(next: Mode): Promise<void> {
  mode.value = next
  if (next === 'communities' && communities.communities.length === 0) await communities.load()
  if (next === 'entities' && entities.graph.entities.length === 0) await entities.load()
}

function selectCanvas(value: string | undefined): void {
  if (mode.value === 'entities') {
    entities.select(value)
    return
  }
  communities.select(value?.startsWith('community:') ? value.slice(10) : undefined)
}

onMounted(entities.load)
onBeforeUnmount(() => {
  entities.dispose()
  communities.dispose()
})
</script>

<template>
  <section class="page graph-page">
    <header class="page-header compact">
      <div>
        <p class="eyebrow">KNOWLEDGE GRAPH</p>
        <h1>浏览<em>实体与层级</em></h1>
      </div>
      <div class="publication-context">
        <span>{{ mode === 'entities' ? '知识版本' : '社区结构' }}</span>
        <code>{{ mode === 'entities' ? '当前正式版本' : (communities.structureID || '尚未加载') }}</code>
      </div>
    </header>

    <div class="graph-mode" aria-label="图浏览方式">
      <button type="button" :class="{ active: mode === 'entities' }" @click="switchMode('entities')">实体关系</button>
      <button type="button" :class="{ active: mode === 'communities' }" @click="switchMode('communities')">Community 层级</button>
    </div>

    <form v-if="mode === 'entities'" class="graph-toolbar" @submit.prevent="entities.load">
      <label><span>查找实体</span><input v-model="entities.search" type="search" placeholder="名称、别名或描述" /></label>
      <label>
        <span>实体类型</span>
        <select v-model="entities.entityType">
          <option value="">全部类型</option>
          <option v-for="value in entities.entityTypes" :key="value" :value="value">{{ value }}</option>
        </select>
      </label>
      <button class="outline-button" type="submit" :disabled="loading">应用筛选</button>
      <button class="outline-button" type="button" :disabled="loading" @click="entities.reset">重置</button>
    </form>
    <form v-else class="graph-toolbar community-toolbar" @submit.prevent="communities.applySearch">
      <label><span>查找 Community</span><input v-model="communities.search" type="search" placeholder="Community ID" /></label>
      <button class="outline-button" type="submit" :disabled="loading">查找</button>
    </form>

    <div v-if="error" class="failure-card" role="alert"><span>GRAPH BROWSE</span><strong>{{ error }}</strong></div>

    <div class="graph-layout">
      <section class="graph-canvas-panel">
        <header>
          <strong>{{ mode === 'entities' ? '有向实体关系图' : 'Community 分层图' }}</strong>
          <span v-if="mode === 'entities'">{{ entities.graph.entities.length }} Entity · {{ entities.graph.relations.length }} Relation</span>
          <span v-else>{{ communities.communities.length }} / {{ communities.total }} Community</span>
        </header>
        <GraphCanvas
          :mode="mode"
          :entities="entities.graph.entities"
          :relations="entities.graph.relations"
          :communities="communities.communities"
          :selected="selectedCanvasID"
          @select="selectCanvas"
        />
        <div v-if="loading" class="graph-overlay">正在读取固定发布…</div>
        <div v-else-if="mode === 'entities' && entities.graph.entities.length === 0" class="graph-overlay">没有匹配的 Entity。</div>
        <div v-else-if="mode === 'communities' && communities.communities.length === 0" class="graph-overlay">没有匹配的 Community。</div>
        <button
          v-if="mode === 'communities' && communities.canLoadMore"
          class="outline-button graph-load-more"
          type="button"
          :disabled="loading"
          @click="communities.load(communities.page + 1, true)"
        >
          加载更多 Community
        </button>
        <p v-if="mode === 'entities' && entities.graph.truncated" class="graph-note">当前图已达到资源上限，可缩小筛选或选择 Entity 继续展开邻域。</p>
      </section>

      <aside class="graph-inspector">
        <template v-if="mode === 'entities' && entities.selectedEntity">
          <p class="eyebrow">ENTITY · v{{ entities.selectedEntity.version }}</p>
          <h2>{{ entities.selectedEntity.title }}</h2>
          <p>{{ entities.selectedEntity.description || '该 Entity 没有描述。' }}</p>
          <dl>
            <div><dt>类型</dt><dd>{{ entities.selectedEntity.type || '—' }}</dd></div>
            <div><dt>Degree</dt><dd>{{ entities.selectedEntity.degree }}</dd></div>
            <div><dt>TextUnitBody</dt><dd>{{ entities.selectedEntity.text_unit_count }}</dd></div>
          </dl>
          <button class="submit-button inspector-action" type="button" :disabled="loading" @click="entities.expandSelected">展开有向邻域</button>
        </template>
        <template v-else-if="mode === 'entities' && entities.selectedRelation">
          <p class="eyebrow">RELATION · v{{ entities.selectedRelation.version }}</p>
          <h2>{{ relationSource?.title || entities.selectedRelation.source_entity_id }} → {{ relationTarget?.title || entities.selectedRelation.target_entity_id }}</h2>
          <p>{{ entities.selectedRelation.description || '该 Relation 没有描述。' }}</p>
          <dl>
            <div><dt>类型</dt><dd>{{ entities.selectedRelation.type || '—' }}</dd></div>
            <div><dt>权重</dt><dd>{{ entities.selectedRelation.weight }}</dd></div>
            <div><dt>CombinedDegree</dt><dd>{{ entities.selectedRelation.combined_degree }}</dd></div>
          </dl>
        </template>
        <template v-else-if="mode === 'communities' && communities.selected">
          <p class="eyebrow">COMMUNITY · LEVEL {{ communities.selected.level }}</p>
          <h2>{{ communities.selected.id }}</h2>
          <dl>
            <div><dt>Entity</dt><dd>{{ communities.selected.entity_count }}</dd></div>
            <div><dt>直接子级</dt><dd>{{ communities.selected.child_count }}</dd></div>
          </dl>
          <div class="inspector-actions">
            <button class="outline-button" type="button" :disabled="loading || communities.selected.child_count === 0" @click="communities.toggleSelected">{{ communities.expanded.includes(communities.selected.id) ? '收起子级' : '展开子级' }}</button>
          </div>
        </template>
        <p v-else class="muted">选择画布中的节点或关系查看详情。</p>
      </aside>
    </div>
  </section>
</template>
