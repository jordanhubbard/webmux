package coordination

import (
	"context"
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
