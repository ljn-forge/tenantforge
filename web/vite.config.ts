import process from 'node:process'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '/api': process.env.VITE_API_PROXY ?? 'http://127.0.0.1:8090',
      '/healthz': process.env.VITE_API_PROXY ?? 'http://127.0.0.1:8090',
    },
  },
  test: { environment: 'jsdom', clearMocks: true, exclude: ['e2e/**', 'node_modules/**'] },
  build: { sourcemap: false, chunkSizeWarningLimit: 1100 },
})
