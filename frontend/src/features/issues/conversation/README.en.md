# Task conversation

[中文](README.md) | English

Task-level conversation reads, sends, pending handoffs, and Agent selection. The current session initiator can confirm a graceful end on the task page; “Stop current execution” still cancels and pauses the queue. A run's Thread remains owned by `issues/thread`; this module does not own execution state or Node connections.

| File | Responsibility |
| --- | --- |
| `api.ts` | Bidirectional task message paging, send/stop/cancel/retry/resume/explicit-discard API and safe error copy. |
| `conversation-panel.tsx` | Combines task comments, Agent Runs and pending messages; offers authorized ending only for the session currently occupying the task. |
| `conversation-composer.tsx` | One Agent target, explicit mention picker, Agent message/plain comment modes and handoff hint. |
| `use-conversation-draft.ts` | One task-scoped draft and history quote; changing the Agent only changes the input target. |
| `task-stop.tsx` / `task-recovery.tsx` | Stop the current execution, retain a task pause and explicitly recover saved/discarded results independently of history browsing. |
| `agent-readiness.tsx` / `use-composer-submit.ts` | Show current-user model/project gaps before sending, share submission eligibility and preserve refused drafts. |
| `*.test.ts(x)` | MSW and component scenarios for closed-session continuation, switching, failure and queueing. |

Depends on generated `src/api`, `features/issues/api`, `features/issues/thread`, `features/spaces/api`, `features/conversation-content`, and `components/ui`. Only server-authorized roster targets are selectable; the selector and `@` picker change recipient only on explicit selection, while an ordinary typed `@name` has no routing effect. Changing a local selection never starts a run. Historical Run selection is read-only and the task composer always addresses the current target. The end action belongs only to the server's `activeRunId` when Thread `canEnd` is true and the current user initiated that session; a later queued Run cannot hide the active session's end action. Pages merge by server seq, and a retry after an unknown response reuses its idempotency key.

The composer occupies normal document flow, never overlays task history, and requires no fixed-height spacer to read the latest message.
