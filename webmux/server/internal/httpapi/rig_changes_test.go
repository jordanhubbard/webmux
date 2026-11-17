package httpapi

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRigChangesDisabledDoesNotProbeManagers(t *testing.T) {
	executable, log := fakeAgentDeck(t)
	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	s, handler := fixture(t, "none")
	setCoordinationIntegrations(t, s, "agent-deck")
	reply := request(handler, "GET", "/api/coordination/changes", "", "")
	requireStatus(t, reply, 200)
	if !strings.Contains(reply.Body.String(), `"contract":"webmux-rig-changes/v1"`) || !strings.Contains(reply.Body.String(), `"available":false`) {
		t.Fatalf("unexpected disabled notification contract: %s", reply.Body.String())
	}
	if reply.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("notification response is cacheable")
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("notification wait probed an unrelated manager")
	}
}

func TestRigChangesConcurrencyDoesNotStartQueuedWork(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Rig hosting requires POSIX")
	}
	s, _ := fixture(t, "none")
	setCoordinationIntegrations(t, s, "rig")
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "")
	bin := t.TempDir()
	log := filepath.Join(bin, "node-called")
	t.Setenv("WEBMUX_RIG_CHANGE_TEST_LOG", log)
	for _, name := range []string{"rig", "tmux", "node"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nprintf called > \"$WEBMUX_RIG_CHANGE_TEST_LOG\"\nexit 99\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	for _, size := range []int{1, 8} {
		s.rigChangesOwners = map[string]bool{"owner": true}
		for i := 1; i < size; i++ {
			s.rigChangesOwners[fmt.Sprint(i)] = true
		}
		owner := "owner"
		if size == 8 {
			owner = "new-owner"
		}
		w := httptest.NewRecorder()
		s.getRigChanges(w, httptest.NewRequest("GET", "/api/coordination/changes", nil), owner)
		if _, err := os.Stat(log); !os.IsNotExist(err) {
			t.Fatal("saturated wait started a subprocess")
		}
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"available":false`) || len(s.rigChangesOwners) != size {
			t.Fatalf("wait was queued or capacity corrupted: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestRigDiscoveryTrafficKeepsConfigurationBudgetAndRemainsLimited(t *testing.T) {
	s, handler := fixture(t, "none")
	setCoordinationIntegrations(t, s)
	for i := 0; i < 301; i++ {
		reply := request(handler, "GET", "/api/coordination/changes", "", "")
		requireStatus(t, reply, 200)
		if reply.Header().Get("RateLimit-Limit") != "1200" {
			t.Fatal("notification wait used general configuration quota")
		}
	}
	canonical := request(handler, "GET", "/api/coordination?provider=rig&observations=rig", "", "")
	requireStatus(t, canonical, 200)
	if canonical.Header().Get("RateLimit-Limit") != "1200" {
		t.Fatal("canonical invalidation read used configuration quota")
	}
	requireStatus(t, request(handler, "GET", "/api/config", "", ""), 200)
	for i := 302; i < 1200; i++ {
		requireStatus(t, request(handler, "GET", "/api/coordination/changes", "", ""), 200)
	}
	limited := request(handler, "GET", "/api/coordination/changes", "", "")
	requireStatus(t, limited, 429)
	if limited.Header().Get("Retry-After") == "" {
		t.Fatal("discovery quota lost retry guidance")
	}
	requireStatus(t, request(handler, "GET", "/api/config", "", ""), 200)
}
