package recovery

import (
	"crypto/md5"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// The running Cursor Agent holds only the selected chat's store open, so the
// open store identifies the current conversation after an in-process switch.
func TestCursorSessionFollowsTheOpenChatStore(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native file descriptors are not exposed on Windows")
	}
	if runtime.GOOS != "linux" {
		if _, err := exec.LookPath("lsof"); err != nil {
			t.Skip("lsof unavailable")
		}
	}
	root, cwd := t.TempDir(), t.TempDir()
	t.Setenv("CURSOR_CONFIG_DIR", root)
	physical, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "chats", fmt.Sprintf("%x", md5.Sum([]byte(physical))))
	open := func(chats, id string) *os.File {
		dir := filepath.Join(chats, id)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(dir, "store.db"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString("store"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = f.Close() })
		return f
	}
	first := "6e902534-c8a6-45b0-8d92-bdb85f085158"
	second := "6acf6870-b5b9-42bf-9e12-db2f63c2136e"

	// Another project's chat is never this terminal's conversation.
	other := open(filepath.Join(root, "chats", "0123456789abcdef0123456789abcdef"), first)
	if id, _ := cursorSessionForProcess(os.Getpid(), cwd); id != "" {
		t.Fatal("accepted a chat store from another project", id)
	}
	_ = other.Close()

	store := open(project, first)
	id, file := cursorSessionForProcess(os.Getpid(), cwd)
	if id != first || file == "" {
		t.Fatalf("did not identify the open chat: %q %q", id, file)
	}
	// Switching closes the previous store and opens the selected one.
	_ = store.Close()
	_ = open(project, second)
	if id, _ := cursorSessionForProcess(os.Getpid(), cwd); id != second {
		t.Fatal("did not follow the switched chat", id)
	}
	// Two open chats are ambiguous and must not select either.
	_ = open(project, first)
	if id, _ := cursorSessionForProcess(os.Getpid(), cwd); id != "" {
		t.Fatal("guessed between two open chats", id)
	}
}

func TestCursorOpenStoreGivesAutomaticConfidenceOnlyWhileObserved(t *testing.T) {
	selected := "6e902534-c8a6-45b0-8d92-bdb85f085158"
	event := Event{CLI: "cursor", SessionID: selected, NativeEvidence: "launch-selector"}
	// A new chat has no store before its first turn.
	observeProcessState(&event, "", "")
	if event.NativeEvidence != "launch-selector" {
		t.Fatal("absence of a store changed the launch selection", event)
	}
	switched := "6acf6870-b5b9-42bf-9e12-db2f63c2136e"
	observeProcessState(&event, switched, "/chats/project/"+switched+"/store.db")
	activity := &Activity{CLI: "cursor", NativeSessionID: event.SessionID, NativeEvidence: event.NativeEvidence}
	if event.SessionID != switched || !activity.CanAutoResumeNative() {
		t.Fatal("observed chat was not the current conversation", event)
	}
	// After starting another, still empty, chat the current conversation is unknown.
	observeProcessState(&event, "", "")
	activity.NativeEvidence = event.NativeEvidence
	if event.SessionID != switched || event.NativeEvidence != "unavailable" || activity.CanAutoResumeNative() {
		t.Fatal("unobserved chat kept automatic confidence", event)
	}
}
