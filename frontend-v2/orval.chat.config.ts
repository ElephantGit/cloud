import { defineConfig } from 'orval'

// Temporary v2-only chat routes (v2-server/chat.openapi.json); the formal Cloud contract has no
// standalone conversations yet.
export default defineConfig({
  chat: {
    input: { target: '../v2-server/chat.openapi.json' },
    output: {
      target: 'src/features/chat/api/generated.ts',
      client: 'react-query',
      httpClient: 'axios',
      clean: true,
      override: { mutator: { path: 'src/lib/api-client.ts', name: 'customInstance' } },
    },
  },
})
