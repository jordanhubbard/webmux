//go:build !linux && !darwin

package credentialhub

import (
	"errors"
	"testing"
)

func TestUnsupportedCustodyFailsClosed(t *testing.T) {
	if e := New(t.TempDir()).Setup("alice"); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
}
