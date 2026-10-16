import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// In development, /api is proxied to the Go API so the browser sees a single origin
// (same as production, where nginx does the proxying). This keeps the session cookie
// first-party and avoids any CORS configuration.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    proxy: { '/api': { target: process.env.API_URL ?? 'http://localhost:8080', changeOrigin: false } },
  },
})
