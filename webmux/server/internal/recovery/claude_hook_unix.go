//go:build darwin || linux

package recovery

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func newClaudeObserver(version string, args []string) *claudeObserver {
	if !claudeHookEligible(version, args) {
		return nil
	}
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return nil
	}
	directory, err := os.MkdirTemp("/tmp", "wm-claude-")
	if err != nil {
		return nil
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	endpoint := filepath.Join(directory, "s")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: endpoint, Net: "unix"})
	if err != nil {
		cleanup()
		return nil
	}
	if os.Chmod(endpoint, 0600) != nil {
		listener.Close()
		cleanup()
		return nil
	}
	token, err := NewInstance()
	if err != nil {
		listener.Close()
		cleanup()
		return nil
	}
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'" }
	command := quote(self) + " --recovery-claude-hook " + quote(endpoint) + " " + quote(token)
	settings, _ := json.Marshal(map[string]any{"hooks": map[string]any{"SessionStart": []any{map[string]any{"matcher": "", "hooks": []any{map[string]any{"type": "command", "command": command, "timeout": 2}}}}}})
	o := &claudeObserver{settings: string(settings), endpoint: endpoint, token: token, observations: make(chan claudeSessionObservation, 8)}
	done := make(chan struct{})
	var start, stop sync.Once
	o.start = func(pid int) {
		start.Do(func() {
			go func() {
				defer close(done)
				for {
					conn, err := listener.AcceptUnix()
					if err != nil {
						return
					}
					func() {
						defer conn.Close()
						_ = conn.SetDeadline(time.Now().Add(time.Second))
						var message claudeHookMessage
						if json.NewDecoder(io.LimitReader(conn, 32<<10)).Decode(&message) != nil || subtle.ConstantTimeCompare([]byte(message.Token), []byte(token)) != 1 || !message.Observation.valid() {
							return
						}
						peer, err := claudeHookPeerPID(conn)
						if err != nil || !nativeProcessDescendant(peer, pid) {
							return
						}
						select {
						case o.observations <- message.Observation:
							_, _ = conn.Write([]byte{1})
						default:
						}
					}()
				}
			}()
		})
	}
	o.close = func() { stop.Do(func() { listener.Close(); o.start(0); <-done; cleanup() }) }
	return o
}
