package maccoord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

func sourceFixture(t *testing.T) (string, *Bindings, *coordination.Binding) {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sourceRun(t, directory, "init", "--initial-branch=main")
	sourceRun(t, directory, "config", "user.name", "Source fixture")
	sourceRun(t, directory, "config", "user.email", "source@example.test")
	sourceRun(t, directory, "config", "commit.gpgsign", "false")
	sourceRun(t, directory, "remote", "add", "origin", "https://example.test/team/alpha.git")
	if err := os.WriteFile(filepath.Join(directory, "tracked.txt"), []byte("initial\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sourceRun(t, directory, "add", "tracked.txt")
	sourceRun(t, directory, "commit", "-m", "initial")
	repo, err := coordination.InspectRepository(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	document := bindingFixture()
	document.Bindings[0].RepositoryID, document.Bindings[0].Repository = repo.ID, repo.Repository
	bindings, err := ParseBindings(bindingBytes(t, document))
	if err != nil {
		t.Fatal(err)
	}
	return directory, bindings, repo
}

func sourceRun(t *testing.T, directory string, args ...string) string {
	t.Helper()
	output, err := sourceGit(t.Context(), directory, args...)
	if err != nil {
		t.Fatalf("fixture Git operation failed: %s, %v", output, err)
	}
	return strings.TrimSpace(string(output))
}

func TestSourceExactCommitDirtyAndDetached(t *testing.T) {
	directory, bindings, repo := sourceFixture(t)
	observe := func(dirty, detached bool) SourceObservation {
		t.Helper()
		got, err := bindings.ObserveSource(t.Context(), "alice", repo)
		if err != nil || got.Schema != SourceSchema || got.Commit != sourceRun(t, directory, "rev-parse", "HEAD") ||
			got.Dirty != dirty || got.Detached != detached || got.BindingRevision == "" || got.ObservedAt.IsZero() ||
			got.RepositoryID != repo.ID || got.Repository != repo.Repository || got.Worktree != repo.Worktree ||
			got.AuthorityID != "fleet-a" || got.ProjectKey != "alpha" || got.Provenance != "verified-git" {
			t.Fatalf("wrong source observation: %+v, %v", got, err)
		}
		return got
	}
	if got := observe(false, false); got.Ref != "refs/heads/main" {
		t.Fatal("branch ref was inferred incorrectly")
	}
	if err := os.WriteFile(filepath.Join(directory, "tracked.txt"), []byte("modified\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := observe(true, false)
	sourceRun(t, directory, "add", "tracked.txt")
	observe(true, false)
	sourceRun(t, directory, "commit", "-m", "next")
	if got := observe(false, false); got.Commit == before.Commit {
		t.Fatal("source commit did not refresh")
	}
	if err := os.WriteFile(filepath.Join(directory, "untracked.txt"), []byte("untracked\n"), 0600); err != nil {
		t.Fatal(err)
	}
	observe(true, false)
	sourceRun(t, directory, "checkout", "--detach")
	if got := observe(true, true); got.Ref != "" {
		t.Fatal("detached commit acquired a branch")
	}
}

func TestSourceUsesActualRepositoryAndAccount(t *testing.T) {
	_, bindings, repo := sourceFixture(t)
	otherDirectory, _, _ := sourceFixture(t)
	// Ensure distinct roots even when fixture commit timestamps coincide.
	sourceRun(t, otherDirectory, "commit", "--amend", "-m", "different root")
	other, err := coordination.InspectRepository(t.Context(), otherDirectory)
	if err != nil || other.ID == repo.ID {
		t.Fatal("invalid independent-root control", err)
	}
	changed := *repo
	changed.Worktree = other.Worktree
	for _, test := range []struct {
		name, account string
		repo          *coordination.Binding
	}{
		{"foreign account", "bob", repo},
		{"unknown account", "absent", repo},
		{"missing repository", "alice", nil},
		{"different actual root with same claimed identity", "alice", &changed},
		{"different selected repository", "alice", other},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := bindings.ObserveSource(t.Context(), test.account, test.repo)
			encoded, _ := json.Marshal(got)
			if err == nil || got.Schema != "" || got.Worktree != "" || got.Commit != "" ||
				bytes.Contains(encoded, []byte(repo.Repository)) || strings.Contains(err.Error(), repo.Worktree) {
				t.Fatalf("refusal leaked source metadata: %+v, %v", got, err)
			}
		})
	}
	// Account authorization precedes validation of the source path.
	changed.Worktree = "relative/unavailable/private-source"
	if _, err := bindings.ObserveSource(t.Context(), "bob", &changed); !errors.Is(err, ErrUnmapped) {
		t.Fatal("foreign account reached source inspection", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got, err := bindings.ObserveSource(ctx, "alice", repo); !errors.Is(err, context.Canceled) || !errors.Is(err, ErrSource) || got.Schema != "" {
		t.Fatal("cancelled source observation returned success or lost cancellation", err)
	}
}

func TestSourceDoesNotFollowInheritedGitRouting(t *testing.T) {
	directory, bindings, repo := sourceFixture(t)
	otherDirectory, _, _ := sourceFixture(t)
	for name, value := range map[string]string{
		"GIT_DIR": filepath.Join(otherDirectory, ".git"), "GIT_WORK_TREE": otherDirectory,
		"GIT_CONFIG_COUNT": "1", "GIT_CONFIG_KEY_0": "remote.origin.url", "GIT_CONFIG_VALUE_0": "https://foreign.example/private.git",
	} {
		t.Setenv(name, value)
	}
	got, err := bindings.ObserveSource(t.Context(), "alice", repo)
	if err != nil || got.Worktree != directory || got.Repository != repo.Repository {
		t.Fatalf("inherited Git routing changed selected source: %+v, %v", got, err)
	}
}

func TestSourceOutputBoundIncludesReaderFrom(t *testing.T) {
	// io.Copy must not bypass the bound via bytes.Buffer's ReaderFrom method.
	var output sourceOutput
	data := strings.Repeat("x", (64<<10)+1)
	n, err := io.Copy(&output, struct{ io.Reader }{strings.NewReader(data)})
	if !errors.Is(err, ErrSource) || n > 64<<10 || len(output.Bytes()) > 64<<10 {
		t.Fatal("source command output escaped its bound", n, err)
	}
}

// Explicit local-worktree qualification; no credentials, MAC request, checkout
// or configuration mutation. The dedicated receipt must not already exist.
func TestNativeLocalSourceObservation(t *testing.T) {
	output := os.Getenv("WEBMUX_MAC_SOURCE_RECEIPT")
	if output == "" {
		t.Skip("requires explicit source worktree, commit and new receipt path")
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repo, err := coordination.InspectRepository(t.Context(), cwd)
	if err != nil || repo.Worktree != os.Getenv("WEBMUX_MAC_SOURCE_EXPECTED_ROOT") {
		t.Fatal("native source worktree does not match explicit expectation")
	}
	document := bindingFixture()
	document.Bindings[0].RepositoryID, document.Bindings[0].Repository = repo.ID, repo.Repository
	bindings, err := ParseBindings(bindingBytes(t, document))
	if err != nil {
		t.Fatal(err)
	}
	got, err := bindings.ObserveSource(t.Context(), "alice", repo)
	if err != nil || got.Commit != os.Getenv("WEBMUX_MAC_SOURCE_EXPECTED_COMMIT") ||
		got.Dirty != (os.Getenv("WEBMUX_MAC_SOURCE_EXPECTED_DIRTY") == "1") {
		t.Fatal("native source observation does not match explicit expectation", err)
	}
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("cannot retain a new private native source receipt")
	}
	if err := json.NewEncoder(file).Encode(got); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
