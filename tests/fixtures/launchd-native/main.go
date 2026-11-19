//go:build !windows

// A private service-manager fixture, never a wrapper around system launchctl.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

type state struct {
	Bootouts      int   `json:"bootouts"`
	Bootstraps    int   `json:"bootstraps"`
	Registered    bool  `json:"registered"`
	RemovingUntil int64 `json:"removingUntil"`
}

func fail(message string, code int) { fmt.Fprintln(os.Stderr, message); os.Exit(code) }
func write(path string, value any) {
	data, err := json.Marshal(value)
	if err == nil {
		err = os.WriteFile(path, data, 0600)
	}
	if err != nil {
		fail("Private fixture state write failed", 1)
	}
}

func main() {
	dir, mode := os.Getenv("WEBMUX_LAUNCHD_FIXTURE"), os.Getenv("WEBMUX_LAUNCHD_MODE")
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() || mode == "" || len(os.Args) < 2 {
		fail("Private launchd fixture configuration is required", 1)
	}
	initialized := filepath.Join(dir, "initialized")
	if _, err := os.Stat(initialized); os.IsNotExist(err) {
		delay := 0
		if text := os.Getenv("WEBMUX_LAUNCHD_PREPARATION_DELAY_MS"); text != "" {
			delay, err = strconv.Atoi(text)
			if err != nil || delay < 0 || delay > 5000 {
				fail("Invalid fixture preparation delay", 1)
			}
		}
		time.Sleep(time.Duration(delay) * time.Millisecond)
		f, err := os.OpenFile(initialized, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			fail("Private fixture initialization failed", 1)
		}
		_, err = f.WriteString("ready")
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			fail("Private fixture initialization failed", 1)
		}
	}
	action := os.Args[1]
	if action == "fixture-ready" {
		self, err := os.Executable()
		if err != nil {
			fail("Private fixture executable unavailable", 1)
		}
		data, err := os.ReadFile(self)
		if err != nil {
			fail("Private fixture executable unavailable", 1)
		}
		sum := sha256.Sum256(data)
		write(filepath.Join(dir, "ready.json"), map[string]string{"kind": "webmux-native-launchd-fixture/v1", "imageSHA256": hex.EncodeToString(sum[:])})
		return
	}
	file := filepath.Join(dir, "state.json")
	var s state
	if data, err := os.ReadFile(file); err == nil {
		if json.Unmarshal(data, &s) != nil {
			fail("Invalid private fixture state", 1)
		}
	} else if !os.IsNotExist(err) {
		fail("Private fixture state unavailable", 1)
	}
	commands, err := os.OpenFile(filepath.Join(dir, "commands"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		fail("Private fixture command receipt unavailable", 1)
	}
	_, err = commands.WriteString(action + "\n")
	closeErr := commands.Close()
	if err != nil || closeErr != nil {
		fail("Private fixture command receipt failed", 1)
	}
	if action == "print" {
		switch mode {
		case "state":
			fmt.Println(os.Getenv("WEBMUX_LAUNCHD_STATE"))
			return
		case "query-timeout":
			time.Sleep(2 * time.Second)
			fmt.Println("state = running\npid = 222")
			return
		case "query-error":
			fail("Query failed: permission denied", 5)
		}
	}
	switch action {
	case "bootout":
		s.Bootouts++
		if mode == "fresh" || mode == "stop-denied" {
			s.Registered = mode == "stop-denied"
			write(file, s)
			if mode == "fresh" {
				fail("Could not find service \"fixture\"", 5)
			}
			fail("Stop failed: permission denied", 5)
		}
		s.RemovingUntil = time.Now().UnixMilli() + 500
		if mode == "interrupt" {
			data, err := os.ReadFile(filepath.Join(dir, "parent.pid"))
			if err != nil {
				fail("Private invoking process unavailable", 1)
			}
			pid, err := strconv.Atoi(string(data))
			if err != nil || pid <= 1 {
				fail("Invalid private invoking process", 1)
			}
			if err := syscall.Kill(-pid, syscall.SIGHUP); err != nil {
				fail("Private invoking process signal failed", 1)
			}
		}
	case "bootstrap":
		s.Bootstraps++
		if time.Now().UnixMilli() < s.RemovingUntil {
			fail("Registration attempted before removal completed", 1)
		}
		if mode == "denied" {
			fail("Bootstrap failed: permission denied", 5)
		}
		s.Registered = mode != "race" || s.Bootstraps > 2
		write(file, s)
		if mode == "race" && s.Bootstraps == 1 {
			fail("Operation already in progress", 5)
		}
	case "print":
		if time.Now().UnixMilli() < s.RemovingUntil {
			fmt.Println("state = running\npid = 111")
		} else if s.Registered {
			if mode == "crash" {
				fmt.Println("state = waiting\nlast exit code = 1")
			} else {
				fmt.Println("state = running\npid = 222")
			}
		} else {
			fail("Could not find service \"fixture\" in domain for user gui: 501", 113)
		}
	default:
		fail("Unexpected private launchctl action", 1)
	}
	write(file, s)
}
