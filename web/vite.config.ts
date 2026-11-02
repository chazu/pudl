import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

// `npm run dev` against a running `pudl ui`: PUDL_API=http://127.0.0.1:7468 npm run dev
// (Read via globalThis so the config needs no @types/node.)
const pudlApi = (globalThis as { process?: { env: Record<string, string | undefined> } }).process?.env.PUDL_API;

export default defineConfig({
  plugins: [react()],
  server: { port: 5178, proxy: pudlApi ? { '/api': pudlApi } : undefined },
  build: {
    // The bundle is embedded in the pudl binary (internal/webui, go:embed) and
    // committed, so building pudl never needs Node. Not "dist": a common global
    // gitignore pattern would silently drop it from commits.
    outDir: '../internal/webui/static',
    emptyOutDir: true,
    // elkjs ships one ~1.6 MB GWT-compiled bundle; keep it in its own chunk.
    chunkSizeWarningLimit: 1800,
    rollupOptions: { output: { manualChunks: { elk: ['elkjs/lib/elk.bundled.js'] } } },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
});
