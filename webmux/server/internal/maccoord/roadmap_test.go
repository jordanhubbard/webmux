package maccoord

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

func roadmapFixture(t *testing.T) (string, *Bindings, *coordination.Binding, SourceObservation) {
	t.Helper()
	directory, _, repository := sourceFixture(t)
	if err := os.MkdirAll(filepath.Join(directory, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "docs", "roadmap.md"), []byte("# Source roadmap\n\n- [ ] First requirement\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sourceRun(t, directory, "add", "docs/roadmap.md")
	sourceRun(t, directory, "commit", "-m", "source intent")
	document := bindingFixture()
	document.Bindings[0].RepositoryID = repository.ID
	document.Bindings[0].Repository = repository.Repository
	document.Bindings[0].RoadmapPath = "docs/roadmap.md"
	document.Bindings[0].Capabilities = []string{RoadmapObserveCapability, ObserveCapability}
	bindings, err := ParseBindings(bindingBytes(t, document))
	if err != nil {
		t.Fatal(err)
	}
	source, err := bindings.ObserveSource(t.Context(), "alice", repository)
	if err != nil {
		t.Fatal(err)
	}
	return directory, bindings, repository, source
}

func TestRoadmapSnapshotNamesExactSourceWithoutReadingWorkingCopy(t *testing.T) {
	directory, bindings, repository, source := roadmapFixture(t)
	want := "# Source roadmap\n\n- [ ] First requirement\n"
	// A modified working copy cannot substitute for the named immutable blob.
	if err := os.WriteFile(filepath.Join(directory, "docs", "roadmap.md"), []byte("uncommitted private replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := bindings.ObserveRoadmap(t.Context(), "alice", repository, source)
	digest := sha256.Sum256([]byte(want))
	if err != nil || got.Schema != RoadmapSchema || got.Text != want || got.Commit != source.Commit || got.Blob != sourceRun(t, directory, "rev-parse", source.Commit+":docs/roadmap.md") || got.SHA256 != hex.EncodeToString(digest[:]) || !got.WorkingCopyDirty || got.TaskLinkState != "unlinked" || got.BindingRevision != source.BindingRevision {
		t.Fatal("source roadmap was replaced or lost exact provenance", got, err)
	}
	if got.ProjectKey != "alpha" || got.AuthorityID != "fleet-a" || got.RepositoryID != repository.ID || got.ObservedAt.IsZero() {
		t.Fatal("source snapshot lost its receiving-project scope")
	}
	sourceRun(t, directory, "add", "docs/roadmap.md")
	sourceRun(t, directory, "commit", "-m", "changed source intent")
	if got, err := bindings.ObserveRoadmap(t.Context(), "alice", repository, source); !errors.Is(err, ErrSource) || got.Text != "" {
		t.Fatal("stale source commit was accepted as the current session source", err)
	}
}

func TestRoadmapAccountSourceAndCapabilityRefusals(t *testing.T) {
	_, bindings, repository, source := roadmapFixture(t)
	if got, err := bindings.ObserveRoadmap(t.Context(), "bob", repository, source); !errors.Is(err, ErrUnmapped) || got.Text != "" {
		t.Fatal("roadmap crossed the receiving-account boundary", err)
	}
	for _, change := range []string{"binding", "project", "repository", "commit", "path"} {
		wrong := source
		switch change {
		case "binding":
			wrong.BindingRevision = strings.Repeat("0", 64)
		case "project":
			wrong.ProjectKey = "another-project"
		case "repository":
			wrong.RepositoryID = "another-root"
		case "commit":
			wrong.Commit = "--all"
		case "path":
			wrong.Worktree = filepath.Join(repository.Worktree, "not-the-root")
		}
		if got, err := bindings.ObserveRoadmap(t.Context(), "alice", repository, wrong); !errors.Is(err, ErrSource) || got.Text != "" {
			t.Fatal("foreign/unverified source substituted into roadmap", change, err)
		}
	}
	document := bindingFixture()
	document.Bindings[0].RepositoryID, document.Bindings[0].Repository = repository.ID, repository.Repository
	plain, err := ParseBindings(bindingBytes(t, document))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := plain.ObserveRoadmap(t.Context(), "alice", repository, source); !errors.Is(err, ErrRoadmapUnconfigured) || got.Text != "" {
		t.Fatal("project observation implicitly granted file-content access", err)
	}
}

func TestRoadmapIgnoresLocalObjectReplacement(t *testing.T) {
	directory, bindings, repository, source := roadmapFixture(t)
	blob := sourceRun(t, directory, "rev-parse", source.Commit+":docs/roadmap.md")
	file := filepath.Join(t.TempDir(), "replacement.txt")
	if err := os.WriteFile(file, []byte("replaced-private-canary"), 0600); err != nil {
		t.Fatal(err)
	}
	replacement := sourceRun(t, directory, "hash-object", "-w", file)
	sourceRun(t, directory, "replace", blob, replacement)
	got, err := bindings.ObserveRoadmap(t.Context(), "alice", repository, source)
	if err != nil || got.Blob != blob || !strings.Contains(got.Text, "First requirement") || strings.Contains(got.Text, "replaced-private-canary") {
		t.Fatal("local replacement ref changed the bytes attributed to the accepted source blob", err)
	}
}

func TestRoadmapRejectsNonTextOversizedAndNonregularBlobs(t *testing.T) {
	for _, value := range []string{"binary\x00private", string([]byte{255, 254}), strings.Repeat("x", (64<<10)+1)} {
		directory, bindings, repository, _ := roadmapFixture(t)
		if err := os.WriteFile(filepath.Join(directory, "docs", "roadmap.md"), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		sourceRun(t, directory, "add", "docs/roadmap.md")
		sourceRun(t, directory, "commit", "-m", "invalid roadmap")
		source, err := bindings.ObserveSource(t.Context(), "alice", repository)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := bindings.ObserveRoadmap(t.Context(), "alice", repository, source); !errors.Is(err, ErrSource) || got.Text != "" {
			t.Fatal("nontext/oversized source content escaped", err)
		}
	}
	directory, bindings, repository, source := roadmapFixture(t)
	outside := filepath.Join(t.TempDir(), "private-secret")
	if err := os.WriteFile(outside, []byte("outside-private-canary"), 0600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(directory, "docs", "roadmap.md")
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, file); err != nil {
		t.Skip("symlink creation unavailable on this worker")
	}
	if got, err := bindings.ObserveRoadmap(t.Context(), "alice", repository, source); err != nil || strings.Contains(got.Text, "outside-private-canary") || !got.WorkingCopyDirty {
		t.Fatal("worktree symlink replaced committed source", err)
	}
	sourceRun(t, directory, "config", "core.symlinks", "true")
	sourceRun(t, directory, "add", "docs/roadmap.md")
	sourceRun(t, directory, "commit", "-m", "symlink is not a roadmap")
	source, err := bindings.ObserveSource(t.Context(), "alice", repository)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := bindings.ObserveRoadmap(t.Context(), "alice", repository, source); !errors.Is(err, ErrSource) || got.Text != "" {
		t.Fatal("nonregular committed blob accepted as a roadmap", err)
	}
}

func TestRoadmapBindingRequiresExplicitSafePathAndCapability(t *testing.T) {
	for _, value := range []string{"../private", "/outside/roadmap.md", ".git/config", "docs/../private", "docs//roadmap.md", "docs/roadmap.md/", "C:/private", "docs/*", ":(glob)**", "docs\\roadmap.md", "docs/roadmap\n.md", ""} {
		document := bindingFixture()
		document.Bindings[0].RoadmapPath = value
		document.Bindings[0].Capabilities = []string{ObserveCapability, RoadmapObserveCapability}
		if parsed, err := ParseBindings(bindingBytes(t, document)); err == nil || parsed != nil {
			t.Fatal("unsafe or implicit roadmap path accepted", value)
		}
	}
	document := bindingFixture()
	document.Bindings[0].RoadmapPath = ".github/roadmap.md"
	if _, err := ParseBindings(bindingBytes(t, document)); err == nil {
		t.Fatal("roadmap path granted content access without capability")
	}
	document.Bindings[0].Capabilities = []string{RoadmapObserveCapability, ObserveCapability}
	first, err := ParseBindings(bindingBytes(t, document))
	if err != nil {
		t.Fatal("explicit GitHub roadmap rejected", err)
	}
	document.Bindings[0].Capabilities = []string{ObserveCapability, RoadmapObserveCapability}
	second, err := ParseBindings(bindingBytes(t, document))
	if err != nil {
		t.Fatal(err)
	}
	one, _ := first.Resolve("alice", repository())
	two, _ := second.Resolve("alice", repository())
	if one.Revision != two.Revision {
		t.Fatal("capability reordering changed source policy")
	}
	one.Project.RoadmapPath = "other.md"
	again, _ := first.Resolve("alice", repository())
	if again.Project.RoadmapPath != ".github/roadmap.md" {
		t.Fatal("resolved copy mutated the accepted source path")
	}
	data := bindingBytes(t, document)
	for _, bad := range []string{strings.Replace(string(data), `"roadmap_path":`, `"Roadmap_path":`, 1), strings.Replace(string(data), `"roadmap_path":".github/roadmap.md"`, `"roadmap_path":null`, 1)} {
		if _, err := ParseBindings([]byte(bad)); err == nil {
			t.Fatal("ambiguous source path policy accepted")
		}
	}
}

func TestNativeLocalRoadmapSnapshot(t *testing.T) {
	directory := os.Getenv("WEBMUX_MAC_ROADMAP_SOURCE_ROOT")
	if directory == "" {
		t.Skip("requires explicit local source roadmap qualification")
	}
	commit := os.Getenv("WEBMUX_MAC_ROADMAP_SOURCE_COMMIT")
	if commit == "" {
		t.Fatal("explicit expected source commit is required")
	}
	repository, err := coordination.InspectRepository(t.Context(), directory)
	if err != nil {
		t.Fatal("native local repository unavailable")
	}
	document := bindingFixture()
	document.Bindings[0].RepositoryID, document.Bindings[0].Repository = repository.ID, repository.Repository
	document.Bindings[0].RoadmapPath = "docs/roadmap.md"
	document.Bindings[0].Capabilities = []string{ObserveCapability, RoadmapObserveCapability}
	bindings, err := ParseBindings(bindingBytes(t, document))
	if err != nil {
		t.Fatal("native source policy unavailable")
	}
	source, err := bindings.ObserveSource(t.Context(), "alice", repository)
	if err != nil || source.Commit != commit {
		t.Fatal("native source changed before roadmap qualification")
	}
	roadmap, err := bindings.ObserveRoadmap(t.Context(), "alice", repository, source)
	if err != nil || roadmap.Commit != commit || roadmap.Blob == "" || !strings.Contains(roadmap.Text, "# WebMux active roadmap") || roadmap.TaskLinkState != "unlinked" {
		t.Fatal("native committed WebMux roadmap not observed with exact provenance")
	}
	if output := os.Getenv("WEBMUX_MAC_ROADMAP_SOURCE_RECEIPT"); output != "" {
		receipt := map[string]any{"schema": "webmux.mac.native-source-roadmap-qualification.v1", "source": source, "roadmap": roadmap,
			"scope": "Local native Git source component only; no MAC network, product enrollment or task linkage", "MAC_mutations": false, "NAS_access": false}
		data, err := json.MarshalIndent(receipt, "", "  ")
		if err != nil || os.WriteFile(output, append(data, '\n'), 0600) != nil {
			t.Fatal("native local source receipt unavailable")
		}
	}
	t.Log("Actual committed WebMux roadmap read with exact Git/blob/content provenance; no MAC admission or task mapping inferred")
}
