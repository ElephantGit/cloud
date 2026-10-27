# Workflow pages and contract wiring

This module composes workflow listing, editing, and run pages and owns API queries and feature translations. Pure graph rules belong to `runtime/`, canvas and inspectors to `editor/`, and run presentation to `run/`.

| Files                         | Responsibility                                  |
| ----------------------------- | ----------------------------------------------- |
| `api.ts`, `error-messages.ts` | Queries, mutations, and stable error mapping    |
| `*-page.tsx`                  | Route page composition                          |
| `*translations.ts`            | Feature copy and localized publication guidance |

Depends on generated APIs, primitives, identity, and space context; application routes consume this module. Publish errors map stable admission codes to actionable guidance without displaying raw server messages. Tests verify query boundaries and page behavior; known admission reasons must not collapse into generic guidance.
Leaving the editor waits for its draft save; after a failure the canvas remains visible while the author chooses to stay or discard changes and leave. Navigation must not retry a rejected save indefinitely.
The run page shows Loop and Iteration rounds: `run/run-rounds.ts` is the only place that projects the round document in `node_states` (archive plus the top-level active round) into rounds, so the final round appears once and members that did not run read as not reached; a successful Loop's recorded `reason` (`condition_satisfied` / `max_iterations` / `loop_exit`) is used when present; without it only committed facts are used, so at the ceiling under `succeed` it only says "completed".
An agent node's inspector reads its recorded conversation through `useWorkflowNodeSession`: the walk is pinned to one attempt and pages backward with the response's own `nextBefore`, so a retry cannot splice another attempt's records onto the page. `run/run-node-session-view.ts` projects the loaded pages oldest first and reports the read state the server proved, the per-page integrity warning, and the node's end reason only when the node's state describes the attempt being read; a state with no records is named (`not_started` / `awaiting_first_record` / `unavailable` / `unknown`) instead of drawn as an empty conversation, and the end reason never doubles as the output.
