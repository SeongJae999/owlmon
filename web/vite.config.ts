import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      // Prometheus API 프록시 (CORS 방지)
      '/api/v1': 'http://localhost:9090',
    },
  },
})
