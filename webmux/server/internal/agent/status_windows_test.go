package agent

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestStatusReaderSharesPendingRenameDeleteHandle(t *testing.T) {
	s := fixture(t)
	const id, name = "alpha", "pending-rename/λ"
	stamp := s.now().UTC().Format(timeFormat)
	if err := s.RecordStatus(id, name, StatusUpdate{Status: "working", Source: "webmux", LastInputAt: &stamp}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.store.Home, "data", "agent-status", id, statusFilename(name))
	wide, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	// Hold the same DELETE access and sharing as Go's Root.Rename after the
	// new name becomes visible and before the rename handle closes. This
	// forces the reader overlap without scheduling, sleeps or retries.
	handle, err := windows.CreateFile(wide, windows.DELETE|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if _, err := os.ReadFile(path); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("absolute reader should conflict with pending rename handle, got %v", err)
	}
	// Exercise the actual production reader while that handle remains open.
	got := s.readStatus(id, name)
	if got.text("agent_id") != id || got.text("name") != name || got.text("status") != "working" ||
		got.text("source") != "webmux" || got.text("last_input_at") != stamp || got.text("updated_at") != stamp {
		t.Fatalf("root reader lost complete published status: %#v", got)
	}
}
