import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  base: process.env.VITE_BASE_PATH || '/',
  plugins: [react(), tailwindcss()],
  resolve: {
    // Ensure consistent React module resolution to prevent duplicate React instances
    // which causes "Invalid hook call" errors in production builds
    dedupe: ['react', 'react-dom', 'scheduler', 'react-router', 'react-router-dom'],
  },
  build: {
    // The app intentionally separates heavyweight UI/markdown libraries into
    // stable vendor chunks. Keep the warning threshold aligned with those
    // deliberate cacheable chunks instead of warning on every production build.
    chunkSizeWarningLimit: 1200,
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
