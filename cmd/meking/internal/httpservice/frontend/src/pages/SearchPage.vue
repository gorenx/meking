<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { api, ApiError, streamQuery } from '../api/client'
import type {
  ApiFailure,
  ConversationTurn,
  QueryMethod,
  QueryRequest,
  QueryResponse,
  QuestionSuggestionResponse,
} from '../api/types'
import { useRuntime } from '../composables/useRuntime'

interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

type QueryState = 'idle' | 'running' | 'completed' | 'failed' | 'stopped'

const methods: Array<{ id: QueryMethod; label: string; note: string }> = [
  { id: 'local', label: 'Local', note: '实体邻域与原文证据' },
  { id: 'global', label: 'Global', note: '社区报告全局归纳' },
  { id: 'basic', label: 'Basic', note: '直接检索 TextUnitBody' },
  { id: 'drift', label: 'DRIFT', note: '全局引导的递归探索' },
]

const { runtime } = useRuntime()
const method = ref<QueryMethod>('local')
const question = ref('')
const responseType = ref('Multiple Paragraphs')
const communityLevel = ref(0)
const dynamicSelection = ref(false)
const streaming = ref(true)
const busy = ref(false)
const suggesting = ref(false)
const answer = ref('')
const result = ref<QueryResponse | null>(null)
const suggestionResult = ref<QuestionSuggestionResponse | null>(null)
const failure = ref<ApiFailure | null>(null)
const messages = ref<ChatMessage[]>([])
const queryState = ref<QueryState>('idle')
let activeController: AbortController | null = null

const availableMethods = computed(() => runtime.value
  ? methods.filter((item) => runtime.value?.capabilities.includes(item.id))
  : methods)
const canSubmit = computed(() => Boolean(
  runtime.value?.ready
  && runtime.value.capabilities.includes(method.value)
  && question.value.trim()
  && !busy.value
  && !suggesting.value,
))
const canSuggest = computed(() => Boolean(
  runtime.value?.ready
  && runtime.value.capabilities.includes('question_suggestions')
  && question.value.trim()
  && !busy.value
  && !suggesting.value,
))
const supportsStreaming = computed(() => Boolean(runtime.value?.capabilities.includes('streaming')))
const submitLabel = computed(() => {
  if (!runtime.value?.ready) return '请先完成索引发布'
  if (!runtime.value.capabilities.includes(method.value)) return '当前发布不支持此查询'
  return '开始查询'
})
const selectedMethod = computed(() => methods.find((item) => item.id === method.value) ?? methods[0])
const answerStatus = computed(() => {
  if (queryState.value === 'running') return '正在推理'
  if (queryState.value === 'failed') return '回答中断'
  if (queryState.value === 'stopped') return '查询已停止'
  return '回答完成'
})
const resultIdentities = computed(() => {
  if (!result.value) return []
  return [
    result.value.epoch_id ? `Epoch ${result.value.epoch_id}` : '',
    result.value.report_set_id ? `ReportSet ${result.value.report_set_id}` : '',
    result.value.community_set_id ? `CommunitySet ${result.value.community_set_id}` : '',
    result.value.corpora_id ? `Corpus ${result.value.corpora_id}` : '',
  ].filter(Boolean)
})

watch(availableMethods, (available) => {
  if (!available.some((item) => item.id === method.value) && available[0]) method.value = available[0].id
})

function conversation(): ConversationTurn[] {
  return messages.value.map((message) => ({ role: message.role, content: message.content }))
}

function requestInput(prompt: string): QueryRequest {
  const input: QueryRequest = {
    method: method.value,
    question: prompt,
    response_type: responseType.value,
  }
  if (method.value === 'global') {
    input.community_level = communityLevel.value
    input.dynamic_community_selection = dynamicSelection.value
  }
  if (method.value === 'local' || method.value === 'global') {
    const history = conversation()
    if (history.length) input.conversation = history
  }
  return input
}

function displayFailure(cause: unknown) {
  failure.value = cause instanceof ApiError
    ? cause.failure
    : { code: 'client_failure', message: cause instanceof Error ? cause.message : '查询未完成' }
}

async function submit() {
  const prompt = question.value.trim()
  if (!prompt || !canSubmit.value) return
  failure.value = null
  result.value = null
  answer.value = ''
  queryState.value = 'running'
  activeController = new AbortController()
  const input = requestInput(prompt)
  question.value = ''
  busy.value = true
  try {
    if (streaming.value && supportsStreaming.value) {
      await streamQuery(input, {
        delta: (text) => { answer.value += text },
        final: (value) => {
          result.value = value
          answer.value = value.response
          queryState.value = 'completed'
        },
        error: (value) => {
          failure.value = value
          queryState.value = 'failed'
        },
      }, activeController.signal)
    } else {
      result.value = await api.query(input, activeController.signal)
      answer.value = result.value.response
      queryState.value = 'completed'
    }
    if (result.value) {
      messages.value.push(
        { role: 'user', content: prompt },
        { role: 'assistant', content: result.value.response },
      )
    }
  } catch (cause) {
    if ((cause as Error)?.name === 'AbortError') {
      queryState.value = 'stopped'
    } else {
      queryState.value = 'failed'
      displayFailure(cause)
    }
  } finally {
    busy.value = false
    activeController = null
  }
}

async function suggestQuestions() {
  const prompt = question.value.trim()
  if (!prompt || !canSuggest.value) return
  failure.value = null
  suggestionResult.value = null
  activeController = new AbortController()
  suggesting.value = true
  const history = messages.value
    .filter((message) => message.role === 'user')
    .map((message) => message.content)
  history.push(prompt)
  try {
    suggestionResult.value = await api.suggestQuestions(
      { history, count: 5 },
      activeController.signal,
    )
  } catch (cause) {
    if ((cause as Error)?.name !== 'AbortError') displayFailure(cause)
  } finally {
    suggesting.value = false
    activeController = null
  }
}

function selectSuggestion(value: string) {
  question.value = value
}

function cancel() {
  if (busy.value) queryState.value = 'stopped'
  activeController?.abort()
}
</script>

<template>
  <section class="page search-page">
    <header class="page-header">
      <div>
        <p class="eyebrow">KNOWLEDGE QUERY</p>
        <h1>从图中寻找<br /><em>可追溯的答案</em></h1>
      </div>
      <div class="publication-context">
        <span>当前统一发布</span>
        <code>{{ runtime?.epoch_id ? `Epoch ${runtime.epoch_id}` : '尚未发布' }}</code>
      </div>
    </header>

    <div class="method-strip" aria-label="查询方法">
      <button
        v-for="item in availableMethods"
        :key="item.id"
        type="button"
        :class="{ active: method === item.id }"
        @click="method = item.id"
      >
        <strong>{{ item.label }}</strong>
        <span>{{ item.note }}</span>
      </button>
    </div>

    <div v-if="!answer && !failure" class="query-intro">
      <div>
        <p>{{ selectedMethod.label }} SEARCH</p>
        <h2>{{ selectedMethod.note }}</h2>
        <p class="muted">每次请求会固定所需的知识、语料、报告与向量版本，并返回实际使用的版本身份。</p>
      </div>
    </div>

    <article v-if="answer || busy" class="answer-card" aria-live="polite">
      <div class="answer-heading">
        <div><span class="pulse" :class="{ active: queryState === 'running' }"></span>{{ answerStatus }}</div>
        <code v-if="result?.epoch_id">Epoch {{ result.epoch_id }}</code>
      </div>
      <div class="answer-copy">{{ answer }}<span v-if="busy" class="cursor">▋</span></div>

      <div v-if="resultIdentities.length" class="answer-identities">
        <code v-for="identity in resultIdentities" :key="identity">{{ identity }}</code>
      </div>

      <details v-if="result?.citation_audit?.items.length" class="citation-list">
        <summary>引用与来源 · {{ result.citation_audit.items.length }}</summary>
        <div v-for="(citation, index) in result.citation_audit.items" :key="`${citation.raw}-${index}`" class="citation-item">
          <div>
            <code>{{ citation.raw }}</code>
            <span :class="['citation-state', citation.status]">{{ citation.status }}</span>
          </div>
          <p v-if="citation.invalid_reason">{{ citation.invalid_reason }}</p>
          <ul v-if="citation.sources.length">
            <li v-for="source in citation.sources" :key="source.text_unit_id">
              <strong>{{ source.document_title }}</strong>
              <small>{{ source.document_location }}</small>
              <span>{{ source.text }}</span>
            </li>
          </ul>
        </div>
      </details>
    </article>

    <div v-if="failure" class="failure-card" role="alert">
      <span>{{ failure.code }}</span>
      <strong>{{ failure.message }}</strong>
      <p v-if="failure.partial_output">回答流已产生部分内容，不会自动重试。</p>
    </div>

    <div v-else-if="queryState === 'stopped'" class="query-status-card" role="status">
      <strong>查询已停止</strong>
      <p>已取消当前请求；未完成的提问不会加入后续会话。</p>
    </div>

    <section v-if="suggestionResult" class="suggestion-card" aria-label="建议问题">
      <div class="answer-heading">
        <div>后续问题建议</div>
        <code>Epoch {{ suggestionResult.epoch_id }}</code>
      </div>
      <button
        v-for="candidate in suggestionResult.questions"
        :key="candidate"
        type="button"
        class="suggestion-item"
        @click="selectSuggestion(candidate)"
      >
        {{ candidate }}
      </button>
    </section>

    <form class="query-composer" @submit.prevent="submit">
      <textarea
        v-model="question"
        rows="3"
        :disabled="busy || suggesting"
        placeholder="输入你想从当前知识图中了解的问题…"
        aria-label="查询问题"
        @keydown.meta.enter="submit"
        @keydown.ctrl.enter="submit"
      ></textarea>
      <div class="composer-controls">
        <label v-if="method === 'global'">
          社区层级
          <input v-model.number="communityLevel" type="number" min="0" />
        </label>
        <label v-if="method === 'global'" class="check-control">
          <input v-model="dynamicSelection" type="checkbox" />动态社区选择
        </label>
        <label v-if="supportsStreaming" class="check-control">
          <input v-model="streaming" type="checkbox" />流式回答
        </label>
        <select v-model="responseType" aria-label="回答格式">
          <option>Multiple Paragraphs</option>
          <option>Single Paragraph</option>
          <option>List of 5-7 Points</option>
        </select>
        <button
          v-if="runtime?.capabilities.includes('question_suggestions')"
          class="suggest-button"
          type="button"
          :disabled="!canSuggest"
          @click="suggestQuestions"
        >
          {{ suggesting ? '正在生成建议' : '建议后续问题' }}
        </button>
        <button v-if="busy || suggesting" class="submit-button cancel" type="button" @click="cancel">停止</button>
        <button v-else class="submit-button" type="submit" :disabled="!canSubmit">{{ submitLabel }}</button>
      </div>
    </form>
  </section>
</template>
