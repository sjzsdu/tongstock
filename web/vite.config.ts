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
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return undefined

          if (id.includes('/react/') || id.includes('/react-dom/') || id.includes('/scheduler/')) {
            return 'react-vendor'
          }
          if (id.includes('react-router')) {
            return 'router-vendor'
          }
          if (id.includes('react-markdown') || id.includes('marked') || id.includes('highlight.js') || id.includes('rehype-') || id.includes('github-markdown-css')) {
            return 'markdown-vendor'
          }
          if (id.includes('lucide-react') || id.includes('@ant-design/icons')) {
            return 'icons-vendor'
          }
          if (id.includes('antd') || id.includes('@ant-design') || id.includes('rc-')) {
            return 'antd-vendor'
          }
          if (id.includes('lightweight-charts')) {
            return 'charts-vendor'
          }
          return 'vendor'
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
