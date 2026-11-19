//go:build !darwin && !linux

package openrigcoord

import "os/exec"

func configureNativeCreateCancellation(cmd *exec.Cmd) {}
