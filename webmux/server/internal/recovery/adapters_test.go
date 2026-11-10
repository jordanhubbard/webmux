package recovery

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestNativeSelectorsNeverGuessLatestOrReplayBypassFlags(t *testing.T) {
	uuid := "12345678-1234-4123-a123-123456789abc"
	for _, tc := range []struct {
		cli, id string
		args    []string
	}{
		{"codex", uuid, []string{"codex", "resume", uuid}},
		{"claude", uuid, []string{"claude", "--resume", uuid}},
		{"cursor", uuid, []string{"agent", "--resume", uuid}},
		{"opencode", "ses_abcdef123456", []string{"opencode", "--session", "ses_abcdef123456"}},
	} {
		args, err := ResumeArgv(tc.cli, tc.id)
		if err != nil || !reflect.DeepEqual(args, tc.args) {
			t.Fatalf("%s: %v %v", tc.cli, args, err)
		}
		if got := SelectedNativeID(tc.cli, args[1:]); got != tc.id {
			t.Fatalf("selected %s instead of %s", got, tc.id)
		}
	}
	for _, id := range []string{"", "--last", "$(touch sentinel)", "../../other-user", uuid + "\nexit"} {
		if _, err := ResumeArgv("codex", id); err == nil {
			t.Fatalf("accepted unsafe selector %q", id)
		}
	}
	if _, err := ResumeArgv("unqualified-cli", uuid); err == nil {
		t.Fatal("invented a resume command")
	}
	for _, tc := range []struct {
		cli  string
		args []string
	}{
		{"claude", []string{"--resume"}}, {"claude", []string{"-r", "named-conversation"}},
		{"cursor", []string{"--resume"}}, {"opencode", []string{"--session", "invalid"}},
		{"cursor", []string{"ls"}}, {"cursor", []string{"resume"}},
		{"claude", []string{"--from-pr", "318"}}, {"claude", []string{"--teleport"}},
		{"claude", []string{"--cloud=conversation"}}, {"claude", []string{"--fork-session"}},
		{"opencode", []string{"--fork"}},
	} {
		if !hasSessionSelection(tc.cli, tc.args) {
			t.Fatal("would replace the user's native session selector with a fresh session")
		}
	}
}

func TestNativeControlCommandsDoNotAllocateConversations(t *testing.T) {
	for _, tc := range []struct {
		cli  string
		args []string
	}{
		{"cursor", []string{"--list-models"}}, {"opencode", []string{"agent", "list"}},
		{"opencode", []string{"acp"}}, {"claude", []string{"agents", "list"}},
		{"claude", []string{"doctor"}}, {"claude", []string{"setup-token"}},
	} {
		if !commandOnly(tc.cli, tc.args) {
			t.Fatalf("would allocate a conversation for %s %v", tc.cli, tc.args)
		}
	}
	if commandOnly("cursor", []string{"agent", "explain this project"}) {
		t.Fatal("Cursor's interactive agent subcommand was excluded")
	}
}

func TestForkedConversationDoesNotRetainTheSourceNativeIdentity(t *testing.T) {
	id := "12345678-1234-4123-a123-123456789abc"
	for _, tc := range []struct {
		cli  string
		args []string
	}{
		{"claude", []string{"--resume", id, "--fork-session"}},
		{"cursor", []string{"--resume", id, "--worktree"}},
		{"opencode", []string{"--session", "ses_abcdef123456", "--fork"}},
	} {
		if SelectedNativeID(tc.cli, tc.args) != "" || !hasSessionSelection(tc.cli, tc.args) {
			t.Fatalf("would recover or replace source conversation: %s %v", tc.cli, tc.args)
		}
	}
}

func TestCursorResolutionRejectsAnUnrelatedAgentAndUsesVerifiedAlias(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable fixtures")
	}
	dir := t.TempDir()
	t.Setenv("WEBMUX_BROWSER_ORIGINAL_PATH", dir)
	write := func(name, help string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nprintf '%s\\n' '"+help+"'\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	write("agent", "Grok Build TUI --resume --session-id")
	if _, _, err := cursorCLI(); err == nil {
		t.Fatal("unrelated agent accepted as Cursor")
	}
	write("cursor-agent", "Start the Cursor Agent --resume create-chat")
	name, binary, err := cursorCLI()
	if err != nil || name != "cursor-agent" || binary != filepath.Join(dir, name) {
		t.Fatalf("verified alias unavailable: %q %q %v", name, binary, err)
	}
}

func TestRecoveryMetadataDoesNotRetainCredentialArguments(t *testing.T) {
	for _, line := range []string{"agent --api-key private-value", "AUTH_TOKEN=private-value codex", "tool --password=private-value"} {
		command, replayable := SafeCommand(line)
		if replayable || command == line {
			t.Fatalf("retained credential-bearing command: %s", line)
		}
	}
	if command, replayable := SafeCommand("git status --short"); !replayable || command != "git status --short" {
		t.Fatal("ordinary command lost")
	}
	args := safeArgv([]string{"agent", "--api-key", "private-value", "prompt"})
	if len(args) != 2 || args[1] != "[credential arguments omitted]" {
		t.Fatal("retained CLI credential")
	}
}

func TestMetadataSignatureIsBoundToThePersistedTerminal(t *testing.T) {
	packet, err := SignedPacket(Event{Kind: "command", Command: "git status", Instance: "shell"}, "terminal-key")
	if err != nil {
		t.Fatal(err)
	}
	var decoder Decoder
	_, events := decoder.Feed(packet)
	if len(events) != 1 || !Verify(events[0], "terminal-key") || Verify(events[0], "other-terminal") {
		t.Fatal("cross-terminal signature accepted")
	}
	events[0].Command = "different command"
	if Verify(events[0], "terminal-key") {
		t.Fatal("tampered command accepted")
	}
	if Verify(Event{Kind: "cli", CLI: "codex"}, "terminal-key") {
		t.Fatal("unsigned output trusted")
	}
}

func TestCodexIdentityComesFromTheOwningProcessAndMatchingProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native file descriptors are not exposed on Windows")
	}
	if runtime.GOOS != "linux" {
		if _, err := exec.LookPath("lsof"); err != nil {
			t.Skip("lsof unavailable")
		}
	}
	home, cwd := t.TempDir(), t.TempDir()
	t.Setenv("CODEX_HOME", home)
	dir := filepath.Join(home, "sessions", "test")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	open := func(name, id string) *os.File {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(map[string]any{"type": "session_meta", "payload": map[string]string{"id": id, "cwd": cwd}})
		if _, err := f.Write(append(data, '\n')); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = f.Close() })
		return f
	}
	uuid := "12345678-1234-4123-a123-123456789abc"
	_ = open("rollout-owned.jsonl", uuid)
	if id, _ := codexSessionForProcess(os.Getpid(), cwd); id != uuid {
		t.Fatalf("did not identify open native state: %s", id)
	}
	_ = open("rollout-ambiguous.jsonl", "12345678-1234-4123-a123-123456789abd")
	if id, _ := codexSessionForProcess(os.Getpid(), cwd); id != "" {
		t.Fatal("guessed between two native conversations")
	}
}

func TestNativeOpenCodeSessionAllocation(t *testing.T) {
	if os.Getenv("WEBMUX_QUALIFY_NATIVE_CLI") != "1" {
		t.Skip("set WEBMUX_QUALIFY_NATIVE_CLI=1 for the installed OpenCode qualification")
	}
	binary, err := exec.LookPath("opencode")
	if err != nil {
		t.Fatal(err)
	}
	home, cwd := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	for _, variable := range []string{"XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME", "XDG_CONFIG_HOME"} {
		t.Setenv(variable, filepath.Join(home, variable))
	}
	t.Chdir(cwd)
	id, err := createOpenCodeSession(binary, cwd)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateOpenCodeState(binary, id, cwd); err != nil {
		t.Fatal(err)
	}
	output, err := cliOutput(binary, "export", id)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Info struct {
			ID        string `json:"id"`
			Directory string `json:"directory"`
		} `json:"info"`
	}
	if json.Unmarshal([]byte(output), &state) != nil || state.Info.ID != id || !sameDirectory(state.Info.Directory, cwd) {
		t.Fatalf("native session was not persisted under the correct project: %s", id)
	}
	t.Logf("native OpenCode session allocation and export passed: %s", id)
}
