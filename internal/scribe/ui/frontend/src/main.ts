import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { useTimelineStore } from './stores/timeline'
import { initUrlSync } from './utils/url-state'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)

const store = useTimelineStore(pinia)
initUrlSync(store)

app.mount('#app')
