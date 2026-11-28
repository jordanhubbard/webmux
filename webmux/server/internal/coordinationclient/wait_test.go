package coordinationclient

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

func TestBoundedQueueWaitOnlyObservesAndReauthenticates(t *testing.T) {
	calls := 0
	previous := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request coordination.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if r.Header.Get("Authorization") != "Bearer wmc1.fixture" || request.Action != "inspect-reservation" || request.Record != strings.Repeat("a", 64) || request.ID == previous {
			t.Error("wait acquired or lost actor/idempotency boundary", request)
		}
		previous = request.ID
		calls++
		_ = json.NewEncoder(w).Encode(coordination.Result{Status: "queued", Record: request.Record, Queue: &coordination.QueuePosition{Sequence: 1, Position: 1, Ready: calls > 1, State: "waiting"}})
	}))
	defer server.Close()
	var out bytes.Buffer
	err := waitReservation([]string{"--resource", "node:fixture:heavy", "--record-id", strings.Repeat("a", 64), "--request-id", "one-wait-attempt", "--timeout-seconds", "3"}, Access{Endpoint: server.URL, Capability: "wmc1.fixture"}, &out)
	if err != nil || calls != 2 || !strings.Contains(out.String(), `"ready":true`) {
		t.Fatal(calls, out.String(), err)
	}
}

func TestBoundedQueueWaitTimeoutDoesNotCancelOrAcquire(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request coordination.Request
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request.Action != "inspect-reservation" {
			t.Error(request)
		}
		_ = json.NewEncoder(w).Encode(coordination.Result{Status: "queued", Queue: &coordination.QueuePosition{Position: 2}})
	}))
	defer server.Close()
	var out bytes.Buffer
	err := waitReservation([]string{"--resource", "node:fixture:heavy", "--record-id", strings.Repeat("a", 64), "--request-id", "bounded-wait-attempt", "--timeout-seconds", "1"}, Access{Endpoint: server.URL, Capability: "wmc1.fixture"}, &out)
	if err == nil || !strings.Contains(err.Error(), "deadline") || !strings.Contains(out.String(), `"status":"queued"`) {
		t.Fatal(out.String(), err)
	}
}

func TestQueueCLIRejectsWrongExecutorBeforeAnyRequest(t *testing.T) {
	requested := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requested = true }))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "access.json")
	if err := WriteAccess(path, Access{Contract: coordination.Contract, Endpoint: server.URL, Capability: "wmc1.fixture-private", Subject: coordination.Ref{Session: "actual-executor"}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBMUX_COORDINATION_FILE", path)
	t.Setenv("WEBMUX_COORDINATION_EXPECTED_SESSION", "intended-other-executor")
	for _, args := range [][]string{{"guide"}, {"act"}, {"wait", "--resource", "node:fixture:heavy", "--record-id", strings.Repeat("a", 64), "--request-id", "one-wait-attempt"}} {
		var out bytes.Buffer
		err := Run(args, strings.NewReader(`{"request_id":"wrong-executor-enqueue","action":"enqueue-reservation","resource":"node:fixture:heavy"}`), &out)
		if err == nil || !strings.Contains(err.Error(), "another session") || requested || strings.Contains(out.String(), "wmc1.fixture-private") {
			t.Fatal(args, out.String(), err)
		}
	}
}
