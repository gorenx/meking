import { createRouter, createWebHistory } from 'vue-router'
import { currentZoneID } from './zoneSession'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/zones', name: 'zones', component: () => import('./pages/ZonesPage.vue') },
    { path: '/', name: 'search', component: () => import('./pages/SearchPage.vue') },
    { path: '/reports/:id?', name: 'reports', component: () => import('./pages/ReportsPage.vue') },
    { path: '/graph', name: 'graph', component: () => import('./pages/GraphPage.vue') },
    { path: '/knowledge', name: 'knowledge', component: () => import('./pages/KnowledgePage.vue') },
    { path: '/journal', name: 'journal', component: () => import('./pages/JournalPage.vue') },
    {
      path: '/journal/streams/:id(.*)', name: 'journal-stream',
      component: () => import('./pages/JournalStreamPage.vue'),
    },
    {
      path: '/knowledge/entities/:id',
      name: 'knowledge-entity',
      component: () => import('./pages/KnowledgeDetailPage.vue'),
      props: (route) => ({ kind: 'entity', id: route.params.id, epochId: Number(route.query.epoch_id) }),
    },
    {
      path: '/knowledge/relations/:id',
      name: 'knowledge-relation',
      component: () => import('./pages/KnowledgeDetailPage.vue'),
      props: (route) => ({ kind: 'relation', id: route.params.id, epochId: Number(route.query.epoch_id) }),
    },
    { path: '/documents', name: 'documents', component: () => import('./pages/DocumentsPage.vue') },
    { path: '/control', name: 'control', component: () => import('./pages/ControlPage.vue') },
  ],
})

router.beforeEach((to) => {
  if (to.name !== 'zones' && !currentZoneID()) return { name: 'zones' }
  return true
})
