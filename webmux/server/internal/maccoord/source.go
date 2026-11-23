package maccoord

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

const SourceSchema = "webmux.mac.source-observation.v1"

var ErrSource = errors.New("MAC project source observation is unavailable")

// SourceObservation describes local Git metadata, not a frozen filesystem or
// source-roadmap import. Dirty includes tracked and ordinary untracked changes;
// ignored build outputs are outside this observation. Commit names the Git
// commit, never the contents of a dirty worktree. No MAC attempt is inferred.
type SourceObservation struct {
	Schema          string    `json:"schema"`
	BindingRevision string    `json:"binding_revision"`
	AuthorityID     string    `json:"authority_id"`
	ProjectKey      string    `json:"project_key"`
	Provenance      string    `json:"provenance"`
	RepositoryID    string    `json:"repository_id"`
	Repository      string    `json:"repository"`
	Worktree        string    `json:"worktree"`
	Ref             string    `json:"ref"`
	Commit          string    `json:"commit"`
	Detached        bool      `json:"detached"`
	Dirty           bool      `json:"dirty"`
	ObservedAt      time.Time `json:"observed_at"`
}

// ObserveSource is for trusted execution-host plumbing. Resolve the receiving
// account before inspecting the server-supplied worktree. Revalidate its actual
// repository witnesses and surrounding HEAD/ref observations before returning
// anything. This never fetches, runs hooks, writes the index, or contacts MAC.
// Concurrent edits can still occur after observation; publication must separately
// validate its accepted source/attempt and cannot use this as a workspace lock.
func (b *Bindings) ObserveSource(parent context.Context, owner string, repository *coordination.Binding) (SourceObservation, error) {
	selection, err := b.Resolve(owner, repository)
	if err != nil {
		return SourceObservation{}, err
	}
	if parent == nil {
		return SourceObservation{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	unavailable := func() (SourceObservation, error) {
		if ctx.Err() != nil {
			return SourceObservation{}, errors.Join(ErrSource, ctx.Err())
		}
		return SourceObservation{}, ErrSource
	}
	if !filepath.IsAbs(repository.Worktree) || ctx.Err() != nil {
		return unavailable()
	}
	inspect := func() bool {
		actual, err := coordination.InspectRepository(ctx, repository.Worktree)
		return err == nil && actual.ID == repository.ID && actual.Repository == repository.Repository &&
			filepath.Clean(actual.Worktree) == filepath.Clean(repository.Worktree)
	}
	if !inspect() {
		return unavailable()
	}
	head := func() (commit, ref string, ok bool) {
		data, err := sourceGit(ctx, repository.Worktree, "rev-parse", "--verify", "HEAD^{commit}")
		if err != nil {
			return "", "", false
		}
		commit = strings.TrimSpace(string(data))
		decoded, err := hex.DecodeString(commit)
		if err != nil || (len(decoded) != 20 && len(decoded) != 32) || strings.ToLower(commit) != commit {
			return "", "", false
		}
		data, err = sourceGit(ctx, repository.Worktree, "symbolic-ref", "--quiet", "HEAD")
		if err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() == 1 {
				return commit, "", true // Git explicitly reports detached HEAD.
			}
			return "", "", false
		}
		ref = strings.TrimSpace(string(data))
		return commit, ref, validText(ref, 1024) && strings.HasPrefix(ref, "refs/heads/")
	}
	commit, ref, ok := head()
	if !ok {
		return unavailable()
	}
	status, err := sourceGit(ctx, repository.Worktree, "status", "--porcelain=v1", "-z", "--untracked-files=normal", "--ignore-submodules=none")
	if err != nil {
		return unavailable()
	}
	if !inspect() {
		return unavailable()
	}
	afterCommit, afterRef, ok := head()
	if !ok || afterCommit != commit || afterRef != ref || ctx.Err() != nil {
		return unavailable()
	}
	return SourceObservation{
		Schema: SourceSchema, BindingRevision: selection.Revision,
		AuthorityID: selection.Authority.ID, ProjectKey: selection.Project.ProjectKey, Provenance: "verified-git",
		RepositoryID: repository.ID, Repository: repository.Repository, Worktree: repository.Worktree,
		Ref: ref, Commit: commit, Detached: ref == "", Dirty: len(status) != 0, ObservedAt: time.Now().UTC(),
	}, nil
}

// Bounded local Git metadata only. Drop inherited Git routing/config overrides,
// disable filesystem-monitor hooks and index writes, and never expose Git stderr.
func sourceGit(ctx context.Context, directory string, args ...string) ([]byte, error) {
	null := os.DevNull
	env := make([]string, 0, len(os.Environ())+4)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(strings.ToUpper(name), "GIT_") {
			env = append(env, entry)
		}
	}
	env = append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+null, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	fixed := []string{"--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-C", directory}
	command := exec.CommandContext(ctx, "git", append(fixed, args...)...)
	command.Env, command.Stderr, command.WaitDelay = env, io.Discard, 200*time.Millisecond
	var output sourceOutput
	command.Stdout = &output
	if err := command.Run(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

type sourceOutput struct{ buffer bytes.Buffer }

func (w *sourceOutput) Bytes() []byte { return w.buffer.Bytes() }

func (w *sourceOutput) Write(p []byte) (int, error) {
	if w.buffer.Len()+len(p) > 64<<10 {
		return 0, ErrSource
	}
	return w.buffer.Write(p)
}
