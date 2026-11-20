//go:build windows

package recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	codexHandleLimit  = 2048
	codexProcessLimit = 32
	codexPathLimit    = 32
)

var errCodexObservation = errors.New("Codex process file observation unavailable")
var queryCodexFileType = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetFileType")
var queryCodexProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryInformationProcess")

type codexProcessIdentity struct {
	PID     uint32 `json:"pid"`
	Created uint64 `json:"created"`
}

type codexFileResult struct {
	Version  int                  `json:"version"`
	Identity codexProcessIdentity `json:"identity"`
	Files    []string             `json:"files"`
}

type codexProcessPin struct {
	handle   windows.Handle
	identity codexProcessIdentity
}

func processCreated(h windows.Handle) (uint64, error) {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return 0, err
	}
	return uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime), nil
}

func processAlive(h windows.Handle) bool {
	result, err := windows.WaitForSingleObject(h, 0)
	return err == nil && result == uint32(windows.WAIT_TIMEOUT)
}

func (p *codexProcessPin) Close() error { return windows.CloseHandle(p.handle) }

func pinCodexProcess(pid int) (*codexProcessPin, error) {
	if pid <= 0 || uint64(pid) > uint64(^uint32(0)) {
		return nil, errCodexObservation
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return nil, err
	}
	created, err := processCreated(h)
	if err != nil || created == 0 || !processAlive(h) {
		windows.CloseHandle(h)
		return nil, errCodexObservation
	}
	return &codexProcessPin{h, codexProcessIdentity{uint32(pid), created}}, nil
}

func newCodexProcessObserver(pid int) (func(string) (string, string), io.Closer) {
	p, err := pinCodexProcess(pid)
	if err != nil {
		return func(string) (string, string) { return "", "" }, nil
	}
	return func(cwd string) (string, string) { return codexSessionFromFiles(p.files(), cwd) }, p
}

func (p *codexProcessPin) files() []string {
	if !processAlive(p.handle) {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return p.filesCommand(ctx, executable, CodexFilesHelper)
}

func (p *codexProcessPin) filesCommand(ctx context.Context, executable string, args ...string) []string {
	if !processAlive(p.handle) {
		return nil
	}
	request, err := json.Marshal(p.identity)
	if err != nil {
		return nil
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	cmd.Stdin = bytes.NewReader(request)
	var output boundedCLIOutput
	cmd.Stdout = &output
	// No inherited terminal handles, target environment changes, or file data
	// access are needed by this worker. CommandContext kills only this worker.
	if err := cmd.Run(); err != nil || output.exceeded || !processAlive(p.handle) {
		return nil
	}
	var result codexFileResult
	decoder := json.NewDecoder(bytes.NewReader(output.buffer.Bytes()))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || result.Version != 1 || result.Identity != p.identity || len(result.Files) > codexPathLimit {
		return nil
	}
	return result.Files
}

// RunCodexFilesHelper reads only process/file metadata. Its caller must enforce
// the deadline: GetFinalPathNameByHandle can block on a filesystem provider.
func RunCodexFilesHelper() error {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return errCodexObservation
	}
	var request codexProcessIdentity
	decoder := json.NewDecoder(io.LimitReader(os.Stdin, 1025))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || request.PID == 0 || request.Created == 0 {
		return errCodexObservation
	}
	files, err := windowsCodexFiles(request)
	if err != nil {
		return err
	}
	data, err := json.Marshal(codexFileResult{1, request, files})
	if err != nil || len(data) > 64<<10 {
		return errCodexObservation
	}
	_, err = os.Stdout.Write(data)
	return err
}

// ProcessHandleInformation is a native, version-dependent interface. Only the
// 64-bit layout is accepted; unknown statuses, sizes, partial scans and races
// fail closed. No system-wide handle table or debug privilege is requested.
type codexHandleEntry struct {
	Value, HandleCount, PointerCount        uintptr
	Access, TypeIndex, Attributes, Reserved uint32
}

func codexHandles(h windows.Handle) ([]codexHandleEntry, error) {
	if unsafe.Sizeof(uintptr(0)) != 8 || unsafe.Sizeof(codexHandleEntry{}) != 40 {
		return nil, errCodexObservation
	}
	size := uint32(4096)
	for attempt := 0; attempt < 3; attempt++ {
		data := make([]byte, size)
		var needed uint32
		status, _, _ := queryCodexProcess.Call(uintptr(h), 51, uintptr(unsafe.Pointer(&data[0])), uintptr(size), uintptr(unsafe.Pointer(&needed)))
		if uint32(status) == 0xc0000004 {
			if needed <= size || needed > 16+codexHandleLimit*40 {
				return nil, errCodexObservation
			}
			size = needed
			continue
		}
		if uint32(status) != 0 || needed < 16 || needed > size {
			return nil, errCodexObservation
		}
		count := *(*uintptr)(unsafe.Pointer(&data[0]))
		if count > codexHandleLimit || 16+count*40 > uintptr(needed) {
			return nil, errCodexObservation
		}
		rows := make([]codexHandleEntry, int(count))
		for i := range rows {
			rows[i] = *(*codexHandleEntry)(unsafe.Pointer(&data[16+i*40]))
		}
		runtime.KeepAlive(data)
		return rows, nil
	}
	return nil, errCodexObservation
}

func codexProcessUser(h windows.Handle) (string, error) {
	var token windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token); err != nil {
		return "", err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

func codexFileObjectType() (uint32, error) {
	executable, err := os.Executable()
	if err != nil {
		return 0, err
	}
	file, err := os.Open(executable)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	rows, err := codexHandles(windows.CurrentProcess())
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		if row.Value == file.Fd() && row.TypeIndex != 0 {
			return row.TypeIndex, nil
		}
	}
	return 0, errCodexObservation
}

func codexSessionsRoot() (string, error) {
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = filepath.Join(userHome, ".codex")
	}
	return filepath.EvalSymlinks(filepath.Join(home, "sessions"))
}

func ordinaryWindowsPath(path string) string {
	if strings.HasPrefix(path, `\\?\UNC\`) {
		return `\\` + path[8:]
	}
	if strings.HasPrefix(path, `\\?\`) && len(path) >= 7 && path[5] == ':' && path[6] == '\\' {
		return path[4:]
	}
	return path
}

func codexPathWithin(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel) && strings.HasSuffix(path, ".jsonl")
}

func duplicateCodexMetadataHandle(process windows.Handle, row codexHandleEntry) (windows.Handle, error) {
	var duplicate windows.Handle
	// Restrict the duplicate to metadata, even if Codex holds read/write data
	// access. DUPLICATE_SAME_ACCESS would expose credential or document contents.
	if err := windows.DuplicateHandle(process, windows.Handle(row.Value), windows.CurrentProcess(), &duplicate, row.Access&windows.FILE_READ_ATTRIBUTES, false, 0); err != nil {
		return 0, err
	}
	return duplicate, nil
}

func codexHandlePath(process windows.Handle, row codexHandleEntry) (string, error) {
	duplicate, err := duplicateCodexMetadataHandle(process, row)
	if err != nil {
		return "", fmt.Errorf("duplicate file metadata handle: %w", err)
	}
	defer windows.CloseHandle(duplicate)
	kind, err := codexFileType(duplicate)
	if err != nil {
		return "", fmt.Errorf("query file handle kind: %w", err)
	}
	if kind != windows.FILE_TYPE_DISK {
		return "", nil
	}
	buffer := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(duplicate, &buffer[0], uint32(len(buffer)), 0)
	if err != nil || n == 0 || n >= uint32(len(buffer)) {
		return "", fmt.Errorf("query disk handle name: %v: %w", err, errCodexObservation)
	}
	return ordinaryWindowsPath(windows.UTF16ToString(buffer[:n])), nil
}

func windowsCodexFiles(root codexProcessIdentity) ([]string, error) {
	sessions, err := codexSessionsRoot()
	if err != nil {
		return nil, err
	}
	fileType, err := codexFileObjectType()
	if err != nil {
		return nil, err
	}
	user, err := codexProcessUser(windows.CurrentProcess())
	if err != nil {
		return nil, err
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)
	children := map[uint32][]uint32{}
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	err = windows.Process32First(snapshot, &entry)
	for count := 0; err == nil; count++ {
		if count >= 32768 {
			return nil, errCodexObservation
		}
		children[entry.ParentProcessID] = append(children[entry.ParentProcessID], entry.ProcessID)
		err = windows.Process32Next(snapshot, &entry)
	}
	if err != windows.ERROR_NO_MORE_FILES {
		return nil, fmt.Errorf("enumerate processes: %w", err)
	}
	// Parent handles stay pinned until the entire scan finishes. Check creation
	// order as well, since Toolhelp's parent PID can refer to an exited process.
	type ownedProcess struct {
		pid           uint32
		parentPID     uint32
		parentCreated uint64
	}
	pending := []ownedProcess{{root.PID, 0, root.Created}}
	seen := map[uint32]bool{}
	var handles []windows.Handle
	defer func() {
		for _, h := range handles {
			windows.CloseHandle(h)
		}
	}()
	files := map[string]bool{}
	for i := 0; i < len(pending); i++ {
		if len(pending) > codexProcessLimit {
			return nil, errCodexObservation
		}
		selected := pending[i]
		if seen[selected.pid] {
			return nil, errCodexObservation
		}
		seen[selected.pid] = true
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_DUP_HANDLE|windows.SYNCHRONIZE, false, selected.pid)
		if err != nil {
			return nil, fmt.Errorf("open owned process: %w", err)
		}
		handles = append(handles, h)
		created, err := processCreated(h)
		if err != nil || !processAlive(h) || created < selected.parentCreated || (i == 0 && created != root.Created) {
			return nil, errCodexObservation
		}
		// Toolhelp is a snapshot: a descendant PID could have been reused
		// before OpenProcess. Verify its current parent against the pinned,
		// still-live ancestor as well as checking creation order.
		var basic windows.PROCESS_BASIC_INFORMATION
		var size uint32
		err = windows.NtQueryInformationProcess(h, windows.ProcessBasicInformation, unsafe.Pointer(&basic), uint32(unsafe.Sizeof(basic)), &size)
		if err != nil || size != uint32(unsafe.Sizeof(basic)) || basic.UniqueProcessId != uintptr(selected.pid) || (i != 0 && basic.InheritedFromUniqueProcessId != uintptr(selected.parentPID)) {
			return nil, errCodexObservation
		}
		owner, err := codexProcessUser(h)
		if err != nil {
			return nil, err
		}
		if owner != user {
			if i == 0 {
				return nil, errCodexObservation
			}
			// Sandboxed tool accounts do not own the signed-in CLI's journal. Never
			// duplicate their handles or infer a conversation from those accounts.
			continue
		}
		for _, child := range children[selected.pid] {
			pending = append(pending, ownedProcess{child, selected.pid, created})
		}
		rows, err := codexHandles(h)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.TypeIndex != fileType {
				continue
			}
			path, err := codexHandlePath(h, row)
			if err != nil {
				return nil, err
			}
			if path != "" && codexPathWithin(path, sessions) {
				files[path] = true
				if len(files) > codexPathLimit {
					return nil, errCodexObservation
				}
			}
		}
	}
	for _, h := range handles {
		if !processAlive(h) {
			return nil, errCodexObservation
		}
	}
	result := make([]string, 0, len(files))
	for path := range files {
		result = append(result, path)
	}
	return result, nil
}

// x/sys's GetFileType wrapper maps a zero return and NO_ERROR to EINVAL.
// Windows explicitly defines this as a valid FILE_TYPE_UNKNOWN (for example,
// crypto-device handles). Preserve the documented last-error distinction;
// only disk handles proceed to a filesystem path query.
// https://learn.microsoft.com/windows/win32/api/fileapi/nf-fileapi-getfiletype
func codexFileType(handle windows.Handle) (uint32, error) {
	kind, _, err := queryCodexFileType.Call(uintptr(handle))
	if kind == 0 && err != syscall.Errno(0) {
		return 0, err
	}
	switch uint32(kind) {
	case windows.FILE_TYPE_UNKNOWN, windows.FILE_TYPE_DISK, windows.FILE_TYPE_CHAR, windows.FILE_TYPE_PIPE:
		return uint32(kind), nil
	default:
		return 0, errCodexObservation
	}
}
