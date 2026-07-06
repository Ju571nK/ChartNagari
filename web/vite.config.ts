/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  // Configurable for GitHub Pages project-site subpaths (e.g. /ChartNagari/).
  // Defaults to '/' for local dev and the embedded production build.
  base: process.env.BASE_PATH || '/',
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      // Forward /api/* to the Go server during development
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        // Split stable vendor libs into their own chunks: parallel download on
        // first load, and long-term browser caching across app releases.
        manualChunks: {
          'vendor-react': ['react', 'react-dom'],
          'vendor-charts': ['lightweight-charts'],
          'vendor-i18n': ['i18next', 'react-i18next', 'i18next-browser-languagedetector'],
        },
      },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test-setup.ts'],
  },
})
