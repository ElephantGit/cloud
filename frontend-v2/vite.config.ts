import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// v2-server's address; the end-to-end tests point it at their own instance.
const backend = process.env.V2_SERVER ?? 'http://127.0.0.1:8090'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { '@': path.resolve(import.meta.dirname, './src') },
  },
  server: {
    // v2-server's default address (see v2-server/README.md). Like Cloud's Gateway, it serves the
    // login routes, the API and the health check; nothing else is proxied.
    proxy: {
      '/api': backend,
      '/auth': backend,
      '/healthz': backend,
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
  },
})
