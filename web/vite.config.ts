import process from 'node:process'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'

export default defineConfig({
  plugins: [vue(), Components({ dirs: [], dts: false, resolvers: [ElementPlusResolver()] })],
  server: {
    proxy: {
      '/api': process.env.VITE_API_PROXY ?? 'http://127.0.0.1:8090',
      '/healthz': process.env.VITE_API_PROXY ?? 'http://127.0.0.1:8090',
    },
  },
  test: {
    environment: 'jsdom',
    clearMocks: true,
    exclude: ['e2e/**', 'node_modules/**'],
    server: { deps: { inline: [/element-plus\/es\/components\/[^/]+\/style\//] } },
  },
  build: { sourcemap: false, manifest: true },
})
