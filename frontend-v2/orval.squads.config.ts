import { defineConfig } from 'orval'

// Temporary v2-only routes; the formal Cloud contract remains unchanged.
export default defineConfig({
  squads: {
    input: { target: '../v2-server/squads.openapi.json' },
    output: {
      target: 'src/features/collaboration/api/generated.ts',
      client: 'react-query',
      httpClient: 'axios',
      clean: true,
      override: { mutator: { path: 'src/lib/api-client.ts', name: 'customInstance' } },
    },
  },
})
