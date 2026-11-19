package nativeprovider

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestOwnerStartupFailureReportsPhaseWithoutLaunchValues(t *testing.T) {
	id, err := instance()
	if err != nil {
		t.Fatal(err)
	}
	id = id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	private := "private-launch-value-must-not-leak"
	c := Config{Root: filepath.Join(t.TempDir(), "native-providers", id), Session: id,
		Key: strings.Repeat("c", 32), Executable: executable,
		Launch: Launch{Program: executable, Environment: []string{"PRIVATE_VALUE=" + private}, Cols: 80, Rows: 24}}
	var input, output bytes.Buffer
	if err := writeFrame(&input, c); err != nil {
		t.Fatal(err)
	}
	called := 0
	err = Serve(context.Background(), &input, &output, func(Launch) (Terminal, error) {
		called++
		return nil, &os.PathError{Op: "start", Path: private, Err: windows.ERROR_ACCESS_DENIED}
	})
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) || called != 1 {
		t.Fatalf("wrong injected startup failure: calls=%d err=%v", called, err)
	}
	raw := append([]byte(nil), output.Bytes()...)
	var reply frame
	if err := readFrame(&output, &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Kind != "ready" || reply.Error != "owner-terminal" || reply.Code != int(windows.ERROR_ACCESS_DENIED) || reply.Identity != nil {
		t.Fatalf("lost safe startup phase: %+v", reply)
	}
	for _, value := range []string{private, c.Key, executable, c.Root} {
		if bytes.Contains(raw, []byte(value)) {
			t.Fatalf("startup reply contains a private launch value")
		}
	}
	if _, err := os.Stat(filepath.Join(c.Root, "identity.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed startup left a published owner record: %v", err)
	}
}
