package nativeprovider

import (
	"fmt"
	"strings"
	"syscall"
	"testing"
)

func TestStartupDiagnosticRetainsOnlyRecognizedEvidence(t *testing.T) {
	private := "private-argv-key-path"
	if got := startupCode(fmt.Errorf("%s: %w", private, syscall.Errno(5))); got != 5 {
		t.Fatalf("lost wrapped OS code: %d", got)
	}
	if got := startupDiagnostic("owner-listen", 10047, 0); got != "phase=owner-listen os=address_family_not_supported exit=none" {
		t.Fatal(got)
	}
	if got := startupDiagnostic("bootstrap-start", 0, 0xc0000142); !strings.Contains(got, "exit=dll_initialization_failed") {
		t.Fatal(got)
	}
	if got := startupDiagnostic("bootstrap-owner-exit", 0xc0000142, 0); !strings.Contains(got, "os=dll_initialization_failed") {
		t.Fatal(got)
	}
	got := startupDiagnostic(private, 0x12345678, 0x87654321)
	if got != "phase=unavailable os=unavailable exit=unavailable" || strings.Contains(got, private) {
		t.Fatalf("unrecognized child evidence leaked: %q", got)
	}
}
