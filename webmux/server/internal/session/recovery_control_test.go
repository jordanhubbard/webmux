package session

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/recovery"
	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

type controlProcess struct {
	*fakeProcess
	mu             sync.Mutex
	key            string
	revision       uint64
	mode           string
	actions        int
	actionError    string
	actionOutcome  string
	failPause      bool
	configuration  *recovery.NativeConfiguration
	nativeEvidence string
}

func (p *controlProcess) RecoveryRPC(_ context.Context, request recovery.RPCRequest) recovery.RPCResponse {
	p.mu.Lock()
	defer p.mu.Unlock()
	if request.Operation == "action" {
		p.actions++
		if p.actionError != "" {
			return recovery.RPCResponse{Protocol: 1, Error: p.actionError, Outcome: p.actionOutcome}
		}
		p.mode = "consumed"
		p.revision++
		return recovery.RPCResponse{Protocol: 1, Outcome: "consumed"}
	}
	if request.Recording != nil && !*request.Recording && p.failPause {
		return recovery.RPCResponse{Protocol: 1, Error: "host unavailable"}
	}
	response := recovery.RPCResponse{Protocol: 1, Revision: p.revision}
	if request.Recording != nil {
		response.Recording = *request.Recording
	}
	for _, event := range []recovery.Event{
		{Kind: "provider", Mode: "waiting", Helper: "/bin/webmux", Platform: "linux"},
		{Kind: "command", Instance: "prior-shell", Platform: "linux", Command: "codex", Cwd: "/workspace"},
		{Kind: "cli", Instance: "prior-shell", Platform: "linux", Cwd: "/workspace", CLI: "codex", Version: "0.167.0", SessionID: "cadc08a9-0d73-4b44-af10-084a60b44f5b", Configuration: p.configuration.Clone(), NativeEvidence: p.nativeEvidence},
		{Kind: "control", Mode: p.mode, Helper: "/bin/webmux", Platform: "linux", Instance: "waiting-worker"},
	} {
		packet, _ := recovery.SignedPacket(event, p.key)
		var decoder recovery.Decoder
		_, events := decoder.Feed(packet)
		response.Events = append(response.Events, events...)
	}
	return response
}

func controlFixture(t *testing.T, logging bool) (*Broker, *controlProcess, Session) {
	t.Helper()
	b, store, _ := fixture(t)
	if logging {
		enableLogging(t, store)
	}
	p := &controlProcess{fakeProcess: fake(), revision: 7, mode: "waiting", nativeEvidence: "process-state"}
	b.launch = func(request terminal.LaunchRequest, _ string) (process, error) {
		p.key = request.RecoveryKey
		return p, nil
	}
	s, err := b.Create("owner", CreateRequest{Transport: "local", Hostname: "localhost"})
	if err != nil {
		t.Fatal(err)
	}
	sendRecovery(t, b, s.ID, p.fakeProcess, recovery.Event{Kind: "provider", Mode: "waiting", Helper: "/bin/webmux", Platform: "linux"})
	eventually(t, func() bool { value, _ := b.Recovery("owner", s.ID); return value.Waiting && value.CanResumeCLI })
	return b, p, s
}

func previewAction(t *testing.T, b *Broker, id, kind string) RecoveryRequest {
	t.Helper()
	view, err := b.Recovery("owner", id)
	if err != nil {
		t.Fatal(err)
	}
	actionID, err := uuid()
	if err != nil {
		t.Fatal(err)
	}
	return RecoveryRequest{ID: actionID, Kind: kind, Revision: view.State.Revision, HostRevision: view.HostRevision, Instance: view.Instance}
}

func TestBrokerRecoveryBindsPreviewOwnerAndConsumesOnlyOnce(t *testing.T) {
	b, p, s := controlFixture(t, false)
	input := previewAction(t, b, s.ID, "native")
	if _, err := b.Recover("other", s.ID, input); !errors.Is(err, ErrNotFound) {
		t.Fatal("crossed owner boundary", err)
	}
	stale := input
	stale.HostRevision--
	if _, err := b.Recover("owner", s.ID, stale); !errors.Is(err, ErrRecoveryConflict) {
		t.Fatal("accepted stale host preview", err)
	}
	if err := b.Input("owner", s.ID, "unapproved draft"); !errors.Is(err, ErrRecoveryWaiting) {
		t.Fatal("waiting provider accepted terminal input", err)
	}
	for i := 0; i < 2; i++ {
		result, err := b.Recover("owner", s.ID, input)
		if err != nil || result.Outcome != "consumed" {
			t.Fatal(result, err)
		}
	}
	p.mu.Lock()
	actions := p.actions
	p.mu.Unlock()
	if actions != 1 {
		t.Fatalf("duplicate execution requests: %d", actions)
	}
	altered := input
	altered.Context, altered.Confirm = "changed handoff", true
	if _, err := b.Recover("owner", s.ID, altered); !errors.Is(err, ErrRecoveryConflict) {
		t.Fatal("reused action identity with different contents", err)
	}
	var saved document
	if err := b.store.ReadSessions(&saved); err != nil {
		t.Fatal(err)
	}
	if saved.Sessions[0].Recovery.Intent.Outcome != "consumed" {
		t.Fatal("consumption was not persisted")
	}
}

func TestBrokerRecoveryDoesNotReplayOrShareWithoutConsent(t *testing.T) {
	b, p, s := controlFixture(t, false)
	for _, kind := range []string{"replay", "native"} {
		input := previewAction(t, b, s.ID, kind)
		if kind == "native" {
			input.Context = "private selected output"
		}
		if _, err := b.Recover("owner", s.ID, input); err == nil {
			t.Fatal("unapproved replay/handoff reached the host")
		}
	}
	p.mu.Lock()
	actions := p.actions
	p.mu.Unlock()
	if actions != 0 {
		t.Fatal("unapproved request invoked execution host")
	}
}

func TestAutomaticRecoveryIsOptInAndDoesNotRetryAnUncertainAction(t *testing.T) {
	b, p, s := controlFixture(t, true)
	p.mu.Lock()
	p.actionError, p.actionOutcome = "response lost", "uncertain"
	p.mu.Unlock()
	input := previewAction(t, b, s.ID, "native")
	if _, err := b.Recover("owner", s.ID, input); err == nil {
		t.Fatal("uncertain action reported success")
	}
	if err := b.SetAutoRecovery("owner", s.ID, true); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	e := b.entries[s.ID]
	r := e.run
	b.mu.Unlock()
	b.syncRecovery(e, r)
	p.mu.Lock()
	actions := p.actions
	p.mu.Unlock()
	if actions != 1 {
		t.Fatal("automatic recovery retried uncertain execution")
	}
	view, _ := b.Recovery("owner", s.ID)
	if view.State.Disposition != "execution-uncertain" {
		t.Fatal("uncertainty was lost", view.State.Disposition)
	}
}

func TestAutomaticRecoveryResumesOnceAfterOptIn(t *testing.T) {
	b, p, s := controlFixture(t, true)
	p.mu.Lock()
	actions := p.actions
	p.mu.Unlock()
	if actions != 0 {
		t.Fatal("native resume ran before opt-in")
	}
	if err := b.SetAutoRecovery("owner", s.ID, true); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	e := b.entries[s.ID]
	r := e.run
	b.mu.Unlock()
	b.syncRecovery(e, r)
	b.syncRecovery(e, r)
	p.mu.Lock()
	actions = p.actions
	p.mu.Unlock()
	if actions != 1 {
		t.Fatalf("automatic resume ran %d times", actions)
	}
}

func TestAutomaticRecoveryRequiresCurrentNativeIdentityEvidence(t *testing.T) {
	for _, evidence := range []string{"", "launch-selector", "unavailable"} {
		t.Run("evidence="+evidence, func(t *testing.T) {
			b, p, s := controlFixture(t, true)
			p.mu.Lock()
			p.nativeEvidence = evidence
			p.revision++
			p.mu.Unlock()
			if err := b.SetAutoRecovery("owner", s.ID, true); err != nil {
				t.Fatal(err)
			}
			b.mu.Lock()
			e, r := b.entries[s.ID], b.entries[s.ID].run
			b.mu.Unlock()
			b.syncRecovery(e, r)
			b.syncRecovery(e, r)
			p.mu.Lock()
			actions := p.actions
			p.mu.Unlock()
			if actions != 0 {
				t.Fatal("automatic resume used an unverified active conversation", actions)
			}
			view, err := b.Recovery("owner", s.ID)
			if err != nil || view.State.Activity.NativeEvidence != evidence || !view.CanResumeCLI {
				t.Fatal("lost uncertainty or the explicit exact-session choice", view, err)
			}
			// The owner can still deliberately select the exact displayed
			// conversation. Missing provenance only excludes automatic choice.
			if _, err := b.Recover("owner", s.ID, previewAction(t, b, s.ID, "native")); err != nil {
				t.Fatal("explicit native continuation was blocked", err)
			}
		})
	}
}

func TestUnrecordedInlinePolicyDoesNotOfferOrAutomaticallyRunNativeRecovery(t *testing.T) {
	b, p, s := controlFixture(t, true)
	p.mu.Lock()
	p.configuration = &recovery.NativeConfiguration{Roots: map[string]string{}, ManualReason: "Original inline settings require manual restoration."}
	p.revision++
	p.mu.Unlock()
	b.mu.Lock()
	e := b.entries[s.ID]
	r := e.run
	b.mu.Unlock()
	b.syncRecovery(e, r)
	if err := b.SetAutoRecovery("owner", s.ID, true); err != nil {
		t.Fatal(err)
	}
	b.syncRecovery(e, r)
	p.mu.Lock()
	actions := p.actions
	p.mu.Unlock()
	view, err := b.Recovery("owner", s.ID)
	if err != nil || actions != 0 || view.CanResumeCLI || !strings.Contains(view.Message, "Original inline settings") {
		t.Fatalf("native recovery discarded the original policy: actions=%d view=%+v error=%v", actions, view, err)
	}
}

func TestPauseRequiresHostAcknowledgementAndSurvivesRestart(t *testing.T) {
	b, p, s := controlFixture(t, true)
	p.mu.Lock()
	p.failPause = true
	p.mu.Unlock()
	if err := b.ToggleTranscript("owner", s.ID, ""); err == nil {
		t.Fatal("pause succeeded without host acknowledgement")
	}
	b.mu.Lock()
	active := b.entries[s.ID].run.log.active()
	b.mu.Unlock()
	if !active {
		t.Fatal("logging paused while host capture remained enabled")
	}
	p.mu.Lock()
	p.failPause = false
	p.mu.Unlock()
	if err := b.ToggleTranscript("owner", s.ID, ""); err != nil {
		t.Fatal(err)
	}
	next, err := newBroker(b.store, nil, func(terminal.LaunchRequest, string) (process, error) { return fake(), nil })
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if err := next.Restore(); err != nil {
		t.Fatal(err)
	}
	next.mu.Lock()
	restored := next.entries[s.ID]
	active = restored.run.log.active()
	preference := restored.value.TranscriptEnabled
	next.mu.Unlock()
	if active || preference == nil || *preference {
		t.Fatal("paused capture was reenabled by the global default")
	}
}
