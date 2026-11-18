//go:build windows

package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const cleanupHandleHelper = "WEBMUX_PRIVATE_CLEANUP_HANDLE_HELPER"
const cleanupHandleLimit = 8192

type cleanupHandleInput struct {
	Owned map[uint32]uint64 `json:"owned"`
	Limit int               `json:"limit"`
}

// PSS_HANDLE_ENTRY from processsnapshot.h. Basic contains the six DWORD
// attributes/access/reference-count/pool-charge fields. Names are not captured.
type cleanupHandleEntry struct {
	Handle            windows.Handle
	Flags, ObjectType uint32
	CaptureTime       windows.Filetime
	Basic             [6]uint32
	CreationTime      windows.Filetime
	TypeNameLength    uint16
	TypeName          uintptr
	ObjectNameLength  uint16
	ObjectName        uintptr
	Specific          [48]byte
}

type cleanupHandleProcess struct{ handle windows.Handle }

// FILE_ID_INFO uses the full 128-bit identifier, including on ReFS.
type cleanupFileIdentity struct {
	Volume uint64
	ID     [16]byte
}

func cleanupIdentity(file windows.Handle) (cleanupFileIdentity, error) {
	var identity cleanupFileIdentity
	err := windows.GetFileInformationByHandleEx(file, windows.FileIdInfo, (*byte)(unsafe.Pointer(&identity)), uint32(unsafe.Sizeof(identity)))
	return identity, err
}

// Start before allocator shutdown, not after the sharing error. The helper
// holds only validated process handles; it never keeps candidate file handles
// across calls, closes a source handle, terminates Chrome, or retries removal.
// Stdin carries private identities and the one failed path; stdout is counts.
func prepareFirstNavigationHandleProbe(owned map[uint32]uint64) func(error, string) *firstNavigationHandleReport {
	return startFirstNavigationHandleProbe(owned, cleanupHandleLimit)
}

func startFirstNavigationHandleProbe(owned map[uint32]uint64, limit int) func(error, string) *firstNavigationHandleReport {
	failed := func(state string) func(error, string) *firstNavigationHandleReport {
		return func(err error, profile string) *firstNavigationHandleReport {
			if !cleanupHandleEligible(err, profile) {
				return nil
			}
			return &firstNavigationHandleReport{State: state}
		}
	}
	if len(owned) == 0 || len(owned) > 64 || limit < 1 || limit > cleanupHandleLimit {
		return failed("identity_scope_unavailable")
	}
	executable, err := os.Executable()
	if err != nil {
		return failed("helper_unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestFirstNavigationHandleSnapshotHelper$")
	cmd.Env = append(os.Environ(), cleanupHandleHelper+"=1")
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return failed("helper_pipe_failed")
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		cancel()
		return failed("helper_pipe_failed")
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		input.Close()
		cancel()
		return failed("helper_start_failed")
	}
	stop := func() { input.Close(); cancel(); _ = cmd.Wait() }
	encoder := json.NewEncoder(input)
	decoder := json.NewDecoder(io.LimitReader(output, 4096))
	var ready struct {
		Ready bool `json:"ready"`
	}
	if encoder.Encode(cleanupHandleInput{Owned: owned, Limit: limit}) != nil || decoder.Decode(&ready) != nil || !ready.Ready {
		stop()
		return failed("helper_ready_failed")
	}
	var once sync.Once
	var report *firstNavigationHandleReport
	return func(removal error, profile string) *firstNavigationHandleReport {
		once.Do(func() {
			defer stop()
			var path string
			needed := cleanupHandleEligible(removal, profile)
			if needed {
				var pe *os.PathError
				errors.As(removal, &pe)
				path = pe.Path
			}
			if err := encoder.Encode(path); err != nil {
				if needed {
					report = &firstNavigationHandleReport{State: "helper_send_failed"}
				}
				return
			}
			var result firstNavigationHandleReport
			err := decoder.Decode(&result)
			if !needed {
				return
			}
			if err != nil {
				report = &firstNavigationHandleReport{State: "helper_result_failed"}
				if ctx.Err() != nil {
					report.State = "helper_deadline"
				}
				return
			}
			report = &result
		})
		return report
	}
}

func cleanupHandleEligible(removal error, profile string) bool {
	var pe *os.PathError
	var code syscall.Errno
	if !errors.As(removal, &pe) || !errors.As(removal, &code) || (code != 32 && code != 33) {
		return false
	}
	rel, err := filepath.Rel(profile, pe.Path)
	if err != nil || !filepath.IsLocal(rel) || rel == "." {
		return false
	}
	return true
}

// This test-only subprocess has a parent-enforced hard deadline. PSS captures
// handle values only, without names, threads, process memory or a VA clone.
// Each duplicated disk handle is compared by volume/file identity, not by path.
func TestFirstNavigationHandleSnapshotHelper(t *testing.T) {
	if os.Getenv(cleanupHandleHelper) != "1" {
		t.Skip("private bounded cleanup helper")
	}
	decoder := json.NewDecoder(io.LimitReader(os.Stdin, 32768))
	var input cleanupHandleInput
	if decoder.Decode(&input) != nil || len(input.Owned) > 64 || input.Limit < 1 || input.Limit > cleanupHandleLimit {
		os.Exit(2)
	}
	result := &firstNavigationHandleReport{State: "queried"}
	processes := []cleanupHandleProcess{}
	for pid, birth := range input.Owned {
		process, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ|windows.PROCESS_DUP_HANDLE, false, pid)
		if err != nil {
			result.ProcessesUnavailable++
			continue
		}
		var created, exited, kernel, user windows.Filetime
		err = windows.GetProcessTimes(process, &created, &exited, &kernel, &user)
		if err != nil || birth == 0 || birth != uint64(created.HighDateTime)<<32|uint64(created.LowDateTime) {
			windows.CloseHandle(process)
			result.IdentityRejected++
			continue
		}
		processes = append(processes, cleanupHandleProcess{process})
		result.ProcessesVerified++
	}
	if json.NewEncoder(os.Stdout).Encode(struct {
		Ready bool `json:"ready"`
	}{true}) != nil {
		os.Exit(2)
	}
	var path string
	if decoder.Decode(&path) != nil {
		os.Exit(2)
	}
	if path != "" {
		snapshotCleanupHandles(path, processes, input.Limit, result)
	}
	for _, p := range processes {
		windows.CloseHandle(p.handle)
	}
	if json.NewEncoder(os.Stdout).Encode(result) != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func snapshotCleanupHandles(path string, processes []cleanupHandleProcess, limit int, r *firstNavigationHandleReport) {
	if unsafe.Sizeof(cleanupHandleEntry{}) != 136 || unsafe.Offsetof(cleanupHandleEntry{}.Specific) != 88 {
		r.State = "abi_unavailable"
		return
	}
	info, err := os.Lstat(path)
	if err != nil {
		r.State = "file_unavailable"
		return
	}
	if !info.Mode().IsRegular() {
		r.State = "not_regular_file"
		return
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		r.State = "file_unavailable"
		return
	}
	file, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		r.State = "file_unavailable"
		return
	}
	defer windows.CloseHandle(file)
	var expected windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(file, &expected) != nil || expected.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		r.State = "file_identity_unavailable"
		return
	}
	expectedID, err := cleanupIdentity(file)
	if err != nil || expectedID.ID == [16]byte{} {
		r.State = "file_identity_unavailable"
		return
	}
	library := windows.NewLazySystemDLL("kernel32.dll")
	capture, free, createMarker, freeMarker, walk := library.NewProc("PssCaptureSnapshot"), library.NewProc("PssFreeSnapshot"), library.NewProc("PssWalkMarkerCreate"), library.NewProc("PssWalkMarkerFree"), library.NewProc("PssWalkSnapshot")
	for _, proc := range []*windows.LazyProc{capture, free, createMarker, freeMarker, walk} {
		if proc.Find() != nil {
			r.State = "api_unavailable"
			return
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for _, process := range processes {
		status, err := windows.WaitForSingleObject(process.handle, 0)
		if err == nil && status == windows.WAIT_OBJECT_0 {
			r.ExitedProcesses++
			continue
		}
		if err != nil {
			r.SnapshotUnavailable++
			continue
		}
		if time.Now().After(deadline) {
			r.State = "query_deadline"
			return
		}
		func() {
			var snapshot, marker windows.Handle
			status, _, _ := capture.Call(uintptr(process.handle), 4, 0, uintptr(unsafe.Pointer(&snapshot))) // PSS_CAPTURE_HANDLES only
			if status != 0 {
				r.SnapshotUnavailable++
				r.OSCode = uint32(status)
				return
			}
			defer free.Call(uintptr(windows.CurrentProcess()), uintptr(snapshot))
			status, _, _ = createMarker.Call(0, uintptr(unsafe.Pointer(&marker)))
			if status != 0 {
				r.SnapshotUnavailable++
				r.OSCode = uint32(status)
				return
			}
			defer freeMarker.Call(uintptr(marker))
			r.ProcessesCaptured++
			matched := false
			for {
				if r.HandlesExamined >= limit {
					r.State = "handle_limit"
					return
				}
				if time.Now().After(deadline) {
					r.State = "query_deadline"
					return
				}
				var entry cleanupHandleEntry
				status, _, _ = walk.Call(uintptr(snapshot), 2, uintptr(marker), uintptr(unsafe.Pointer(&entry)), unsafe.Sizeof(entry)) // PSS_WALK_HANDLES
				if status == 259 {
					return
				} // ERROR_NO_MORE_ITEMS
				if status != 0 {
					r.SnapshotUnavailable++
					return
				}
				r.HandlesExamined++
				var copy windows.Handle
				if windows.DuplicateHandle(process.handle, entry.Handle, windows.CurrentProcess(), &copy, 0, false, windows.DUPLICATE_SAME_ACCESS) != nil {
					r.DuplicateFailed++
					continue
				}
				func() {
					defer windows.CloseHandle(copy)
					kind, err := windows.GetFileType(copy)
					if err != nil || kind != windows.FILE_TYPE_DISK {
						return
					}
					actual, err := cleanupIdentity(copy)
					if err != nil {
						r.FilesUnqueryable++
						return
					}
					if actual == expectedID {
						r.MatchedHandles++
						if !matched {
							r.MatchedProcesses++
							matched = true
						}
					}
				}()
			}
		}()
		if r.State != "queried" {
			return
		}
	}
}

func TestFirstNavigationHandleSnapshotOwnedFileAndIdentityRefusal(t *testing.T) {
	profile := t.TempDir()
	name := filepath.Join(profile, "owned-private-file-token")
	if err := os.WriteFile(name, []byte("synthetic fixture only"), 0600); err != nil {
		t.Fatal("native handle control setup/cleanup failed")
	}
	path, err := windows.UTF16PtrFromString(name)
	if err != nil {
		t.Fatal("native handle control setup/cleanup failed")
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal("native handle control setup/cleanup failed")
	}
	defer func() {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
	}()
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernel, &user); err != nil {
		t.Fatal("native handle control setup/cleanup failed")
	}
	birth := uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	owned := map[uint32]uint64{uint32(os.Getpid()): birth}
	probe := startFirstNavigationHandleProbe(owned, cleanupHandleLimit)
	wrong := startFirstNavigationHandleProbe(map[uint32]uint64{uint32(os.Getpid()): birth + 1}, cleanupHandleLimit)
	bounded := startFirstNavigationHandleProbe(owned, 1)
	nonmatching := startFirstNavigationHandleProbe(owned, cleanupHandleLimit)
	defer probe(nil, profile)
	defer wrong(nil, profile)
	defer bounded(nil, profile)
	defer nonmatching(nil, profile)
	removal := os.RemoveAll(profile)
	if removal == nil {
		t.Fatal("controlled handle did not refuse removal")
	}
	report := probe(removal, profile)
	if report == nil || report.State != "queried" || report.ProcessesVerified != 1 || report.ProcessesCaptured != 1 || report.MatchedProcesses != 1 || report.MatchedHandles < 1 {
		t.Fatalf("owned failed-file identity not matched: %+v", report)
	}
	refused := wrong(removal, profile)
	if refused == nil || refused.IdentityRejected != 1 || refused.ProcessesVerified != 0 || refused.ProcessesCaptured != 0 || refused.MatchedHandles != 0 {
		t.Fatalf("PID creation-time mismatch admitted: %+v", refused)
	}
	limit := bounded(removal, profile)
	if limit == nil || limit.State != "handle_limit" || limit.HandlesExamined != 1 {
		t.Fatalf("handle budget not enforced: %+v", limit)
	}
	other := filepath.Join(profile, "different-private-file")
	if err := os.WriteFile(other, []byte("unheld synthetic file"), 0600); err != nil {
		t.Fatal("native handle control setup/cleanup failed")
	}
	unmatched := nonmatching(&os.PathError{Op: "remove", Path: other, Err: syscall.Errno(32)}, profile)
	if unmatched == nil || unmatched.State != "queried" || unmatched.ProcessesCaptured != 1 || unmatched.MatchedHandles != 0 {
		t.Fatalf("a different file identity was counted: %+v", unmatched)
	}
	outside := &os.PathError{Op: "remove", Path: filepath.Join(filepath.Dir(profile), "outside-private"), Err: syscall.Errno(32)}
	if cleanupHandleEligible(outside, profile) || cleanupHandleEligible(nil, profile) || cleanupHandleEligible(&os.PathError{Path: profile, Err: syscall.Errno(32)}, profile) {
		t.Fatal("unsafe removal scope admitted")
	}
	encoded, err := json.Marshal(report)
	if err != nil || len(encoded) > 1024 || bytes.Contains(encoded, []byte("private")) || bytes.Contains(encoded, []byte(profile)) || bytes.Contains(encoded, []byte(name)) {
		t.Fatal("native handle projection leaked private data")
	}
	if os.RemoveAll(profile) == nil {
		t.Fatal("probe closed the original native handle")
	}
	if err := windows.CloseHandle(handle); err != nil {
		t.Fatal("native handle control setup/cleanup failed")
	}
	handle = 0
	if err := os.RemoveAll(profile); err != nil {
		t.Fatal("fixture removal failed after releasing its own handle")
	}
}
