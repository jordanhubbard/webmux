package coordination

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

type remoteHarness struct {
	source   *harness
	receiver *Service
	node     string
	inputs   []MemberInput
	token    string
	route    RemoteRoute
	scope    SenderScope
}

func remoteFixture(t *testing.T) *remoteHarness {
	t.Helper()
	return remoteFixtureFromSource(t, fixture(t))
}

func remoteFixtureFromSource(t *testing.T, h *harness) *remoteHarness {
	t.Helper()
	r := &remoteHarness{source: h, receiver: New(t.TempDir()), node: strings.Repeat("b", 32)}
	// Install the fixture clock before Sync persists the authority timestamp.
	r.receiver.Now = h.s.Now
	t.Cleanup(func() { _ = r.receiver.Close() })
	input := h.inputs[0]
	input.Ref.Node = r.node
	input.Ref.Owner = "receiver"
	r.inputs = []MemberInput{input}
	if err := r.receiver.Sync(r.node, r.inputs); err != nil {
		t.Fatal(err)
	}
	var err error
	r.token, err = r.receiver.Provision(r.node, input)
	if err != nil {
		t.Fatal(err)
	}
	v, err := r.receiver.AgentView(r.node, r.token, r.inputs, 0)
	if err != nil {
		t.Fatal(err)
	}
	r.route = RemoteRoute{Project: input.Project.ID, Peer: "peer", Node: r.node, Owner: "receiver", TLSKey: strings.Repeat("c", 64), Epoch: v.HistoryEpoch}
	r.scope = SenderScope{Node: testNode, Owner: "owner", ReceiverOwner: "receiver"}
	return r
}
func (r *remoteHarness) queue(t *testing.T, id string) (Request, Result, WireMessage) {
	t.Helper()
	request := Request{ID: id, Action: "message", Target: &r.inputs[0].Ref, Kind: "handoff", Text: "Please review the patch."}
	result, err := r.source.s.ExecuteRouted(testNode, r.source.tokens[0], r.source.inputs, request, &r.route)
	if err != nil || result.Status != "admitted" || result.Transport != "pending" {
		t.Fatal(result, err)
	}
	records, err := r.source.s.RemoteRecords(testNode)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.Envelope.ID == result.Record {
			return request, result, record.Envelope
		}
	}
	t.Fatal("missing durable outbox")
	return Request{}, Result{}, WireMessage{}
}
func TestRemoteMessageLostReplyReopenAndExplicitReceipts(t *testing.T) {
	r := remoteFixture(t)
	request, result, w := r.queue(t, "handoff-remote")
	allowed, err := r.source.s.BeginRemoteAttempt(testNode, w.ID, r.source.inputs)
	if err != nil || !allowed {
		t.Fatal(allowed, err)
	}
	receipt, err := r.receiver.Exchange(r.node, r.scope, r.inputs, "admit", w)
	if err != nil || receipt.State != "admitted" || !receipt.Valid(w) {
		t.Fatal(receipt, err)
	}
	// Lose the reply and reconstruct both authorities from their durable homes.
	receiverHome := r.receiver.home
	if err := r.receiver.Close(); err != nil {
		t.Fatal(err)
	}
	r.receiver = New(receiverHome)
	if err := r.source.s.Close(); err != nil {
		t.Fatal(err)
	}
	r.source.s = New(r.source.home)
	records, err := r.source.s.RemoteRecords(testNode)
	if err != nil || len(records) != 1 || !records[0].Attempted || records[0].Admitted {
		t.Fatal(records, err)
	}
	receipt, err = r.receiver.Exchange(r.node, r.scope, r.inputs, "status", w)
	if err != nil || receipt.State != "admitted" {
		t.Fatal(receipt, err)
	}
	if err = r.source.s.ApplyRemoteReceipt(testNode, w, receipt); err != nil {
		t.Fatal(err)
	}
	repeat, err := r.source.s.ExecuteRouted(testNode, r.source.tokens[0], r.source.inputs, request, nil)
	if err != nil || !reflect.DeepEqual(result, repeat) {
		t.Fatal("original intent changed", repeat, err)
	}
	request.Text = "different"
	if _, err = r.source.s.ExecuteRouted(testNode, r.source.tokens[0], r.source.inputs, request, &r.route); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		view, err := r.receiver.AgentView(r.node, r.token, r.inputs, 0)
		if err != nil {
			t.Fatal(err)
		}
		messages := messagesOfKind(view.Inbox, "handoff")
		if len(messages) != 1 || messages[0].State != "admitted" {
			t.Fatal(messages)
		}
	}
	for _, action := range []string{"delivered", "acknowledge", "complete"} {
		got, err := r.receiver.Execute(r.node, r.token, r.inputs, Request{ID: "receipt-" + action, Action: action, Record: w.ID})
		if err != nil || got.Status != "recorded" {
			t.Fatal(got, err)
		}
		latest, err := r.receiver.Exchange(r.node, r.scope, r.inputs, "admit", w)
		if err != nil || latest.State != got.State {
			t.Fatal(latest, err)
		}
		if err = r.source.s.ApplyRemoteReceipt(testNode, w, latest); err != nil {
			t.Fatal(err)
		}
		if err = r.source.s.ApplyRemoteReceipt(testNode, w, receipt); err != nil {
			t.Fatal(err)
		} // old reply cannot regress
	}
	view, err := r.source.s.AgentView(testNode, r.source.tokens[0], r.source.inputs, 0)
	if err != nil {
		t.Fatal(err)
	}
	messages := messagesOfKind(view.Outbox, "handoff")
	if len(messages) != 1 || messages[0].State != "completed" {
		t.Fatal(messages)
	}
	records, err = r.source.s.RemoteRecords(testNode)
	if err != nil || len(records) != 0 {
		t.Fatal(records, err)
	}
}
func TestRemoteMessageRefusalsAndHistoryBoundary(t *testing.T) {
	r := remoteFixture(t)
	_, _, w := r.queue(t, "remote-boundary")
	bad := r.scope
	bad.Owner = "foreign"
	if _, err := r.receiver.Exchange(r.node, bad, r.inputs, "admit", w); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	receipt, err := r.receiver.Exchange(r.node, r.scope, r.inputs, "status", w)
	if err != nil || receipt.State != "absent" {
		t.Fatal(receipt, err)
	}
	r.inputs[0].Active = false
	receipt, err = r.receiver.Exchange(r.node, r.scope, r.inputs, "admit", w)
	if err != nil || receipt.State != "refused" {
		t.Fatal(receipt, err)
	}
	r.inputs[0].Active = true
	receipt, err = r.receiver.Exchange(r.node, r.scope, r.inputs, "admit", w)
	if err != nil || receipt.State != "refused" {
		t.Fatal("refusal changed", receipt, err)
	}
	if err = r.source.s.ApplyRemoteReceipt(testNode, w, receipt); err != nil {
		t.Fatal(err)
	}
	_, _, w = r.queue(t, "remote-new-history")
	fresh := New(t.TempDir())
	defer fresh.Close()
	if err = fresh.Sync(r.node, r.inputs); err != nil {
		t.Fatal(err)
	}
	receipt, err = fresh.Exchange(r.node, r.scope, r.inputs, "admit", w)
	if err != nil || receipt.State != "history-changed" || !receipt.Valid(w) {
		t.Fatal(receipt, err)
	}
	if err = r.source.s.ApplyRemoteReceipt(testNode, w, receipt); err != nil {
		t.Fatal(err)
	}
	records, err := r.source.s.RemoteRecords(testNode)
	if err != nil || len(records) != 0 {
		t.Fatal(records, err)
	}
}
func TestRemoteMessageSourceAdmissionRequiresCurrentLiveProject(t *testing.T) {
	r := remoteFixture(t)
	_, _, w := r.queue(t, "remote-live-source")
	r.source.inputs[0].Active = false
	allowed, err := r.source.s.BeginRemoteAttempt(testNode, w.ID, r.source.inputs)
	if err != nil || allowed {
		t.Fatal(allowed, err)
	}
	records, _ := r.source.s.RemoteRecords(testNode)
	if len(records) != 1 || records[0].Attempted || records[0].Transport != "sender-inactive" {
		t.Fatal(records)
	}
	r.source.inputs[0].Active = true
	r.source.inputs[0].Project = nil
	allowed, err = r.source.s.BeginRemoteAttempt(testNode, w.ID, r.source.inputs)
	if err != nil || allowed {
		t.Fatal(allowed, err)
	}
	records, _ = r.source.s.RemoteRecords(testNode)
	if len(records) != 0 {
		t.Fatal(records)
	}
}
func TestRemoteSchemaMigrationPreservesCapabilitiesAndFailsClosed(t *testing.T) {
	h := fixture(t)
	request := Request{ID: "preserved-intent", Action: "work", Text: "review"}
	want := h.execute(t, 0, request)
	db, _ := h.s.database()
	if err := db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{"remote_outbox", "remote_inbox", "remote_active"} {
			if err := tx.DeleteBucket([]byte(name)); err != nil {
				return err
			}
		}
		if err := bucket(tx, "meta").Delete([]byte("history_epoch")); err != nil {
			return err
		}
		return bucket(tx, "meta").Put([]byte("version"), []byte("1"))
	}); err != nil {
		t.Fatal(err)
	}
	if got := h.execute(t, 0, request); !reflect.DeepEqual(got, want) {
		t.Fatal(got, want)
	}
	if h.view(t, 0).HistoryEpoch == "" {
		t.Fatal("migration omitted epoch")
	}
	if err := db.Update(func(tx *bolt.Tx) error { return tx.DeleteBucket([]byte("remote_inbox")) }); err != nil {
		t.Fatal(err)
	}
	if _, err := h.s.AgentView(testNode, h.tokens[0], h.inputs, 0); err == nil {
		t.Fatal("missing history silently recreated")
	}
}

func TestRemoteInboxCommitSurvivesReceiverProcessLoss(t *testing.T) {
	r := remoteFixture(t)
	_, _, w := r.queue(t, "crashed-remote-inbox")
	allowed, err := r.source.s.BeginRemoteAttempt(testNode, w.ID, r.source.inputs)
	if err != nil || !allowed {
		t.Fatal(allowed, err)
	}
	home := r.receiver.home
	if err = r.receiver.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(home, "wire.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(executable, "-test.run=^TestRemoteInboxCrashHelper$")
	child.Env = append(os.Environ(), "WEBMUX_REMOTE_INBOX_CRASH="+home)
	output, err := child.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 73 {
		t.Fatalf("receiver did not reach committed crash boundary: %v %s", err, output)
	}
	r.receiver = New(home)
	receipt, err := r.receiver.Exchange(r.node, r.scope, r.inputs, "status", w)
	if err != nil || receipt.State != "admitted" {
		t.Fatal(receipt, err)
	}
	if err = r.source.s.ApplyRemoteReceipt(testNode, w, receipt); err != nil {
		t.Fatal(err)
	}
	receipt, err = r.receiver.Exchange(r.node, r.scope, r.inputs, "admit", w)
	if err != nil || receipt.State != "admitted" {
		t.Fatal(receipt, err)
	}
	view, err := r.receiver.AgentView(r.node, r.token, r.inputs, 0)
	if err != nil || len(messagesOfKind(view.Inbox, "handoff")) != 1 {
		t.Fatal(view.Inbox, err)
	}
	// An accepted dedupe entry with missing message history is corruption, never absence.
	db, _ := r.receiver.database()
	if err = db.Update(func(tx *bolt.Tx) error { return bucket(tx, "messages").Delete([]byte(w.ID)) }); err != nil {
		t.Fatal(err)
	}
	if _, err = r.receiver.Exchange(r.node, r.scope, r.inputs, "status", w); err == nil {
		t.Fatal("lost accepted history was treated as absent")
	}
}
func TestRemoteInboxCrashHelper(t *testing.T) {
	home := os.Getenv("WEBMUX_REMOTE_INBOX_CRASH")
	if home == "" {
		t.Skip("explicit receiver process loss control")
	}
	data, err := os.ReadFile(filepath.Join(home, "wire.json"))
	if err != nil {
		t.Fatal(err)
	}
	var w WireMessage
	if err = json.Unmarshal(data, &w); err != nil {
		t.Fatal(err)
	}
	input := membersFixture()[0]
	input.Ref = w.To
	service := New(home)
	receipt, err := service.Exchange(w.To.Node, SenderScope{Node: w.From.Node, Owner: w.From.Owner, ReceiverOwner: w.To.Owner}, []MemberInput{input}, "admit", w)
	if err != nil || receipt.State != "admitted" {
		t.Fatal(receipt, err)
	}
	os.Exit(73)
}

func TestRemoteOutboxRefusesDamagedAuthorityBeforeNetworkSelection(t *testing.T) {
	for _, damage := range []string{"version", "key", "clock", "remote_inbox"} {
		t.Run(damage, func(t *testing.T) {
			r := remoteFixture(t)
			r.queue(t, "damaged-outbox-intent")
			db, _ := r.source.s.database()
			if err := db.Update(func(tx *bolt.Tx) error {
				if damage == "remote_inbox" {
					return tx.DeleteBucket([]byte(damage))
				}
				return bucket(tx, "meta").Put([]byte(damage), []byte("invalid"))
			}); err != nil {
				t.Fatal(err)
			}
			if records, err := r.source.s.RemoteRecords(testNode); err == nil || len(records) != 0 {
				t.Fatal("damaged outbox selected delivery work", records, err)
			}
		})
	}
}

// A source initialized in the past deterministically represents receiver setup
// taking longer than prepareClaims' former one-second clock padding. No sleep
// or machine-speed assumption is needed to expose the wall-clock handoff.
func TestRemoteFixtureClockHandoffAfterSlowSetup(t *testing.T) {
	h := fixtureAt(t, time.Now().UTC().Add(-24*time.Hour))
	r := remoteFixtureFromSource(t, h)
	r.source.now = r.source.now.Add(time.Second)
	r.receiver.Now = func() time.Time { return r.source.now }
	result, err := r.receiver.Execute(r.node, r.token, r.inputs, Request{
		ID: "slow-setup-claim", Action: "claim", Kind: "file", Resource: "src/main.go",
	})
	if err != nil || result.Status != "claimed" {
		t.Fatalf("receiver initialized outside the fixture clock: %+v %v", result, err)
	}
}
