//go:build !windows

package browser

import "context"

func observeFirstNavigationCleanup(context.Context, *firstNavigationEvidence) func() *firstNavigationCleanupProcesses {
	return func() *firstNavigationCleanupProcesses { return nil }
}

func queryFirstNavigationRemovalHolders(error, string, map[uint32]uint64) *firstNavigationRemovalHolders {
	return nil
}

func prepareFirstNavigationHandleProbe(map[uint32]uint64) func(error, string) *firstNavigationHandleReport {
	return func(error, string) *firstNavigationHandleReport { return nil }
}
