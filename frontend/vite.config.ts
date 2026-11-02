import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  build: {
    target: 'safari16',
    outDir: 'dist',
    emptyOutDir: true,
    assetsInlineLimit: 0,
    reportCompressedSize: false,
  },
  server: { strictPort: true, port: 5180 },
})
