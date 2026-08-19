import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 开发模式：/api 代理到本地 backend（生产走 Nginx）
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
