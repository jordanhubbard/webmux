//go:build !linux && !windows

package recovery

import (
	"context"
	"os/exec"
)

func scopedProviderCommand(context.Context, string, string, []string) *exec.Cmd { return nil }
func providerServiceWarning(context.Context, string, string, []string) string   { return "" }
