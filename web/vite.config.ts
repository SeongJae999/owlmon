import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      // 모든 /api 요청을 백엔드 서버로 프록시
      '/api': 'http://localhost:8080',
    },
  },
})
