<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { api, ApiError } from '../api/client'
import type {
  ApiFailure,
  ControlAction,
  ControlActionStatus,
  ControlInvocation,
  ControlPolicyMode,
} from '../api/types'
import { useZoneSession } from '../zoneSession'

interface ActionDefinition {
  action: ControlAction
  owner: string
  title: string
  description: string
  invokeLabel: string
}

const definitions: ActionDefinition[] = [
  {
    action: 'convert_document',
    owner: 'Corpus',
    title: '转换文档',
    description: '把已记录的原始文档转换为标准文本。',
    invokeLabel: '开始转换文档',
  },
  {
    action: 'create_text_units',
    owner: 'Corpus',
    title: '创建 TextUnit',
    description: '从标准文本建立可引用的文本单元与 Corpus。',
    invokeLabel: '开始创建 TextUnit',
  },
  {
    action: 'extract_knowledge',
    owner: 'Knowledge Extraction',
    title: '抽取知识',
    description: '由 Agent 从 Corpus 提取 Entity、Relation 与 Claim。',
    invokeLabel: '开始抽取知识',
  },
  {
    action: 'index_entity_vectors',
    owner: 'Knowledge Vector Index',
    title: '索引实体向量',
    description: '为已发布的正式 Entity VersionSet 建立精确向量集合。',
    invokeLabel: '开始索引实体',
  },
  {
    action: 'derive_community_structure',
    owner: 'Community',
    title: '建立社区结构',
    description: '按最新知识边界建立或复用社区层级。',
    invokeLabel: '开始建立社区结构',
  },
  {
    action: 'publish_epoch',
    owner: 'Epoch',
    title: '发布一致边界',
    description: '发布相容的 Corpus、Knowledge 与 Community Structure。',
    invokeLabel: '开始发布 Epoch',
  },
  {
    action: 'generate_community_reports',
    owner: 'Community Report',
    title: '生成社区报告',
    description: '在已发布 Epoch 后独立生成可查询的社区报告。',
    invokeLabel: '开始生成报告',
  },
]

const { activeZoneID } = useZoneSession()
const statuses = ref<ControlActionStatus[]>([])
const loading = ref(false)
const refreshing = ref(false)
const savingPolicy = ref(false)
const invoking = ref<ControlAction>()
const editing = ref<ControlAction>()
const failure = ref<ApiFailure>()
const invocation = ref<ControlInvocation>()
const policyEditor = ref<HTMLElement>()
const policyDraft = reactive({
  mode: 'automatic' as ControlPolicyMode,
  minimumPending: 1,
  maximumWait: '5m',
  expectedRevision: 0,
})
let statusController: AbortController | undefined
let refreshTimer: number | undefined

const statusByAction = computed(() => new Map(statuses.value.map((status) => [status.action, status])))
const actionRows = computed(() => definitions.map((definition) => ({
  definition,
  status: statusByAction.value.get(definition.action),
})))
const pendingActions = computed(() => statuses.value.filter((status) => status.pending_count > 0).length)

function showFailure(cause: unknown, fallback: string) {
  failure.value = cause instanceof ApiError
    ? cause.failure
    : { code: 'client_failure', message: cause instanceof Error ? cause.message : fallback }
}

async function loadStatuses(quiet = false) {
  statusController?.abort()
  const controller = new AbortController()
  statusController = controller
  if (quiet) refreshing.value = true
  else loading.value = true
  try {
    const result = await api.controlActions(controller.signal)
    statuses.value = result.actions
    failure.value = undefined
  } catch (cause) {
    if (!controller.signal.aborted) showFailure(cause, '无法读取 Action 状态')
  } finally {
    if (statusController === controller) {
      loading.value = false
      refreshing.value = false
    }
  }
}

function modeLabel(mode: ControlPolicyMode): string {
  if (mode === 'automatic') return '自动'
  if (mode === 'manual') return '手动'
  return '暂停'
}

function actionTitle(action: ControlAction): string {
  return definitions.find((definition) => definition.action === action)?.title ?? action
}

function formatTime(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(date)
}

function openPolicy(action: ControlAction, status?: ControlActionStatus) {
  editing.value = action
  policyDraft.mode = status?.policy.mode ?? 'manual'
  policyDraft.minimumPending = status?.policy.mode === 'automatic'
    ? status.policy.minimum_pending
    : 1
  policyDraft.maximumWait = status?.policy.mode === 'automatic'
    ? status.policy.maximum_wait
    : '5m'
  policyDraft.expectedRevision = status?.policy.revision ?? 0
  invocation.value = undefined
  failure.value = undefined
  void nextTick(() => policyEditor.value?.focus())
}

function chooseMode(mode: ControlPolicyMode) {
  policyDraft.mode = mode
  if (mode === 'automatic') {
    if (policyDraft.minimumPending < 1) policyDraft.minimumPending = 1
    if (!policyDraft.maximumWait || policyDraft.maximumWait === '0s') policyDraft.maximumWait = '5m'
  }
}

async function savePolicy() {
  if (!editing.value || savingPolicy.value) return
  savingPolicy.value = true
  failure.value = undefined
  try {
    await api.publishControlPolicy(editing.value, {
      mode: policyDraft.mode,
      minimum_pending: policyDraft.mode === 'automatic' ? policyDraft.minimumPending : 0,
      maximum_wait: policyDraft.mode === 'automatic' ? policyDraft.maximumWait : '0s',
      expected_revision: policyDraft.expectedRevision,
    })
    await loadStatuses(true)
    editing.value = undefined
  } catch (cause) {
    showFailure(cause, '无法保存 Policy')
  } finally {
    savingPolicy.value = false
  }
}

async function invokeAction(definition: ActionDefinition, status: ControlActionStatus) {
  if (invoking.value || status.policy.mode !== 'manual' || status.pending_count === 0) return
  invoking.value = definition.action
  invocation.value = undefined
  failure.value = undefined
  try {
    invocation.value = await api.invokeControlAction(definition.action)
    await loadStatuses(true)
  } catch (cause) {
    showFailure(cause, `无法${definition.invokeLabel}`)
  } finally {
    invoking.value = undefined
  }
}

onMounted(() => {
  void loadStatuses()
  refreshTimer = window.setInterval(() => void loadStatuses(true), 5000)
})

onBeforeUnmount(() => {
  statusController?.abort()
  if (refreshTimer !== undefined) window.clearInterval(refreshTimer)
})
</script>

<template>
  <section class="page control-page">
    <header class="page-header control-heading">
      <div>
        <p class="eyebrow">ACTION CONTROL</p>
        <h1>处理<em>控制</em></h1>
        <p class="control-lead">查看七个领域 Action 的新输入，并决定何时交给领域处理。</p>
      </div>
      <div class="publication-context control-zone">
        <span>ACTIVE ZONE</span>
        <code>{{ activeZoneID }}</code>
      </div>
    </header>

    <section class="control-signal-board" aria-label="控制面摘要">
      <div>
        <span>当前 Zone</span>
        <strong>{{ activeZoneID }}</strong>
      </div>
      <div>
        <span>有新输入的 Action · Policy {{ statuses.length }}/{{ definitions.length }}</span>
        <strong>{{ pendingActions }} / {{ definitions.length }}</strong>
      </div>
      <div class="control-boundary-note">
        <span>处理范围</span>
        <p>每次调用只触发当前 Zone；其他 Zone 需要分别调用或由自动 Policy 分别评估。</p>
      </div>
      <button class="outline-button" type="button" :disabled="loading || refreshing" @click="loadStatuses(true)">
        {{ refreshing ? '正在刷新…' : '刷新状态' }}
      </button>
    </section>

    <div v-if="invocation" class="control-receipt" aria-live="polite">
      <span>STARTED</span>
      <strong>{{ actionTitle(invocation.action) }}</strong>
      <p>当前 Zone 的待处理输入已交给领域后台处理；处理成功并推进消费位置后，待处理数量才会更新。</p>
    </div>

    <div v-if="failure" class="failure-card" role="alert">
      <span>{{ failure.code }}</span>
      <strong>{{ failure.message }}</strong>
    </div>

    <div v-if="!loading && statuses.length < definitions.length" class="control-setup-note">
      <strong>还有 {{ definitions.length - statuses.length }} 个 Action 未配置 Policy</strong>
      <span>未配置的 Action 不会自动接受输入；逐项选择自动、手动或暂停。</span>
    </div>

    <div v-if="loading && !statuses.length" class="control-loading">正在读取 Action 状态…</div>

    <section v-else class="control-line" aria-label="Action 控制线路">
      <template v-for="(row, index) in actionRows" :key="row.definition.action">
        <article
          :class="[
            'control-action',
            `mode-${row.status?.policy.mode ?? 'unconfigured'}`,
            { pending: (row.status?.pending_count ?? 0) > 0 },
          ]"
        >
          <div class="control-node" aria-hidden="true">
            <span>{{ String(index + 1).padStart(2, '0') }}</span>
          </div>

          <div class="control-action-main">
            <header>
              <div>
                <p>{{ row.definition.owner }}</p>
                <h2>{{ row.definition.title }}</h2>
                <code>{{ row.definition.action }}</code>
              </div>
              <span :class="['policy-mode', `mode-${row.status?.policy.mode ?? 'unconfigured'}`]">
                {{ row.status ? modeLabel(row.status.policy.mode) : '未配置' }}
              </span>
            </header>
            <p class="control-action-description">{{ row.definition.description }}</p>

            <dl class="control-facts">
              <div>
                <dt>相关新输入</dt>
                <dd>{{ row.status?.pending_count ?? '—' }}</dd>
              </div>
              <div>
                <dt>最早等待</dt>
                <dd>{{ formatTime(row.status?.pending_since) }}</dd>
              </div>
              <div>
                <dt>当前规则</dt>
                <dd v-if="row.status?.policy.mode === 'automatic'">
                  ≥ {{ row.status.policy.minimum_pending }} 或 {{ row.status.policy.maximum_wait }}
                </dd>
                <dd v-else-if="row.status?.policy.mode === 'manual'">仅人工调用</dd>
                <dd v-else-if="row.status?.policy.mode === 'suspended'">停止接受新边界</dd>
                <dd v-else>等待配置 Project Policy</dd>
              </div>
            </dl>
          </div>

          <div class="control-action-buttons">
            <p
              v-if="invocation?.action === row.definition.action"
              class="control-action-receipt"
              role="status"
            >
              <strong>已受理</strong>
              当前 Zone 正在后台处理。处理成功前，相关新输入仍会保留。
            </p>
            <button
              v-if="row.status?.policy.mode === 'manual'"
              class="submit-button"
              type="button"
              :disabled="row.status.pending_count === 0 || Boolean(invoking)"
              @click="invokeAction(row.definition, row.status)"
            >
              {{ invoking === row.definition.action
                ? '正在发送请求…'
                : invocation?.action === row.definition.action
                  ? '再次触发'
                  : row.definition.invokeLabel }}
            </button>
            <p v-else-if="row.status?.policy.mode === 'automatic'">
              达到数量或等待阈值后自动触发当前 Zone。
            </p>
            <p v-else-if="row.status?.policy.mode === 'suspended'">已暂停；已经接受的领域处理不会停止。</p>
            <p v-else>先选择这个 Action 何时处理输入。</p>
            <button
              class="text-button"
              type="button"
              @click="openPolicy(row.definition.action, row.status)"
            >
              {{ row.status ? '修改 Policy' : '配置 Policy' }}
            </button>
          </div>
        </article>
      </template>
    </section>

    <section
      v-if="editing"
      ref="policyEditor"
      class="policy-editor"
      role="dialog"
      aria-labelledby="policy-editor-heading"
      tabindex="-1"
      @keydown.esc="editing = undefined"
    >
      <header>
        <div>
          <p class="eyebrow">PROJECT POLICY</p>
          <h2 id="policy-editor-heading">
            {{ definitions.find((item) => item.action === editing)?.title }}
          </h2>
        </div>
        <button class="text-button" type="button" @click="editing = undefined">关闭</button>
      </header>
      <p>Policy 属于整个 Project；保存后适用于所有 Zone 中的这个 Action。</p>

      <form @submit.prevent="savePolicy">
        <fieldset class="policy-mode-picker">
          <legend>调用方式</legend>
          <button
            v-for="mode in (['automatic', 'manual', 'suspended'] as ControlPolicyMode[])"
            :key="mode"
            type="button"
            :class="{ active: policyDraft.mode === mode }"
            @click="chooseMode(mode)"
          >
            <strong>{{ modeLabel(mode) }}</strong>
            <span v-if="mode === 'automatic'">达到阈值后调用</span>
            <span v-else-if="mode === 'manual'">只允许页面调用</span>
            <span v-else>不接受新边界</span>
          </button>
        </fieldset>

        <div v-if="policyDraft.mode === 'automatic'" class="automatic-fields">
          <label>
            <span>最少待处理数量</span>
            <input v-model.number="policyDraft.minimumPending" type="number" min="1" step="1" required>
          </label>
          <span class="policy-or">或</span>
          <label>
            <span>最长等待时间</span>
            <input v-model.trim="policyDraft.maximumWait" type="text" placeholder="5m" required>
            <small>Go duration，例如 30s、5m、1h</small>
          </label>
        </div>

        <footer>
          <span>当前 Revision {{ policyDraft.expectedRevision }}</span>
          <button class="submit-button" type="submit" :disabled="savingPolicy">
            {{ savingPolicy ? '正在保存…' : '保存 Policy' }}
          </button>
        </footer>
      </form>
    </section>
  </section>
</template>
