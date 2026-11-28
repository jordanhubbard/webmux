package session

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/recovery"
	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

type coordinationObserveProcess struct {
	*fakeProcess
	observe func(context.Context, recovery.RPCRequest) recovery.RPCResponse
}

func (p *coordinationObserveProcess) RecoveryRPC(ctx context.Context, request recovery.RPCRequest) recovery.RPCResponse {
	return p.observe(ctx, request)
}

func TestCoordinationMissingDirectoryRequiresCurrentLiveProvider(t *testing.T) {
	for _, boundary := range []string{"success", "cli-only", "error", "protocol", "relative", "empty", "control", "oversize", "invalid-utf8", "unknown-platform", "owner", "created", "key", "newer-observation", "newer-cli", "replaced-run", "inactive", "provider", "closed", "cancelled", "stale-snapshot", "snapshot-owner", "already-known", "exec", "non-live"} {
		t.Run(boundary, func(t *testing.T) {
			b, _, _ := fixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := &coordinationObserveProcess{fakeProcess: fake()}
			var mutate func()
			probes := 0
			p.observe = func(_ context.Context, request recovery.RPCRequest) recovery.RPCResponse {
				probes++
				if request.Operation != "coordination-observe" || request.CoordinationDirectory != "" || request.Action != nil || request.Recording != nil {
					t.Fatal("directory probe carried caller routing or lifecycle input", request.Operation)
				}
				if _, err := b.Get("owner", "missing"); !errors.Is(err, ErrNotFound) {
					t.Fatal("provider probe held the broker lock", err)
				}
				if mutate != nil {
					mutate()
				}
				response := recovery.RPCResponse{Protocol: 1, CoordinationPlatform: "linux", CoordinationDirectory: "/live/provider/project"}
				switch boundary {
				case "error":
					response.Error = "provider unavailable"
				case "protocol":
					response.Protocol = 0
				case "relative":
					response.CoordinationDirectory = "relative"
				case "empty":
					response.CoordinationDirectory = ""
				case "control":
					response.CoordinationDirectory = "/work/\nproject"
				case "oversize":
					response.CoordinationDirectory = "/" + strings.Repeat("x", 4096)
				case "invalid-utf8":
					response.CoordinationDirectory = "/work/\xff"
				case "unknown-platform":
					response.CoordinationPlatform = "unknown"
				}
				return response
			}
			b.launch = func(terminal.LaunchRequest, string) (process, error) { return p, nil }
			snapshot, err := b.Create("owner", CreateRequest{Transport: "local"})
			if err != nil {
				t.Fatal(err)
			}
			b.mu.Lock()
			e := b.entries[snapshot.ID]
			e.value.State, e.value.ExecCwd, e.value.RecoveryKey = "connected", "", "fixture-owner-key"
			e.value.CoordinationActivity = nil
			if boundary == "cli-only" {
				e.value.CoordinationActivity = &CoordinationObservation{CLI: "codex", Interface: "native-fixture"}
			}
			originalValue, originalRun := e.value.clone(), e.run
			snapshot = b.coordinationSnapshotLocked()[0]
			b.mu.Unlock()
			t.Cleanup(func() {
				b.mu.Lock()
				e.value, e.run, b.closed = originalValue, originalRun, false
				b.mu.Unlock()
			})
			if !snapshot.CoordinationLive || snapshot.CoordinationRun == 0 {
				t.Fatal("fixture omitted its live broker run")
			}
			mutate = func() {
				b.mu.Lock()
				defer b.mu.Unlock()
				switch boundary {
				case "owner":
					e.value.Owner = "foreign"
				case "created":
					e.value.CreatedAt = "replacement"
				case "key":
					e.value.RecoveryKey = "replacement-key"
				case "newer-observation":
					e.value.CoordinationActivity = &CoordinationObservation{Directory: "/newer/signed/project"}
				case "newer-cli":
					e.value.CoordinationActivity = &CoordinationObservation{CLI: "newer-cli"}
				case "replaced-run":
					e.run = &run{process: p, generation: originalRun.generation + 1}
				case "inactive":
					e.value.State = "disconnected"
				case "provider":
					e.value.Rig = &RigIdentity{Provider: "foreign-provider"}
				case "closed":
					b.closed = true
				case "cancelled":
					cancel()
				}
			}
			switch boundary {
			case "stale-snapshot":
				snapshot.CoordinationRun--
			case "snapshot-owner":
				snapshot.Owner = "foreign"
			case "already-known":
				snapshot.ExecCwd = "/retained/exec/context"
			case "exec":
				snapshot.Transport = "exec"
			case "non-live":
				snapshot.CoordinationLive = false
			}
			observed, err := b.ObserveCoordinationDirectory(ctx, snapshot)
			if boundary == "success" || boundary == "cli-only" {
				if err != nil || observed.CoordinationActivity == nil || observed.CoordinationActivity.Directory != "/live/provider/project" || !observed.CoordinationLive || observed.CoordinationRun != snapshot.CoordinationRun {
					t.Fatal("live provider context was not restored", observed, err)
				}
				if boundary == "cli-only" && (observed.CoordinationActivity.CLI != "codex" || observed.CoordinationActivity.Interface != "native-fixture") {
					t.Fatal("directory observation discarded the live CLI interface")
				}
			} else if err == nil || observed.ID != "" {
				t.Fatal("unavailable or stale provider became directory authority", observed, err)
			}
			if boundary == "stale-snapshot" || boundary == "snapshot-owner" || boundary == "already-known" || boundary == "exec" || boundary == "non-live" {
				if probes != 0 {
					t.Fatal("ineligible snapshot probed a provider")
				}
			} else if probes != 1 {
				t.Fatal("expected one read-only live provider probe", probes)
			}
			if len(p.input) != 0 {
				t.Fatal("directory observation typed terminal input")
			}
		})
	}
}
