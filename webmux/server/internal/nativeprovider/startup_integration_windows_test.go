package nativeprovider_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/nativeprovider"
)

func TestNativeProviderStartupFailureReportsSanitizedPhase(t *testing.T) {
	permitBreakaway(t)
	c, counter := config(t)
	secret := "private-program-environment-not-for-diagnostics"
	c.Launch.Program = filepath.Join(filepath.Dir(counter), secret+".exe")
	c.Launch.Environment = append(c.Launch.Environment, "PRIVATE="+secret)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, _, err := nativeprovider.Ensure(ctx, c)
	if err == nil || !strings.Contains(err.Error(), "phase=owner-terminal os=unavailable") {
		t.Fatalf("missing sanitized native startup phase: %v", err)
	}
	// exec.LookPath collapses a missing executable into exec.ErrNotFound.
	// No syscall errno survives that API; a Windows code must not be invented.
	if strings.Contains(err.Error(), "Windows error") {
		t.Fatalf("lookup failure invented an unavailable OS error code: %v", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), c.Key) {
		t.Fatal("native startup failure exposed private launch data")
	}
	if _, err := os.Stat(counter); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed native launch started a replacement job: %v", err)
	}
}
