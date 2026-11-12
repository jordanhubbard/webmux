package recovery

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func sameDirectory(a, b string) bool {
	left, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false
	}
	right, err := filepath.EvalSymlinks(b)
	return err == nil && left == right
}

func codexMeta(file, cwd string) (string, bool) {
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		userHome, _ := os.UserHomeDir()
		home = filepath.Join(userHome, ".codex")
	}
	root, err := filepath.EvalSymlinks(filepath.Join(home, "sessions"))
	if err != nil {
		return "", false
	}
	real, err := filepath.EvalSymlinks(file)
	if err != nil || !strings.HasPrefix(real, root+string(os.PathSeparator)) || !strings.HasSuffix(real, ".jsonl") {
		return "", false
	}
	f, err := os.Open(real)
	if err != nil {
		return "", false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 64<<10)
	if !scanner.Scan() {
		return "", false
	}
	var meta struct {
		Type    string `json:"type"`
		Payload struct {
			ID  string `json:"id"`
			Cwd string `json:"cwd"`
		} `json:"payload"`
	}
	if json.Unmarshal(scanner.Bytes(), &meta) != nil || meta.Type != "session_meta" || !ValidNativeID("codex", meta.Payload.ID) || !sameDirectory(meta.Payload.Cwd, cwd) {
		return "", false
	}
	return meta.Payload.ID, true
}

// The owning process's open rollout file is authoritative. Searching for the
// latest file by directory or timestamp can select another concurrent agent.
func codexSessionForProcess(pid int, cwd string) (string, string) {
	id, path := "", ""
	for _, file := range processTreeOpenFiles(pid, func(file string) bool { return strings.HasSuffix(file, ".jsonl") }) {
		if candidate, ok := codexMeta(file, cwd); ok {
			if id != "" && candidate != id {
				return "", ""
			}
			id, path = candidate, file
		}
	}
	return id, path
}

// processTreeOpenFiles lists bounded open-file paths accepted by keep for pid
// and its descendants. It returns nothing where open files cannot be observed.
func processTreeOpenFiles(pid int, keep func(string) bool) []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pids := []int{pid}
	if output, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=").Output(); err == nil && len(output) < 1<<20 {
		children := map[int][]int{}
		for _, line := range strings.Split(string(output), "\n") {
			parts := strings.Fields(line)
			if len(parts) != 2 {
				continue
			}
			child, e1 := strconv.Atoi(parts[0])
			parent, e2 := strconv.Atoi(parts[1])
			if e1 == nil && e2 == nil {
				children[parent] = append(children[parent], child)
			}
		}
		for i := 0; i < len(pids) && len(pids) < 32; i++ {
			pids = append(pids, children[pids[i]]...)
		}
		if len(pids) > 32 {
			pids = pids[:32]
		}
	}
	var files []string
	if runtime.GOOS == "linux" {
		for _, child := range pids {
			dir := "/proc/" + strconv.Itoa(child) + "/fd"
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for i, entry := range entries {
				if i >= 256 {
					break
				}
				if file, err := os.Readlink(filepath.Join(dir, entry.Name())); err == nil && keep(file) {
					files = append(files, file)
				}
			}
		}
		return files
	}
	selected := make([]string, len(pids))
	for i, child := range pids {
		selected[i] = strconv.Itoa(child)
	}
	output, _ := exec.CommandContext(ctx, "lsof", "-a", "-p", strings.Join(selected, ","), "-Fn").Output()
	if len(output) > 1<<20 {
		return nil
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "n") && keep(line[1:]) {
			files = append(files, line[1:])
		}
	}
	return files
}

// Absence of an owned native state file is not evidence that a previously
// selected conversation disappeared. A shared Codex daemon owns those files
// instead of the TUI, and a new Cursor chat has no store until its first turn.
// Keep the last reliable reference for an explicit choice, while removing
// current-process confidence as soon as ownership can no longer be observed.
func observeProcessState(event *Event, id, file string) {
	if id != "" {
		event.SessionID, event.StateFile, event.NativeEvidence = id, file, "process-state"
	} else if event.NativeEvidence == "process-state" {
		event.NativeEvidence = "unavailable"
	}
}
