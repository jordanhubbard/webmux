package nativeprovider

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/screenstate"
)

func TestClientNegotiatesOnlyWithAdvertisedRendererOwner(t *testing.T) {
	for _, tc := range []struct {
		name, replay, offered, required, token string
		accepted                               bool
	}{
		{"legacy-ordinary", "screen-v1", screenstate.XtermRendererProfile, "", "", true},
		{"modern-ordinary", "screen-v2", screenstate.XtermRendererProfile, "", "", true},
		{"modern-profile", "screen-v2", screenstate.XtermRendererProfile, screenstate.XtermRendererProfile, strings.Repeat("a", 32), true},
		{"legacy-unsolicited", "screen-v1", screenstate.XtermRendererProfile, screenstate.XtermRendererProfile, strings.Repeat("a", 32), false},
		{"modern-unsolicited", "screen-v2", "", screenstate.XtermRendererProfile, strings.Repeat("a", 32), false},
		{"unknown", "screen-v2", screenstate.XtermRendererProfile, "unknown", strings.Repeat("a", 32), false},
		{"invalid-token", "screen-v2", screenstate.XtermRendererProfile, screenstate.XtermRendererProfile, strings.Repeat("X", 32), false},
		{"orphan-token", "screen-v2", screenstate.XtermRendererProfile, "", strings.Repeat("a", 32), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// AF_UNIX has a short pathname limit on Windows. Go's TempDir
			// includes this long test/subtest name and exceeds that limit.
			directory, err := os.MkdirTemp("", "np-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.RemoveAll(directory); err != nil {
					t.Error(err)
				}
			})
			endpoint := filepath.Join(directory, "profile.sock")
			listener, err := net.Listen("unix", endpoint)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			identity := Identity{Session: "owned", Instance: "instance", OwnerPID: 1234, Replay: tc.replay, Endpoint: endpoint}
			done := make(chan error, 1)
			go func() {
				c, err := listener.Accept()
				if err != nil {
					done <- err
					return
				}
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(3 * time.Second))
				var request map[string]any
				if err := readFrame(c, &request); err != nil {
					done <- err
					return
				}
				if tc.replay == "screen-v1" || tc.offered == "" {
					if _, ok := request["renderer"]; ok {
						done <- context.Canceled
						return
					}
				} else if request["renderer"] != tc.offered {
					done <- context.Canceled
					return
				}
				if err := writeFrame(c, frame{Kind: "attached", Identity: &identity, RequiredRenderer: tc.required, RendererToken: tc.token}); err != nil {
					done <- err
					return
				}
				if tc.accepted {
					_ = writeFrame(c, frame{Kind: "exit"})
				}
				done <- nil
			}()
			client, err := AttachWithProfile(context.Background(), identity, strings.Repeat("b", 32), 80, 24, tc.offered)
			if tc.accepted {
				if err != nil {
					t.Fatal(err)
				}
				profile, token := client.SnapshotProfile()
				if profile != tc.required || token != tc.token {
					t.Fatal("handshake metadata changed")
				}
				_, _ = client.Wait()
				_ = client.Close()
			} else if err == nil {
				_ = client.Close()
				t.Fatal("unnegotiated metadata accepted")
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal("request contract failed", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("handshake did not finish")
			}
		})
	}
}
