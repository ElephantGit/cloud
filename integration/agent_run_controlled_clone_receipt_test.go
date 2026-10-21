package integration

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/wanglongan587/cloud/internal/controlpb"
	"github.com/wanglongan587/cloud/internal/core"
)

// Only the preexisting business failure is staged. Both sources acquire controlled
// authority through ManageResidual, and both clone events use the real takeover RPC.
func TestControlledResidualCloneReceiptConflictThroughRegisteredIntentsOverGRPC(t *testing.T) {
	for _, source := range []string{"legacy_d8", "residual_ledger"} {
		t.Run(source, func(t *testing.T) {
			f, c, run, workspace, operation, original := cloneReceiptScene(t)
			ctx := context.Background()
			legacyOperation := ""
			if source == "residual_ledger" {
				_, err := c.operations.DeferOperation(c.ctx, &controlpb.DeferOperationRequest{
					SubmissionId: c.sub(), Epoch: c.epoch, OperationId: operation.GetId(), Version: operation.GetVersion(),
					State: controlpb.DeferState_DEFER_STATE_BLOCKED, Reason: controlpb.DeferReason_DEFER_REASON_NODE_UNAVAILABLE, RetrySeconds: 60,
				})
				must(t, err)
				_, err = f.store.Pool.Exec(`UPDATE issue_runs SET workspace_deadline_at=clock_timestamp()-interval '1 second' WHERE id=$1`, run)
				must(t, err)
				must(t, f.store.GiveUpBlockedWorkspaceCreationsOnce(ctx))
			} else {
				// A D8 Workspace has a failed historical delete and no residual ledger.
				// Do not create a ledger and then delete it to manufacture this source.
				_, err := f.store.Pool.Exec(`UPDATE issue_runs SET phase='done',status='failed',thread_state='ended',idle_since=NULL,
 failure_reason='agent_start_failed',completed_at=clock_timestamp(),version=version+1,
 result='{"deliveryState":"failed","testBoundary":"preexisting_business_terminal"}'::jsonb WHERE id=$1`, run)
				must(t, err)
				_, err = f.store.Pool.Exec(`UPDATE workspaces SET admission_open=false,admission_epoch=admission_epoch+1,
 version=version+1 WHERE id=$1`, workspace)
				must(t, err)
				_, err = f.store.Pool.Exec(`UPDATE operations SET state='failed',error_code='node_unavailable',
 version=version+1,updated_at=clock_timestamp() WHERE id=$1`, operation.GetId())
				must(t, err)
				legacyOperation = uuid.NewString()
				_, err = f.store.Pool.Exec(`INSERT INTO operations(id,tenant_id,actor_user_id,project_id,workspace_id,
 kind,state,step,request,idempotency_key,request_hash,error_code,controller_epoch)
 SELECT $2::uuid,w.tenant_id,w.creator_user_id,w.project_id,w.id,'delete_workspace','failed','terminate',
 '{}','controlled-clone-legacy-delete','controlled-clone-legacy-fixture','node_unavailable',$3
 FROM workspaces w WHERE w.id=$1`, workspace, legacyOperation, c.epoch)
				must(t, err)
			}

			termination := core.ResidualManagementRequest{
				Action: core.ResidualTerminate, TenantID: f.tid, RunID: run, WorkspaceID: workspace, OperationID: legacyOperation,
				Key: "registered-clone-terminate", Reason: "Terminate the original residual while retaining its data", ImpactConfirmed: true,
			}
			accepted, err := f.store.ManageResidual(ctx, &termination)
			must(t, err)
			root := accepted.O("intent").S("id")
			if root == "" {
				t.Fatalf("production termination command returned no intent: %v", accepted)
			}
			var registered bool
			must(t, f.store.Pool.QueryRow(`SELECT i.source=$2 AND i.command_kind='terminate' AND i.root_intent_id IS NULL
 AND i.run_id=$3 AND i.workspace_id=$4 AND i.impact_confirmed
 AND EXISTS(SELECT 1 FROM agent_run_reclamation_command_receipts cr WHERE cr.intent_id=i.id AND cr.idempotency_key=$5)
 AND EXISTS(SELECT 1 FROM jsonb_array_elements(i.audit) a WHERE a->>'kind'='registered')
 AND CASE WHEN $2='legacy_d8' THEN i.legacy_delete_operation_id::text=$6
 AND NOT EXISTS(SELECT 1 FROM agent_run_residuals r WHERE r.run_id=i.run_id)
 ELSE EXISTS(SELECT 1 FROM agent_run_residuals r WHERE r.run_id=i.run_id
 AND r.reclaim_mode='controlled' AND r.reclaim_state='reclaiming' AND r.reclaim_id=i.reclaim_id) END
 FROM agent_run_reclamation_intents i WHERE i.id=$1`, root, source, run, workspace, termination.Key,
				legacyOperation).Scan(&registered))
			if !registered {
				t.Fatal("controlled intent did not come from the requested real source and production command")
			}

			before := cloneReceiptCanonicalSnapshot(t, f, original.GetExecutionId())
			conflict := proto.Clone(original).(*controlpb.TakeOverNodeEventRequest)
			conflict.SubmissionId, conflict.Event = c.sub(), []byte("clone event B")
			response, err := f.executions.TakeOverNodeEvent(c.ctx, conflict)
			expectStatus(t, err, codes.Aborted, controlpb.ErrorCode_ERROR_CODE_CONFLICT)
			if response != nil || status.Convert(err).Message() != "residual_fact_conflict" {
				t.Fatalf("registered intent lost receipt-only conflict: response=%v error=%v", response, err)
			}

			// The legacy adapter must persist through legacyIntentId into the root intent;
			// ledger-backed authority must persist into its existing residual row instead.
			var audited bool
			must(t, f.store.Pool.QueryRow(`SELECT
 CASE WHEN i.source='legacy_d8' THEN i.conflicted_at IS NOT NULL AND r.run_id IS NULL
 ELSE r.reclaim_conflicted_at IS NOT NULL END
 AND (SELECT count(*) FROM jsonb_array_elements(
 CASE WHEN i.source='legacy_d8' THEN i.evidence ELSE r.reclaim_evidence END) x
 WHERE x->>'key'=$2 AND x->'fact'->>'kind'='conflict'
 AND x->'fact'->'original'->>'kind'='receipt' AND x->'fact'->'original'->>'event'=$3
 AND x->'fact'->'incoming'->>'kind'='receipt' AND x->'fact'->'incoming'->>'event'=$4
 AND x->>'receivedAt'<>'' AND x->'source'->>'submissionId'=$5)=1
 AND EXISTS(SELECT 1 FROM control_submissions WHERE submission_id=$5 AND response->>'residualFactConflict'='true')
 FROM agent_run_reclamation_intents i LEFT JOIN agent_run_residuals r ON r.run_id=i.run_id WHERE i.id=$1`,
				root, fmt.Sprintf("clone_receipt:%s:%d", original.GetExecutionId(), original.GetSequence()),
				base64.StdEncoding.EncodeToString(original.GetEvent()), base64.StdEncoding.EncodeToString(conflict.GetEvent()),
				conflict.GetSubmissionId()).Scan(&audited))
			if !audited {
				t.Fatal("conflict response preceded committed receipt evidence, conflict time or submission response on its source")
			}
			if after := cloneReceiptCanonicalSnapshot(t, f, original.GetExecutionId()); after != before {
				t.Fatalf("receipt-only conflict changed original clone result or receipt:\nbefore=%s\nafter=%s", before, after)
			}

			// Fresh command keys force the production validation path instead of replaying
			// the successful registration receipt. Both actions must fence the conflict first.
			termination.Key = "registered-clone-terminate-after-conflict"
			for _, request := range []*core.ResidualManagementRequest{
				&termination,
				{
					Action: core.ResidualDeleteData, TenantID: f.tid, IntentID: root,
					Key: "registered-clone-delete-after-conflict", Reason: "Separately authorize retained data deletion", DataDeleteConfirmed: true,
				},
			} {
				out, commandErr := f.store.ManageResidual(ctx, request)
				var fault *core.Fault
				if out != nil || !errors.As(commandErr, &fault) || fault.Status != 409 || fault.Code != "residual_fact_conflict" {
					t.Fatalf("%s bypassed registered receipt conflict: response=%v error=%v", request.Action, out, commandErr)
				}
			}
			if f.scalar(`SELECT count(*) FROM agent_run_reclamation_intents WHERE root_intent_id=$1`, root) != 0 ||
				f.scalar(`SELECT count(*) FROM external_effects WHERE workspace_id=$1 AND kind='workspace_data_delete'`, workspace) != 0 {
				t.Fatal("blocked controlled commands created a data-delete intent or effect")
			}
		})
	}
}
