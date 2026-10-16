package terminalfiles

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
