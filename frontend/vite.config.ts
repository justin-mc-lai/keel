import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  build: {
    // Keep the directory itself: the Go side embeds frontend/dist, so a fresh
    // clone must still compile before the frontend has ever been built.
    emptyOutDir: false
  }
})
