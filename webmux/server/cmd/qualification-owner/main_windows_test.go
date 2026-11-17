package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// Actual child processes run inside the owned job, not a simulated accounting
// source. The root forks immediately, before printing readiness.
func TestQualificationOwnedChild(t *testing.T) {
	mode := os.Getenv("WEBMUX_QUALIFICATION_CHILD")
	if mode == "" {
		return
	}
	if mode == "root" {
		executable, err := os.Executable()
		if err != nil {
			panic(err)
		}
		leaf := exec.Command(executable, "-test.run=^TestQualificationOwnedChild$")
		leaf.Env = append(os.Environ(), "WEBMUX_QUALIFICATION_CHILD=leaf")
		leaf.Stdout, leaf.Stderr = os.Stdout, os.Stderr
		if err := leaf.Start(); err != nil {
			panic(err)
		}
		fmt.Printf("QUALIFICATION_ROOT_READY %d %d\n", os.Getpid(), leaf.Process.Pid)
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestWindowsQualificationOwnerReapsActualTree(t *testing.T) {
	directory := t.TempDir()
	owner := filepath.Join(directory, "qualification owner.exe")
	build := exec.Command("go", "build", "-o", owner, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build owner: %v: %s", err, output)
	}
	for _, mode := range []string{"explicit-stop", "lost-control", "owner-crash"} {
		t.Run(mode, func(t *testing.T) {
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			receiptPath := filepath.Join(t.TempDir(), "ownership.json")
			child := exec.Command(owner, "--receipt", receiptPath, "--", executable, "-test.run=^TestQualificationOwnedChild$")
			child.Env = append(os.Environ(), "WEBMUX_QUALIFICATION_CHILD=root")
			input, err := child.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			output, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			child.Stderr = os.Stderr
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			defer child.Process.Kill()
			lines := make(chan string, 1)
			go func() {
				scanner := bufio.NewScanner(output)
				for scanner.Scan() {
					line := scanner.Text()
					if strings.HasPrefix(line, "QUALIFICATION_ROOT_READY ") {
						lines <- line
					}
				}
			}()
			var line string
			select {
			case line = <-lines:
			case <-time.After(15 * time.Second):
				t.Fatal("owned process tree did not become ready")
			}
			fields := strings.Fields(line)
			if len(fields) != 3 {
				t.Fatal(line)
			}
			handles := []windows.Handle{}
			for _, field := range fields[1:] {
				pid, err := strconv.ParseUint(field, 10, 32)
				if err != nil {
					t.Fatal(err)
				}
				handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
				if err != nil {
					t.Fatal(err)
				}
				defer windows.CloseHandle(handle)
				handles = append(handles, handle)
			}
			// A running receipt cannot be mistaken for proof of shutdown.
			var before receipt
			data, err := os.ReadFile(receiptPath)
			if err != nil {
				t.Fatal(err)
			}
			if json.Unmarshal(data, &before) != nil || before.Phase != "running" || before.ActiveProcesses == 0 {
				t.Fatal("premature shutdown receipt", string(data))
			}
			switch mode {
			case "explicit-stop":
				if _, err := input.Write([]byte("stop\n")); err != nil {
					t.Fatal(err)
				}
				_ = input.Close()
			case "lost-control":
				_ = input.Close()
			case "owner-crash":
				if err := child.Process.Kill(); err != nil {
					t.Fatal(err)
				}
			}
			done := make(chan error, 1)
			go func() { done <- child.Wait() }()
			select {
			case err := <-done:
				if (mode == "explicit-stop") != (err == nil) {
					t.Fatalf("unexpected owner outcome: %v", err)
				}
			case <-time.After(15 * time.Second):
				t.Fatal("qualification owner did not finish")
			}
			for _, handle := range handles {
				event, err := windows.WaitForSingleObject(handle, 5000)
				if err != nil || event != windows.WAIT_OBJECT_0 {
					t.Fatalf("owned process survived cleanup: event=%d err=%v", event, err)
				}
			}
			if mode != "owner-crash" {
				data, err := os.ReadFile(receiptPath)
				if err != nil {
					t.Fatal(err)
				}
				var after receipt
				if json.Unmarshal(data, &after) != nil || after.Phase != "stopped" || after.TotalProcesses < 2 || after.ActiveProcesses != 0 || after.ObservedDescendants < 1 || after.LiveDescendants != 0 || after.Requested != (mode == "explicit-stop") {
					t.Fatal("invalid final job accounting", string(data))
				}
			}
		})
	}
}
