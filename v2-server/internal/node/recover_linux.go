//go:build linux

package node

import (
	"context"
	"errors"
	"time"
)

// RecoverRuntime confirms that a previously owned runtime has no live workload or unknown
// process left. It takes the same exclusive lock as StartRuntime and preserves checkouts,
// history and databases. Call it before publishing restart recovery to page consumers.
func RecoverRuntime(ctx context.Context, root string, grace time.Duration) error {
	if grace <= 0 {
		return errors.New("invalid runtime recovery grace")
	}
	paths, lock, err := claimRoot(root)
	if err != nil {
		return err
	}
	return errors.Join(cleanOwned(ctx, paths, grace), lock.Close())
}
