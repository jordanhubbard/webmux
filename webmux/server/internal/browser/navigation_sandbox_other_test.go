//go:build !windows

package browser

import "context"

func inspectFirstNavigationNetworkSandbox(context.Context) *firstNavigationNetworkSandbox { return nil }
