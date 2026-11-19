//go:build !windows

package browser

import "context"

func prepareBrowserProcessExit(context.Context) (func(context.Context) error, error) { return nil, nil }
