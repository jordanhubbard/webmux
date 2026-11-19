package openrigcoord

import (
	"bufio"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed rig-changes.mts
var rigChanges []byte

//go:embed changes-runner.mts
var changesRunner []byte

type Change struct {
	Available bool `json:"available"`
	Changed   bool `json:"changed"`
}

// WaitChange is read-only and must never hold the coordination mutation mutex.
// Callers bound duration and concurrency. Only invalidation bits cross this boundary.
func WaitChange(ctx context.Context, home, owner string, ready func()) Change {
	scope := sha256.Sum256([]byte(owner))
	directory := filepath.Join(home, "data", "rig-changes", hex.EncodeToString(scope[:12]))
	if os.MkdirAll(directory, 0700) != nil {
		return Change{}
	}
	for name, source := range map[string][]byte{"rig-changes.mts": rigChanges, "changes-runner.mts": changesRunner} {
		if os.WriteFile(filepath.Join(directory, name), source, 0600) != nil {
			return Change{}
		}
	}
	cmd := exec.CommandContext(ctx, "node", filepath.Join(directory, "changes-runner.mts"))
	configureChangeProcess(cmd)
	cmd.Env = ControlEnvironment()
	out, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		return Change{}
	}
	scan := bufio.NewScanner(out)
	scan.Buffer(make([]byte, 1024), 1024)
	var result Change
	seenReady, seenResult := false, false
	valid := true
	for scan.Scan() {
		var row struct {
			Ready     bool `json:"ready"`
			Available bool `json:"available"`
			Changed   bool `json:"changed"`
		}
		if json.Unmarshal(scan.Bytes(), &row) != nil || seenResult {
			valid = false
			break
		}
		if row.Ready {
			if seenReady || row.Available || row.Changed {
				valid = false
				break
			}
			seenReady = true
			if ready != nil {
				ready()
			}
		} else {
			seenResult = true
			if row.Available && !seenReady {
				valid = false
				break
			}
			result = Change{Available: row.Available, Changed: row.Available && row.Changed}
		}
	}
	if scan.Err() != nil {
		valid = false
	}
	if !valid {
		_ = cmd.Cancel()
	}
	err = cmd.Wait()
	if err != nil || !valid || !seenResult {
		return Change{}
	}
	return result
}
