package nativeprovider

import (
	"bytes"
	"github.com/jordanhubbard/webmux/server/internal/screenstate"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type snapshotTerminal struct{ closed atomic.Int32 }

func (t *snapshotTerminal) Read([]byte) (int, error)    { return 0, io.EOF }
func (t *snapshotTerminal) Write(p []byte) (int, error) { return len(p), nil }
func (t *snapshotTerminal) Close() error                { t.closed.Add(1); return nil }
func (t *snapshotTerminal) Resize(int, int) error       { return nil }
func (t *snapshotTerminal) Wait() (int, error)          { return 0, nil }
func (t *snapshotTerminal) PID() int                    { return 3344 }

func TestSnapshotRefusalPreservesOwnerTerminal(t *testing.T) {
	screen, err := screenstate.New(80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer screen.Close()
	if err := screen.Feed([]byte("\x1b[?7l\x1b[4;80HX")); err != nil {
		t.Fatal(err)
	}
	terminal := &snapshotTerminal{}
	o := &owner{identity: Identity{Session: "owned-session", Instance: "one-instance", OwnerPID: 1122, RootPID: terminal.PID(), Replay: "screen-v1"}, key: strings.Repeat("a", 32), terminal: terminal, screen: screen, changed: make(chan struct{}), viewers: make(map[*subscriber]bool)}
	connect := func() (net.Conn, <-chan struct{}) {
		server, client := net.Pipe()
		t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
		done := make(chan struct{})
		go func() { defer close(done); o.handle(server) }()
		_ = client.SetDeadline(time.Now().Add(3 * time.Second))
		if err := writeFrame(client, frame{Protocol: 1, Kind: "attach", Session: o.identity.Session, Instance: o.identity.Instance, Key: o.key, Cols: 80, Rows: 24}); err != nil {
			t.Fatal(err)
		}
		return client, done
	}
	first, done := connect()
	var reply frame
	if err := readFrame(first, &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Kind != "rejected" || reply.Error != screenstate.ErrUnsupported.Error() {
		t.Fatal("unsafe snapshot was not refused", reply.Kind, reply.Error)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("refused attachment did not finish")
	}
	if terminal.closed.Load() != 0 || len(o.viewers) != 0 {
		t.Fatal("refusal stopped the terminal or retained a viewer")
	}
	// Later output makes the cut unambiguous; the same owner accepts a viewer.
	o.publish(frame{Kind: "data", Data: []byte("\x1b[HSAFE")})
	second, done := connect()
	if err := readFrame(second, &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Kind != "attached" || reply.Identity == nil || *reply.Identity != o.identity {
		t.Fatal("refusal replaced the owner", reply.Kind)
	}
	var output []byte
	for !bytes.Contains(output, []byte("SAFE")) {
		if err := readFrame(second, &reply); err != nil {
			t.Fatal(err)
		}
		if reply.Kind != "data" {
			t.Fatal("unexpected snapshot frame", reply.Kind)
		}
		output = append(output, reply.Data...)
		if len(output) > screenstate.MaxSnapshotBytes {
			t.Fatal("unbounded snapshot")
		}
	}
	_ = second.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("detached viewer did not finish")
	}
	if terminal.closed.Load() != 0 {
		t.Fatal("viewer detach closed the retained terminal")
	}
}
