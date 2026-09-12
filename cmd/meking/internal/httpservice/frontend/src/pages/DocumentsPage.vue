<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api, ApiError } from '../api/client'
import type { ApiFailure, DocumentCatalogEntry, DocumentReceipt } from '../api/types'

const selectedFile = ref<File>()
const fileInput = ref<HTMLInputElement>()
const uploading = ref(false)
const receipt = ref<DocumentReceipt>()
const documents = ref<DocumentCatalogEntry[]>([])
const catalogLoading = ref(false)
const failure = ref<ApiFailure>()
let catalogController: AbortController | null = null

function showFailure(cause: unknown, fallback: string) {
  failure.value = cause instanceof ApiError
    ? cause.failure
    : { code: 'client_failure', message: cause instanceof Error ? cause.message : fallback }
}

function selectFile(event: Event) {
  selectedFile.value = (event.target as HTMLInputElement).files?.[0]
  receipt.value = undefined
  failure.value = undefined
}

async function loadDocuments() {
  catalogController?.abort()
  const controller = new AbortController()
  catalogController = controller
  catalogLoading.value = true
  failure.value = undefined
  try {
    const all: DocumentCatalogEntry[] = []
    let offset = 0
    while (true) {
      const page = await api.documents(offset, 100, controller.signal)
      all.push(...page.documents)
      if (!page.has_more || page.next_offset === undefined) break
      offset = page.next_offset
    }
    documents.value = all
  } catch (cause) {
    if (!controller.signal.aborted) showFailure(cause, '无法读取文件目录')
  } finally {
    if (catalogController === controller) catalogLoading.value = false
  }
}

async function upload() {
  if (!selectedFile.value || uploading.value) return
  uploading.value = true
  receipt.value = undefined
  failure.value = undefined
  try {
    receipt.value = await api.submitDocument(selectedFile.value)
    selectedFile.value = undefined
    if (fileInput.value) fileInput.value.value = ''
    await loadDocuments()
  } catch (cause) {
    showFailure(cause, '文档上传失败')
  } finally {
    uploading.value = false
  }
}

function formatBytes(size: number): string {
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KiB`
  return `${(size / (1024 * 1024)).toFixed(1)} MiB`
}

onMounted(loadDocuments)
onBeforeUnmount(() => catalogController?.abort())
</script>

<template>
  <section class="page documents-page">
    <header class="page-header compact">
      <div>
        <p class="eyebrow">CORPUS INPUT</p>
        <h1>Zone <em>文档</em></h1>
      </div>
      <div class="publication-context">
        <span>处理方式</span>
        <code>UPLOAD → CONTROL POLICY</code>
      </div>
    </header>

    <section class="operation-panel" aria-labelledby="document-upload-heading">
      <div class="operation-copy">
        <p class="eyebrow">UPLOAD</p>
        <h2 id="document-upload-heading">上传 Zone 文档</h2>
        <p>上传事务同时保存不可变 Document、关联当前 Zone 并记录事实；后续转换时机由 Control Policy 决定。</p>
      </div>

      <form class="upload-form" @submit.prevent="upload">
        <label class="file-picker">
          <span>选择文件</span>
          <input
            ref="fileInput"
            type="file"
            name="file"
            accept=".txt,.md,.pdf,.docx,.pptx,.xlsx,.html,.htm"
            :disabled="uploading"
            @change="selectFile"
          >
          <strong>{{ selectedFile?.name ?? '尚未选择文件' }}</strong>
          <small>TXT · Markdown · PDF · Office · HTML，格式与大小由 Project 配置校验</small>
        </label>
        <button class="submit-button operation-submit" type="submit" :disabled="!selectedFile || uploading">
          {{ uploading ? '正在上传…' : '上传文件' }}
        </button>
      </form>

      <article v-if="receipt" class="receipt-card" aria-live="polite">
        <header>
          <div>
            <p class="eyebrow">DOCUMENT RECEIPT</p>
            <h3>文件已加入当前 Zone</h3>
          </div>
          <strong>{{ receipt.status }}</strong>
        </header>
        <dl>
          <div><dt>Document</dt><dd>{{ receipt.document_id }}</dd></div>
          <div><dt>SHA-256</dt><dd>{{ receipt.content_digest }}</dd></div>
        </dl>
      </article>
    </section>

    <section class="document-catalog operation-panel" aria-labelledby="document-catalog-heading">
      <header class="catalog-heading">
        <div class="operation-copy">
          <p class="eyebrow">CATALOG</p>
          <h2 id="document-catalog-heading">Project 文档目录</h2>
          <p>目录展示 Project 文档及其与当前 Zone 的追加式关联，不提供旧的 Start 选择。</p>
        </div>
        <button class="text-button" type="button" :disabled="catalogLoading" @click="loadDocuments">刷新目录</button>
      </header>

      <div v-if="documents.length" class="document-list">
        <article v-for="item in documents" :key="item.document_id" class="document-row">
          <span class="file-glyph" aria-hidden="true">FILE</span>
          <span class="document-identity">
            <strong>{{ item.name }}</strong>
            <small>{{ item.media_type }} · {{ formatBytes(item.size) }}</small>
            <code>{{ item.document_id }}</code>
          </span>
          <span :class="['selection-state', { selected: item.selected }]">
            {{ item.selected ? '已在 Zone' : '其他 Zone' }}
          </span>
        </article>
      </div>
      <p v-else-if="!catalogLoading" class="empty-state">还没有文件。先上传一个受支持的文档。</p>
      <div v-if="catalogLoading" class="journal-loading">正在读取文件目录…</div>
    </section>

    <div v-if="failure" class="failure-card" role="alert">
      <span>{{ failure.code }}</span>
      <strong>{{ failure.message }}</strong>
    </div>
  </section>
</template>
