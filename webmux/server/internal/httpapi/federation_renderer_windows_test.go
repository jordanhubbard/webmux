//go:build windows

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jordanhubbard/webmux/server/internal/nativeprovider"
	"github.com/jordanhubbard/webmux/server/internal/screenstate"
	"github.com/jordanhubbard/webmux/server/internal/session"
	"golang.org/x/sys/windows"
)

func TestFederationNativeRendererProfilePreservesOwner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	probe := exec.CommandContext(ctx, "cmd.exe", "/d", "/c", "exit", "0")
	probe.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_BREAKAWAY_FROM_JOB}
	err := probe.Run()
	cancel()
	if err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) && os.Getenv("WEBMUX_REQUIRE_NATIVE_FEDERATION") != "1" {
			t.Skip("outer runner job forbids independent providers; dedicated native federation task requires a real pass")
		}
		t.Fatal("native provider breakaway unavailable", err)
	}
	cache := t.TempDir()
	starts := filepath.Join(cache, "fixture-pid")
	t.Setenv("WEBMUX_TEST_FEDERATION_RENDERER", "1")
	t.Setenv("WEBMUX_TEST_FEDERATION_START", starts)
	t.Setenv("WEBMUX_RECOVERY_CACHE_HOME", cache)
	a, b, handler, _, _ := federationPair(t)
	// Retain launch diagnostics from this isolated broker; the general HTTP
	// fixture deliberately discards logs and Create reports failures as state.
	if err := b.sessions.Close(); err != nil {
		t.Fatal(err)
	}
	b.sessions, err = session.New(b.store, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	current, err := b.sessions.Create("owner", session.CreateRequest{Transport: "local", Cols: 8, Rows: 4})
	if err != nil || current.State == "error" {
		t.Fatal("native local owner", current.State, err)
	}
	current, err = b.sessions.Get("owner", current.ID)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Identity nativeprovider.Identity `json:"identity"`
	}
	data, err := os.ReadFile(filepath.Join(cache, "webmux", "native-providers", current.ID, "identity.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := nativeprovider.Stop(ctx, saved.Identity, current.RecoveryKey); err != nil {
			t.Error("stop owned native fixture", err)
		}
	})
	central := httptest.NewServer(handler)
	defer central.Close()
	endpoint := "ws" + strings.TrimPrefix(central.URL, "http") + "/api/servers/remote/term/" + current.ID + "?token=" + token(t, a, "owner")
	waitOutput := func(c *websocket.Conn, needle string) {
		t.Helper()
		_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
		var text string
		for !strings.Contains(text, needle) {
			var event session.Event
			if err := c.ReadJSON(&event); err != nil {
				t.Fatalf("native output %q: %v", needle, err)
			}
			if value, ok := event["data"].(string); ok {
				text += value
			}
		}
	}
	c := dialSocket(t, endpoint+"&snapshot_profile="+screenstate.XtermUnderlineRendererProfile)
	waitOutput(c, "READY;")
	pid, err := os.ReadFile(starts)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WriteJSON(session.Event{"type": "input", "data": "R"}); err != nil {
		t.Fatal(err)
	}
	waitOutput(c, "Z")
	if err := b.sessions.Resize("owner", current.ID, 12, 4); err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
	// Recapture the same native owner after widening the edge overlap. This is
	// the actual renderer-required state from screenstate's repeat-width oracle.
	if _, err := b.sessions.Reconnect("owner", current.ID, ""); err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{"", "unknown", screenstate.XtermRendererProfile} {
		refused := dialSocket(t, endpoint+"&snapshot_profile="+profile)
		event := socketEvent(t, refused, "error")
		if event["message"] != session.ErrRendererUnsupported.Error() {
			t.Fatal("wrong refusal", event)
		}
		_, _, err := refused.ReadMessage()
		var closed *websocket.CloseError
		if !errors.As(err, &closed) || closed.Text != session.ErrRendererUnsupported.Error() {
			t.Fatal("lost native refusal", err)
		}
	}
	c = dialSocket(t, endpoint+"&snapshot_profile="+screenstate.XtermUnderlineRendererProfile)
	socketEvent(t, c, "viewer_join")
	status := socketEvent(t, c, "status")
	if status["snapshot_profile"] != screenstate.XtermUnderlineRendererProfile {
		t.Fatal("native profile not negotiated", status)
	}
	waitOutput(c, "\x1b]7777;webmux.replay.v2;")
	if err := c.WriteJSON(session.Event{"type": "input", "data": "C"}); err != nil {
		t.Fatal(err)
	}
	waitOutput(c, "\x1b[2b")
	if err := c.WriteJSON(session.Event{"type": "input", "data": "P"}); err != nil {
		t.Fatal(err)
	}
	waitOutput(c, "P:"+string(pid)+";")
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	identity, err := nativeprovider.Probe(ctx, saved.Identity, current.RecoveryKey)
	if err != nil || identity != saved.Identity {
		t.Fatal("native owner changed", err)
	}
	if inventory := b.sessions.List("owner"); len(inventory) != 1 || inventory[0].ID != current.ID {
		t.Fatal("provider inventory changed")
	}
	if _, err := b.sessions.JoinWithProfile("other", current.ID, screenstate.XtermUnderlineRendererProfile); !errors.Is(err, session.ErrNotFound) {
		t.Fatal("profile bypassed owner", err)
	}
	t.Log("native ConPTY: original owner and shell PID retained; absent/unknown viewer refused; negotiated replay and continued output passed")
}
