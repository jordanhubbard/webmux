// Package macsetup verifies filesystem access in the service's responsibility
// context. It never edits TCC databases or attempts to grant itself permission.
package macsetup

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
	"sync"
	"time"
)

type Folder struct {
	Name  string `json:"name"`
	State string `json:"state"`
}
type Result struct {
	State     string   `json:"state"`
	Folders   []Folder `json:"folders,omitempty"`
	Detail    string   `json:"detail,omitempty"`
	CheckedAt string   `json:"checked_at,omitempty"`
}

var instance = time.Now().UTC().Format(time.RFC3339Nano)

type Status struct {
	Instance         string `json:"instance"`
	Supported        bool   `json:"supported"`
	Hostname         string `json:"hostname"`
	Executable       string `json:"executable"`
	PermissionTarget string `json:"permission_target"`
	CanOpenSettings  bool   `json:"can_open_settings"`
	Local            Result `json:"local"`
	SSH              Result `json:"ssh"`
}
type Manager struct {
	mu         sync.Mutex
	local, ssh Result
	// Serialize probes: repeated requests must not accumulate blocked helpers.
	running bool
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	exe, _ := os.Executable()
	host, _ := os.Hostname()
	target := exe
	if i := strings.Index(exe, ".app/Contents/MacOS/"); i >= 0 {
		target = exe[:i+4]
	}
	local, ssh := m.local, m.ssh
	if local.State == "" {
		local.State = "unchecked"
	}
	if ssh.State == "" {
		ssh.State = "unchecked"
	}
	return Status{Instance: instance, Supported: runtime.GOOS == "darwin", Hostname: host, Executable: exe, PermissionTarget: target, Local: local, SSH: ssh}
}

// ProbeWorker runs only in a disposable child: a TCC dialog can block open(2).
// Read at most one directory entry, discard it, and never return filenames.
func ProbeWorker(out io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	folders := make([]Folder, 0, 3)
	for _, name := range []string{"Desktop", "Documents", "Downloads"} {
		state := "allowed"
		f, err := os.Open(filepath.Join(home, name))
		if err == nil {
			_, err = f.Readdirnames(1)
			_ = f.Close()
		}
		switch {
		case err == nil || errors.Is(err, io.EOF):
		case errors.Is(err, os.ErrNotExist):
			state = "missing"
		case errors.Is(err, os.ErrPermission):
			state = "denied"
		default:
			state = "error"
		}
		folders = append(folders, Folder{Name: name, State: state})
		// Publish each decision before the next folder can block on a prompt.
		if err := json.NewEncoder(out).Encode(folders); err != nil {
			return err
		}
	}
	return nil
}

const sshProbe = `for folder in Desktop Documents Downloads; do
if [ ! -d "$HOME/$folder" ]; then state=missing
elif /bin/ls -A "$HOME/$folder" >/dev/null 2>&1; then state=allowed
else state=denied; fi
printf '%s %s\n' "$folder" "$state"
done`

func (m *Manager) Check(ctx context.Context, transport string) (Result, error) {
	if runtime.GOOS != "darwin" {
		return Result{}, errors.New("macOS setup is unavailable on this host")
	}
	if transport != "local" && transport != "ssh" {
		return Result{}, errors.New("choose local or ssh")
	}
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return Result{}, errors.New("a permission check is already running")
	}
	m.running = true
	m.mu.Unlock()
	if transport == "local" {
		// The HTTP request only starts the check. Desktop approval can take minutes;
		// disconnecting the browser must not kill the prompt or start another one.
		m.mu.Lock()
		m.local = Result{State: "waiting", Folders: []Folder{{Name: "Desktop", State: "waiting"}, {Name: "Documents", State: "waiting"}, {Name: "Downloads", State: "waiting"}}}
		initial := m.local
		m.mu.Unlock()
		go m.checkLocal()
		return initial, nil
	}
	defer func() { m.mu.Lock(); m.running = false; m.mu.Unlock() }()
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	// Fixed host and command, no password prompts, config-supplied proxy commands,
	// host-key changes, or arbitrary user-provided commands.
	cmd := exec.CommandContext(ctx, "/usr/bin/ssh", "-F", "/dev/null", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=3", "-o", "ConnectionAttempts=1", "-o", "ClearAllForwardings=yes", "localhost", sshProbe)
	cmd.WaitDelay = time.Second
	result := runProbe(ctx, cmd, transport)
	m.mu.Lock()
	m.ssh = result
	m.mu.Unlock()
	return result, nil
}

// Keep exactly one bounded helper alive while publishing decisions for polling clients.
func (m *Manager) checkLocal() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	exe, err := os.Executable()
	result := Result{State: "error", Detail: "The permission helper could not start."}
	if err == nil {
		cmd := exec.CommandContext(ctx, exe, "--macos-permission-probe")
		cmd.WaitDelay = time.Second
		result = runProbeProgress(ctx, cmd, "local", func(folders []Folder) {
			if len(folders) > 3 {
				return
			}
			for i, folder := range folders {
				if folder.Name != []string{"Desktop", "Documents", "Downloads"}[i] || (folder.State != "allowed" && folder.State != "denied" && folder.State != "missing") {
					return
				}
			}
			pending := append([]Folder(nil), folders...)
			for _, name := range []string{"Desktop", "Documents", "Downloads"}[len(folders):] {
				pending = append(pending, Folder{Name: name, State: "waiting"})
			}
			m.mu.Lock()
			m.local = Result{State: "waiting", Folders: pending}
			m.mu.Unlock()
		})
	}
	m.mu.Lock()
	// A timed-out helper has exited; show a retry instead of polling forever.
	if result.State == "waiting" {
		result.State = "error"
		result.Folders = m.local.Folders
	}
	m.local, m.running = result, false
	m.mu.Unlock()
}

func runProbe(ctx context.Context, cmd *exec.Cmd, transport string) Result {
	return runProbeProgress(ctx, cmd, transport, nil)
}

func runProbeProgress(ctx context.Context, cmd *exec.Cmd, transport string, progress func([]Folder)) Result {
	output := probeOutput{progress: progress}
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	err := cmd.Run()
	data := output.Bytes()
	if output.overflow {
		err = errors.New("permission helper output exceeded limit")
	}
	result := Result{State: "allowed", CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	if ctx.Err() != nil {
		result.State, result.Detail = "waiting", "The check timed out. macOS may be waiting for approval on this Mac. Complete any privacy prompt, then retry."
		return result
	}
	if err != nil {
		result.State, result.Detail = "error", "The permission helper could not complete."
		if transport == "ssh" {
			result.State, result.Detail = "unavailable", "Could not verify SSH. Enable Remote Login and configure passwordless localhost SSH with a trusted host key, or verify in your existing SSH terminal: ls -A ~/Downloads"
		}
		return result
	}
	if transport == "local" {
		lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
		err = json.Unmarshal(lines[len(lines)-1], &result.Folders)
	} else {
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			parts := strings.Fields(line)
			if len(parts) != 2 {
				err = errors.New("invalid SSH probe output")
				break
			}
			result.Folders = append(result.Folders, Folder{Name: parts[0], State: parts[1]})
		}
	}
	if err != nil || len(result.Folders) != 3 {
		result.State, result.Detail = "error", "The permission helper returned an invalid result."
		return result
	}
	verified := false
	for i, folder := range result.Folders {
		if folder.Name != []string{"Desktop", "Documents", "Downloads"}[i] {
			result.State = "error"
			break
		}
		switch folder.State {
		case "allowed":
			verified = true
		case "missing":
		case "denied":
			if result.State != "error" {
				result.State = "denied"
			}
		default:
			result.State = "error"
		}
	}
	if result.State == "allowed" && !verified {
		result.State, result.Detail = "unavailable", "None of these folders exists for this account; protected-folder access could not be verified."
	}
	return result
}

func Open(ctx context.Context, action string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("macOS setup is unavailable on this host")
	}
	var args []string
	switch action {
	case "privacy":
		args = []string{"x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles"}
	case "sharing":
		args = []string{"x-apple.systempreferences:com.apple.Sharing-Settings.extension"}
	case "reveal":
		args = []string{"-R", (&Manager{}).Status().PermissionTarget}
	default:
		return fmt.Errorf("unknown setup action")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "/usr/bin/open", args...).Run()
}

// A shell startup hook on the SSH path may print arbitrary output. Keep it bounded.
type probeOutput struct {
	buffer   bytes.Buffer
	overflow bool
	progress func([]Folder)
}

func (b *probeOutput) Bytes() []byte { return b.buffer.Bytes() }

func (b *probeOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 4096 - b.buffer.Len()
	if len(p) > remaining {
		p = p[:remaining]
		b.overflow = true
	}
	_, _ = b.buffer.Write(p)
	if b.progress != nil && !b.overflow {
		lines := bytes.Split(b.buffer.Bytes(), []byte("\n"))
		if len(lines) > 1 {
			var folders []Folder
			if json.Unmarshal(lines[len(lines)-2], &folders) == nil {
				b.progress(folders)
			}
		}
	}
	return n, nil
}
