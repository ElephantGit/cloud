# Scheduled tasks page

This module owns the tenant scheduled-task management screen: it reads definitions and the acceptance gate, renders tasks and occurrence history, and opens the creation form only when the Cloud deployment enables acceptance. It does not choose the gate, calculate due points, or execute unattended runs; the Cloud API and worker own those concerns.

| File | Responsibility |
| --- | --- |
| `api.ts` | Wraps HTTP queries and mutations for tasks, Issues, and agents. |
| `scheduled-tasks-page.tsx` | Composes the list, gate-controlled creation entry point, and history dialog. |
| `scheduled-task-form.tsx` | Collects and submits definition fields and unattended consent. |
| `scheduled-task-history.tsx` | Renders occurrence history for one task. |
| `scheduled-tasks-page.test.tsx` | Verifies the list, deployed gate, creation entry point, and key actions. |

The module depends on the generated API client, shared UI components, and TanStack Query; tenant routes may depend on its page component. The invariant is that the create entry point trusts only the list response's `acceptanceEnabled`, while the server independently validates writes. Tests use MSW to model that same list response and never require a live backend.
