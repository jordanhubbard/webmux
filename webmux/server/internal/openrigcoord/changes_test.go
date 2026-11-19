package openrigcoord

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestChangeWaitScopesAndProjectsNativeHelper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Rig hosting requires POSIX")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required")
	}
	bin := t.TempDir()
	if err = os.Symlink(node, filepath.Join(bin, "node")); err != nil {
		t.Fatal(err)
	}
	canceled := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/api/events?replay=0" || r.Header.Get("Last-Event-ID") != "" {
			t.Error("history requested")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "id: 1\ndata: {\"seq\":1,\"private\":\"PRIVATE_EVENT\"}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		canceled <- struct{}{}
	}))
	defer upstream.Close()
	port := strings.TrimPrefix(upstream.URL, "http://127.0.0.1:")
	rig := "#!/bin/sh\n[ \"$OPENRIG_HOST_SELECTED\" = local ] || exit 99\nif [ \"$1\" = --version ]; then echo '0.6.14 (df1c1517)'; else echo 'running on port " + port + "'; fi\n"
	if err = os.WriteFile(filepath.Join(bin, "rig"), []byte(rig), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("OPENRIG_HOST_SELECTED", "unrelated")
	t.Setenv("OPENRIG_URL", "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// A read-only wait must complete while mutations are locked by another operation.
	mutation.Lock()
	result := WaitChange(ctx, t.TempDir(), "owner", nil)
	mutation.Unlock()
	if !result.Available || !result.Changed {
		t.Fatalf("notification not projected: %+v", result)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("upstream subscription leaked")
	}
}

func TestChangeWaitCancellationTerminatesHungCLIChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Rig hosting requires POSIX")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required")
	}
	bin := t.TempDir()
	if err = os.Symlink(node, filepath.Join(bin, "node")); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(bin, "child.pid")
	t.Setenv("WEBMUX_CHANGE_CHILD_PID", marker)
	rig := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo '0.6.14 (df1c1517)'; else echo $$ > \"$WEBMUX_CHANGE_CHILD_PID\"; exec /bin/sleep 30; fi\n"
	if err = os.WriteFile(filepath.Join(bin, "rig"), []byte(rig), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Change, 1)
	go func() { done <- WaitChange(ctx, t.TempDir(), "owner", nil) }()
	var pid string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(marker); err == nil {
			pid = strings.TrimSpace(string(data))
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == "" {
		t.Fatal("native CLI child did not start")
	}
	terminated := false
	t.Cleanup(func() {
		if !terminated {
			_ = exec.Command("/bin/kill", "-KILL", pid).Run()
		}
	})
	cancel()
	select {
	case result := <-done:
		if result.Available || result.Changed {
			t.Fatal("canceled wait invented a change")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("wait did not stop")
	}
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		state, err := exec.Command("/bin/ps", "-o", "stat=", "-p", pid).Output()
		if err != nil || strings.TrimSpace(string(state)) == "" || strings.HasPrefix(strings.TrimSpace(string(state)), "Z") {
			terminated = true
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("canceled Node wait left its hung CLI child running")
}
