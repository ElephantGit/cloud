import { defineConfig } from 'orval'

// Temporary v2-only read-only workspace routes (v2-server/workspace.openapi.json). The client lives
// under src/lib because both the chat page and its workflow runs show the workspace panel.
export default defineConfig({
  workspace: {
    input: { target: '../v2-server/workspace.openapi.json' },
    output: {
      target: 'src/lib/workspace-api/generated.ts',
      client: 'react-query',
      httpClient: 'axios',
      clean: true,
      override: {
        mutator: { path: 'src/lib/api-client.ts', name: 'customInstance' },
        query: { version: 5 },
      },
    },
  },
})
