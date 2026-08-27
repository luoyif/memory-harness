import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  base: './',
  build: {
    outDir: '../internal/server/web',
    emptyOutDir: true,
    sourcemap: false,
    target: 'es2022',
  },
})
