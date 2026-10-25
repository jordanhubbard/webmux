//go:build darwin

package main

import (
	"context"
	"os/exec"
	"time"
)

func readDarwinLoad() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "/usr/sbin/sysctl", "-n", "vm.loadavg").Output()
	return string(output), err
}
