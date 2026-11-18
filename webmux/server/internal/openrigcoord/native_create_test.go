package openrigcoord

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestNativeCreateUsesOwnerDirectoryAndMaterializesDependencies(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("native creator requires POSIX")
	}
	home, bins := t.TempDir(), t.TempDir()
	for name, contents := range map[string]string{"tmux": "#!/bin/sh\necho 'tmux 3.8'\n", "rig": "#!/bin/sh\necho unqualified\n"} {
		if err := os.WriteFile(filepath.Join(bins, name), []byte(contents), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bins+string(os.PathListSeparator)+os.Getenv("PATH"))
	foreign := filepath.Join(t.TempDir(), "foreign")
	input, _ := json.Marshal(map[string]string{"directory": foreign, "cwd": home, "rigId": "01JQPWV4E7QP26M799M3S0MR6E", "pod": "agents", "member": "alpha"})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := Run(ctx, home, "owner", "native-create", string(input)); err == nil {
		t.Fatal("unqualified manager was accepted")
	}
	if _, err := os.Stat(foreign); !os.IsNotExist(err) {
		t.Fatalf("input escaped owner namespace: %v", err)
	}
	directories, err := filepath.Glob(filepath.Join(home, "data", "coordination", "*", "requests"))
	if err != nil || len(directories) != 1 {
		t.Fatalf("owner directory missing: %v %v", directories, err)
	}
	for _, name := range []string{"codex-owner-plan.mts", "codex-launch-policy.mts", "codex-launch-wrapper.mts", "codex-launch-environment.mts", "codex-environment-channel.mts", "rig-create-intent.mts", "rig-native-create.mts", "rig-native-resume.mts", "rig-native-launch.mts", "rig-native-capture.mts", "rig-native-project.mts", "rig-http-create.mts"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(directories[0]), name)); err != nil {
			t.Fatal(err)
		}
	}
	resume, _ := json.Marshal(map[string]string{"directory": foreign, "cwd": home, "rigId": "01JQPWV4E7QP26M799M3S0MR6E",
		"pod": "agents", "member": "alpha", "nodeId": "01JQPWV4E7QP26M799M3S0MR6F", "snapshotId": "01JQPWV4E7QP26M799M3S0MR6G",
		"historySessionId": "01JQPWV4E7QP26M799M3S0MR6H", "sessionName": "agents-alpha@qualified",
		"nativeSessionId": "01a1eff2-f58a-7082-be6b-69f9eca4a07c", "operationId": "9aa1753a-1d61-4420-af42-e6b6fa591c47"})
	if _, err := Run(ctx, home, "owner", "native-resume", string(resume)); err == nil {
		t.Fatal("resume without an original owner launch was accepted")
	}
	if _, err := os.Stat(foreign); !os.IsNotExist(err) {
		t.Fatal("resume input escaped the owner namespace")
	}
}

func TestNativeCreateCancellationAllowsHelperCleanup(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("native creator requires POSIX")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable")
	}
	root := t.TempDir()
	ready, cleaned := filepath.Join(root, "ready"), filepath.Join(root, "cleaned")
	readyJSON, _ := json.Marshal(ready)
	cleanedJSON, _ := json.Marshal(cleaned)
	script := filepath.Join(root, "helper.mjs")
	code := "import fs from 'node:fs';process.on('SIGTERM',()=>{fs.writeFileSync(" + string(cleanedJSON) + ",'cleaned');process.exit(0)});fs.writeFileSync(" + string(readyJSON) + ",'ready');setInterval(()=>{},1000);"
	if err := os.WriteFile(script, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, node, script)
	configureNativeCreateCancellation(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	_ = cmd.Wait()
	if _, err := os.Stat(cleaned); err != nil {
		t.Fatalf("helper cleanup was skipped: %v", err)
	}
}
