-- The scheduled artifact upload is a run-scoped Node execution like the session and the Revision
-- delivery, so it belongs to the same closed kind set.

ALTER TABLE node_executions DROP CONSTRAINT node_executions_kind_check;
ALTER TABLE node_executions ADD CONSTRAINT node_executions_kind_check
 CHECK (kind IN ('install_plugins','remove_plugins','agent_session','deliver_revision','deliver_scheduled_artifacts'));

-- One unfinished execution per run *and* kind. A scheduled run holds its Revision delivery and its
-- artifact upload at the same time, so the previous one-per-run rule would have refused the upload
-- while the delivery was still running, which is exactly when it must be allowed.
DROP INDEX one_pending_run_execution;
CREATE UNIQUE INDEX one_pending_run_execution ON node_executions(operation_id, kind)
 WHERE kind IN ('agent_session','deliver_revision','deliver_scheduled_artifacts')
   AND result IS NULL AND terminated_by_force_stop_id IS NULL;
