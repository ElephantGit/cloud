# Database migration directory

[中文](README.md) | [English](README.en.md)

SQL is embedded in the binary through `embed.FS` in `core/store.go`. `Store.Migrate` executes unapplied files in lexicographic order of complete filenames under a transaction advisory lock. `schema_migrations` stores complete filenames, SHA-256 and application times. Numeric prefixes are not unique identities: both `0027` files and both `0033` files apply independently.

## File index

There are currently 61 SQL files.

| SQL file | Schema change |
|---|---|
| [0001_core.sql](0001_core.sql) | Base domain tables |
| [0002_aggregate_guards.sql](0002_aggregate_guards.sql) | Aggregate ownership constraints |
| [0003_resource_versions.sql](0003_resource_versions.sql) | Resource versions |
| [0004_effect_intent_and_ticket_scope.sql](0004_effect_intent_and_ticket_scope.sql) | Effect intent and Ticket scope |
| [0005_gateway_auth.sql](0005_gateway_auth.sql) | Gateway login and sessions |
| [0006_collab_spaces.sql](0006_collab_spaces.sql) | Collaboration spaces and members |
| [0007_project_space_scope.sql](0007_project_space_scope.sql) | Default space and Project scope |
| [0008_issues.sql](0008_issues.sql) | Issue baseline |
| [0009_issue_extensions.sql](0009_issue_extensions.sql) | Issue statuses, comments, labels and views |
| [0010_issue_collaboration.sql](0010_issue_collaboration.sql) | Collaboration targets, runs, activities and references |
| [0011_issue_interactions.sql](0011_issue_interactions.sql) | Collaboration interactions |
| [0012_issue_interaction_input.sql](0012_issue_interaction_input.sql) | Confirmed interaction input |
| [0013_project_space_optional.sql](0013_project_space_optional.sql) | Nullable space scope (superseded by 0017) |
| [0014_clone_coordination.sql](0014_clone_coordination.sql) | Clone requests, executions and receipts |
| [0015_plugins.sql](0015_plugins.sql) | Plugin catalog, selection and instances |
| [0016_workspace_runtime_follows_node.sql](0016_workspace_runtime_follows_node.sql) | Workspace-local data and Node clone |
| [0017_tenant_membership_and_join.sql](0017_tenant_membership_and_join.sql) | One space per tenant and join flows |
| [0018_runtime_creators.sql](0018_runtime_creators.sql) | Runtime creators |
| [0019_runtime_control.sql](0019_runtime_control.sql) | Control sessions and epochs |
| [0020_force_stop_and_restart.sql](0020_force_stop_and_restart.sql) | Force-stop and restart |
| [0021_node_execution_identity.sql](0021_node_execution_identity.sql) | Node execution identity |
| [0022_plugin_maintenance_pending.sql](0022_plugin_maintenance_pending.sql) | Pending plugin maintenance intent |
| [0023_repository_credential_continuity.sql](0023_repository_credential_continuity.sql) | Repository credential continuity |
| [0024_agent_run_control_plane.sql](0024_agent_run_control_plane.sql) | Agent control plane |
| [0025_agent_control_integrity.sql](0025_agent_control_integrity.sql) | Command ordinals and session constraints |
| [0026_run_runtime_control.sql](0026_run_runtime_control.sql) | Independent run maintenance binding |
| [0027_verified_revisions.sql](0027_verified_revisions.sql) | Revision registration and verification |
| [0027_workflows.sql](0027_workflows.sql) | Workflow graphs |
| [0028_workflow_snapshots.sql](0028_workflow_snapshots.sql) | Published snapshots |
| [0029_workflow_runs.sql](0029_workflow_runs.sql) | Workflow run records |
| [0030_agent_run_business_lifecycle.sql](0030_agent_run_business_lifecycle.sql) | Agent business lifecycle |
| [0031_agent_run_thread_entries.sql](0031_agent_run_thread_entries.sql) | Thread entries |
| [0032_user_git_identities.sql](0032_user_git_identities.sql) | User Git identities |
| [0033_personal_model_connections.sql](0033_personal_model_connections.sql) | Personal model connections and bindings |
| [0033_revision_resume.sql](0033_revision_resume.sql) | Revision resume |
| [0034_agent_startup_windows.sql](0034_agent_startup_windows.sql) | Startup windows and residuals |
| [0035_model_attempt_statistics.sql](0035_model_attempt_statistics.sql) | Model request observations |
| [0036_workflow_asset_imports.sql](0036_workflow_asset_imports.sql) | Workflow asset imports |
| [0037_repository_bindings_and_operations.sql](0037_repository_bindings_and_operations.sql) | Repository Bindings, Operations and snapshots |
| [0038_agent_workspace_blocked_deadline.sql](0038_agent_workspace_blocked_deadline.sql) | Workspace blocked deadlines |
| [0039_workflow_run_identity.sql](0039_workflow_run_identity.sql) | WorkflowRun identity |
| [0040_executor_slot_bindings.sql](0040_executor_slot_bindings.sql) | Executor slots and bindings |
| [0041_workflow_execution_node.sql](0041_workflow_execution_node.sql) | Execution workflow-node ownership |
| [0042_workflow_run_plugin_version.sql](0042_workflow_run_plugin_version.sql) | Frozen run plugin version |
| [0043_external_git_line.sql](0043_external_git_line.sql) | Independent credentials and external Git line |
| [0044_model_attempt_workflow_run.sql](0044_model_attempt_workflow_run.sql) | WorkflowRun model request registration |
| [0045_agent_configuration_catalog.sql](0045_agent_configuration_catalog.sql) | Agent declaration association, static copy and validation issues in the plugin catalog |
| [0046_task_agent_conversations.sql](0046_task_agent_conversations.sql) | Task-level Agent conversations, message queue and Thread visible text |
| [0047_thread_preview_positions.sql](0047_thread_preview_positions.sql) | Node history positions for settled Thread entries |
| [0048_task_conversation_manual_pause.sql](0048_task_conversation_manual_pause.sql) | Manual pause for task conversations |
| [0049_thread_commands_workflow_run.sql](0049_thread_commands_workflow_run.sql) | WorkflowRun session-command ownership and node scope |
| [0050_personal_agent_configurations.sql](0050_personal_agent_configurations.sql) | Persistent personal Agent configuration revisions and private run snapshots |
| [0051_workflow_run_cancel.sql](0051_workflow_run_cancel.sql) | WorkflowRun cancellation requests |
| [0052_legacy_workflow_issue_run_settlement.sql](0052_legacy_workflow_issue_run_settlement.sql) | One-time settlement of legacy queued workflow IssueRuns |
| [0053_retire_tenant_level_clones.sql](0053_retire_tenant_level_clones.sql) | Retire tenant-level clones |
| [0054_git_delivery_refs.sql](0054_git_delivery_refs.sql) | Revisions delivered over Git |
| [0055_workflow_run_workspace_release.sql](0055_workflow_run_workspace_release.sql) | Workspace release facts for WorkflowRuns |
| [0056_workflow_invocation_reference.sql](0056_workflow_invocation_reference.sql) | Invocation reference from workflow IssueRuns to WorkflowRuns |
| [0057_agent_run_residual_reclamation.sql](0057_agent_run_residual_reclamation.sql) | Residual reclamation responsibility, appended evidence and disposition |
| [0058_agent_run_controlled_reclamation.sql](0058_agent_run_controlled_reclamation.sql) | Deployment-controlled reclamation intents, dependent data deletion and per-target termination |
| [0059_agent_restore_stop_evidence.sql](0059_agent_restore_stop_evidence.sql) | Named unconfirmed-stop evidence without changing residual responsibility or reclaim authority |

0050 copies no personal connection or key and backfills no historical run. Clearing configuration retains its revision; run options are stored separately from shared run input.

## Immutability and upgrades

- Never edit or rename applied SQL; corrections use new forward migrations. Repeated application skips applied files with matching checksums.
- `CheckSchema` only reads and rejects missing migrations, checksum mismatches and unknown database versions. Production services do not run DDL; operators migrate through [cloudctl](../../../cmd/cloudctl/README.en.md).
- Files describe their application-time changes, not the final schema alone. For example, 0017 supersedes 0013's nullable space scope; 0016 retains old storage/worktree records while retiring their steps.
- 0017 does not silently split multiple spaces or rename duplicate slugs; data violating one-space-per-tenant constraints needs prior handling.
- 0042 does not invent plugin versions for historical runs; code rejects execution without a frozen version. 0043 revokes old credential references and retains audit without treating old secret references as redeemable.
- 0045 adds `agent_configuration` (association and byte-exact static copy) and `agent_configuration_issue` (bounded reason) without backfilling existing rows. Only validated declarations are consumable; absence and invalid declarations are distinct, and invalid declarations do not block package installation. See [Plugin marketplace](../../../docs/plugins/marketplace.en.md) and `TestMigration0045AgentConfigurationCatalogAppliesFreshAndUpgrades` for fresh/upgrade/repeat coverage.
- 0046–0048 backfill no old messages, Threads, history positions or pause state; `visible_text` is stored separately from the raw `record`, and old tasks are not paused automatically. Fresh, upgrade and repeated application are covered by `TestMigration0046TaskConversationFreshAndUpgrade` and the integration migration tests.
- 0049 lets `thread_commands` accept a WorkflowRun owner and requires a graph node on workflow commands; each node holds at most one `end_session`, so the scheduler can replay it safely. Every old row carries `run_id` and is not backfilled. Semantics are in [workflows](../../../docs/workflows/workflows.en.md); coverage is in the `TestWorkflowSessionEndsAfterSuccessfulTurn` DB tests.
- 0051 adds the cancellation-request instant to `workflow_runs` and backfills no old row; the request records intent only and is settled by the scheduler on later passes. Semantics are in [workflows](../../../docs/workflows/workflows.en.md#scheduling); coverage is in the `TestWorkflowRunCancelBeforeStartNeverStarts` DB tests.
- 0052 settles the live `executor_type='workflow'` IssueRuns still `queued`/`dispatched` at upgrade time: they have no executor and no frozen workflow identity, so the ADR's D3 marks them failed with `failure_reason='workflow_execution_unavailable'` instead of migrating them. Other runs and soft-deleted rows are untouched. Rows of that shape stop being produced only once the workflow launch gate binds a manual run to a real WorkflowRun; until that gate (circus#71) lands, `createRun` keeps producing new ones, and this migration runs once at upgrade time and does not settle them. Coverage is `TestMigration0052LegacyWorkflowIssueRunSettlementAppliesFreshAndUpgrades`.
- 0053 is the switch to Workspaces cloned from Git Server snapshots and migrates no data: clone executions that belong to no Workspace and their receipts are deleted, `clone_executions.clone_request_id` and the `one_request_execution` index are dropped, `clone_executions.workspace_id` becomes NOT NULL, and `clone_requests` is dropped; Workspace clone executions and their receipts stay, and it is retry-safe. See [Workspaces](../../../docs/runtime/workspaces.en.md); upgrading a database that still holds tenant-level clones is covered by `TestMigration0053RetiresTenantLevelClonesKeepingWorkspaceClones`.
- 0054 is the switch to Revisions delivered over Git and migrates no data: `revisions` gains `delivery_ref`, and `revisions_bundle_shape` is replaced by `revisions_shape` (with a NULL `delivery_ref` a row keeps a legacy bundle shape; with one it must be a well-formed `refs/ora/deliveries/<UUID>` and carry no bundle column); the partial unique index `revisions_delivery_ref` lets one delivery ref belong to one Revision only. Existing bundle rows are never rewritten, and it is retry-safe. See [Revisions](../../../docs/agent-runs/revisions.en.md); the constraints are covered by `TestRevisionShapeKeepsGitAndBundleRowsApart`.
- 0055 adds the forward-only `workspace_release_state` (`pending`/`releasing`/`released`/`abandoned`/`not_required`), `workspace_release_error` and `workspace_released_at` to `workflow_runs`, and backfills by upgrade-time state: non-terminal rows stay `pending`; terminal rows with no Workspace become `not_required`, those whose Workspace is already soft-deleted become `released` (using that row's `deleted_at`), and the rest become `releasing`. Two CHECK constraints require a release instant for `released` and a reason for `abandoned`. Only `released` means Cloud observed the Workspace deleted; `abandoned` keeps the Workspace row and writes no release instant. Semantics are in [workflows](../../../docs/workflows/workflows.en.md#scheduling); coverage is `TestMigration0055WorkflowRunWorkspaceReleaseAppliesFreshAndUpgrades` and `internal/core/workflow_run_release_db_test.go`.
- 0056 first settles every unlinked non-terminal historical workflow IssueRun (including running and deferred) as failed with `workflow_execution_unavailable_before_invocation_linkage`, preserving its input and outcome without inventing an execution. It then adds the `(workflow_invocation_ref, tenant_id)` → `workflow_runs(id, tenant_id)` composite foreign key (targeting the `workflow_run_tenant_identity` unique constraint 0044 already added, without duplicating it), a unique reference, and a CHECK that non-terminal workflow rows carry a reference while other executors carry none, all unconditionally validated. Historical terminal workflow rows may keep a NULL reference. Fresh, seeded upgrade, repeated application and constraint validation coverage is in `integration/workflow_invocation_migration_test.go`.
- 0057 only adds residual-ledger columns: defaults initialize existing D4/D5 rows as `reconciling`, without backfilling targets or evidence, rewriting old `reason`, `responsibility` or business settlement, or backfilling D8. Legacy rows with non-null `cleared_at` cause an explicit migration error and rollback pending compatibility handling (A2). Reentrant DDL preserves progressed rows. The schema accepts `released`, `terminated_data_retained` and `externally_confirmed`; this step's automatic path implements only `released`, leaving the other dispositions to later controlled entry points. The authorization transaction checks the operation's tenant and Workspace relationships; a UUID foreign key alone does not establish resource scope. See [agent_run_residual_reclaim_migration_test.go](../../../integration/agent_run_residual_reclaim_migration_test.go) for fresh, upgrade, DDL replay and constraint coverage.
- 0058 preserves 0057 history, responsibility, evidence, disposition and first `cleared_at`, and never backfills D8. Only an explicit root intent owns protection and evidence for ledgerless D8. Root termination and dependent data deletion each have three states; each resource has one root, at most one deletion intent and one fixed deletion operation. Command keys are unique throughout a tenant. Successful targets require both boolean confirmations, so a missing JSON field cannot pass through SQL NULL. Fresh, 0057-upgrade and constraint coverage is in [agent_run_controlled_reclaim_migration_test.go](../../../integration/agent_run_controlled_reclaim_migration_test.go).
- 0059 only adds `stop_evidence`, initially empty for existing rows; no historical stop observation is invented. Covered by `TestMigration0059RestoreStopEvidenceFreshUpgradeAndRepeat`.
- New migrations must verify fresh databases, upgrades from the preceding schema, repeated application and key constraints. See [migration_upgrade_path_test.go](../../../integration/migration_upgrade_path_test.go) and domain upgrade tests in integration.

See [Persistence](../../../docs/core/persistence.en.md) for ownership/transactions, and [Workflows](../../../docs/workflows/workflows.en.md) / [External Git line](../../../docs/repositories/external-git.en.md) for runtime semantics.
