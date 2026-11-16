import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  // Browser mode: cmd/owon-web serves the API on this port.
  server: {
    proxy: {'/api': 'http://127.0.0.1:34199'},
  },
})
