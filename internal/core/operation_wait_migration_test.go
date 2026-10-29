package core

import "testing"

func TestOperationWaitMigrationPreservesLegacyExecutionAndDefaults(t *testing.T) {
	s := operationWaitDBThrough(t, "0073_member_join_provenance.sql")
	a := seedOperationWait(t, s, true)
	seedControllerLease(t, s)
	operationWaitClaim(t, s)
	execution := operationWaitDispatch(t, s, a)
	operationWaitResult(t, s, a, execution, "clone_ready")
	body, err := migrations.ReadFile("migrations/0074_node_unavailable_attempts.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Explicitly exercise the forward upgrade and its documented retry behavior.
	for i := 0; i < 2; i++ {
		if _, err := s.Pool.Exec(string(body)); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	var node, commit string
	if err := s.Pool.QueryRow(`SELECT o.node_unavailable_attempts,e.node_id,e.result->>'commit'
 FROM operations o JOIN clone_executions e ON e.operation_id=o.id WHERE e.execution_id=$1`, execution).Scan(&count, &node, &commit); err != nil {
		t.Fatal(err)
	}
	if count != 0 || node != a.node || commit != baseCommit {
		t.Fatalf("migration rewrote legacy state: %d/%s/%s", count, node, commit)
	}
	if _, err := s.Pool.Exec("UPDATE operations SET node_unavailable_attempts=-1 WHERE id=$1", a.operation); err == nil {
		t.Fatal("negative count accepted")
	}
}
