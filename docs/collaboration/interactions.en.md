# Issue Collaboration: @ Targets, Interactions and Execution Intent

English | [中文](interactions.md)

`@`-mentioning a target in an Issue comment means "select a collaborator", not "execute". The selection
can be a person, an Agent, a team or a workflow, and each type produces a different interaction mode;
only an explicit execution intent — a task given to an Agent, or the confirmation of a workflow form —
creates a run. This document describes that model and its implementation in Cloud; the authority is the
[Issue collaboration ADR](https://github.com/ora-space/specs/blob/main/decisions/cloud/issue-collaboration/0-typed-targets-and-explicit-execution-intent.md).

## Three layers

```text
Comment ──< Interaction (one @ selection) ──0..1── IssueRun
 one comment     one row per selected target       created only by explicit intent
```

- **Comment ≠ Interaction ≠ Run**: one comment can `@` several targets and yield several interactions;
  an interaction produces at most one initial run. The comments table has no `run_id` and must never get
  one: a run's origin is recorded on the run itself in `trigger_evidence_kind` and
  `trigger_evidence_ref_id` (a comment, an interaction, or manual creation).
- `@x` or `mention://type/id` in the comment body is for display only and is not a routing protocol;
  targets arrive in the comment request's `targets` array:
  `[{"type":"agent","id":"…","task":"…"}]`.
- **A workflow is not an ActorRef.** It appears only on the target side, so a workflow's output is
  written as an activity by `system` rather than as a comment in the workflow's name.

## Target types and interaction modes

| Target `type` | Mode | What happens when the comment is posted |
|---|---|---|
| `user` | Mention | The interaction is recorded and **no** run is created. `@person` never triggers execution |
| `agent` | Task | Requires a non-empty `task` (separate from the comment body); the initial run is created in the same transaction |
| `team` | Task | The same contract as an Agent; no leader, members or delegation structure is assumed |
| `workflow` | Form | Only a "selected, unconfirmed" interaction is recorded and no run is created; execution happens after the user fills in and confirms the form |

Each selected target becomes one `issue_interactions` row with `mode` `mention`, `task` or `form`.
Interactions have no status column: `run_id IS NULL` means unconfirmed, a value means confirmed.

**Target discovery.** `GET /tenants/:tid/collaboration/targets?q=` returns what can be `@`-mentioned:
people come from the tenant's current members, Agents from the Space's Agent roster (`space_agents`,
formed by installed Agent plugins, see [Plugin marketplace](../plugins/marketplace.en.md)), and workflows
from the tenant's workflow documents. Each target carries an interaction descriptor
`{mode, requiresTask, formRef}`. Teams have no real backend in any deployment, so they never appear in
production target lists.

## Task Mode: @Agent

When the comment is posted, Cloud does all of this in one transaction:

1. Resolve the target in-transaction against `space_agents`; it must be an **active** Space Agent of this
   tenant. If it cannot be resolved there it falls back to the directory, and if still not found the
   answer is `404`, without revealing whether the target exists.
2. Apply the **Project gate**: the Issue must reference, through `projectRef`, an existing Project in the
   same Space as the Agent. Without a Project there is no repository and the run could never start, so
   the whole comment is rolled back together with its interactions and runs, returning
   `409 issue_project_required`.
3. Build the context snapshot (Issue title and description, the task, recent comments, context
   references) and write the Agent plugin's identity and its then-pinned version into the run input, so a
   later plugin upgrade never changes what a historical run executed.
4. Create a `queued` IssueRun and a `run.enqueued` activity. An Issue has at most one queued or dispatched
   run per executor at a time.

The run is dispatched only after the comment's transaction commits; Agent runs are carried forward by
Cloud's own background loops, see [Agent IssueRuns](../agent-runs/issue-runs.en.md).

## Form Mode: @workflow

A workflow has exactly one execution boundary: an explicit **confirm**.

```text
select workflow ──▶ read form descriptor ──▶ fill in (optional AI assist) ──▶ confirm ──▶ WorkflowRun + IssueRun
  no execution        no execution             no execution, no run              the only execution boundary
```

- The **form descriptor** (`GET /tenants/:tid/collaboration/forms/:formRef?issueId=…`) is an
  Issue-facing rendering description: `formRef`, title, description and a list of fields. It is not the
  workflow's schema and binds to no schema technology. Fields come in three bands: platform fields, the
  variables declared by the workflow's Start node, and the graph's global variables.
  The frontend first reads the current document only to locate the platform version selector and its
  default version. Once the default (the newest published version) is known, the initial form is read
  from that version through `GET /tenants/:tid/workflows/:wfid/snapshots/:snapshotId/form?issueId=…`,
  and switching the selector reloads the new version the same way. The form shown and the version Confirm
  validates are therefore always the same, and fields the live draft has not published never appear in
  the default view.
  That promise assumes the platform can name the version, and the selector itself comes from the live
  draft. When the live draft declares the reserved key `snapshotId` (or removes it through a launch
  field) there is no version to choose: the @ form renders the live draft while Confirm still applies
  the live draft's version rule and runs the newest published version. When the selector does exist but
  the chosen version's own graph declares `snapshotId` (or its select offers options that are not all
  published ids), that version's snapshot form has no platform field while Confirm still treats the key
  as the platform's: the author's value is read as a version cursor, so an empty value runs the newest
  published version and a non-id value is refused. Both are the boundary of an author occupying a
  reserved key, not a supported configuration: authors must not name their own variables `snapshotId`.
  Whether switching to such a version should be refused or warned about is a later decision (circus#405).
- **Platform fields** are injected only when the form is opened for an Issue: repository URL and branch
  (defaulting to the repository and default branch of the Issue's Project; required), a prompt, the
  published version to run `snapshotId`, and additional context references `context_refs`. When the
  workflow declares a variable of the same name, the workflow's declaration wins.
- **AI assist** (`POST .../issues/:iid/collaboration/assist`) only returns a field-level suggestion patch
  and suggested context references: it never creates a run and never writes persistent context
  references. Applying a suggestion is the user's action. It judges the shape of the submission against
  the same version descriptor Confirm uses, so once anything is published it accepts only the fields
  that version declares. Production has no assist provider wired in and returns `503 assist_unavailable`.
- **Confirm** (`POST .../issues/:iid/interactions/:ixid/confirm`) is the only transition that turns a
  workflow interaction into execution intent:
  - Frontend validation is only for user experience. On confirm Cloud resolves the target **again** and
    validates the submitted values against the form of **the published version being confirmed**; a
    workflow with no published version cannot be confirmed, because an unpublished draft cannot run.
  - Cloud validates against the selected published version's own fields alone, no longer comparing
    them with the live draft. Choosing an older version runs that snapshot: a submission that matches
    its form is accepted even when its fields differ from the current draft, and a missing required
    field or a field that version never declared is refused by that version's schema.
  - The submitted values are kept as the interaction's input and the executed version is frozen into the
    run input, so the run still knows which version it executed after the workflow is edited or
    published again.
  - One WorkflowRun and one IssueRun are created in the same transaction, and the IssueRun's
    `workflowInvocationRef` points at that WorkflowRun (a unique composite foreign key with the tenant).
    Direct runs, manual workflow IssueRuns and confirm share one creation function. A linked IssueRun is
    never handed to `ExecutionDispatcher`.
  - The interaction is claimed with a compare-and-set on `run_id IS NULL`; a repeated confirm returns
    `409 interaction_already_confirmed`, and confirming a non-form interaction returns
    `409 interaction_not_confirmable`.

## Ports: Issues own only the consuming-side contract

Issue collaboration asks external capabilities to "do one thing" through a set of ports, without
defining what those capabilities are internally:

| Port | Purpose | Production |
|---|---|---|
| `CollaborationDirectory` | List and resolve non-human targets | A chained directory: the Space Agent roster first, then the workflow directory |
| `FormDescriptorProvider` | Produce the form descriptor for a `formRef` | Forms projected from workflow documents |
| `InputAssistProvider` | AI assist for forms | Not wired (503) |
| `ContextBuilder` | Assemble a run's context snapshot | Not wired; falls back to a deterministic snapshot of task, form values, target and raw context references |
| `ExecutionDispatcher`, `ExecutionObserver` | Hand queued runs to an external executor and observe their progress | Not wired: team runs stay `queued` in production; workflow IssueRuns bypass it and execute through their linked WorkflowRun |

Agent runs do not go through `ExecutionDispatcher`; Cloud's own scheduling carries them (see
[Agent IssueRuns](../agent-runs/issue-runs.en.md)). Runs started directly from a workflow are a different
aggregate, WorkflowRun; see [Workflows](../workflows/workflows.en.md).

For a linked workflow IssueRun, actual WorkflowRun status updates advance it in the same transaction:
updates affecting no rows do not project, `awaiting_input` leaves it unchanged, and contract conflicts
roll back the transaction. Output summaries are `system` `run.message` activities bounded to 20000
bytes of valid UTF-8, never workflow-authored comments. Direct workflow runs without a linked IssueRun
create none of these collaboration records.

**Development fixtures.** `collaboration.development_fixtures: true` (environment variable
`CLOUD_COLLABORATION_DEVELOPMENT_FIXTURES`) installs the in-process doubles from `internal/collab` on the
Store: Agent and team targets with fixed IDs, a deterministic context builder (at most the 10 latest
comments, no AI), and a dispatcher and AI assist with fixed outputs. Workflow targets still come from the
database. The switch is for development and demos only and is off by default in production; it is fully
independent of authentication — enabling GitHub login never turns fixtures on, and an authentication
failure never falls back to a fixture identity. The doubles create no simulated domain tables.

## Related decisions

- [Issue collaboration: `@` only selects typed targets; execution comes only from explicit intent](https://github.com/ora-space/specs/blob/main/decisions/cloud/issue-collaboration/0-typed-targets-and-explicit-execution-intent.md)
- [An @Agent IssueRun on an Issue is orchestrated by Cloud](https://github.com/ora-space/specs/blob/main/decisions/cloud/issue-run/0-agent-run-in-disposable-isolated-workspace.md)
- [Workflows are executed by a Cloud-side scheduler](https://github.com/ora-space/specs/blob/main/decisions/cloud/workflow/0-cloud-side-scheduler-owns-workflow-execution.md) (approved)
