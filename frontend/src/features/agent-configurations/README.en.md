# agent-configurations: personal plugin configuration

[English](README.en.md) | [中文](README.md)

Owns My Configuration for the space's selected Agent release: non-secret options, personal connection selection and explicit repair. Space selection, execution and protocol conversion belong to their respective modules.

| File | Responsibility |
| --- | --- |
| `api.ts` | Personally scoped queries, versioned writes and bounded failure messages |
| `draft.ts` | Typed options, defaults, edit baselines and removed-field comparison |
| `numbers.ts` | Numeric response literals and exact request serialization for integers and decimals |
| `dialog.tsx` | Member-accessible entry that reads only when opened and returns the latest context after a write |
| `editor-state.ts` | Editing sessions, explicit conflict reconciliation, clear confirmation and the credential target |
| `editor.tsx` | Composition of options and separate connection forms |
| `options-form.tsx` / `setting-fields.tsx` | Typed fields and repair/clear actions |
| `reconcile.tsx` | Latest-value comparison with retained drafts or server adoption |
| `connections.tsx` | Protocol-compatible model choices, inline creation and the credential form of the selected connection |
| `dialog.test.tsx` / `*.test.*` / `test-fixtures.ts` | Production-dialog, option and HTTP form behavior tests with synthetic contexts |

Depends on generated clients, `spaces`, `model-connections`, `lib` and UI primitives; consumed by `plugins`. Lower layers must not depend on it. Connections and credentials reuse the personal library. OpenCode reuses the global default without copying model references into plugin options, and its credential controls follow the connection chosen in that default draft.

Edit baselines advance only after a successful write or explicit reconciliation; a write advances the option baseline in place from the freshly read context instead of rebuilding the editing session, so the independent default-model draft and its CAS version never revert silently. Background refreshes and conflicts retain drafts. Declaration changes require confirmed repair; unknown fields are never silently migrated or discarded. Clearing options does not delete connections. API keys use the existing write-only boundary and never enter configuration requests, query/mutation caches or persistent storage. Forms are siblings.

Tests use production generated clients and MSW for types, repair, conflicts, inline connections and credential isolation. `dialog.test.tsx` drives the production dialog to prove that save and clear advance only the option baseline, and the `plugins` tests cover lazy reading through the member entry.
