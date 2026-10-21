/// <reference types="vitest" />
import { mkdirSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const KEEP = `# Keeps web/dist in version control.
#
# //go:embed all:dist needs the directory to exist and to hold at least one
# file, so a fresh clone must be buildable before the frontend has ever been
# built. Vite empties this directory on each build, so it is rewritten here -
# in the build itself, rather than in a wrapper script that a direct
# "npm run build" would skip.
`

/** Restores the placeholder that keeps web/dist buildable from a clone. */
function keepDistDirectory(outDir: string): Plugin {
  return {
    name: 'codelearn-keep-dist',
    apply: 'build',
    closeBundle() {
      const target = resolve(__dirname, outDir)
      mkdirSync(target, { recursive: true })
      writeFileSync(resolve(target, '.gitkeep'), KEEP)
    },
  }
}

// The build output lands in web/dist, which Go embeds into the binary, so a
// released CodeLearn needs neither Node.js nor a separate web server.
export default defineConfig({
  plugins: [react(), tailwindcss(), keepDistDirectory('../web/dist')],
  base: './',
  build: {
    outDir: '../web/dist',
    emptyOutDir: true,
    // Monaco is bundled in full; the local viewer is not size constrained.
    chunkSizeWarningLimit: 4000,
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    css: false,
  },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:8765',
    },
  },
})
