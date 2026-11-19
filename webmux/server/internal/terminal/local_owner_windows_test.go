package terminal

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/nativeprovider"
)

func TestLocalOwnerCreationAndImageRefuseRetargeting(t *testing.T) {
	target, err := captureLocalOwner(nativeprovider.Identity{Session: "11111111-1111-4111-8111-111111111111", Instance: strings.Repeat("a", 32), Endpoint: filepath.Join(t.TempDir(), "unused"), OwnerPID: os.Getpid(), RootPID: os.Getpid(), BinarySHA256: strings.Repeat("b", 64)})
	if err != nil {
		t.Fatal(err)
	}
	if err := InspectLocalOwner(context.Background(), target, ""); err == nil {
		t.Fatal("live original process reported gone")
	}
	if err := StopLocalOwner(context.Background(), target, ""); err == nil || !strings.Contains(err.Error(), "image changed") {
		t.Fatal("wrong image reached stop dispatch", err)
	}
	original := target
	target.OwnerCreation--
	if err := InspectLocalOwner(context.Background(), target, ""); err != nil {
		t.Fatal("old process identity not distinguished from reused PID", err)
	}
	if err := StopLocalOwner(context.Background(), target, ""); err != nil {
		t.Fatal("old target completion failed", err)
	}
	if err := InspectLocalOwner(context.Background(), original, ""); err == nil {
		t.Fatal("different process identity was stopped")
	}
	target = original
	target.Scope = strings.Repeat("c", 64)
	if err := InspectLocalOwner(context.Background(), target, ""); err == nil {
		t.Fatal("foreign execution host/account was treated as an exited owner")
	}
	if err := StopLocalOwner(context.Background(), target, ""); err == nil {
		t.Fatal("foreign execution host/account reached stop")
	}
}
