import { defineConfig } from 'orval'

export default defineConfig({ dashboard: {
  input: { target: '../v2-server/dashboard.openapi.json' },
  output: {
    target: 'src/features/dashboard/api/generated.ts', client: 'react-query', httpClient: 'axios', clean: true,
    override: { mutator: { path: 'src/lib/api-client.ts', name: 'customInstance' }, query: { version: 5 } },
  },
} })
