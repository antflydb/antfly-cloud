import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const remote = process.env.REMOTE === '1'
const apiTarget = remote ? (process.env.ANTFLY_PROXY_URL ?? 'http://127.0.0.1:8765') : (process.env.ANTFLY_PROXY_URL ?? 'http://127.0.0.1:8765')
const nsfw = process.env.NSFW === '1'

// https://vite.dev/config/
export default defineConfig({
  define: {
    __NSFW_MODE__: JSON.stringify(nsfw),
  },
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      '/api': {
        target: apiTarget,
        changeOrigin: true,
      },
      ...(!remote && {
        '/termite': {
          target: 'http://localhost:11434',
          changeOrigin: true,
          rewrite: (path: string) => path.replace(/^\/termite/, ''),
        },
      }),
    },
  },
})
