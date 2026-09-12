<script setup lang="ts">
defineProps<{
  ready: boolean
  epochId?: number
  loading?: boolean
}>()
</script>

<template>
  <div class="runtime-badge" :class="{ ready }">
    <span class="runtime-dot" aria-hidden="true"></span>
    <span v-if="loading">正在连接</span>
    <template v-else-if="ready">
      <span>Epoch</span>
      <code>{{ epochId }}</code>
    </template>
    <span v-else>尚未发布统一版本</span>
  </div>
</template>
