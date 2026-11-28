package session

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

func TestUncleanExitDetection(t *testing.T) {
	const boot = "boot-a"
	same := ProcessObservation{Alive: true, StartedAt: "100", BootID: boot}
	cases := []struct {
		name     string
		previous Lifecycle
		observed ProcessObservation
		unclean  bool
	}{
		{name: "first start", previous: Lifecycle{}, observed: same, unclean: false},
		{name: "clean shutdown", previous: Lifecycle{Version: 1, Phase: "clean", PID: 42, StartedAt: "100", BootID: boot}, observed: ProcessObservation{Alive: false, BootID: boot}, unclean: false},
		{name: "clean wins over a dead pid", previous: Lifecycle{Version: 1, Phase: "clean", PID: 42, BootID: boot}, observed: ProcessObservation{Alive: false, BootID: "boot-b"}, unclean: false},
		{name: "killed process", previous: Lifecycle{Version: 1, Phase: "running", PID: 42, StartedAt: "100", BootID: boot}, observed: ProcessObservation{Alive: false, BootID: boot}, unclean: true},
		{name: "same process still alive", previous: Lifecycle{Version: 1, Phase: "running", PID: 42, StartedAt: "100", BootID: boot}, observed: same, unclean: false},
		{name: "pid reused", previous: Lifecycle{Version: 1, Phase: "running", PID: 42, StartedAt: "100", BootID: boot}, observed: ProcessObservation{Alive: true, StartedAt: "900", BootID: boot}, unclean: true},
		{name: "reboot", previous: Lifecycle{Version: 1, Phase: "running", PID: 42, StartedAt: "100", BootID: boot}, observed: ProcessObservation{Alive: true, StartedAt: "100", BootID: "boot-b"}, unclean: true},
		{name: "alive without start time stays manual", previous: Lifecycle{Version: 1, Phase: "running", PID: 42, StartedAt: "100", BootID: boot}, observed: ProcessObservation{Alive: true, BootID: boot}, unclean: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := uncleanPrevious(tc.previous, tc.observed); got != tc.unclean {
				t.Fatalf("unclean = %v, want %v", got, tc.unclean)
			}
		})
	}
}

func TestThisProcessObservationIsAlive(t *testing.T) {
	started := currentProcessStarted()
	observed := observeProcess(os.Getpid())
	if !observed.Alive {
		t.Fatal("this process looked dead")
	}
	if started != "" && observed.StartedAt != started {
		t.Fatalf("start time changed: %q vs %q", started, observed.StartedAt)
	}
	if observed.BootID != currentBootID() {
		t.Fatalf("boot id changed: %q vs %q", observed.BootID, currentBootID())
	}
	if runtime.GOOS == "linux" && (started == "" || observed.BootID == "") {
		t.Fatal("linux did not expose a process start time and boot id")
	}
	dead := observeProcess(unusedPID(t))
	if dead.Alive {
		t.Fatal("unused pid looked alive")
	}
}

func TestCheckpointRecordsResumeIdentity(t *testing.T) {
	b, _, s := controlFixture(t, true)
	b.checkpoint()
	body := string(readSessions(t, b))
	if !strings.Contains(body, "phase: running") || !strings.Contains(body, "checkpoint_at:") {
		t.Fatalf("lifecycle checkpoint missing:\n%s", body)
	}
	for _, want := range []string{"/workspace", "cadc08a9-0d73-4b44-af10-084a60b44f5b", "process-state", "prior-shell", providerSessionName(s.ID)} {
		if !strings.Contains(body, want) {
			t.Fatalf("checkpoint missing %q", want)
		}
	}
	if !b.checkpointStarted {
		if err := b.Restore(); err != nil {
			t.Fatal(err)
		}
	}
	if !b.checkpointStarted {
		t.Fatal("restore did not start the checkpoint loop")
	}
}

func TestCleanExitDoesNotAutoResume(t *testing.T) {
	b, _, s := controlFixture(t, true)
	if err := b.MarkCleanShutdown(); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	plantLifecycle(t, b, Lifecycle{Version: 1, Phase: "clean", PID: unusedPID(t), StartedAt: "dead", BootID: "previous-boot"})
	next, p := reopen(t, b)
	if next.uncleanExit {
		t.Fatal("clean exit was treated as unclean")
	}
	if err := next.Restore(); err != nil {
		t.Fatal(err)
	}
	syncRestored(t, next, s.ID)
	syncRestored(t, next, s.ID)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.actions != 0 {
		t.Fatalf("clean exit auto-resumed %d times (%s)", p.actions, p.lastKind)
	}
}

func TestUncleanExitResumesLastCheckpointOnce(t *testing.T) {
	b, _, s := controlFixture(t, true)
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	saved := readDocument(t, b)
	saved.Lifecycle.Phase = "running"
	saved.Lifecycle.Version = 1
	saved.Lifecycle.PID = unusedPID(t)
	saved.Lifecycle.StartedAt = "dead-process"
	saved.Lifecycle.BootID = currentBootID()
	writeDocument(t, b, saved)
	if !strings.Contains(string(readSessions(t, b)), "cadc08a9-0d73-4b44-af10-084a60b44f5b") {
		t.Fatal("last checkpoint lost the native session before restart")
	}
	next, p := reopen(t, b)
	if !next.uncleanExit {
		t.Fatal("killed process was treated as a clean exit")
	}
	launches := 0
	next.launch = func(request terminal.LaunchRequest, _ string) (process, error) {
		launches++
		p.key = request.RecoveryKey
		return p, nil
	}
	if err := next.Restore(); err != nil {
		t.Fatal(err)
	}
	if launches != 1 {
		t.Fatalf("restore launched %d providers", launches)
	}
	syncRestored(t, next, s.ID)
	syncRestored(t, next, s.ID)
	if err := next.Restore(); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if launches != 1 || p.actions != 1 || p.lastKind != "native" {
		t.Fatalf("resume launches=%d actions=%d kind=%s", launches, p.actions, p.lastKind)
	}
}

func TestUncleanExitDoesNotTouchLiveOrUnresumableSession(t *testing.T) {
	b, p, s := controlFixture(t, true)
	p.mu.Lock()
	p.mode = "active"
	p.survived = true
	p.revision++
	p.mu.Unlock()
	b.mu.Lock()
	b.uncleanExit = true
	e, r := b.entries[s.ID], b.entries[s.ID].run
	b.mu.Unlock()
	b.syncRecovery(e, r)
	p.mu.Lock()
	actions, kind := p.actions, p.lastKind
	p.mu.Unlock()
	if actions != 0 || kind != "" {
		t.Fatalf("live session was changed: actions=%d kind=%s", actions, kind)
	}

	launches := 0
	b.mu.Lock()
	b.launch = func(terminal.LaunchRequest, string) (process, error) {
		launches++
		return fake(), nil
	}
	b.mu.Unlock()
	if err := b.Restore(); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	live := b.entries[s.ID] != nil && b.entries[s.ID].run != nil
	b.mu.Unlock()
	if launches != 0 || !live {
		t.Fatalf("live session was duplicated, launches=%d live=%v", launches, live)
	}

	p.mu.Lock()
	p.mode = "waiting"
	p.survived = false
	p.nativeEvidence = "launch-selector"
	p.revision++
	p.mu.Unlock()
	b.syncRecovery(e, r)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.actions != 0 {
		t.Fatal("unclean exit resumed a conversation that has no process-state handle", p.actions)
	}
}

func TestLivingProcessIsNotAnUncleanExit(t *testing.T) {
	b, _, _ := controlFixture(t, true)
	b.checkpoint()
	next, err := newBroker(b.store, nil, func(terminal.LaunchRequest, string) (process, error) { return fake(), nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = next.Close() })
	if next.uncleanExit {
		t.Fatal("a still-running webmux was treated as killed")
	}
}

func unusedPID(t *testing.T) int {
	t.Helper()
	for pid := 1 << 20; pid < 1<<20+4096; pid++ {
		if !observeProcess(pid).Alive {
			return pid
		}
	}
	t.Fatal("no unused pid")
	return 0
}

func readSessions(t *testing.T, b *Broker) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(b.store.Home, "data", "sessions", "sessions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func readDocument(t *testing.T, b *Broker) document {
	t.Helper()
	var saved document
	if err := b.store.ReadSessions(&saved); err != nil {
		t.Fatal(err)
	}
	return saved
}

func writeDocument(t *testing.T, b *Broker, saved document) {
	t.Helper()
	if err := b.store.WriteSessions(saved); err != nil {
		t.Fatal(err)
	}
}

func plantLifecycle(t *testing.T, b *Broker, life Lifecycle) {
	t.Helper()
	saved := readDocument(t, b)
	saved.Lifecycle = life
	writeDocument(t, b, saved)
}

func reopen(t *testing.T, previous *Broker) (*Broker, *controlProcess) {
	t.Helper()
	p := &controlProcess{fakeProcess: fake(), revision: 7, mode: "waiting", nativeEvidence: "process-state"}
	next, err := newBroker(previous.store, nil, func(request terminal.LaunchRequest, _ string) (process, error) {
		p.key = request.RecoveryKey
		return p, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = next.Close() })
	return next, p
}

func syncRestored(t *testing.T, b *Broker, id string) {
	t.Helper()
	b.mu.Lock()
	e := b.entries[id]
	var r *run
	if e != nil {
		r = e.run
	}
	b.mu.Unlock()
	if e == nil || r == nil {
		t.Fatal("restored session has no provider")
	}
	b.syncRecovery(e, r)
}
