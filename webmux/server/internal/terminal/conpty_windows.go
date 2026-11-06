package terminal

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Use Microsoft's redistributable on every supported Windows version. The
// system ConPTY on Server 2022 consumes alternate-screen switches, so a VT
// viewer cannot preserve full-screen replay semantics using that backend.
//
//go:embed conpty/runtime.nupkg
var conptyPackage []byte

const conptyPackageSHA256 = "175640566a3b59c4b132070ee96c2c77e5ab7edd2e92732a5eb3610bbf63d90e"

type conptyAPI struct{ create, resize, close, release *windows.Proc }

var conptyOnce sync.Once
var conptyRuntime *conptyAPI
var conptyRuntimeErr error

func loadConPTY() (*conptyAPI, error) {
	conptyOnce.Do(func() { conptyRuntime, conptyRuntimeErr = extractConPTY() })
	return conptyRuntime, conptyRuntimeErr
}

func extractConPTY() (*conptyAPI, error) {
	if fmt.Sprintf("%x", sha256.Sum256(conptyPackage)) != conptyPackageSHA256 {
		return nil, fmt.Errorf("ConPTY package integrity mismatch")
	}
	arch := map[string]string{"amd64": "x64", "arm64": "arm64", "386": "x86"}[runtime.GOARCH]
	if arch == "" {
		return nil, fmt.Errorf("unsupported ConPTY architecture: %s", runtime.GOARCH)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(cache, "webmux", "conpty", conptyPackageSHA256, arch)
	archive, err := zip.NewReader(bytes.NewReader(conptyPackage), int64(len(conptyPackage)))
	if err != nil {
		return nil, err
	}
	files := map[string]string{"runtimes/win-" + arch + "/native/conpty.dll": "conpty.dll"}
	// Match the package's MSBuild deployment rules: x64 programs can run
	// under ARM64 emulation, but ConPTY still starts a native console host.
	hosts := map[string][]string{"x86": {"x86", "x64", "arm64"}, "x64": {"x64", "arm64"}, "arm64": {"arm64"}}[arch]
	for _, host := range hosts {
		files["build/native/runtimes/"+host+"/OpenConsole.exe"] = filepath.Join(host, "OpenConsole.exe")
	}
	for source, destination := range files {
		entry, err := archive.Open(source)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(entry, 32*1024*1024))
		entry.Close()
		if err != nil {
			return nil, err
		}
		target := filepath.Join(directory, destination)
		existing, readErr := os.ReadFile(target)
		if readErr == nil && bytes.Equal(existing, data) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return nil, err
		}
		file, err := os.CreateTemp(filepath.Dir(target), ".conpty-*")
		if err != nil {
			return nil, err
		}
		temporary := file.Name()
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if writeErr == nil {
			writeErr = closeErr
		}
		if writeErr == nil {
			writeErr = os.Rename(temporary, target)
		}
		os.Remove(temporary)
		if writeErr != nil {
			// Another WebMux process may have installed and loaded this exact runtime.
			installed, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(installed, data) {
				return nil, writeErr
			}
		}
	}
	name := filepath.Join(directory, "conpty.dll")
	handle, err := windows.LoadLibraryEx(name, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return nil, fmt.Errorf("load ConPTY runtime: %w", err)
	}
	dll := &windows.DLL{Name: name, Handle: handle}
	api := &conptyAPI{}
	for name, destination := range map[string]**windows.Proc{"ConptyCreatePseudoConsole": &api.create, "ConptyResizePseudoConsole": &api.resize, "ConptyClosePseudoConsole": &api.close, "ConptyReleasePseudoConsole": &api.release} {
		*destination, err = dll.FindProc(name)
		if err != nil {
			dll.Release()
			return nil, err
		}
	}
	return api, nil
}

func (api *conptyAPI) createConsole(size windows.Coord, input, output windows.Handle, console *windows.Handle) error {
	result, _, _ := api.create.Call(uintptr(*(*uint32)(unsafe.Pointer(&size))), uintptr(input), uintptr(output), 0, uintptr(unsafe.Pointer(console)))
	if uint32(result) != 0 {
		return fmt.Errorf("ConptyCreatePseudoConsole HRESULT %#x", uint32(result))
	}
	return nil
}
func (api *conptyAPI) resizeConsole(console windows.Handle, size windows.Coord) error {
	result, _, _ := api.resize.Call(uintptr(console), uintptr(*(*uint32)(unsafe.Pointer(&size))))
	if uint32(result) != 0 {
		return fmt.Errorf("ConptyResizePseudoConsole HRESULT %#x", uint32(result))
	}
	return nil
}
func (api *conptyAPI) closeConsole(console windows.Handle) { api.close.Call(uintptr(console)) }

// After the child has attached, release the host's keep-alive reference. The
// HPCON remains valid for resize/close, while child exit can finish output.
func (api *conptyAPI) releaseConsole(console windows.Handle) error {
	result, _, _ := api.release.Call(uintptr(console))
	if uint32(result) != 0 {
		return fmt.Errorf("ConptyReleasePseudoConsole HRESULT %#x", uint32(result))
	}
	return nil
}
