import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [
    react(),
  ],
  server: {
    // Proxy API and SSE to the Go server in development
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
