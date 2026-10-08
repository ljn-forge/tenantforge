import { createApp } from 'vue'
import 'element-plus/theme-chalk/dark/css-vars.css'
import App from './App.vue'
import { router } from './router'
import { bootstrap, state } from './stores/app'
import './style.css'

document.documentElement.classList.toggle('dark', state.dark)
document.documentElement.lang = state.locale === 'zh' ? 'zh-CN' : 'en'
const app = createApp(App)
bootstrap().finally(() => {
  app.use(router).mount('#app')
})
