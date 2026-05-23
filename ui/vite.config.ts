import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://150.136.8.120:31952',
        changeOrigin: true,
        headers: {
          Host: 'api.tinycloud.local',
        },
        rewrite: (path) => path.replace(/^\/api/, ''),
      },
    },
  },
})
