package browser

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

type firstNavigationRemovalError struct {
	Operation    string `json:"operation"`
	Class        string `json:"class"`
	OSCode       uint64 `json:"osCode,omitempty"`
	FileCategory string `json:"fileCategory"`
}

type firstNavigationCleanupProcesses struct {
	State            string `json:"state"`
	ParentState      string `json:"parentState"`
	ReportedChildren int    `json:"reportedChildren"`
	ObservedChildren int    `json:"observedChildren"`
	ExitedChildren   int    `json:"exitedChildren"`
	RunningChildren  int    `json:"runningChildren"`
	Unqueryable      int    `json:"unqueryable"`
}

// Only fixed categories and OS error numbers leave the anonymous fixture.
// PathError.Error(), raw paths and arbitrary error strings are never retained.
func classifyFirstNavigationRemoval(err error, profile string) *firstNavigationRemovalError {
	if err == nil {
		return nil
	}
	r := &firstNavigationRemovalError{Operation: "other", Class: "other", FileCategory: "unknown"}
	var pathError *os.PathError
	if errors.As(err, &pathError) {
		switch pathError.Op {
		case "remove", "RemoveAll", "unlinkat", "rmdir", "open", "openfdat", "readdir", "lstat", "stat":
			r.Operation = pathError.Op
		}
		rel, relErr := filepath.Rel(profile, pathError.Path)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			r.FileCategory = "outside_profile"
		} else if rel == "." {
			r.FileCategory = "profile_root"
		} else {
			name := strings.ToLower(filepath.Base(rel))
			switch name {
			case "cookies", "cookies-journal":
				r.FileCategory = "cookie_store"
			case "history", "history-journal":
				r.FileCategory = "history_store"
			case "local state", "devtoolsactiveport", "singletoncookie", "singletonlock", "singletonsocket":
				r.FileCategory = "browser_control"
			default:
				if strings.Contains(strings.ToLower(filepath.ToSlash(rel)), "cache") {
					r.FileCategory = "cache"
				} else {
					r.FileCategory = "profile_child"
				}
			}
		}
	}
	var code syscall.Errno
	if errors.As(err, &code) {
		r.OSCode = uint64(code)
		r.Class = "os_error"
	}
	if errors.Is(err, fs.ErrPermission) {
		r.Class = "permission"
	}
	if errors.Is(err, fs.ErrNotExist) {
		r.Class = "not_found"
	}
	if runtime.GOOS == "windows" {
		switch r.OSCode {
		case 32, 33:
			r.Class = "sharing_violation"
		case 145:
			r.Class = "directory_not_empty"
		}
	}
	return r
}

func TestFirstNavigationCleanupProjectionDoesNotRetainPrivatePathsOrErrors(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-home-cookie-token")
	for _, item := range []struct{ rel, category string }{
		{".", "profile_root"}, {"Default/Cookies", "cookie_store"}, {"Default/History-journal", "history_store"},
		{"Local State", "browser_control"}, {"Default/Cache/opaque-private-file", "cache"},
		{"Default/private-cookie-token", "profile_child"}, {"../outside-private-cookie-token", "outside_profile"},
	} {
		err := &os.PathError{Op: "remove", Path: filepath.Join(root, filepath.FromSlash(item.rel)), Err: fs.ErrPermission}
		r := classifyFirstNavigationRemoval(err, root)
		if r.FileCategory != item.category || r.Class != "permission" || r.Operation != "remove" {
			t.Fatalf("wrong safe classification: %+v", r)
		}
		encoded, marshalErr := json.Marshal(r)
		if marshalErr != nil || len(encoded) > 256 || strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), root) {
			t.Fatal("cleanup projection leaked a private value")
		}
	}
	if got := classifyFirstNavigationRemoval(errors.New("private-url?token=not-exported"), root); got.Class != "other" || got.FileCategory != "unknown" {
		t.Fatal(got)
	}
	if classifyFirstNavigationRemoval(nil, root) != nil {
		t.Fatal("successful removal reported an error")
	}
	unsafe := &os.PathError{Op: "private-url?token=secret", Path: filepath.Join(root, "secret"), Err: errors.New("private-message")}
	encoded, err := json.Marshal(classifyFirstNavigationRemoval(unsafe, root))
	if err != nil || strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "secret") {
		t.Fatal("arbitrary operation or error was exported")
	}
}
