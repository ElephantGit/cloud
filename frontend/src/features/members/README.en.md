# members: tenant membership management

## Responsibility

Shows the selected collaboration space's tenant roster and lets administrators change active roles or disable members. A disabled member rejoins only after a fresh directory check, invitation, or approved request. Huawei deployments search employed people through Tianzhou and add them; public deployments create and revoke invitation/application links and decide requests.

It does not own login, space switching, or durable plaintext invitation tokens.

## Files

| File | Purpose |
| --- | --- |
| `api.ts` | Generated-client hooks for members, directory, links, and requests; local random tokens |
| `members-page.tsx` | Roster and management entry points selected by login provider |
| `members-page.test.tsx` | Tests for permissions, disabling, link creation, and directory additions |

## Dependencies and invariants

Depends on `features/spaces/current-space`, `features/auth/providers`, the generated client, and UI components; the settings route consumes it.

- Roles and status come only from tenant membership; removal writes `disabled` and retains the row.
- The tenant member update endpoint cannot create or reactivate membership directly, preserving directory employment checks and private joining.
- Management is displayed only to `admin`; corporate additions are rechecked by Ora server-side without exposing Tianzhou keys.
- The browser creates 256-bit tokens and shows a share link only after successful creation; later lists hold metadata only.

## Testing

The default roster is separate from administrator-only requests and join links. `member-roster.tsx` owns member actions and admission history; `join-links.tsx` and `join-requests.tsx` own entry management. `create-link-dialog.tsx` owns transient plaintext and close confirmation; `browser.ts` owns clipboard, local dates and expiry refresh. `directory-add.tsx` preserves directory admission. `translations.ts` owns member and joining copy in both languages.

Notes identify links without binding a recipient. Closing the generation dialog or switching spaces discards plaintext; copying again adds no uses. Unknown historical provenance is never inferred from a redemption. Ordinary members never fetch management history.

MSW verifies real endpoints, strict request bodies, and controls for each deployment mode.
