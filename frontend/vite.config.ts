import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'
import { fileURLToPath } from 'url'
import { execSync } from 'child_process'
import process from 'node:process'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

// 版本号检测逻辑
let version = process.env.VITE_APP_VERSION || 'dev'
if (version === 'dev') {
  try {
    version = execSync('git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"').toString().trim()
  } catch {
    version = 'v0.0.0'
  }
}

export default defineConfig({
  plugins: [react(), tailwindcss()],
  define: {
    'import.meta.env.VITE_APP_VERSION': JSON.stringify(version)
  },
  build: {
    outDir: 'dist',
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return

          if (id.includes('/react/') || id.includes('/react-dom/')) {
            return 'react-vendor'
          }
          if (id.includes('/@radix-ui/') || id.includes('/@base-ui/')) {
            return 'ui-vendor'
          }
          if (id.includes('/axios/')) {
            return 'axios-vendor'
          }
          if (id.includes('/lucide-react/')) {
            return 'icons-vendor'
          }
          if (id.includes('/sonner/') || id.includes('/next-themes/')) {
            return 'app-vendor'
          }
        },
      },
    },
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:38765',
        changeOrigin: true,
        ws: true,
      },
    },
  },
})
