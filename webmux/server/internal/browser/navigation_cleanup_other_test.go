//go:build !windows

package browser

import "context"

func observeFirstNavigationCleanup(context.Context, *firstNavigationEvidence) func() *firstNavigationCleanupProcesses {
	return func() *firstNavigationCleanupProcesses { return nil }
}

func queryFirstNavigationRemovalHolders(error, string, map[uint32]uint64) *firstNavigationRemovalHolders {
	return nil
}
