# squads: Squad management entry

[中文](README.md)

Squad management is unavailable. The list and legacy detail URLs explain this state and link to Settings → Members in the current workspace, where users can find real members who accepted invitations. These pages do not read demo squads, members or projects, fabricate counts, presence or execution capabilities, or interpret workspace membership as Squad membership.

| File | Responsibility |
| --- | --- |
| `squads-page.tsx` | Unavailable explanation and real member navigation |
| `squad-detail-page.tsx` | Reuses the explanation for legacy detail URLs without resolving demo identities |
| `*.test.tsx` | Workspace navigation, legacy details and exclusion of demo records |

Depends on unavailable and lib/paths; consumed by the route composition root. The members module and backend own real invitations and authorization. This module changes no memberships and starts no work. Navigation always preserves the supplied workspace slug instead of falling back to a demo workspace.
