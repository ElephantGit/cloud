package integration

import "testing"

func TestMemberJoinMigrationPreservesUnknownHistoryAndRepeats(t *testing.T) {
	pool, _ := testSchema(t, "test_join_provenance_upgrade_")
	applyMigrationsUpTo(t, pool, migrationsBefore(t, "0073_member_join_provenance.sql"))
	seed := seedOwnershipChain(t, pool)
	var before, after string
	must(t, pool.QueryRow("SELECT row_to_json(m)::text FROM tenant_memberships m WHERE tenant_id=$1 AND user_id=$2", seed.tenantID, seed.userID).Scan(&before))
	migratedTwice(t, newStoreOnSchema(t, pool))
	must(t, pool.QueryRow("SELECT row_to_json(m)::text FROM tenant_memberships m WHERE tenant_id=$1 AND user_id=$2", seed.tenantID, seed.userID).Scan(&after))
	if before != after {
		t.Fatal("upgrade rewrote existing membership")
	}
	var count int
	must(t, pool.QueryRow("SELECT count(*) FROM membership_join_events").Scan(&count))
	if count != 0 {
		t.Fatal("upgrade invented historical provenance")
	}
}
