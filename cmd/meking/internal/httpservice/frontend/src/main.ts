import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import { installRouteAssetRecovery } from './routeAssetRecovery'
import './styles.css'

installRouteAssetRecovery(
  window,
  window.sessionStorage,
  () => window.location.reload(),
)

createApp(App).use(router).mount('#app')
