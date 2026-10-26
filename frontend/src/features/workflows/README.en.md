# Workflow pages and contract wiring

This module composes workflow listing, editing, and run pages and owns API queries and feature translations. Pure graph rules belong to `runtime/`, canvas and inspectors to `editor/`, and run presentation to `run/`.

| Files                         | Responsibility                                  |
| ----------------------------- | ----------------------------------------------- |
| `api.ts`, `error-messages.ts` | Queries, mutations, and stable error mapping    |
| `*-page.tsx`                  | Route page composition                          |
| `*translations.ts`            | Feature copy and localized publication guidance |

Depends on generated APIs, primitives, identity, and space context; application routes consume this module. Publish errors map stable admission codes to actionable guidance without displaying raw server messages. Tests verify query boundaries and page behavior; known admission reasons must not collapse into generic guidance. The run page shows Loop and Iteration rounds: `run/run-rounds.ts` is the only place that projects the round document in `node_states` (archive plus the top-level active round) into rounds, so the final round appears once and members that did not run read as not reached; a successful Loop does not persist why it ended, so at the ceiling under `succeed` it only says "completed".
