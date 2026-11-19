package recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestClaudeHookAuthenticatesDescendantWithoutPS(t *testing.T) {
	const helper = "WEBMUX_CLAUDE_ANCESTRY_TEST_CHILD"
	type message struct {
		Endpoint    string
		Token       string
		Observation claudeSessionObservation
	}
	if os.Getenv(helper) == "1" {
		var input message
		if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(input.Observation)
		if err != nil {
			t.Fatal(err)
		}
		if err := ClaudeSessionHook([]string{input.Endpoint, input.Token}, bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
		return
	}

	t.Setenv("CLAUDE_CODE_SAFE_MODE", "")
	observation, _ := claudeObservationFixture(t)
	o := newClaudeObserver("2.1.322 (Claude Code)", nil)
	if o == nil {
		t.Fatal("private observer unavailable")
	}
	defer o.close()
	o.start(os.Getpid())

	// The peer is a real child with kernel socket credentials. Its ancestry
	// must not depend on starting a command found through the caller's PATH.
	bin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ps-invoked")
	if err := os.WriteFile(filepath.Join(bin, "ps"), []byte("#!/bin/sh\n: > \"$WEBMUX_HOOK_PS_WITNESS\"\nexit 97\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("WEBMUX_HOOK_PS_WITNESS", marker)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(message{o.endpoint, o.token, observation})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, executable, "-test.run=^TestClaudeHookAuthenticatesDescendantWithoutPS$")
	child.Env = append(os.Environ(), helper+"=1")
	child.Stdin = bytes.NewReader(data)
	if err := child.Run(); err != nil {
		t.Fatal("private native hook child failed", err)
	}
	select {
	case got := <-o.observations:
		if got != observation {
			t.Fatal("descendant observation changed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("valid descendant observation was not delivered")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("native ancestry invoked external ps", err)
	}
	if nativeProcessDescendant(child.Process.Pid, os.Getpid()) {
		t.Fatal("exited child remained authorized")
	}
}
