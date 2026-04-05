import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { execSync } from 'child_process'

let version = process.env.VITE_APP_VERSION || 'dev'
if (version === 'dev') {
  try {
    version = execSync('git describe --tags --abbrev=0').toString().trim()
  } catch (e) {
    version = 'v4.03'
  }
}

export default defineConfig({
  plugins: [react(), tailwindcss()],
  define: {
    'import.meta.env.VITE_APP_VERSION': JSON.stringify(version)
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:38765',
        changeOrigin: true,
      },
    },
  },
})
