import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: { port: 5178 },
  build: {
    // elkjs ships one ~1.6 MB GWT-compiled bundle; keep it in its own chunk.
    chunkSizeWarningLimit: 1800,
    rollupOptions: { output: { manualChunks: { elk: ['elkjs/lib/elk.bundled.js'] } } },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
});
