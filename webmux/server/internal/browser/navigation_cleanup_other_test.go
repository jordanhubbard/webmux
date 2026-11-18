//go:build !windows

package browser

import "context"

func observeFirstNavigationCleanup(context.Context) func() *firstNavigationCleanupProcesses {
	return func() *firstNavigationCleanupProcesses { return nil }
}
