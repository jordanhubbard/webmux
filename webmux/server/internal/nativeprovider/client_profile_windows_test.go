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
	v1, v2, token := screenstate.XtermRendererProfile, screenstate.XtermUnderlineRendererProfile, strings.Repeat("a", 32)
	for _, tc := range []struct {
		name, replay, offered, sent, required, token, replyReplay string
		accepted                                                  bool
	}{
		{name: "legacy-ordinary", replay: "screen-v1", offered: v1, accepted: true},
		{name: "legacy-new-controller", replay: "screen-v1", offered: v2, accepted: true},
		{name: "modern-ordinary", replay: "screen-v2", offered: v1, sent: v1, accepted: true},
		{name: "modern-profile", replay: "screen-v2", offered: v1, sent: v1, required: v1, token: token, accepted: true},
		{name: "downlevel-old-owner", replay: "screen-v2", offered: v2, sent: v1, required: v1, token: token, accepted: true},
		{name: "old-owner-v2-unsolicited", replay: "screen-v2", offered: v2, sent: v1, required: v2, token: token},
		{name: "v3-v2-profile", replay: "screen-v3", offered: v2, sent: v2, required: v2, token: token, accepted: true},
		{name: "v3-v1-requirement", replay: "screen-v3", offered: v2, sent: v2, required: v1, token: token, accepted: true},
		{name: "v3-older-viewer", replay: "screen-v3", offered: v1, sent: v1, required: v1, token: token, accepted: true},
		{name: "v3-unsolicited-v2", replay: "screen-v3", offered: v1, sent: v1, required: v2, token: token},
		{name: "v3-unoffered-profile", replay: "screen-v3", required: v2, token: token},
		{name: "v3-version-changed", replay: "screen-v3", replyReplay: "screen-v2", offered: v2, sent: v2, required: v1, token: token},
		{name: "legacy-unsolicited", replay: "screen-v1", offered: v1, required: v1, token: token},
		{name: "modern-unsolicited", replay: "screen-v2", required: v1, token: token},
		{name: "unknown", replay: "screen-v2", offered: v1, sent: v1, required: "unknown", token: token},
		{name: "invalid-token", replay: "screen-v3", offered: v2, sent: v2, required: v2, token: strings.Repeat("X", 32)},
		{name: "orphan-token", replay: "screen-v3", offered: v2, sent: v2, token: token},
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
				if tc.sent == "" {
					if _, ok := request["renderer"]; ok {
						done <- context.Canceled
						return
					}
				} else if request["renderer"] != tc.sent {
					done <- context.Canceled
					return
				}
				replyIdentity := identity
				if tc.replyReplay != "" {
					replyIdentity.Replay = tc.replyReplay
				}
				if err := writeFrame(c, frame{Kind: "attached", Identity: &replyIdentity, RequiredRenderer: tc.required, RendererToken: tc.token}); err != nil {
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
