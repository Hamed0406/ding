import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { VitePWA } from 'vite-plugin-pwa'

export default defineConfig({
  plugins: [
    react(),
    VitePWA({
      registerType: 'autoUpdate',
      manifest: {
        name: 'Ding Network Scanner',
        short_name: 'Ding',
        description: 'See who is on your network',
        theme_color: '#0f172a',
        background_color: '#0f172a',
        display: 'standalone',
        scope: '/',
        start_url: '/',
        icons: [
          { src: 'icon.svg', sizes: 'any', type: 'image/svg+xml' },
        ],
      },
      workbox: {
        globPatterns: ['**/*.{js,css,html,svg,ico,webmanifest}'],
        navigateFallback: 'index.html',
        runtimeCaching: [
          {
            // Never cache API responses — always hit the network
            urlPattern: /^\/api\//,
            handler: 'NetworkOnly',
          },
        ],
      },
    }),
  ],
  server: {
    // Proxy API and SSE to the Go server in development
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
