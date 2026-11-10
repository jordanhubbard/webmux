//go:build !windows

package recovery

import (
	"os/exec"
	"strings"
	"testing"
)

func TestNativeSubprocessDirectoryAndPWDRemainConsistent(t *testing.T) {
	binary, err := exec.LookPath("printenv")
	if err != nil {
		t.Skip("printenv is unavailable")
	}
	t.Setenv("PWD", t.TempDir())
	cwd := t.TempDir()
	output, err := cliOutputAt(cwd, binary, "PWD")
	if err != nil || output != cwd {
		t.Fatalf("native process saw a different project: %q, %v", output, err)
	}
}

func TestNativeHelpOnStderrDoesNotContaminateIdentityOutput(t *testing.T) {
	command := "printf native-id; printf 'supported --session selector' >&2"
	metadata, err := cliOutputAt(t.TempDir(), "/bin/sh", "-c", command)
	if err != nil || metadata != "native-id" {
		t.Fatal("stderr contaminated native identity", metadata, err)
	}
	help, err := cliHelpAt(t.TempDir(), "/bin/sh", "-c", command)
	if err != nil || !strings.Contains(help, "--session") {
		t.Fatal("successful native help on stderr was discarded", help, err)
	}
}

func TestNativeMetadataOutputIsBoundedDuringCapture(t *testing.T) {
	output := &boundedCLIOutput{}
	for i := 0; i < 64; i++ {
		data := strings.Repeat("x", 4096)
		if n, err := output.Write([]byte(data)); n != len(data) || err != nil {
			t.Fatal(n, err)
		}
	}
	if !output.exceeded || output.buffer.Len() != 64<<10 {
		t.Fatal("metadata capture grew past its bound")
	}
}
