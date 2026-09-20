package session

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func event(t *testing.T, v *Viewer, kind string) Event {
	t.Helper()
	select {
	case value := <-v.Events():
		if value["type"] != kind {
			t.Fatalf("event %v, wanted %s", value, kind)
		}
		return value
	case <-time.After(5 * time.Second):
		t.Fatalf("waiting for %s", kind)
		return nil
	}
}
func TestViewerPresenceFocusAndReplay(t *testing.T) {
	b, _, launched := fixture(t)
	s := create(t, b, "owner")
	(<-launched).output(t, "prior output\n")
	eventually(t, func() bool { text, _ := b.Scrollback("owner", s.ID); return text != "" })
	if _, err := b.Join("foreign", s.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign viewer admitted")
	}
	first, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	joined := event(t, first, "viewer_join")
	if joined["focus_owner"] != first.ID || joined["viewer_count"] != 1 {
		t.Fatalf("first join: %v", joined)
	}
	status := event(t, first, "status")
	if status["viewer_id"] != first.ID || status["state"] != "connected" {
		t.Fatalf("status: %v", status)
	}
	if replay := event(t, first, "output"); replay["data"] != "prior output\n" {
		t.Fatalf("replay: %v", replay)
	}
	second, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if joined := event(t, first, "viewer_join"); joined["viewer_count"] != 2 || joined["focus_owner"] != first.ID {
		t.Fatalf("second join: %v", joined)
	}
	event(t, second, "viewer_join")
	event(t, second, "status")
	event(t, second, "output")
	if err := b.Focus("owner", s.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	for _, v := range []*Viewer{first, second} {
		if focused := event(t, v, "focus"); focused["focus_owner"] != second.ID {
			t.Fatalf("focus: %v", focused)
		}
	}
	b.Leave("owner", s.ID, second.ID)
	left := event(t, first, "viewer_leave")
	if _, ok := left["focus_owner"]; ok {
		t.Fatal("focus was silently transferred")
	}
	if left["viewer_count"] != 1 {
		t.Fatal("wrong viewer count")
	}
	if err := b.Delete("owner", s.ID); err != nil {
		t.Fatal(err)
	}
	code, reason := first.CloseReason()
	if code != 1000 || reason != "Session deleted" {
		t.Fatalf("delete close: %d %s", code, reason)
	}
}

func TestSlowViewerCannotBlockTerminal(t *testing.T) {
	b, _, launched := fixture(t)
	s := create(t, b, "owner")
	p := <-launched
	v, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	for range 140 {
		p.output(t, "chunk\n")
	}
	select {
	case <-v.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("slow viewer was not bounded")
	}
	if code, _ := v.CloseReason(); code != 1013 {
		t.Fatalf("slow viewer close code %d", code)
	}
	if value, err := b.Get("owner", s.ID); err != nil || value.State != "connected" {
		t.Fatal("slow viewer stopped terminal")
	}
	b.Leave("owner", s.ID, v.ID)
	next, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	event(t, next, "viewer_join")
	event(t, next, "status")
	if replay := event(t, next, "output"); replay["data"] == "" {
		t.Fatal("reconnect lost scrollback")
	}
}

func TestJoinReplayAndLiveOutputHaveNoGapOrDuplication(t *testing.T) {
	b, _, launched := fixture(t)
	s := create(t, b, "owner")
	p := <-launched
	var expected strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&expected, "line:%03d\n", i)
	}
	for i := 0; i < 25; i++ {
		p.output(t, fmt.Sprintf("line:%03d\n", i))
	}
	v, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 25; i < 100; i++ {
		p.output(t, fmt.Sprintf("line:%03d\n", i))
	}
	var received strings.Builder
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for received.Len() < expected.Len() {
		select {
		case value := <-v.Events():
			if value["type"] == "output" {
				received.WriteString(value["data"].(string))
			}
		case <-timeout.C:
			t.Fatal("missing terminal output")
		}
	}
	if received.String() != expected.String() {
		t.Fatalf("replay/live boundary changed output: %q", received.String())
	}
}

func TestBlockedInputIsBoundedAndShutdownCancelsWriter(t *testing.T) {
	b, _, launched := fixture(t)
	s := create(t, b, "owner")
	p := <-launched
	p.input = make(chan string) // Simulate a PTY that has stopped reading input.
	busy := false
	for range 300 {
		err := b.Input("owner", s.ID, "input")
		if errors.Is(err, ErrInputBusy) {
			busy = true
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if !busy {
		t.Fatal("input was not bounded")
	}
	if len(b.List("owner")) != 1 {
		t.Fatal("blocked input prevented session lookup")
	}
	done := make(chan error, 1)
	go func() { done <- b.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown left blocked input writer")
	}
}
