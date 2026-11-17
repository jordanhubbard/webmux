package terminalfiles

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/session"
	"github.com/jordanhubbard/webmux/server/internal/storage"
	"github.com/jordanhubbard/webmux/server/internal/upload"
)

func TestTransferUsesBoundSessionAndPreservesLocalPath(t *testing.T) {
	store, err := storage.Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteConfig("keys.yaml", map[string]any{"keys": []map[string]string{{"id": "key", "private_key_path": "/private/key with spaces"}}}); err != nil {
		t.Fatal(err)
	}
	s := session.Session{Transport: "ssh", Hostname: "remote.example", Username: "alice", Port: 2222, KeyID: "key"}
	cmd, err := Command(context.Background(), s, store, "/private/control/socket", "nonce-image.png")
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(cmd.Args, "\x00")
	for _, part := range []string{"BatchMode=yes", "StrictHostKeyChecking=yes", "/private/key with spaces", "/private/control/socket", "2222", "alice", "remote.example"} {
		if !strings.Contains(args, part) {
			t.Fatalf("missing %s", part)
		}
	}
	for _, name := range []string{"../image.png", "image.png;touch /tmp/unsafe", "image\n.png", "a'b.png"} {
		if _, err := Command(context.Background(), s, store, "", name); err == nil {
			t.Fatalf("unsafe name accepted: %q", name)
		}
	}
	local := upload.Result{Path: filepath.Join(t.TempDir(), "image.png"), Name: "image.png", Size: 5}
	if err := os.WriteFile(local.Path, []byte("image"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Deliver(context.Background(), session.Session{Transport: "local"}, store, "", local)
	if err != nil || got != local {
		t.Fatalf("local file changed: %+v %v", got, err)
	}
	if _, err := Deliver(context.Background(), session.Session{Transport: "exec"}, store, "", local); err == nil {
		t.Fatal("unknown exec host accepted")
	}
}

func TestSSHResponseOutputCopyBound(t *testing.T) {
	for _, size := range []int{8192, 8193, 65536} {
		var out limitedOutput
		n, err := io.Copy(&out, io.LimitReader(strings.NewReader(strings.Repeat("x", size)), int64(size)))
		if err != nil || n != int64(size) || len(out.String()) != min(size, 8192) || out.overflow != (size > 8192) {
			t.Errorf("size=%d copied=%d retained=%d overflow=%v error=%v", size, n, len(out.String()), out.overflow, err)
		}
	}
}

func TestSSHResponseOutputChild(t *testing.T) {
	if os.Getenv("WEBMUX_TRANSFER_OUTPUT_CHILD") != "1" {
		return
	}
	if _, err := io.WriteString(os.Stdout, strings.Repeat("x", 65536)); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestSSHResponseOutputBoundsActualChildStdout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSSHResponseOutputChild$")
	cmd.Env = append(os.Environ(), "WEBMUX_TRANSFER_OUTPUT_CHILD=1")
	var out limitedOutput
	cmd.Stdout = &out
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		t.Fatal("bounded collector did not drain the producer", err)
	}
	if len(out.String()) != 8192 || !out.overflow {
		t.Fatalf("retained=%d overflow=%v", len(out.String()), out.overflow)
	}
}
