package session

import (
	"runtime"
	"time"
)

// checkpointInterval is how often a running server rewrites the session
// document. A kill loses only the changes since the last rewrite or the last
// meaningful session change, whichever is newer. Host observations are also
// copied by the recovery poll, which persists as soon as they change.
var checkpointInterval = 30 * time.Second

// Lifecycle is the process marker stored in the existing session document.
// Phase running means this process started and has not recorded a clean exit.
// Phase clean means shutdown, an interrupt, or an intentional restart finished
// on purpose. There is no separate checkpoint database.
type Lifecycle struct {
	Version      int    `yaml:"version"`
	Phase        string `yaml:"phase"`
	PID          int    `yaml:"pid"`
	StartedAt    string `yaml:"started_at,omitempty"`
	BootID       string `yaml:"boot_id,omitempty"`
	CheckpointAt string `yaml:"checkpoint_at,omitempty"`
}

// ProcessObservation is a point-in-time look at the process named by a saved
// lifecycle, plus the boot id of the machine reading it.
type ProcessObservation struct {
	Alive     bool
	StartedAt string
	BootID    string
}

// uncleanPrevious reports whether the saved marker belongs to a process that
// did not exit cleanly. A marker whose process is still this same live process
// is not a death: a second server must not treat the running one as killed.
func uncleanPrevious(previous Lifecycle, observed ProcessObservation) bool {
	if previous.Version == 0 || previous.Phase != "running" || previous.PID <= 0 {
		return false
	}
	if previous.BootID != "" && observed.BootID != "" && previous.BootID != observed.BootID {
		return true
	}
	if !observed.Alive {
		return true
	}
	if previous.StartedAt != "" && observed.StartedAt != "" && previous.StartedAt != observed.StartedAt {
		return true
	}
	return false
}

func (b *Broker) lifecycleLocked() Lifecycle {
	phase := "running"
	if b.cleanExit {
		phase = "clean"
	}
	return Lifecycle{Version: 1, Phase: phase, PID: processID(), StartedAt: b.processStarted, BootID: b.bootID, CheckpointAt: now()}
}

// noteProviderSessionsLocked records the tmux or native-provider identity the
// manual resume path already uses. It is derived from the session id.
func (b *Broker) noteProviderSessionsLocked() {
	for _, id := range b.order {
		e := b.entries[id]
		if e == nil || e.value.Recovery == nil || e.value.Rig != nil || e.value.Agent() || e.value.Transport == "exec" || e.value.ID == "" {
			continue
		}
		name := providerSessionName(e.value.ID)
		if e.value.Recovery.ProviderSession != name {
			e.value.Recovery.ProviderSession = name
		}
	}
}

func providerSessionName(id string) string {
	if runtime.GOOS == "windows" {
		return id
	}
	return "webmux-" + id
}

// MarkCleanShutdown records a deliberate exit. A panic, kill, or reboot never
// calls it, so the running marker stays behind for the next start.
func (b *Broker) MarkCleanShutdown() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrClosed
	}
	b.cleanExit = true
	return b.persistLocked()
}

func (b *Broker) startCheckpointLocked() {
	if b.checkpointStarted {
		return
	}
	b.checkpointStarted = true
	b.workers.Add(1)
	interval := checkpointInterval
	go func() {
		defer b.workers.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-b.policyStop:
				return
			case <-ticker.C:
				b.checkpoint()
			}
		}
	}()
}

func (b *Broker) checkpoint() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || len(b.order) == 0 {
		return
	}
	if err := b.persistLocked(); err != nil {
		b.logger.Warn("checkpoint sessions", "error", err)
	}
}
