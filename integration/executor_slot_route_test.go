package integration

import (
	"testing"

	"github.com/wanglongan587/cloud/internal/core"
)

// The executor-slot routes address one mapping by the `:slot` path parameter. The router must hand
// that parameter to core; without it every PUT and DELETE reads as the collection and answers 405.
func TestExecutorSlotRoutesCarryTheSlotName(t *testing.T) {
	f := setup(t)
	// A missing mapping is 404 whatever version the body claims (not the collection's 405).
	if out := f.call("DELETE", "/api/v1/me/executor-slot-mappings/coder", core.Object{"version": 1}, "slot-delete", 404); out.S("code") != "not_found" {
		t.Fatalf("deleting a missing slot mapping = %v, want not_found", out)
	}
	// The slot name itself is validated, which only happens once core sees it.
	if out := f.call("PUT", "/api/v1/me/executor-slot-mappings/default", core.Object{"version": 0, "agentCli": "x", "connectionId": "x", "modelId": "x"}, "", 400); out.S("code") != "invalid_slot_name" {
		t.Fatalf("writing the reserved default slot = %v, want invalid_slot_name", out)
	}
}
