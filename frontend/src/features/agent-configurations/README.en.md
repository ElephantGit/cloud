# agent-configurations: personal plugin configuration

[English](README.en.md) | [中文](README.md)

Owns My Configuration for the space's selected Agent release: non-secret options, personal connection selection and explicit repair. Space selection, execution and protocol conversion belong to their respective modules.

| File | Responsibility |
| --- | --- |
| `api.ts` | Personally scoped queries, versioned writes and bounded failure messages |
| `draft.ts` | Typed options, defaults, edit baselines and removed-field comparison |
| `dialog.tsx` | Member-accessible entry that reads only when opened |
| `editor-state.ts` | Editing sessions, explicit conflict reconciliation and clear confirmation |
| `editor.tsx` | Composition of options and separate connection forms |
| `options-form.tsx` / `setting-fields.tsx` | Typed fields and repair/clear actions |
| `reconcile.tsx` | Latest-value comparison with retained drafts or server adoption |
| `connections.tsx` | Protocol-compatible model choices, inline creation and credential forms |
| `*.test.*` / `test-fixtures.ts` | Synthetic contexts, option and HTTP form behavior tests |

Depends on generated clients, `spaces`, `model-connections`, `lib` and UI primitives; consumed by `plugins`. Lower layers must not depend on it. Connections and credentials reuse the personal library. OpenCode reuses the global default without copying model references into plugin options.

Edit baselines advance only after a successful write or explicit reconciliation. Background refreshes and conflicts retain drafts. Declaration changes require confirmed repair; unknown fields are never silently migrated or discarded. Clearing options does not delete connections. API keys use the existing write-only boundary and never enter configuration requests, query/mutation caches or persistent storage. Forms are siblings.

Tests use production generated clients and MSW for types, repair, conflicts, inline connections and credential isolation. The `plugins` tests cover lazy reading through the member entry.
