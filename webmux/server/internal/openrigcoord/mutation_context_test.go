package openrigcoord

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCanceledCoordinatorWaitDoesNotMaterializeOrRunHelper(t *testing.T) {
	home := t.TempDir()
	mutation.Lock()
	locked := true
	defer func() {
		if locked {
			mutation.Unlock()
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := Run(ctx, home, "owner", "native-recovery-resume", `{}`); done <- err }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		mutation.Unlock()
		locked = false
		<-done
		t.Fatal("canceled waiter remained behind the active operation")
	}
	if _, err := os.Stat(filepath.Join(home, "data")); !os.IsNotExist(err) {
		t.Fatal("canceled waiter materialized owner state", err)
	}
}
