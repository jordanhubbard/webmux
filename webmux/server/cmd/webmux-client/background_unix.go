//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func startBackground(server, state string, allowHTTP bool) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"--server", server, "--state", state, "--run-child"}
	if allowHTTP {
		args = append(args, "--allow-insecure-http")
	}
	command := exec.Command(executable, args...)
	command.Stdin, command.Stdout = nil, nil
	if err := os.MkdirAll(filepath.Dir(state), 0700); err != nil {
		return err
	}
	log, err := os.OpenFile(state+".log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	command.Stderr = log
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return err
	}
	fmt.Printf("webmux-client started in background (pid %d)\n", command.Process.Pid)
	return command.Process.Release()
}
