//go:build windows

package recovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsWaitingSocketHasProtectedAccountDACL(t *testing.T) {
	listener, endpoint, cleanup, err := newWaitingSocket()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	defer listener.Close()
	descriptor, err := windows.GetNamedSecurityInfo(filepath.Dir(endpoint), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("waiting socket directory inherits access: %v", err)
	}
	want, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		t.Fatal(err)
	}
	// NTFS may retain the harmless auto-inherited (AI) status bit even after
	// protecting the DACL. Verify the sole grant, separately from that flag.
	actual, expected := descriptor.String(), want.String()
	start, wantStart := strings.IndexByte(actual, '('), strings.IndexByte(expected, '(')
	if start < 0 || wantStart < 0 || actual[start:] != expected[wantStart:] {
		t.Fatalf("waiting socket directory has unexpected access: %s", descriptor.String())
	}
	cleanup()
	if _, err := os.Stat(filepath.Dir(endpoint)); !os.IsNotExist(err) {
		t.Fatalf("private socket directory retained: %v", err)
	}
}

func TestWindowsWaitingWorkerRejectsReplayBeforeConsumption(t *testing.T) {
	request := waitingFixture(t)
	request.Action.Kind, request.Action.Confirm = "replay", true
	if _, err := acceptWaitingAction(request, testHostSession, testHostKey, "current-worker", prepareWaitingAction); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatal("Windows worker accepted shell replay", err)
	}
	state := hostSnapshot(t)
	if len(state.Consumed) != 0 || state.Control.Mode != "waiting" {
		t.Fatal("rejected replay changed execution authority")
	}
}
