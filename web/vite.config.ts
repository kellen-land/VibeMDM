import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const backend = 'http://localhost:8080'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: { outDir: '../internal/webui/dist', emptyOutDir: true },
  server: {
    port: 5173,
    proxy: {
      '/api': backend,
      '/agent': backend,
      '/enroll': backend,
      '/healthz': backend,
    },
  },
})
