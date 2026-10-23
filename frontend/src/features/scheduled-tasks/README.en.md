# Scheduled tasks

This module owns the **screens** for scheduled tasks: the list and form, and one task's run history.
It owns no server fact: admission, verdicts and artifact counts all come from Cloud's REST
resources; the module requests, renders and turns user actions into mutations.

## Files

| File | Responsibility |
| --- | --- |
| `api.ts` | The queries and mutations this module uses (list, history, create, enable/disable) |
| `scheduled-tasks-page.tsx` | The page: list, creation entry, opening a task's history |
| `scheduled-task-form.tsx` | Create and edit form |
| `scheduled-task-history.tsx` | History dialog: each occurrence's admission result, verdict and artifact counts |
| `scheduled-tasks-page.test.tsx` | Page behavior tests |
| `scheduled-run-artifacts.tsx` | Expands registered artifacts and requests separate short-lived downloads |
| `scheduled-run-artifacts.test.tsx` | Download authorization, refusal, retry and link safety tests |
| `scheduled-task-history.test.tsx` | History rendering tests, including "not reported" |

## Dependencies

- Depends on: the generated client under `@/api`, `@/components/ui`, and the boundary adapters in
  `@/lib`.
- May be depended on by: the route/page composition layer.

## Invariants

- **"Not reported" is a value, not a missing field.** A run without a verdict renders 未报告; do not
  hide the line, and never treat an empty string as a pass.
- **The counts describe registered objects only.** `artifactCount` comes from Cloud's index and
  `rejectedArtifactCount` counts refused candidates; they do not add up, and neither says whether the
  reader may download anything - Cloud authorises downloads separately, by issue visibility.
- **Server facts are not cached in the frontend.** History and counts are read through TanStack Query
  only and are never copied into component state.

## Tests

The history tests serve fixed responses through MSW and assert what renders (the verdict text and the
rejection count). Do not mock internal modules.

The creation entry trusts the Cloud list response’s `acceptanceEnabled`; this module does not choose the gate, and the server independently validates writes.

Artifact lists carry no download capability. Explicit clicks request current authorization; collapsing the list discards links. Downloads accept only HTTP(S) URLs without embedded credentials and open separately without a referrer; artifact content is never rendered inside the app.

History uses `artifactsExpired` to identify artifacts removed after successful delivery; a run that produced none is never labeled expired.
