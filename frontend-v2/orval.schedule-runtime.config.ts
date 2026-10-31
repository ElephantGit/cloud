import { defineConfig } from 'orval'

// Exact candidate contracts supplied by the schedule action/history owners.
export default defineConfig(Object.fromEntries(['actions', 'history'].map(name => [name, {
  input: { target: `../v2-server/schedule-${name}.openapi.json` },
  output: {
    target: `src/features/schedules/api/${name}/generated.ts`,
    client: 'react-query', httpClient: 'axios', clean: true,
    override: { mutator: { path: 'src/lib/api-client.ts', name: 'customInstance' } },
  },
}])))
