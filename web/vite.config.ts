import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// Static SPA embedded in the styrman Go binary (ADR-006). The dev server
// proxies API + WebSocket calls to a locally running styrman server.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:8080',
      '/ws': { target: 'ws://127.0.0.1:8080', ws: true },
    },
  },
  build: { chunkSizeWarningLimit: 1200 },
})
