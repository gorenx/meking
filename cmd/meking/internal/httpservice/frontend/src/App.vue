<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { RouterLink, RouterView } from 'vue-router'
import RuntimeBadge from './components/RuntimeBadge.vue'
import { useRuntime } from './composables/useRuntime'
import { useZoneSession } from './zoneSession'

const { runtime, loading, error, refresh, startPolling, stopPolling } = useRuntime()
const { activeZoneID } = useZoneSession()
onMounted(startPolling)
onUnmounted(stopPolling)
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <RouterLink class="brand" to="/zones" aria-label="Meking Zone 管理">
        <span>
          <strong>Meking</strong>
          <small>PROJECT</small>
        </span>
      </RouterLink>

      <nav class="primary-nav" aria-label="主导航">
        <RouterLink to="/zones">
          <span>Zone 管理</span>
        </RouterLink>
        <RouterLink to="/">
          <span>知识查询</span>
        </RouterLink>
        <RouterLink to="/reports">
          <span>社区报告</span>
        </RouterLink>
        <RouterLink to="/graph">
          <span>图谱浏览</span>
        </RouterLink>
        <RouterLink to="/knowledge">
          <span>知识明细</span>
        </RouterLink>
        <RouterLink to="/journal">
          <span>事件日志</span>
        </RouterLink>
        <RouterLink to="/documents">
          <span>Zone 文档</span>
        </RouterLink>
        <RouterLink to="/control">
          <span>处理控制</span>
        </RouterLink>
      </nav>

      <div class="sidebar-foot">
        <RouterLink class="active-zone" to="/zones">
          <span>ACTIVE ZONE</span>
          <code>{{ activeZoneID || '尚未选择' }}</code>
        </RouterLink>
        <RuntimeBadge
          :ready="runtime?.ready ?? false"
          :epoch-id="runtime?.epoch_id"
          :loading="loading"
        />
        <button v-if="error" class="text-button" type="button" @click="refresh">重新连接</button>
        <p v-else>单 Project 配置 · Zone 隔离</p>
      </div>
    </aside>

    <main class="project-main">
      <RouterView />
    </main>
  </div>
</template>
