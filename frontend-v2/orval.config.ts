import { defineConfig } from 'orval'

// Same contract and shape as frontend/: generated TanStack Query hooks over one axios instance.
export default defineConfig({
  cloud: {
    input: { target: '../api/openapi.json' },
    output: {
      mode: 'tags-split',
      target: 'src/api/generated.ts',
      client: 'react-query',
      httpClient: 'axios',
      clean: true,
      override: {
        mutator: { path: 'src/lib/api-client.ts', name: 'customInstance' },
      },
    },
  },
})
