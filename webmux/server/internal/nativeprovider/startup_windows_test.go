package nativeprovider

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestStartupFailureOnlyExposesFixedPhaseAndOSCode(t *testing.T) {
	secret := "private-launch-key-path-environment"
	raw := &os.PathError{Op: secret, Path: secret, Err: windows.ERROR_ACCESS_DENIED}
	for _, phase := range []string{startupConfig, startupRoot, startupOwnership, startupSocket, startupImage,
		startupScreen, startupRecord, startupTerminal, startupOwnerProcess, startupOwnerReply, startupBootstrap, startupReply} {
		t.Run(phase, func(t *testing.T) {
			reply := startupFailureFrame(phase, raw)
			err := startupReplyError(reply)
			if err == nil || !strings.Contains(err.Error(), phase) || !strings.Contains(err.Error(), "Windows error 5") || strings.Contains(err.Error(), secret) {
				t.Fatalf("unsafe or incomplete phase diagnostic: %v", err)
			}
		})
	}
	if err := startupReplyError(frame{Error: secret, Code: 5}); err != nil {
		t.Fatalf("unrecognized child text became a diagnostic: %v", err)
	}
	if reply := startupFailureFrame(secret, errors.New(secret)); reply.Error != startupConfig || reply.Code != 0 {
		t.Fatalf("untrusted producer phase leaked: %#v", reply)
	}
}

func TestServeStartupFailureRetainsPhaseWithoutPayload(t *testing.T) {
	x, err := instance()
	if err != nil {
		t.Fatal(err)
	}
	id := x[:8] + "-" + x[8:12] + "-" + x[12:16] + "-" + x[16:20] + "-" + x[20:]
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	secret := "private-launch-credential-value"
	c := Config{Root: filepath.Join(t.TempDir(), "native-providers", id), Session: id, Key: strings.Repeat("c", 32),
		Executable: self, Launch: Launch{Program: self, Environment: []string{"PRIVATE=" + secret}, Cols: 80, Rows: 24}}
	var input, output bytes.Buffer
	if err := writeFrame(&input, c); err != nil {
		t.Fatal(err)
	}
	want := &os.PathError{Op: secret, Path: secret, Err: windows.ERROR_FILE_NOT_FOUND}
	err = Serve(context.Background(), &input, &output, func(Launch) (Terminal, error) { return nil, want })
	if !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		t.Fatalf("original failure lost: %v", err)
	}
	if bytes.Contains(output.Bytes(), []byte(secret)) || bytes.Contains(output.Bytes(), []byte(c.Key)) {
		t.Fatal("startup reply exposed private launch data")
	}
	var reply frame
	if err := readFrame(&output, &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Kind != "ready" || reply.Error != startupTerminal || reply.Code != 2 || output.Len() != 0 {
		t.Fatalf("expected one sanitized terminal failure reply: %#v; trailing=%d", reply, output.Len())
	}
	if _, err := os.Stat(filepath.Join(c.Root, "identity.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed owner retained a live rendezvous: %v", err)
	}
}
