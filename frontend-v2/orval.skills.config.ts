import { defineConfig } from 'orval'

// Temporary v2-only routes; the formal Cloud contract remains unchanged.
export default defineConfig({
  skills: {
    input: { target: '../v2-server/skills.openapi.json' },
    output: {
      target: 'src/lib/skills-api/generated.ts',
      client: 'react-query',
      httpClient: 'axios',
      clean: true,
      override: {
        mutator: {
          path: 'src/lib/api-client.ts',
          name: 'customInstance',
        },
      },
    },
  },
})
