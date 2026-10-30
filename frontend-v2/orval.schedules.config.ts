import { defineConfig } from 'orval'

// Temporary v2-only routes; the formal Cloud contract remains unchanged.
export default defineConfig({
  schedules: {
    input: { target: '../v2-server/schedules.openapi.json' },
    output: {
      target: 'src/features/schedules/api/generated.ts',
      client: 'react-query',
      httpClient: 'axios',
      clean: true,
      override: { mutator: { path: 'src/lib/api-client.ts', name: 'customInstance' } },
    },
  },
})
