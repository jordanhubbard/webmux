package httpapi

import (
	"context"
	"testing"
	"time"

	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

func TestRemoteCoordinationRepositoryHasNoControllerFilesystemFallback(t *testing.T) {
	s, _ := fixture(t, "local")
	directory := t.TempDir()
	coordinationGit(t, directory, "init")
	coordinationGit(t, directory, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit", "--allow-empty", "-m", "root")
	coordinationGit(t, directory, "remote", "add", "origin", "https://example.test/team/controller-only.git")
	if _, err := plane.InspectRepository(context.Background(), directory); err != nil {
		t.Fatal(err)
	}
	snapshot := session.Session{ID: "unavailable-remote-session", Owner: "owner", CreatedAt: "current", Transport: "ssh", Hostname: "remote.example.test", Username: "alice", Port: 22,
		CoordinationActivity: &session.CoordinationObservation{Directory: directory}}
	s.refreshCoordinationBindings(context.Background(), []session.Session{snapshot})
	input := s.coordinationInputs("node", []session.Session{snapshot})[0]
	if input.Project != nil || input.ProjectStatus != "execution-host-verification-unavailable" || input.Interface != "execution-host-onboarding-pending" {
		t.Fatal("remote path was verified against the controller or advertised agent onboarding", input)
	}
	// ExecCwd and a display title are not authenticated remote observations.
	snapshot.CoordinationActivity = nil
	snapshot.ExecCwd = directory
	if coordinationDirectory(snapshot) != "" {
		t.Fatal("unobserved SSH session acquired a directory from local launch metadata")
	}
}

func TestRemoteCoordinationProjectExpiresAndDoesNotCrossSessionAccountOrDestination(t *testing.T) {
	s, _ := fixture(t, "local")
	snapshot := session.Session{ID: "remote-session", Owner: "owner", CreatedAt: "current", Transport: "mosh", Hostname: "remote.example.test", Username: "alice", Port: 22,
		CoordinationActivity: &session.CoordinationObservation{Directory: "/remote/work"}}
	key := coordinationBindingKey(snapshot)
	project := &plane.Binding{ID: "verified", Repository: "example.test/team/remote", Source: "verified-git", Worktree: "/remote/work"}
	s.sessionCoord.bindings[key] = coordinationProjectObservation{project: project, expires: time.Now().Add(time.Minute)}
	if input := s.coordinationInputs("node", []session.Session{snapshot})[0]; input.Project == nil || input.ProjectStatus != "verified-git" || input.Interface != "execution-host-onboarding-pending" {
		t.Fatal("current execution-host context missing or claimed onboarding", input)
	}
	for name, change := range map[string]func(*session.Session){
		"owner":         func(v *session.Session) { v.Owner = "other" },
		"session":       func(v *session.Session) { v.ID = "other" },
		"incarnation":   func(v *session.Session) { v.CreatedAt = "recreated" },
		"host":          func(v *session.Session) { v.Hostname = "other.example.test" },
		"account":       func(v *session.Session) { v.Username = "bob" },
		"port":          func(v *session.Session) { v.Port = 2222 },
		"ssh-key":       func(v *session.Session) { v.KeyID = "other" },
		"provider-view": func(v *session.Session) { v.Rig = &session.RigIdentity{Provider: "rig"} },
	} {
		t.Run(name, func(t *testing.T) {
			copy := snapshot
			change(&copy)
			if input := s.coordinationInputs("node", []session.Session{copy})[0]; input.Project != nil {
				t.Fatal("execution-host context crossed its verification namespace", input)
			}
		})
	}
	s.sessionCoord.bindings[key] = coordinationProjectObservation{project: project, expires: time.Now().Add(-time.Second)}
	if input := s.coordinationInputs("node", []session.Session{snapshot})[0]; input.Project != nil || input.ProjectStatus != "execution-host-verification-unavailable" {
		t.Fatal("expired remote context remained verified", input)
	}
}

func TestDisabledCoordinationWithdrawsRepositoryVerification(t *testing.T) {
	s, _ := fixture(t, "local")
	node, err := s.coordinationNode()
	if err != nil {
		t.Fatal(err)
	}
	s.sessionCoord.bindings["prior-context"] = coordinationProjectObservation{project: &plane.Binding{ID: "prior"}, expires: time.Now().Add(time.Minute)}
	if err := s.sessionCoord.service.SetEnabled(node, false); err != nil {
		t.Fatal(err)
	}
	if err := s.maintainSessionCoordination(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.sessionCoord.bindings["prior-context"] = coordinationProjectObservation{expires: time.Now().Add(time.Minute)}
	s.refreshCoordinationBindings(context.Background(), nil)
	if len(s.sessionCoord.bindings) != 0 {
		t.Fatal("request refresh ignored disabled policy")
	}
	if len(s.sessionCoord.bindings) != 0 {
		t.Fatal("disabled coordination retained verified execution-host context")
	}
	enabled, err := s.sessionCoord.service.Enabled(node)
	if err != nil || enabled {
		t.Fatal("maintenance changed global policy", enabled, err)
	}
}

func TestCoordinationSubsetRefreshPreservesOtherOwnersContext(t *testing.T) {
	s, _ := fixture(t, "local")
	a := session.Session{ID: "a", Owner: "one", Transport: "ssh", Hostname: "remote", CoordinationActivity: &session.CoordinationObservation{Directory: "/a"}}
	b := a
	b.ID, b.Owner = "b", "two"
	key := coordinationBindingKey(b)
	s.sessionCoord.bindings[key] = coordinationProjectObservation{project: &plane.Binding{ID: "other-owner"}, expires: time.Now().Add(time.Minute)}
	s.refreshCoordinationBindings(context.Background(), []session.Session{a})
	if s.sessionCoord.bindings[key].project == nil {
		t.Fatal("one owner's subset refresh evicted another owner's project")
	}
	s.pruneCoordinationBindings([]session.Session{a, b})
	if s.sessionCoord.bindings[key].project == nil {
		t.Fatal("complete current inventory lost a live context")
	}
	s.pruneCoordinationBindings([]session.Session{a})
	if _, exists := s.sessionCoord.bindings[key]; exists {
		t.Fatal("removed session's context was not pruned")
	}
}
