//go:build !windows

package coordinationclient

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

func TestSocketChannelProvesIdentityBeforeReplacingAccess(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "wmc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	t.Setenv("WEBMUX_COORDINATION_CACHE_HOME", root)
	const session = "10000000-0000-4000-8000-000000000001"
	const channel = "20000000-0000-4000-8000-000000000001"
	key := strings.Repeat("a", 32)
	source := Access{Contract: coordination.Contract, Endpoint: "http://127.0.0.1:12345", Capability: "wmc1.fixture-scoped-token", Subject: coordination.Ref{Node: strings.Repeat("b", 32), Owner: "owner", Session: session, Incarnation: strings.Repeat("c", 32)}}
	name, err := PrepareSocket(session, key, channel)
	if err != nil {
		t.Fatal(err)
	}
	locator, _ := Locator(session, key)
	if _, err := os.Stat(locator); !os.IsNotExist(err) {
		t.Fatal("prepare published access before a working channel existed")
	}
	l, err := net.Listen("unix", name)
	if err != nil {
		t.Fatal(err)
	}
	status, ref := http.StatusOK, source.Subject
	var mu sync.Mutex
	h := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+source.Capability || r.Header.Get(LeaseProtocolHeader) != "1" || r.URL.Path != "/view" {
			t.Error("forwarded request lost scope or protocol", r.URL.Path)
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(coordination.View{Contract: coordination.Contract, Enabled: true, Self: &coordination.Member{MemberInput: coordination.MemberInput{Ref: ref}}})
	})}
	go h.Serve(l)
	t.Cleanup(func() { _ = h.Close() })
	if err := InstallSocket(context.Background(), session, key, channel, source); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(locator)
	var installed Access
	if json.Unmarshal(data, &installed) != nil || installed.Socket != name || installed.Endpoint != SocketEndpoint {
		t.Fatal("private socket access was not installed")
	}
	t.Setenv("WEBMUX_COORDINATION_FILE", locator)
	t.Setenv("WEBMUX_COORDINATION_EXPECTED_SESSION", session)
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	for _, failure := range []string{"foreign-subject", "refused"} {
		t.Run(failure, func(t *testing.T) {
			mu.Lock()
			if failure == "foreign-subject" {
				ref.Owner = "foreign"
			} else {
				status = http.StatusForbidden
			}
			mu.Unlock()
			if err := InstallSocket(context.Background(), session, key, channel, source); err == nil {
				t.Fatal("unverified forwarded channel replaced current access")
			}
			current, _ := os.ReadFile(locator)
			if !bytes.Equal(current, data) {
				t.Fatal("failed installation changed existing agent access")
			}
		})
	}
	_ = h.Close()
	if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte("not a socket"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("regular file accepted as a coordination socket")
	}
	if other, err := PrepareSocket("other-session", key, channel); err != nil || other == name {
		t.Fatal("socket scope crossed sessions", err)
	}
	if err := os.Chmod(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareSocket(session, key, channel); err == nil {
		t.Fatal("public socket directory accepted")
	}
}
