package maccoord

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

const RoadmapSchema = "webmux.mac.source-roadmap-observation.v1"

var ErrRoadmapUnconfigured = errors.New("source roadmap observation is not configured")

// RoadmapObservation names immutable Git bytes, not a live working-copy file or
// a derived task queue. Text is untrusted project data, never an instruction to
// mutate MAC. No issue state, requirement/task link or priority verdict is inferred.
type RoadmapObservation struct {
	Schema           string    `json:"schema"`
	BindingRevision  string    `json:"binding_revision"`
	AuthorityID      string    `json:"authority_id"`
	ProjectKey       string    `json:"project_key"`
	RepositoryID     string    `json:"repository_id"`
	Path             string    `json:"path"`
	Commit           string    `json:"commit"`
	Blob             string    `json:"blob"`
	SHA256           string    `json:"sha256"`
	Text             string    `json:"text"`
	WorkingCopyDirty bool      `json:"working_copy_dirty"`
	TaskLinkState    string    `json:"task_link_state"`
	ObservedAt       time.Time `json:"observed_at"`
}

func validRoadmapPath(value string) bool {
	if !validText(value, 1024) || path.IsAbs(value) || path.Clean(value) != value || strings.ContainsAny(value, "\\:*?[]") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." || strings.EqualFold(part, ".git") {
			return false
		}
	}
	return true
}

// ObserveRoadmap reads only the operator-authorized regular Git blob from the
// already verified local source commit. Git never follows a worktree symlink.
// The caller must establish local execution-host provenance before invoking it.
func (b *Bindings) ObserveRoadmap(parent context.Context, owner string, repository *coordination.Binding, source SourceObservation) (RoadmapObservation, error) {
	var empty RoadmapObservation
	selected, err := b.Resolve(owner, repository)
	if err != nil {
		return empty, err
	}
	if selected.Project.RoadmapPath == "" || !slices.Contains(selected.Project.Capabilities, RoadmapObserveCapability) {
		return empty, ErrRoadmapUnconfigured
	}
	if parent == nil || source.Schema != SourceSchema || source.BindingRevision != selected.Revision || source.AuthorityID != selected.Authority.ID || source.ProjectKey != selected.Project.ProjectKey || source.RepositoryID != repository.ID || source.Repository != repository.Repository || source.Worktree != repository.Worktree {
		return empty, ErrSource
	}
	commit, err := hex.DecodeString(source.Commit)
	if err != nil || (len(commit) != 20 && len(commit) != 32) || source.Commit != strings.ToLower(source.Commit) {
		return empty, ErrSource
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	check := func() bool {
		actual, err := b.ObserveSource(ctx, owner, repository)
		return err == nil && actual.Commit == source.Commit && actual.Ref == source.Ref
	}
	if !check() {
		return empty, ErrSource
	}
	file := selected.Project.RoadmapPath
	listing, err := sourceGit(ctx, repository.Worktree, "ls-tree", "-z", source.Commit, "--", file)
	if err != nil || len(listing) == 0 || listing[len(listing)-1] != 0 || bytes.Count(listing, []byte{0}) != 1 {
		return empty, ErrSource
	}
	header, gotPath, found := strings.Cut(string(listing[:len(listing)-1]), "\t")
	fields := strings.Fields(header)
	if !found || gotPath != file || len(fields) != 3 || (fields[0] != "100644" && fields[0] != "100755") || fields[1] != "blob" {
		return empty, ErrSource
	}
	blob, err := hex.DecodeString(fields[2])
	if err != nil || len(blob) != len(commit) || fields[2] != strings.ToLower(fields[2]) {
		return empty, ErrSource
	}
	text, err := sourceGit(ctx, repository.Worktree, "cat-file", "blob", fields[2])
	if err != nil || !utf8.Valid(text) || bytes.ContainsRune(text, 0) {
		return empty, ErrSource
	}
	status, err := sourceGit(ctx, repository.Worktree, "status", "--porcelain=v1", "-z", "--untracked-files=normal", "--", file)
	if err != nil || !check() || ctx.Err() != nil {
		return empty, ErrSource
	}
	digest := sha256.Sum256(text)
	return RoadmapObservation{Schema: RoadmapSchema, BindingRevision: selected.Revision, AuthorityID: selected.Authority.ID, ProjectKey: selected.Project.ProjectKey,
		RepositoryID: repository.ID, Path: file, Commit: source.Commit, Blob: fields[2], SHA256: hex.EncodeToString(digest[:]), Text: string(text),
		WorkingCopyDirty: len(status) != 0, TaskLinkState: "unlinked", ObservedAt: time.Now().UTC()}, nil
}
