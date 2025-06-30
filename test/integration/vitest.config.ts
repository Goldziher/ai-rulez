import { defineConfig } from 'vitest/config'

export default defineConfig({
  test: {
    include: ['**/*.{test,spec}.{js,mjs,cjs,ts,mts,cts}'],
    exclude: ['**/node_modules/**', '**/dist/**'],
    timeout: 60000,
    testTimeout: 60000,
    hookTimeout: 60000,
    teardownTimeout: 60000,
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
      exclude: [
        '**/*.{test,spec}.{js,mjs,cjs,ts,mts,cts}',
        '**/node_modules/**',
        '**/dist/**',
        '**/*.d.ts',
      ],
    },
    reporters: ['verbose'],
  },
})