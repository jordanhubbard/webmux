//go:build !windows

package recovery

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCanceledNativePreflightCleansUpItsPrivateServer(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "server.pid")
	binary := filepath.Join(dir, "fixture-server")
	quoted := "'" + strings.ReplaceAll(pidFile, "'", "'\"'\"'") + "'"
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s\\n' $$ > "+quoted+"\nexec sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := openCodeSessionContext(ctx, binary, dir, "ses_abcdef123456"); done <- err }()
	deadline := time.Now().Add(5 * time.Second)
	var pid int
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(pidFile); err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
			if pid > 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("private server did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled native preflight reported success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled native preflight did not finish cleanup")
	}
	if syscall.Kill(pid, 0) == nil {
		t.Fatal("private native server survived canceled preflight")
	}
}
