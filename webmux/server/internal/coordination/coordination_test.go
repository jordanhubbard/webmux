package coordination

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

var testNode = strings.Repeat("a", 32)

func membersFixture() []MemberInput {
	project := &Binding{ID: "repository:one", Repository: "https://example.test/team/one", Worktree: "/work/one", Branch: "main", Source: "verified-git"}
	second := *project
	second.Worktree = "/work/one-linked"
	other := *project
	other.ID = "repository:other"
	other.Repository = "https://example.test/team/other"
	return []MemberInput{
		{Ref: Ref{testNode, "owner", "one", strings.Repeat("1", 32)}, Active: true, Transport: "local", ExecutionHost: "host-a", Project: project},
		{Ref: Ref{testNode, "owner", "two", strings.Repeat("2", 32)}, Active: true, Transport: "exec", ExecutionHost: "host-a", Project: &second},
		{Ref: Ref{testNode, "owner", "unrelated", strings.Repeat("3", 32)}, Active: true, Transport: "ssh", ExecutionHost: "host-b", Project: &other},
		{Ref: Ref{testNode, "foreign", "private", strings.Repeat("4", 32)}, Active: true, Transport: "local", ExecutionHost: "host-a", Project: project},
	}
}

type harness struct {
	s      *Service
	inputs []MemberInput
	tokens []string
	now    time.Time
	home   string
}

func fixture(t *testing.T) *harness {
	t.Helper()
	return fixtureAt(t, time.Now().UTC())
}

func fixtureAt(t *testing.T, now time.Time) *harness {
	t.Helper()
	h := &harness{home: t.TempDir(), inputs: membersFixture(), now: now}
	h.s = New(h.home)
	h.s.Now = func() time.Time { return h.now }
	t.Cleanup(func() { _ = h.s.Close() })
	if err := h.s.Sync(testNode, h.inputs); err != nil {
		t.Fatal(err)
	}
	for _, input := range h.inputs {
		token, err := h.s.Provision(testNode, input)
		if err != nil {
			t.Fatal(err)
		}
		h.tokens = append(h.tokens, token)
	}
	return h
}
func (h *harness) execute(t *testing.T, who int, r Request) Result {
	t.Helper()
	v, err := h.s.Execute(testNode, h.tokens[who], h.inputs, r)
	if err != nil {
		t.Fatal(r.Action, err)
	}
	return v
}
func (h *harness) view(t *testing.T, who int) View {
	t.Helper()
	v, err := h.s.AgentView(testNode, h.tokens[who], h.inputs, 0)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func messagesOfKind(messages []Message, kind string) []Message {
	out := []Message{}
	for _, m := range messages {
		if m.Kind == kind {
			out = append(out, m)
		}
	}
	return out
}

func TestMembershipIsDurableAndProjectAccountScoped(t *testing.T) {
	h := fixture(t)
	for _, who := range []int{0, 1} {
		view := h.view(t, who)
		if len(view.Members) != 2 || view.Self.Ref != h.inputs[who].Ref {
			t.Fatal(view.Members)
		}
	}
	if len(h.view(t, 2).Members) != 1 || len(h.view(t, 3).Members) != 1 {
		t.Fatal("unrelated repository or account leaked")
	}
	owner, err := h.s.OwnerView(testNode, "owner", h.inputs)
	if err != nil || !owner.Enabled || len(owner.Members) != 3 {
		t.Fatal(owner, err)
	}
	if len(messagesOfKind(h.view(t, 0).Inbox, "peer-joined")) != 1 {
		t.Fatal("missing automatic join notification")
	}
	if err = h.s.Sync(testNode, h.inputs); err != nil {
		t.Fatal(err)
	}
	if len(messagesOfKind(h.view(t, 0).Inbox, "peer-joined")) != 1 {
		t.Fatal("reconnect duplicated join notification")
	}
	if err = h.s.Close(); err != nil {
		t.Fatal(err)
	}
	h.s = New(h.home)
	h.s.Now = func() time.Time { return h.now }
	if len(messagesOfKind(h.view(t, 0).Inbox, "peer-joined")) != 1 {
		t.Fatal("restart lost identity/inbox state")
	}
}
func TestCapabilityRevocationAndGlobalPolicy(t *testing.T) {
	h := fixture(t)
	if _, err := h.s.AgentView(testNode, "administrator-token", h.inputs, 0); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	r := Request{ID: "current-task", Action: "work", Text: "review"}
	if got := h.execute(t, 0, r); got.Status != "updated" {
		t.Fatal(got)
	}
	h.inputs[0].Active = false
	if _, err := h.s.Execute(testNode, h.tokens[0], h.inputs, r); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("inactive capability admitted", err)
	}
	h.inputs[0].Active = true
	if got := h.execute(t, 0, r); got.Status != "updated" {
		t.Fatal(got)
	}
	removed := h.inputs[0]
	h.inputs = h.inputs[1:]
	if _, err := h.s.Execute(testNode, h.tokens[0], h.inputs, r); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("deleted capability admitted", err)
	}
	// Failed authentication still commits the authoritative tombstone.
	h.inputs = append([]MemberInput{removed}, h.inputs...)
	if _, err := h.s.AgentView(testNode, h.tokens[0], h.inputs, 0); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("stale incarnation revived", err)
	}
	if err := h.s.SetEnabled(testNode, false); err != nil {
		t.Fatal(err)
	}
	if _, err := h.s.AgentView(testNode, h.tokens[1], h.inputs, 0); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
	view, err := h.s.OwnerView(testNode, "owner", h.inputs)
	if err != nil || view.Enabled {
		t.Fatal(view, err)
	}
	if err = h.s.SetEnabled(testNode, true); err != nil {
		t.Fatal(err)
	}
	if h.view(t, 1).Self.Ref != h.inputs[1].Ref {
		t.Fatal("policy change replaced session identity")
	}
}
func TestHandoffReceiptsAndLostResponseRetries(t *testing.T) {
	h := fixture(t)
	target := h.inputs[1].Ref
	r := Request{ID: "handoff-original", Action: "message", Kind: "handoff", Target: &target, Text: "Please review the ownership fix.\nKeep the original provider."}
	first := h.execute(t, 0, r)
	if first.Status != "admitted" {
		t.Fatal(first)
	}
	if repeat := h.execute(t, 0, r); !reflect.DeepEqual(first, repeat) {
		t.Fatal("retry changed admission", first, repeat)
	}
	view := h.view(t, 1)
	messages := messagesOfKind(view.Inbox, "handoff")
	if len(messages) != 1 || messages[0].State != "admitted" {
		t.Fatal(messages)
	}
	// Reading a response is not proof the caller received it. An explicit receipt
	// advances delivery; lost GET responses leave the message admitted.
	if m := messagesOfKind(h.view(t, 1).Inbox, "handoff"); len(m) != 1 || m[0].State != "admitted" {
		t.Fatal(m)
	}
	early := Request{ID: "early-ack-request", Action: "acknowledge", Record: first.Record}
	if got := h.execute(t, 1, early); got.Status != "receipt-order" {
		t.Fatal(got)
	}
	for _, step := range []struct{ action, state string }{{"delivered", "delivered"}, {"acknowledge", "acknowledged"}, {"complete", "completed"}} {
		got := h.execute(t, 1, Request{ID: "receipt-" + step.action, Action: step.action, Record: first.Record})
		if got.Status != "recorded" || got.State != step.state {
			t.Fatal(got)
		}
	}
	if got := h.execute(t, 1, early); got.Status != "receipt-order" {
		t.Fatal("old denial was not frozen", got)
	}
	if got := h.execute(t, 3, Request{ID: "foreign-receipt", Action: "complete", Record: first.Record}); got.Status != "not-owned" {
		t.Fatal("foreign owner acknowledged handoff", got)
	}
	changed := r
	changed.Text = "different work"
	if _, err := h.s.Execute(testNode, h.tokens[0], h.inputs, changed); !errors.Is(err, ErrInvalid) {
		t.Fatal("request ID reused with different work", err)
	}
	if err := h.s.Close(); err != nil {
		t.Fatal(err)
	}
	h.s = New(h.home)
	h.s.Now = func() time.Time { return h.now }
	if got := h.execute(t, 0, r); !reflect.DeepEqual(first, got) {
		t.Fatal("restart recreated delivery", got)
	}
	if messages := messagesOfKind(h.view(t, 0).Outbox, "handoff"); len(messages) != 1 || messages[0].State != "completed" {
		t.Fatal(messages)
	}
	// Capabilities for an unrelated project cannot message this recipient.
	if got := h.execute(t, 2, Request{ID: "unrelated-send", Action: "message", Kind: "message", Target: &target, Text: "wrong project"}); got.Status != "recipient-unavailable" {
		t.Fatal(got)
	}
}
func TestClaimsNotifyOverlapWithoutPretendingToLockFiles(t *testing.T) {
	h := fixture(t)
	first := h.execute(t, 0, Request{ID: "claim-directory", Action: "claim", Kind: "directory", Resource: "src"})
	second := h.execute(t, 1, Request{ID: "claim-file-one", Action: "claim", Kind: "file", Resource: `src\main.go`})
	if first.Status != "claimed" || second.Status != "claimed" || len(second.Conflicts) != 1 {
		t.Fatal(first, second)
	}
	if len(messagesOfKind(h.view(t, 0).Inbox, "overlap")) != 1 || len(messagesOfKind(h.view(t, 1).Inbox, "overlap")) != 1 {
		t.Fatal("overlap notification missing")
	}
	h.execute(t, 1, Request{ID: "renew-file-one", Action: "claim", Kind: "file", Resource: "src/main.go"})
	if len(messagesOfKind(h.view(t, 0).Inbox, "overlap")) != 1 {
		t.Fatal("renewal duplicated overlap notification")
	}
	if got := h.execute(t, 2, Request{ID: "other-project-file", Action: "claim", Kind: "file", Resource: "src/main.go"}); len(got.Conflicts) != 0 {
		t.Fatal("unrelated project leaked conflict", got)
	}
	if got := h.execute(t, 3, Request{ID: "other-owner-file", Action: "claim", Kind: "file", Resource: "src/main.go"}); len(got.Conflicts) != 0 {
		t.Fatal("foreign owner leaked conflict", got)
	}
	h.now = h.now.Add(121 * time.Second)
	if claims := h.view(t, 0).Claims; len(claims) != 0 {
		t.Fatal("expired claims retained as active", claims)
	}
	old := h.execute(t, 0, Request{ID: "claim-directory", Action: "claim", Kind: "directory", Resource: "src"})
	if old.Record != first.Record || !old.Expires.Before(h.now) {
		t.Fatal("old request reacquired expired work", old)
	}
	fresh := h.execute(t, 0, Request{ID: "new-directory-cycle", Action: "claim", Kind: "directory", Resource: "src"})
	if fresh.Record == first.Record {
		t.Fatal("new claim cycle reused an expired identity")
	}
}
func TestWorkerReservationsAreExclusiveFencedAndIdempotent(t *testing.T) {
	h := fixture(t)
	resource := "node:" + testNode + ":heavy"
	type admission struct {
		request Request
		result  Result
		err     error
	}
	results := make(chan admission, 32)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			request := Request{ID: fmt.Sprintf("concurrent-reserve-%03d", i), Action: "reserve", Resource: resource}
			result, err := h.s.Execute(testNode, h.tokens[0], h.inputs, request)
			results <- admission{request, result, err}
		}(i)
	}
	wg.Wait()
	close(results)
	winners := []admission{}
	denials := []admission{}
	for item := range results {
		if item.err != nil {
			t.Fatal(item.err)
		}
		if item.result.Status == "reserved" {
			winners = append(winners, item)
		} else if item.result.Status == "busy" {
			denials = append(denials, item)
		} else {
			t.Fatal(item.result)
		}
	}
	if len(winners) != 1 || len(denials) != 31 || winners[0].result.Fence != 1 {
		t.Fatal("worker admitted duplicate holders", len(winners), len(denials))
	}
	winner := winners[0]
	if got := h.execute(t, 0, winner.request); !reflect.DeepEqual(got, winner.result) {
		t.Fatal("retry acquired a second reservation", got)
	}
	foreign := h.view(t, 3).Resources[0]
	if !foreign.Occupied || foreign.Holder != nil {
		t.Fatal("cross-account reservation metadata leaked", foreign)
	}
	h.now = h.now.Add(121 * time.Second)
	next := h.execute(t, 1, Request{ID: "second-reservation-cycle", Action: "reserve", Resource: resource})
	if next.Status != "reserved" || next.Fence != 2 {
		t.Fatal(next)
	}
	oldRenew := h.execute(t, 0, Request{ID: "stale-lease-renew", Action: "renew-reservation", Resource: resource, Record: winner.result.Record, Fence: 1})
	if oldRenew.Status != "lease-lost" {
		t.Fatal("stale holder renewed after replacement", oldRenew)
	}
	if denied := h.execute(t, 0, denials[0].request); denied.Status != "busy" {
		t.Fatal("uncertain denied request later acquired work", denied)
	}
	if err := h.s.Close(); err != nil {
		t.Fatal(err)
	}
	h.s = New(h.home)
	h.s.Now = func() time.Time { return h.now }
	released := h.execute(t, 1, Request{ID: "release-current-lease", Action: "release-reservation", Resource: resource, Record: next.Record, Fence: next.Fence})
	if released.Status != "released" {
		t.Fatal(released)
	}
	third := h.execute(t, 0, Request{ID: "third-reservation-cycle", Action: "reserve", Resource: resource})
	if third.Status != "reserved" || third.Fence != 3 {
		t.Fatal("restart reset the fencing counter", third)
	}
}
func TestUnknownProjectAndInvalidPathsFailClosed(t *testing.T) {
	h := fixture(t)
	h.inputs[0].Project = nil
	if got := h.execute(t, 0, Request{ID: "unknown-project-claim", Action: "claim", Kind: "task", Resource: "task"}); got.Status != "project-unavailable" {
		t.Fatal(got)
	}
	if len(h.view(t, 0).Members) != 1 {
		t.Fatal("unknown project inferred from folder or title")
	}
	h.inputs[0].Project = membersFixture()[0].Project
	for _, resource := range []string{"../outside", "/absolute", `C:\outside`, "src/../other", "src//file", "src/./file", "src/"} {
		_, err := h.s.Execute(testNode, h.tokens[0], h.inputs, Request{ID: "invalid-path-request", Action: "claim", Kind: "file", Resource: resource})
		if !errors.Is(err, ErrInvalid) {
			t.Fatal(resource, err)
		}
	}
	for _, resource := range []string{"src/file.go", `src\file.go`} {
		if _, ok := canonicalResource("file", resource); !ok {
			t.Fatal(resource)
		}
	}
	a := Claim{Project: "one", Kind: "directory", Resource: "SRC", CaseFold: true}
	b := Claim{Project: "one", Kind: "file", Resource: "src/file.go"}
	if !overlaps(a, b) {
		t.Fatal("case-fold policy missed overlap")
	}
	b.Resource = "src-other/file.go"
	if overlaps(a, b) {
		t.Fatal("directory prefix crossed path boundary")
	}
}
func TestClockRegressionAndCorruptionDoNotResetAuthority(t *testing.T) {
	h := fixture(t)
	resource := "node:" + testNode + ":heavy"
	first := h.execute(t, 0, Request{ID: "clock-guard-reserve", Action: "reserve", Resource: resource})
	h.now = h.now.Add(-time.Hour)
	if _, err := h.s.Execute(testNode, h.tokens[1], h.inputs, Request{ID: "clock-regressed-reserve", Action: "reserve", Resource: resource}); !errors.Is(err, ErrClock) {
		t.Fatal(err)
	}
	h.now = h.now.Add(time.Hour)
	if got := h.execute(t, 0, Request{ID: "clock-guard-reserve", Action: "reserve", Resource: resource}); !reflect.DeepEqual(first, got) {
		t.Fatal(got)
	}
	db, err := h.s.database()
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Update(func(tx *bolt.Tx) error { return tx.DeleteBucket([]byte("receipts")) }); err != nil {
		t.Fatal(err)
	}
	if _, err = h.s.AgentView(testNode, h.tokens[0], h.inputs, 0); err == nil {
		t.Fatal("missing receipt history was silently recreated")
	}
	if err = db.View(func(tx *bolt.Tx) error {
		if bucket(tx, "receipts") != nil {
			t.Fatal("missing history replaced")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func TestDatabaseCustodyAndPermissions(t *testing.T) {
	h := fixture(t)
	other := New(h.home)
	defer other.Close()
	started := time.Now()
	if err := other.Sync(testNode, h.inputs); err == nil || time.Since(started) > 3*time.Second {
		t.Fatal("second owner was not refused within its lock deadline", err)
	}
	info, err := os.Stat(filepath.Join(h.home, "data", "coordination", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		t.Fatal("capability signing store is not private", info.Mode())
	}
	if err = h.s.Close(); err != nil {
		t.Fatal(err)
	}
	if err = other.Sync(testNode, h.inputs); err != nil {
		t.Fatal("released database custody not reusable", err)
	}
	if err = other.Sync(strings.Repeat("b", 32), h.inputs); !errors.Is(err, ErrIdentity) {
		t.Fatal("node identity silently replaced", err)
	}
}
func TestCommittedRequestSurvivesCallerCrashBeforeResponse(t *testing.T) {
	h := fixture(t)
	token := h.tokens[0]
	if err := h.s.Close(); err != nil {
		t.Fatal(err)
	}
	credential := filepath.Join(h.home, "fixture-capability")
	if err := os.WriteFile(credential, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestCoordinationCrashHelper$")
	cmd.Env = append(os.Environ(), "WEBMUX_COORDINATION_CRASH_FIXTURE="+h.home)
	output, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 73 {
		t.Fatalf("crash fixture did not reach committed boundary: %v %s", err, output)
	}
	h.now = time.Now().UTC().Add(time.Second)
	h.s = New(h.home)
	h.s.Now = func() time.Time { return h.now }
	target := h.inputs[1].Ref
	request := Request{ID: "crashed-before-response", Action: "message", Kind: "handoff", Target: &target, Text: "Committed before the caller lost its response."}
	result := h.execute(t, 0, request)
	messages := messagesOfKind(h.view(t, 1).Inbox, "handoff")
	if result.Status != "admitted" || len(messages) != 1 || messages[0].ID != result.Record {
		t.Fatal("crash/retry duplicated or lost work", result, messages)
	}
	if h.view(t, 0).Self.Task == "uncommitted" {
		t.Fatal("uncommitted transaction survived process loss")
	}
}
func TestCoordinationCrashHelper(t *testing.T) {
	home := os.Getenv("WEBMUX_COORDINATION_CRASH_FIXTURE")
	if home == "" {
		t.Skip("explicit process-loss control")
	}
	data, err := os.ReadFile(filepath.Join(home, "fixture-capability"))
	if err != nil {
		t.Fatal(err)
	}
	service := New(home)
	inputs := membersFixture()
	target := inputs[1].Ref
	result, err := service.Execute(testNode, string(data), inputs, Request{ID: "crashed-before-response", Action: "message", Kind: "handoff", Target: &target, Text: "Committed before the caller lost its response."})
	if err != nil || result.Status != "admitted" {
		t.Fatal(result, err)
	}
	db, err := service.database()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(true)
	if err != nil {
		t.Fatal(err)
	}
	member, _, err := get[Member](bucket(tx, "members"), inputs[0].Ref.Key())
	if err != nil {
		t.Fatal(err)
	}
	member.Task = "uncommitted"
	if err = put(bucket(tx, "members"), member.Ref.Key(), member); err != nil {
		t.Fatal(err)
	}
	os.Exit(73)
}

func TestMissingAuthorityClockDoesNotResetLeases(t *testing.T) {
	h := fixture(t)
	h.execute(t, 0, Request{ID: "reserve-before-clock-loss", Action: "reserve", Resource: "node:" + testNode + ":heavy"})
	db, err := h.s.database()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error { return bucket(tx, "meta").Delete([]byte("clock")) }); err != nil {
		t.Fatal(err)
	}
	if _, err := h.s.AgentView(testNode, h.tokens[0], h.inputs, 0); err == nil {
		t.Fatal("missing authority clock was silently recreated")
	}
}
