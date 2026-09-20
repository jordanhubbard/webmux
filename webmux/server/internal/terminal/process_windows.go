package terminal

import (
	"errors"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsProcess struct {
	input, output *os.File
	process       *os.Process
	console       windows.Handle
	consoleMu     sync.Mutex
}

func startNative(spec Command) (_ nativeProcess, resultErr error) {
	// Resolve before changing directories, as node-pty does for ConPTY.
	path, err := exec.LookPath(spec.Path)
	if err != nil {
		return nil, err
	}
	application, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	line := windows.ComposeCommandLine(append([]string{path}, spec.Args...))
	if spec.ShellCommand != "" {
		// cmd.exe does not understand the backslash escaping used by the CRT.
		line = windows.EscapeArg(path) + ` /d /s /c "` + spec.ShellCommand + `"`
	}
	commandLine, err := windows.UTF16PtrFromString(line)
	if err != nil {
		return nil, err
	}
	var directory *uint16
	if spec.Dir != "" {
		directory, err = windows.UTF16PtrFromString(spec.Dir)
		if err != nil {
			return nil, err
		}
	}
	environment := environmentBlock(spec.Env)
	inputRead, inputWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer inputRead.Close()
	outputRead, outputWrite, err := os.Pipe()
	if err != nil {
		inputWrite.Close()
		return nil, err
	}
	defer outputWrite.Close()
	p := &windowsProcess{input: inputWrite, output: outputRead}
	defer func() {
		if resultErr != nil {
			if p.process != nil {
				_ = p.process.Kill()
				_, _ = p.process.Wait()
			}
			_ = p.Close()
		}
	}()
	if err := windows.CreatePseudoConsole(windows.Coord{X: int16(spec.Cols), Y: int16(spec.Rows)}, windows.Handle(inputRead.Fd()), windows.Handle(outputWrite.Fd()), 0, &p.console); err != nil {
		return nil, err
	}
	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, err
	}
	defer attributes.Delete()
	// Unlike most attributes, this API takes the opaque HPCON value, not &HPCON.
	// HPCON is an opaque native pointer represented as uintptr by x/sys. Copy
	// its pointer representation without integer-to-Go-pointer arithmetic.
	consolePointer := *(*unsafe.Pointer)(unsafe.Pointer(&p.console))
	if err := attributes.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, consolePointer, unsafe.Sizeof(p.console)); err != nil {
		return nil, err
	}
	startup := windows.StartupInfoEx{}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.ProcThreadAttributeList = attributes.List()
	var info windows.ProcessInformation
	err = windows.CreateProcess(application, commandLine, nil, nil, false,
		windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT,
		&environment[0], directory, &startup.StartupInfo, &info)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(info.Thread)
	defer windows.CloseHandle(info.Process)
	p.process, err = os.FindProcess(int(info.ProcessId))
	if err != nil {
		_ = windows.TerminateProcess(info.Process, 1)
		return nil, err
	}
	return p, nil
}

// Windows environment blocks are sorted, case insensitive, and double-NUL
// terminated. Preserve special =C:=... drive-directory entries from os.Environ.
func environmentBlock(environment []string) []uint16 {
	if environment == nil {
		environment = os.Environ()
	}
	values := map[string]string{}
	for _, item := range environment {
		index := strings.IndexByte(item, '=')
		if index == 0 {
			if end := strings.IndexByte(item[1:], '='); end >= 0 {
				index = end + 1
			}
		}
		if index > 0 {
			values[strings.ToUpper(item[:index])] = item
		}
	}
	if _, ok := values["SYSTEMROOT"]; !ok {
		values["SYSTEMROOT"] = "SYSTEMROOT=" + os.Getenv("SYSTEMROOT")
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var block []uint16
	for _, key := range keys {
		block = append(block, utf16.Encode([]rune(values[key]))...)
		block = append(block, 0)
	}
	return append(block, 0)
}

func (p *windowsProcess) Read(data []byte) (int, error)  { return p.output.Read(data) }
func (p *windowsProcess) Write(data []byte) (int, error) { return p.input.Write(data) }
func (p *windowsProcess) resize(cols, rows int) error {
	p.consoleMu.Lock()
	defer p.consoleMu.Unlock()
	if p.console == 0 {
		return os.ErrClosed
	}
	return windows.ResizePseudoConsole(p.console, windows.Coord{X: int16(cols), Y: int16(rows)})
}
func (p *windowsProcess) closeConsole() {
	p.consoleMu.Lock()
	defer p.consoleMu.Unlock()
	if p.console != 0 {
		windows.ClosePseudoConsole(p.console)
		p.console = 0
	}
}
func (p *windowsProcess) Close() error {
	// Break both pipes before waiting for ClosePseudoConsole. This also releases
	// a concurrent graceful close that is blocked emitting its final frame.
	err := errors.Join(p.input.Close(), p.output.Close())
	p.closeConsole()
	return err
}
func (p *windowsProcess) kill() error { return p.process.Kill() }
func (p *windowsProcess) wait() Exit {
	state, err := p.process.Wait()
	// Closing ConPTY after exit emits the final frame and breaks the output pipe.
	// The caller drains Read concurrently; explicit Close can cancel that drain.
	p.closeConsole()
	code := -1
	if state != nil {
		code = state.ExitCode()
	}
	return Exit{Code: code, Err: err}
}
