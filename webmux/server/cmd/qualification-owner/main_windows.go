// Command qualification-owner owns a disposable Windows qualification tree.
// It is built only by tests/packaging and is never shipped in a native bundle.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type accounting struct {
	TotalUserTime, TotalKernelTime, ThisPeriodTotalUserTime, ThisPeriodTotalKernelTime int64
	TotalPageFaultCount, TotalProcesses, ActiveProcesses, TotalTerminatedProcesses     uint32
}

type receipt struct {
	ObservedDescendants uint32 `json:"observedDescendants"`
	LiveDescendants     uint32 `json:"liveDescendants"`
	Version             int    `json:"version"`
	Phase               string `json:"phase"`
	RootPID             uint32 `json:"rootPID"`
	Requested           bool   `json:"requested"`
	TotalProcesses      uint32 `json:"totalProcesses"`
	ActiveProcesses     uint32 `json:"activeProcesses"`
}

func writeReceipt(path string, value receipt) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func jobAccounting(job windows.Handle) (accounting, error) {
	var value accounting
	err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&value)), uint32(unsafe.Sizeof(value)), nil)
	return value, err
}

// Retain read/wait handles for descendants visible at shutdown as well as job
// accounting. This includes an OS-hosted console if it appears in the tree but
// is outside ordinary process-job inheritance. These handles never kill by PID.
func descendantHandles(root uint32, rootHandle windows.Handle) ([]windows.Handle, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	rows := []windows.ProcessEntry32{}
	err = windows.Process32First(snapshot, &entry)
	for err == nil {
		rows = append(rows, entry)
		err = windows.Process32Next(snapshot, &entry)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, err
	}
	var created, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(rootHandle, &created, &exit, &kernel, &user); err != nil {
		return nil, err
	}
	timestamp := func(value windows.Filetime) uint64 { return uint64(value.HighDateTime)<<32 | uint64(value.LowDateTime) }
	seen := map[uint32]bool{root: true}
	handles := []windows.Handle{}
	success := false
	defer func() {
		if !success {
			for _, h := range handles {
				windows.CloseHandle(h)
			}
		}
	}()
	for {
		changed := false
		for _, row := range rows {
			if seen[row.ProcessID] || !seen[row.ParentProcessID] {
				continue
			}
			h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, row.ProcessID)
			if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
				continue
			} // Already exited.
			if err != nil {
				return nil, fmt.Errorf("observe descendant %d: %w", row.ProcessID, err)
			}
			var childCreated windows.Filetime
			if err := windows.GetProcessTimes(h, &childCreated, &exit, &kernel, &user); err != nil {
				windows.CloseHandle(h)
				return nil, err
			}
			if timestamp(childCreated) < timestamp(created) {
				windows.CloseHandle(h)
				continue
			}
			seen[row.ProcessID] = true
			changed = true
			handles = append(handles, h)
		}
		if !changed {
			break
		}
	}
	success = true
	return handles, nil
}

func inheritHandle(source windows.Handle) (windows.Handle, error) {
	var copied windows.Handle
	current := windows.CurrentProcess()
	err := windows.DuplicateHandle(current, source, current, &copied, 0, true, windows.DUPLICATE_SAME_ACCESS)
	return copied, err
}

func run(args []string) error {
	flags := flag.NewFlagSet("qualification-owner", flag.ContinueOnError)
	receiptPath := flags.String("receipt", "", "private qualification receipt path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	command := flags.Args()
	if *receiptPath == "" || len(command) == 0 {
		return errors.New("receipt and executable are required")
	}
	if _, err := os.Lstat(*receiptPath); !errors.Is(err, os.ErrNotExist) {
		return errors.New("receipt must be a new path")
	}
	executable, err := filepath.Abs(command[0])
	if err != nil {
		return err
	}
	application, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return err
	}
	line, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{executable}, command[1:]...)))
	if err != nil {
		return err
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	// No breakaway is allowed. Closing the last owner handle also kills the tree
	// if its parent disappears before the explicit stop protocol completes.
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return err
	}
	input, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer input.Close()
	sources := []windows.Handle{windows.Handle(input.Fd()), windows.Handle(os.Stdout.Fd()), windows.Handle(os.Stderr.Fd())}
	handles := make([]windows.Handle, 0, 3)
	for _, source := range sources {
		copied, err := inheritHandle(source)
		if err != nil {
			return err
		}
		defer windows.CloseHandle(copied)
		handles = append(handles, copied)
	}
	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return err
	}
	defer attributes.Delete()
	if err := attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return err
	}
	startup := windows.StartupInfoEx{}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = windows.STARTF_USESTDHANDLES
	startup.StdInput, startup.StdOutput, startup.StdErr = handles[0], handles[1], handles[2]
	startup.ProcThreadAttributeList = attributes.List()
	var process windows.ProcessInformation
	if err := windows.CreateProcess(application, line, nil, nil, true, windows.CREATE_SUSPENDED|windows.CREATE_NO_WINDOW|windows.EXTENDED_STARTUPINFO_PRESENT, nil, nil, &startup.StartupInfo, &process); err != nil {
		return err
	}
	defer windows.CloseHandle(process.Thread)
	defer windows.CloseHandle(process.Process)
	// Assign before executing any code, so even an immediately forking fixture
	// cannot escape the accounting/termination boundary.
	if err := windows.AssignProcessToJobObject(job, process.Process); err != nil {
		_ = windows.TerminateProcess(process.Process, 1)
		return err
	}
	value := receipt{Version: 1, Phase: "running", RootPID: process.ProcessId, ActiveProcesses: 1, TotalProcesses: 1}
	if err := writeReceipt(*receiptPath, value); err != nil {
		return err
	}
	if _, err := windows.ResumeThread(process.Thread); err != nil {
		return err
	}
	control := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 64), 64)
		control <- scanner.Scan() && scanner.Text() == "stop"
	}()
waitForStop:
	for {
		select {
		case value.Requested = <-control:
			break waitForStop
		default:
		}
		event, err := windows.WaitForSingleObject(process.Process, 0)
		if err != nil {
			return err
		}
		if event == windows.WAIT_OBJECT_0 {
			break
		}
		if event != uint32(windows.WAIT_TIMEOUT) {
			return errors.New("unexpected root wait state")
		}
		time.Sleep(10 * time.Millisecond)
	}
	descendants, err := descendantHandles(process.ProcessId, process.Process)
	if err != nil {
		return err
	}
	for _, handle := range descendants {
		defer windows.CloseHandle(handle)
	}
	value.ObservedDescendants = uint32(len(descendants))
	value.Phase = "stopping"
	if counts, err := jobAccounting(job); err == nil {
		value.TotalProcesses = counts.TotalProcesses
		value.ActiveProcesses = counts.ActiveProcesses
	} else {
		return err
	}
	if err := writeReceipt(*receiptPath, value); err != nil {
		return err
	}
	if err := windows.TerminateJobObject(job, 0); err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		counts, err := jobAccounting(job)
		if err != nil {
			return err
		}
		value.TotalProcesses = counts.TotalProcesses
		value.ActiveProcesses = counts.ActiveProcesses
		value.LiveDescendants = 0
		for _, handle := range descendants {
			event, err := windows.WaitForSingleObject(handle, 0)
			if err != nil {
				return err
			}
			if event != windows.WAIT_OBJECT_0 {
				value.LiveDescendants++
			}
		}
		if counts.ActiveProcesses == 0 && value.LiveDescendants == 0 {
			break
		}
		if time.Now().After(deadline) {
			value.Phase = "timeout"
			_ = writeReceipt(*receiptPath, value)
			return errors.New("owned qualification job still has active processes")
		}
		time.Sleep(10 * time.Millisecond)
	}
	value.Phase = "stopped"
	if err := writeReceipt(*receiptPath, value); err != nil {
		return err
	}
	if !value.Requested {
		return errors.New("qualification owner lost control or root exited before stop")
	}
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "qualification-owner:", err)
		os.Exit(1)
	}
}
