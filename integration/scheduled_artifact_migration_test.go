package integration

import (
	"context"
	"strings"
	"testing"
)

func TestMigration0064And0065ScheduledArtifactFreshUpgradeAndRepeat(t *testing.T) {
	for _, mode := range []string{"fresh", "upgrade-from-0063"} {
		t.Run(mode, func(t *testing.T) {
			pool, _ := testSchema(t, "test_artifact_upgrade_")
			if mode == "upgrade-from-0063" {
				applyMigrationsUpTo(t, pool, migrationsBefore(t, "0064_scheduled_artifact_execution_kind.sql"))
				_, err := pool.Exec(`INSERT INTO node_executions
					(execution_id,kind,operation_id,node_id,node_operation_id,input,dispatched_epoch)
					VALUES('legacy-plugin','install_plugins',gen_random_uuid(),'node','legacy-op','{}',1)`)
				must(t, err)
			}
			store := newStoreOnSchema(t, pool)
			must(t, store.Migrate(context.Background()))
			must(t, store.Migrate(context.Background()))
			must(t, store.CheckSchema(context.Background()))
			// Keep plugin executions and admit independent deliveries of the same run together.
			_, err := pool.Exec(`INSERT INTO node_executions
				(execution_id,kind,operation_id,node_id,node_operation_id,input,dispatched_epoch)
				SELECT 'new-'||kind,kind,'11111111-1111-4111-8111-111111111111'::uuid,'node','op-'||kind,'{}',1
				FROM unnest(ARRAY['install_plugins','deliver_revision','deliver_scheduled_artifacts']) AS kind`)
			must(t, err)
			_, err = pool.Exec(`INSERT INTO node_executions
				(execution_id,kind,operation_id,node_id,node_operation_id,input,dispatched_epoch)
				VALUES('duplicate','deliver_scheduled_artifacts','11111111-1111-4111-8111-111111111111','node','duplicate','{}',1)`)
			if err == nil {
				t.Fatal("the migrated index must reject a second pending upload for the same run")
			}
			var index string
			must(t, pool.QueryRow(`SELECT indexdef FROM pg_indexes
				WHERE schemaname=current_schema() AND indexname='scheduled_run_artifact_deliveries_gc'`).Scan(&index))
			if !strings.Contains(index, "delivered") || strings.Contains(index, "artifacts_gc_at IS NULL") {
				t.Fatal("GC index must retain already-swept and pruned successful deliveries")
			}
		})
	}
}
