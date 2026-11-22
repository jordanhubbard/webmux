package session

import (
	"context"
	"errors"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/recovery"
	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

func TestCoordinationDirectoryUsesExecutionHostPathSyntax(t *testing.T) {
	for _, test := range []struct {
		platform, directory string
		want                bool
	}{
		{"linux", "/home/project", true}, {"darwin", "/Users/agent/project", true},
		{"windows", `C:\work\project`, true}, {"windows", "C:/work/project", true},
		{"windows", `\\host\share\project`, true}, {"windows", `C:relative`, false},
		{"windows", `\relative`, false}, {"windows", `\\host`, false},
		{"linux", "relative", false}, {"unknown", "/work/project", false},
	} {
		if got := coordinationObservedDirectory(recovery.Event{Platform: test.platform, Cwd: test.directory}); got != test.want {
			t.Errorf("%s %q: got %t want %t", test.platform, test.directory, got, test.want)
		}
	}
}

type coordinationRepositoryProcess struct {
	*fakeProcess
	inspect func(recovery.RPCRequest) recovery.RPCResponse
}

func (p *coordinationRepositoryProcess) RecoveryRPC(_ context.Context, request recovery.RPCRequest) recovery.RPCResponse {
	if request.Operation == "coordination-project" {
		return p.inspect(request)
	}
	return recovery.RPCResponse{Protocol: 1}
}

func TestRemoteCoordinationRepositoryBindsCurrentRunAndDirectory(t *testing.T) {
	for _, boundary := range []string{"success", "owner", "directory", "destination", "port", "username", "key", "replaced-run", "inactive", "provider", "cancelled", "old-helper", "unverified-source"} {
		t.Run(boundary, func(t *testing.T) {
			b, _, _ := fixture(t)
			project := &coordination.Binding{ID: "verified-fixture", Repository: "example.test/team/repo", Worktree: "/remote-only/project", Source: "verified-git"}
			var mutate func()
			p := &coordinationRepositoryProcess{fakeProcess: fake()}
			p.inspect = func(request recovery.RPCRequest) recovery.RPCResponse {
				if request.CoordinationDirectory != "/remote-only/project" || request.Action != nil || request.Recording != nil {
					t.Error("inspection carried an action or a directory not observed on this session", request.Operation)
				}
				// Get takes b.mu: this also proves network work does not hold it.
				if _, err := b.Get("owner", "missing"); !errors.Is(err, ErrNotFound) {
					t.Error(err)
				}
				if mutate != nil {
					mutate()
				}
				if boundary == "old-helper" {
					return recovery.RPCResponse{Protocol: 1, Error: "unknown recovery control operation"}
				}
				if boundary == "unverified-source" {
					copy := *project
					copy.Source = "explicit-map"
					return recovery.RPCResponse{Protocol: 1, CoordinationProject: &copy}
				}
				return recovery.RPCResponse{Protocol: 1, CoordinationProject: project}
			}
			b.launch = func(terminal.LaunchRequest, string) (process, error) { return p, nil }
			snapshot, err := b.Create("owner", CreateRequest{Transport: "ssh", Hostname: "execution.example.test", Username: "remote-account"})
			if err != nil {
				t.Fatal(err)
			}
			b.mu.Lock()
			e := b.entries[snapshot.ID]
			e.value.State = "connected"
			e.value.CoordinationActivity = &CoordinationObservation{Directory: "/remote-only/project", CLI: "claude"}
			snapshot = e.value.clone()
			originalRun := e.run
			b.mu.Unlock()
			t.Cleanup(func() {
				b.mu.Lock()
				e.run = originalRun
				b.mu.Unlock()
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			mutate = func() {
				b.mu.Lock()
				defer b.mu.Unlock()
				switch boundary {
				case "owner":
					e.value.Owner = "foreign"
				case "directory":
					e.value.CoordinationActivity.Directory = "/changed"
				case "destination":
					e.value.Hostname = "replacement.example.test"
				case "port":
					e.value.Port++
				case "username":
					e.value.Username = "another-account"
				case "key":
					e.value.KeyID = "replacement-ssh-key"
				case "replaced-run":
					e.run = &run{process: p}
				case "inactive":
					e.value.State = "disconnected"
				case "provider":
					e.value.Rig = &RigIdentity{Provider: "rig"}
				case "cancelled":
					cancel()
				}
			}
			binding, err := b.InspectCoordinationRepository(ctx, snapshot)
			if boundary == "success" {
				if err != nil || binding == nil || *binding != *project {
					t.Fatal(binding, err)
				}
			} else if err == nil || binding != nil {
				t.Fatal("stale or unavailable host response became current project authority", binding, err)
			}
		})
	}
}
