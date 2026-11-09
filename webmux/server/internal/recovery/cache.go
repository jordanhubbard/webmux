package recovery

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// HostState is scoped to an execution-host account and a stable WebMux session.
// Control survives logging pause; activity capture is gated by Recording.
type HostState struct {
	KeyDigest string        `json:"key_digest"`
	Recording bool          `json:"recording"`
	Revision  uint64        `json:"revision"`
	Command   *Event        `json:"command,omitempty"`
	CLI       *Event        `json:"cli,omitempty"`
	Complete  *Event        `json:"complete,omitempty"`
	Shell     *Event        `json:"shell,omitempty"`
	Provider  *Event        `json:"provider,omitempty"`
	Control   *Event        `json:"control,omitempty"`
	Consumed  []string      `json:"consumed,omitempty"`
	Replay    *ReplayIntent `json:"replay,omitempty"`
}

type ReplayIntent struct {
	Instance string `json:"instance"`
	Cwd      string `json:"cwd"`
	Command  string `json:"command"`
}

func keyDigest(key string) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:])
}

func hostStateDirectory(id string) (string, error) {
	if !nativeUUID.MatchString(id) {
		return "", errors.New("invalid recovery session identity")
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	base := filepath.Join(cache, "webmux", "recovery")
	if err := os.MkdirAll(base, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(base, id)
	if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("recovery state directory is not an owned directory")
	}
	if err := os.Chmod(path, 0700); err != nil {
		return "", err
	}
	return path, nil
}

func updateHostState(id, key string, create bool, change func(*HostState) error) (HostState, error) {
	if len(key) != 32 {
		return HostState{}, errors.New("missing recovery authentication key")
	}
	if _, err := hex.DecodeString(key); err != nil {
		return HostState{}, errors.New("invalid recovery authentication key")
	}
	dir, err := hostStateDirectory(id)
	if err != nil {
		return HostState{}, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return HostState{}, err
	}
	defer root.Close()
	if info, err := root.Lstat("state.lock"); err == nil && !info.Mode().IsRegular() {
		return HostState{}, errors.New("recovery lock is not a regular file")
	}
	lock, err := root.OpenFile("state.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return HostState{}, err
	}
	defer lock.Close()
	if err := lockStateFile(lock); err != nil {
		return HostState{}, err
	}
	defer unlockStateFile(lock)
	var state HostState
	if info, err := root.Lstat("state.json"); err == nil && !info.Mode().IsRegular() {
		return HostState{}, errors.New("recovery state is not a regular file")
	}
	file, err := root.Open("state.json")
	if err == nil {
		info, statErr := file.Stat()
		if statErr != nil || !info.Mode().IsRegular() || info.Size() > 192<<10 {
			_ = file.Close()
			return HostState{}, errors.New("invalid execution-host recovery state")
		}
		err = json.NewDecoder(io.LimitReader(file, 192<<10)).Decode(&state)
		_ = file.Close()
		if err != nil {
			return HostState{}, err
		}
	} else if errors.Is(err, os.ErrNotExist) && create {
		state.KeyDigest = keyDigest(key)
	} else {
		return HostState{}, err
	}
	if subtle.ConstantTimeCompare([]byte(state.KeyDigest), []byte(keyDigest(key))) != 1 {
		return HostState{}, errors.New("recovery session authentication failed")
	}
	before, _ := json.Marshal(state)
	if change != nil {
		if err := change(&state); err != nil {
			return HostState{}, err
		}
	}
	after, err := json.Marshal(state)
	if err != nil {
		return HostState{}, err
	}
	if !bytes.Equal(before, after) || file == nil && create {
		state.Revision++
		after, err = json.Marshal(state)
		if err != nil {
			return HostState{}, err
		}
		if len(after) > 192<<10 {
			return HostState{}, errors.New("execution-host recovery state exceeds its limit")
		}
		name, err := NewInstance()
		if err != nil {
			return HostState{}, err
		}
		name = "state-" + name + ".tmp"
		temporary, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return HostState{}, err
		}
		defer root.Remove(name)
		_, writeErr := temporary.Write(after)
		syncErr := temporary.Sync()
		closeErr := temporary.Close()
		if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
			return HostState{}, err
		}
		if err := root.Rename(name, "state.json"); err != nil {
			return HostState{}, err
		}
	}
	return state, nil
}

func InitializeHostState(id, key string, recording bool) error {
	_, err := updateHostState(id, key, true, func(state *HostState) error { state.Recording = recording; return nil })
	return err
}

func RecordHostEvent(event Event) error {
	id, key := os.Getenv("WEBMUX_RECOVERY_SESSION"), os.Getenv("WEBMUX_RECOVERY_KEY")
	if id == "" || key == "" {
		return nil
	}
	if err := event.Validate(); err != nil {
		return err
	}
	if _, err := SignedPacket(event, key); err != nil {
		return err
	}
	_, err := updateHostState(id, key, false, func(state *HostState) error {
		event.Signature = ""
		switch event.Kind {
		case "provider":
			state.Provider = &event
		case "control":
			state.Control = &event
		case "shell":
			// No new command/directory content is retained while capture is paused.
			if state.Recording {
				state.Shell = &event
			}
		case "command":
			if state.Recording {
				var replayable bool
				event.Command, replayable = SafeCommand(event.Command)
				event.Redacted = event.Redacted || !replayable
				state.Command, state.CLI, state.Complete = &event, nil, nil
			}
		case "cli":
			if state.Recording {
				event.Argv = safeArgv(event.Argv)
				state.CLI = &event
				state.Complete = nil
			}
		case "complete":
			if state.Recording {
				state.Complete = &event
			}
		}
		return nil
	})
	return err
}

func SignedHostEvents(state HostState, key string) ([]Event, error) {
	var result []Event
	for _, event := range []*Event{state.Provider, state.Command, state.CLI, state.Complete, state.Shell, state.Control} {
		if event == nil {
			continue
		}
		packet, err := SignedPacket(*event, key)
		if err != nil {
			return nil, err
		}
		var decoder Decoder
		_, events := decoder.Feed(packet)
		result = append(result, events...)
	}
	return result, nil
}
