package router

import "github.com/wanglongan587/cloud/internal/core"

// legacyOperationResponse preserves the retired development JSON contract. The persistent
// scheduling counter travels only through the production gRPC operation snapshot, not this
// historical HTTP shape. Control returns an operation directly for advance/defer, and nested
// under operation for claim/snapshot/effect actions, so both forms need the same projection.
func legacyOperationResponse(out core.Object) {
	delete(out, "nodeUnavailableAttempts")
	delete(out.O("operation"), "nodeUnavailableAttempts")
}
