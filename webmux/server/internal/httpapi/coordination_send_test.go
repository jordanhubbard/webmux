package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func sendIntentFixture(t *testing.T) (string, coordinationSendInput) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Agent Deck delivery requires a Unix host with directory Sync; Windows routes refuse delivery")
	}
	home := t.TempDir()
	return home, coordinationSendInput{Scope: coordinationSendScope(home, "owner"), RequestID: "c19a6ca1-ce5c-4c51-9750-1e7ab6f6fcaa", Provider: "agent-deck", coordinationSendRequest: coordinationSendRequest{Target: "local:worker", Message: "first instruction", Queue: true}}
}
func TestCoordinationSendClaimSurvivesConcurrencyResponseLossAndReconstruction(t *testing.T) {
	home, input := sendIntentFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := consumeCoordinationSend(context.Background(), home, "owner", input, func(context.Context, coordinationSendRequest) (coordinationSendResult, error) {
			calls.Add(1)
			close(started)
			<-release
			return coordinationSendResult{}, errors.New("reply lost after owner accepted")
		})
		if err != nil {
			t.Error(err)
		}
	}()
	<-started
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := consumeCoordinationSend(context.Background(), home, "owner", input, func(context.Context, coordinationSendRequest) (coordinationSendResult, error) {
				calls.Add(1)
				return coordinationSendResult{}, nil
			})
			if err != nil || out.RequestState != "uncertain" {
				t.Errorf("duplicate: %+v %v", out, err)
			}
		}()
	}
	close(release)
	wg.Wait()
	out, err := consumeCoordinationSend(context.Background(), home, "owner", input, func(context.Context, coordinationSendRequest) (coordinationSendResult, error) {
		t.Fatal("reconstructed request redispatched")
		return coordinationSendResult{}, nil
	})
	if err != nil || out.RequestState != "uncertain" || calls.Load() != 1 {
		t.Fatal(out, err, calls.Load())
	}
	inspected, err := inspectCoordinationSend(home, "owner", input.RequestID)
	if err != nil || inspected.Request == nil || inspected.Request.Message != input.Message {
		t.Fatal(inspected, err)
	}
	for _, changed := range []coordinationSendInput{func() coordinationSendInput { v := input; v.Message = "changed"; return v }(), func() coordinationSendInput { v := input; v.Target = "remote:other"; return v }(), func() coordinationSendInput { v := input; v.Queue = false; return v }()} {
		if _, err := consumeCoordinationSend(context.Background(), home, "owner", changed, func(context.Context, coordinationSendRequest) (coordinationSendResult, error) {
			t.Fatal("changed intent dispatched")
			return coordinationSendResult{}, nil
		}); err == nil {
			t.Fatal("changed body accepted")
		}
	}
	if _, err := consumeCoordinationSend(context.Background(), home, "other", input, func(context.Context, coordinationSendRequest) (coordinationSendResult, error) {
		t.Fatal("other owner dispatched")
		return coordinationSendResult{}, nil
	}); err == nil {
		t.Fatal("other owner obtained original")
	}
	if _, err := inspectCoordinationSend(home, "other", input.RequestID); err == nil {
		t.Fatal("instruction leaked")
	}
}
func TestCoordinationSendCompletionAndPartialOutcomeNeverRedispatch(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "partial"}[partial], func(t *testing.T) {
			home, input := sendIntentFixture(t)
			calls := 0
			out, err := consumeCoordinationSend(context.Background(), home, "owner", input, func(context.Context, coordinationSendRequest) (coordinationSendResult, error) {
				calls++
				if partial {
					if err := os.WriteFile(coordinationSendPath(home, input.RequestID, "result"), []byte(`{"owner":`), 0600); err != nil {
						t.Fatal(err)
					}
				}
				return coordinationSendResult{Target: input.Target, SendID: "native-original", State: "queued", Queued: true}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if partial && out.RequestState != "uncertain" || !partial && (out.RequestState != "completed" || out.SendID != "native-original") {
				t.Fatal(out)
			}
			again, err := consumeCoordinationSend(context.Background(), home, "owner", input, func(context.Context, coordinationSendRequest) (coordinationSendResult, error) {
				calls++
				return coordinationSendResult{}, nil
			})
			if err != nil || calls != 1 || again.RequestState != out.RequestState || again.SendID != out.SendID {
				t.Fatal(again, err, calls)
			}
		})
	}
}
func TestCoordinationSendDismissFencesDelayedOriginalAndCannotUndoDispatch(t *testing.T) {
	home, input := sendIntentFixture(t)
	out, err := dismissCoordinationSend(home, "owner", input.RequestID)
	if err != nil || out.RequestState != "cancelled" {
		t.Fatal(out, err)
	}
	out, err = consumeCoordinationSend(context.Background(), home, "owner", input, func(context.Context, coordinationSendRequest) (coordinationSendResult, error) {
		t.Fatal("delayed request escaped fence")
		return coordinationSendResult{}, nil
	})
	if err != nil || out.RequestState != "cancelled" {
		t.Fatal(out, err)
	}
	home, input = sendIntentFixture(t)
	_, err = consumeCoordinationSend(context.Background(), home, "owner", input, func(context.Context, coordinationSendRequest) (coordinationSendResult, error) {
		return coordinationSendResult{}, errors.New("unknown")
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err = dismissCoordinationSend(home, "owner", input.RequestID)
	if err != nil || out.RequestState != "uncertain" {
		t.Fatal("dismiss invented cancellation", out, err)
	}
}
func TestCoordinationSendDismissDispatchRaceAndInterruptedClaim(t *testing.T) {
	for i := 0; i < 12; i++ {
		home, input := sendIntentFixture(t)
		var calls atomic.Int32
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = dismissCoordinationSend(home, "owner", input.RequestID) }()
		go func() {
			defer wg.Done()
			_, _ = consumeCoordinationSend(context.Background(), home, "owner", input, func(context.Context, coordinationSendRequest) (coordinationSendResult, error) {
				calls.Add(1)
				return coordinationSendResult{Target: input.Target, State: "queued"}, nil
			})
		}()
		wg.Wait()
		out, err := inspectCoordinationSend(home, "owner", input.RequestID)
		if err != nil || calls.Load() > 1 || (out.RequestState == "cancelled" && calls.Load() != 0) {
			t.Fatal(out, err, calls.Load())
		}
	}
	home, input := sendIntentFixture(t)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected simulated interruption")
			}
		}()
		_, _ = consumeCoordinationSend(context.Background(), home, "owner", input, func(context.Context, coordinationSendRequest) (coordinationSendResult, error) {
			panic("controller died")
		})
	}()
	out, err := consumeCoordinationSend(context.Background(), home, "owner", input, func(context.Context, coordinationSendRequest) (coordinationSendResult, error) {
		t.Fatal("interrupted dispatch repeated")
		return coordinationSendResult{}, nil
	})
	if err != nil || out.RequestState != "uncertain" {
		t.Fatal(out, err)
	}
}
func TestCoordinationSendLargePrivateIntentAndStorageRefusal(t *testing.T) {
	home, input := sendIntentFixture(t)
	input.Message = strings.Repeat("<", 256*1024)
	_, err := consumeCoordinationSend(context.Background(), home, "owner", input, func(context.Context, coordinationSendRequest) (coordinationSendResult, error) {
		return coordinationSendResult{}, coordinationSendRefusal("provider_unavailable")
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := inspectCoordinationSend(home, "owner", input.RequestID)
	if err != nil || out.RequestState != "refused" || out.Request == nil || out.Request.Message != input.Message {
		t.Fatal("large private intent", err)
	}
	info, err := os.Stat(coordinationSendPath(home, input.RequestID, "intent"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
	home = filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(home, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = consumeCoordinationSend(context.Background(), home, "owner", input, func(context.Context, coordinationSendRequest) (coordinationSendResult, error) {
		t.Fatal("storage failure reached provider")
		return coordinationSendResult{}, nil
	})
	if err == nil {
		t.Fatal("storage failure accepted")
	}
}
func TestCoordinationSendHTTPRepeatedAcceptedRequestKeepsOwnerSendID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Agent Deck uses Unix interactive transports")
	}
	s, handler := fixture(t, "none")
	setCoordinationIntegrations(t, s, "agent-deck")
	executable, log := fakeAgentDeck(t)
	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	body := `{"scope":"` + coordinationSendScope(s.store.Home, "anonymous") + `","requestId":"c19a6ca1-ce5c-4c51-9750-1e7ab6f6fcaa","provider":"agent-deck","target":"local:local-1","message":"one original instruction","queue":true}`
	for i := 0; i < 2; i++ {
		reply := request(handler, "POST", "/api/coordination/messages", body, "")
		requireStatus(t, reply, 200)
		if !strings.Contains(reply.Body.String(), `"sendId":"send-local"`) {
			t.Fatal(reply.Body.String())
		}
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(data), "session send local-1 --message-file - --json --require-input-prompt --queue"); count != 1 {
		t.Fatalf("owner invoked %d times; want exactly one: %s", count, data)
	}
	reply := scopedInstructionRequest(handler, "GET", "/api/coordination/send-intents/c19a6ca1-ce5c-4c51-9750-1e7ab6f6fcaa", coordinationSendScope(s.store.Home, "anonymous"))
	requireStatus(t, reply, 200)
	var out coordinationSendOutcome
	if json.Unmarshal(reply.Body.Bytes(), &out) != nil || out.Request == nil || out.Request.Message != "one original instruction" || out.RequestState != "completed" {
		t.Fatal(reply.Body.String())
	}
	if reply.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("instruction may be cached")
	}
}

func TestCoordinationSendProcessHelper(t *testing.T) {
	if os.Getenv("WEBMUX_SEND598_HELPER") != "1" {
		return
	}
	input := coordinationSendInput{Scope: coordinationSendScope(os.Getenv("WEBMUX_SEND598_HOME"), "owner"), RequestID: "c19a6ca1-ce5c-4c51-9750-1e7ab6f6fcaa", Provider: "agent-deck", coordinationSendRequest: coordinationSendRequest{Target: "local:worker", Message: "cross-process original", Queue: true}}
	_, _ = consumeCoordinationSend(context.Background(), os.Getenv("WEBMUX_SEND598_HOME"), "owner", input, func(context.Context, coordinationSendRequest) (coordinationSendResult, error) {
		f, err := os.OpenFile(filepath.Join(os.Getenv("WEBMUX_SEND598_HOME"), "provider-calls"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.WriteString("dispatch\n")
		if closeErr := f.Close(); err != nil || closeErr != nil {
			t.Fatal(err, closeErr)
		}
		time.Sleep(50 * time.Millisecond)
		return coordinationSendResult{Target: input.Target, SendID: "one-cross-process-send", State: "queued", Queued: true}, nil
	})
}
func TestCoordinationSendCrossProcessAndFreshReaderShareOneDispatch(t *testing.T) {
	home, _ := sendIntentFixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCoordinationSendProcessHelper$")
			cmd.Env = append(os.Environ(), "WEBMUX_SEND598_HELPER=1", "WEBMUX_SEND598_HOME="+home)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("helper: %v %s", err, out)
			}
		}()
	}
	wg.Wait()
	data, err := os.ReadFile(filepath.Join(home, "provider-calls"))
	if err != nil || string(data) != "dispatch\n" {
		t.Fatal(string(data), err)
	}
	out, err := inspectCoordinationSend(home, "owner", "c19a6ca1-ce5c-4c51-9750-1e7ab6f6fcaa")
	if err != nil || out.SendID != "one-cross-process-send" || out.RequestState != "completed" {
		t.Fatal(out, err)
	}
}
func TestCoordinationSendInvalidClaimAndRelativeHomeFailClosed(t *testing.T) {
	home, input := sendIntentFixture(t)
	intent := coordinationSendIntent{Owner: "owner", Input: input, Hash: coordinationSendHash(input)}
	if err := writeRigStopRecord(home, coordinationSendPath(home, input.RequestID, "intent"), intent); err != nil {
		t.Fatal(err)
	}
	claim := coordinationSendPath(home, input.RequestID, "claim")
	if err := os.WriteFile(claim, []byte(`{"owner":`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := consumeCoordinationSend(context.Background(), home, "owner", input, func(context.Context, coordinationSendRequest) (coordinationSendResult, error) {
		t.Fatal("partial claim replayed")
		return coordinationSendResult{}, nil
	})
	if err == nil {
		t.Fatal("partial claim was trusted")
	}
	other, original := sendIntentFixture(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, other)
	if err != nil {
		t.Fatal(err)
	}
	_, err = consumeCoordinationSend(context.Background(), relative, "owner", original, func(context.Context, coordinationSendRequest) (coordinationSendResult, error) {
		return coordinationSendResult{Target: original.Target, State: "queued"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := inspectCoordinationSend(other, "owner", original.RequestID)
	if err != nil || out.RequestState != "completed" {
		t.Fatal(out, err)
	}
}
func TestCoordinationSendHTTPRefusesInvalidRequestBeforeOwnerDiscovery(t *testing.T) {
	s, handler := fixture(t, "none")
	setCoordinationIntegrations(t, s, "agent-deck")
	for _, body := range []string{
		`{"provider":"agent-deck","target":"local:worker","message":"no UUID","queue":true}`,
		`{"requestId":"../../outside","provider":"agent-deck","target":"local:worker","message":"invalid UUID","queue":true}`,
		`{"requestId":"c19a6ca1-ce5c-4c51-9750-1e7ab6f6fcaa","provider":"agent-deck","target":"local:worker","message":"original","unexpected":true}`,
	} {
		requireStatus(t, request(handler, "POST", "/api/coordination/messages", body, ""), 400)
	}
	if _, err := os.Stat(filepath.Join(s.store.Home, "data", "coordination-sends")); !os.IsNotExist(err) {
		t.Fatal("invalid request created a ledger", err)
	}
}

func scopedInstructionRequest(handler http.Handler, method, path, scope string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("X-WebMux-Send-Scope", scope)
	reply := httptest.NewRecorder()
	handler.ServeHTTP(reply, req)
	return reply
}
func TestCoordinationSendOwnerScopeBindsUnobservedRequestsAndMetadata(t *testing.T) {
	home, input := sendIntentFixture(t)
	input.Scope = coordinationSendScope(home, "another-owner")
	_, err := consumeCoordinationSend(context.Background(), home, "owner", input, func(context.Context, coordinationSendRequest) (coordinationSendResult, error) {
		t.Fatal("unobserved instruction changed owner")
		return coordinationSendResult{}, nil
	})
	if err == nil {
		t.Fatal("wrong owner scope was accepted")
	}
	if _, err := os.Stat(filepath.Join(home, "data", "coordination-sends")); !os.IsNotExist(err) {
		t.Fatal("wrong scope wrote an intent", err)
	}
	s, handler := fixture(t, "none")
	id := input.RequestID
	for _, method := range []string{"GET", "POST"} {
		path := "/api/coordination/send-intents/" + id
		if method == "POST" {
			path += "/dismiss"
		}
		requireStatus(t, scopedInstructionRequest(handler, method, path, coordinationSendScope(s.store.Home, "someone-else")), 409)
	}
	if _, err := os.Stat(filepath.Join(s.store.Home, "data", "coordination-sends")); !os.IsNotExist(err) {
		t.Fatal("wrong scope wrote a fence", err)
	}
}
