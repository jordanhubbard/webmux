package coordination

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/gitobserve"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitFixture(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture git %v: %v %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}
func repositoryFixture(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	dir := t.TempDir()
	gitFixture(t, dir, "init")
	gitFixture(t, dir, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit", "--allow-empty", "-m", "root")
	gitFixture(t, dir, "remote", "add", "origin", "git@example.test:team/project.git")
	return dir
}
func TestRepositoryIdentityUsesMetadataAndRootsAcrossWorktrees(t *testing.T) {
	repo := repositoryFixture(t)
	root, err := InspectRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(repo, ".worktrees", "peer")
	gitFixture(t, repo, "worktree", "add", "-b", "peer", linked, "HEAD")
	second, err := InspectRepository(context.Background(), linked)
	if err != nil {
		t.Fatal(err)
	}
	if root.ID != second.ID || root.Worktree == second.Worktree || second.Branch != "peer" {
		t.Fatal(root, second)
	}
	gitFixture(t, repo, "remote", "set-url", "origin", "https://secret-user:secret-password@EXAMPLE.TEST/team/project.git")
	rewritten, err := InspectRepository(context.Background(), repo)
	if err != nil || rewritten.ID != root.ID || strings.Contains(rewritten.Repository, "secret-") {
		t.Fatal(rewritten, err)
	}
	// An inherited repository selection must not redirect inspection away from
	// the directory observed by the session's own execution host.
	foreign := repositoryFixture(t)
	gitFixture(t, foreign, "remote", "set-url", "origin", "https://example.test/another/project.git")
	t.Setenv("GIT_DIR", filepath.Join(foreign, ".git"))
	t.Setenv("GIT_WORK_TREE", foreign)
	checked, err := InspectRepository(context.Background(), repo)
	if err != nil || checked.ID != root.ID {
		t.Fatal("inherited Git context changed repository identity", checked, err)
	}
	if err := os.Mkdir(filepath.Join(repo, "child"), 0700); err != nil {
		t.Fatal(err)
	}
	child, err := InspectRepository(context.Background(), filepath.Join(repo, "child"))
	if err != nil || child.ID != root.ID {
		t.Fatal(child, err)
	}
}
func TestRepositoryIdentityRefusesUnverifiedOrAmbiguousSources(t *testing.T) {
	repo := repositoryFixture(t)
	gitFixture(t, repo, "remote", "remove", "origin")
	if _, err := InspectRepository(context.Background(), repo); err == nil {
		t.Fatal("remote-less repository was guessed from its basename")
	}
	if _, err := InspectRepository(context.Background(), t.TempDir()); err == nil {
		t.Fatal("ordinary directory became a project")
	}
	if _, err := InspectRepository(context.Background(), "relative/path"); err == nil {
		t.Fatal("relative execution-host directory accepted")
	}
	for _, remote := range []string{"/local/repo", "file:///local/repo", "https://host/team/repo?token=secret", "https://host/team/../repo", "host:../repo"} {
		if _, ok := canonicalRepositoryRemote(remote); ok {
			t.Fatal("ambiguous remote accepted", remote)
		}
	}
	for _, remote := range []string{"git@EXAMPLE.test:team/project.git", "example.test:team/project.git", "https://example.test/team/project.git", "ssh://git@example.test:22/team/project.git"} {
		if got, ok := canonicalRepositoryRemote(remote); !ok || got != "example.test/team/project" {
			t.Fatal(got, ok)
		}
	}
	left, _ := canonicalRepositoryRemote("ssh://git@example.test:443/team/project.git")
	right, _ := canonicalRepositoryRemote("https://example.test/team/project.git")
	if left == right {
		t.Fatal("different nondefault remote ports collapsed")
	}
}

func TestRepositoryRemoteAtSignInPathDoesNotChooseHost(t *testing.T) {
	for _, input := range []string{"git@example.com:team/repo@release.git", "example.com:team/repo@release.git"} {
		got, ok := canonicalRepositoryRemote(input)
		if !ok || got != "example.com/team/repo@release" {
			t.Fatalf("remote %q: got %q, %v", input, got, ok)
		}
	}
	if _, ok := canonicalRepositoryRemote("user@:repo"); ok {
		t.Fatal("accepted missing host")
	}
}

// Exercise the actual witness path, including its optional config value. A
// source-only trace cannot diagnose a failure in these earlier commands.
func TestRepositoryWitnessTraceRetainsIdentityAndDeadline(t *testing.T) {
	directory := repositoryFixture(t)
	plain, err := InspectRepository(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	var events []gitobserve.Event
	traced, err := InspectRepository(gitobserve.WithTrace(t.Context(), func(event gitobserve.Event) {
		events = append(events, event)
	}), directory)
	if err != nil || traced == nil || plain.ID != traced.ID || plain.Worktree != traced.Worktree {
		t.Fatal("trace changed repository identity", err)
	}
	var phases []string
	for i, event := range events {
		wantResolution := "reused"
		if i == 0 {
			wantResolution = "initial"
		}
		if event.ExecutableResolution != wantResolution {
			t.Fatal("witness executable resolution changed", event)
		}
		phases = append(phases, event.Phase)
		if !event.HasDeadline || event.Remaining <= 0 || event.Remaining > 2*time.Second {
			t.Fatal("witness deadline changed", event)
		}
	}
	if !slices.Equal(phases, []string{"repository.layout", "repository.config", "repository.roots"}) {
		t.Fatal("missing witness evidence", phases)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	events = nil
	_, err = InspectRepository(gitobserve.WithTrace(ctx, func(event gitobserve.Event) {
		events = append(events, event)
	}), directory)
	if !errors.Is(err, ErrRepository) || len(events) != 1 || events[0].Outcome != "canceled" {
		t.Fatal("trace changed cancellation refusal", err, events)
	}
}

func TestRepositoryIdentityRefusesLateCancellation(t *testing.T) {
	for _, phase := range []string{"repository.layout", "repository.config", "repository.roots"} {
		t.Run(phase, func(t *testing.T) {
			directory := repositoryFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			observed := false
			ctx = gitobserve.WithTrace(ctx, func(event gitobserve.Event) {
				if event.Phase == phase {
					observed = true
					cancel()
				}
			})
			binding, err := InspectRepository(ctx, directory)
			if !observed {
				t.Fatal("did not reach late witness phase")
			}
			if !errors.Is(err, ErrRepository) || binding != nil {
				t.Fatal("canceled witness published repository identity", binding, err)
			}
		})
	}
}

func TestRepositoryIdentityAllowsDetachedAndMissingIgnorecase(t *testing.T) {
	directory := repositoryFixture(t)
	gitFixture(t, directory, "config", "--local", "core.ignorecase", "false")
	gitFixture(t, directory, "config", "--local", "--unset", "core.ignorecase")
	attached, err := InspectRepository(t.Context(), directory)
	if err != nil || attached == nil || attached.CaseFold || attached.Branch == "(detached)" {
		t.Fatal("missing optional ignorecase refused attached repository", attached, err)
	}
	gitFixture(t, directory, "checkout", "--detach", "HEAD")
	detached, err := InspectRepository(t.Context(), directory)
	if err != nil || detached == nil || detached.ID != attached.ID || detached.Branch != "(detached)" || detached.CaseFold {
		t.Fatal("detached repository with missing optional ignorecase refused", detached, err)
	}
}

func TestRepositoryBatchedConfigMatchesLocalGet(t *testing.T) {
	directory := repositoryFixture(t)
	gitFixture(t, directory, "config", "--local", "--add", "remote.origin.url", "https://example.test/last/project.git")
	gitFixture(t, directory, "config", "--local", "--replace-all", "core.ignorecase", "false")
	gitFixture(t, directory, "config", "--local", "--add", "core.ignorecase", "true")
	remote := gitFixture(t, directory, "config", "--local", "--get", "remote.origin.url")
	folded := gitFixture(t, directory, "config", "--local", "--get", "core.ignorecase")
	canonical, ok := canonicalRepositoryRemote(remote)
	if !ok || folded != "true" {
		t.Fatal("fixture does not exercise last-value semantics")
	}
	got, err := InspectRepository(t.Context(), directory)
	if err != nil || got == nil || got.Repository != canonical || !got.CaseFold {
		t.Fatal("batched config differs from local --get", got, err)
	}
	// Git emits valueless entries without a key/value newline. Previously
	// --get returned an empty string, which does not enable case folding.
	config := filepath.Join(directory, ".git", "config")
	file, err := os.OpenFile(config, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString("\n[core]\nignorecase\n")
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal(writeErr, closeErr)
	}
	if value := gitFixture(t, directory, "config", "--local", "--get", "core.ignorecase"); value != "" {
		t.Fatal("valueless fixture changed", value)
	}
	got, err = InspectRepository(t.Context(), directory)
	if err != nil || got == nil || got.Repository != canonical || got.CaseFold {
		t.Fatal("valueless optional config changed identity", got, err)
	}
}
