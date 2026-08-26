import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { tanstackStart } from '@tanstack/react-start/plugin/vite'

// TanStack Start in SPA mode (operator decision superseding ADR-012): the
// build prerenders a static shell + client bundle into dist/client, which is
// embedded in the styrman Go binary. No Node at runtime — the Go server is
// the only backend. The dev server proxies API/WS to a running styrman.
export default defineConfig({
  plugins: [
    tanstackStart({
      spa: { enabled: true },
    }),
    react(),
    tailwindcss(),
  ],
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:8080',
      '/ws': { target: 'ws://127.0.0.1:8080', ws: true },
    },
  },
  build: { chunkSizeWarningLimit: 1200 },
})
