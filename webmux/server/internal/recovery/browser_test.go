package recovery

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSealedBrowserRequestAuthenticatesSessionContentsIdentityAndExpiry(t *testing.T) {
	now := time.Now()
	raw := "https://login.example/authorize?state=private-synthetic-oauth-state"
	r, err := SealBrowserRequest(testHostSession, testHostKey, raw, now)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(r)
	if strings.Contains(string(encoded), "private-synthetic") || strings.Contains(string(encoded), "login.example") {
		t.Fatal("pending metadata contains plaintext URL")
	}
	if got, err := OpenBrowserRequest(testHostSession, testHostKey, *r, now); err != nil || got != raw {
		t.Fatal(got, err)
	}
	for _, change := range []struct {
		name   string
		mutate func(*BrowserRequest)
	}{
		{"identity", func(r *BrowserRequest) { r.ID = strings.Repeat("0", 32) }},
		{"expiry", func(r *BrowserRequest) { r.Expires++ }},
		{"ciphertext", func(r *BrowserRequest) { r.Ciphertext[len(r.Ciphertext)-1] ^= 1 }},
		{"truncated", func(r *BrowserRequest) { r.Ciphertext = r.Ciphertext[:3] }},
		{"oversized", func(r *BrowserRequest) { r.Ciphertext = make([]byte, MaxBrowserURL+100) }},
	} {
		t.Run(change.name, func(t *testing.T) {
			copy := *r
			copy.Ciphertext = append([]byte(nil), r.Ciphertext...)
			change.mutate(&copy)
			if _, err := OpenBrowserRequest(testHostSession, testHostKey, copy, now); err == nil {
				t.Fatal("accepted changed envelope")
			}
		})
	}
	if _, err := OpenBrowserRequest("7b06908a-83c3-4050-9e98-a83bbd412681", testHostKey, *r, now); err == nil {
		t.Fatal("cross-session launch")
	}
	if _, err := OpenBrowserRequest(testHostSession, strings.Repeat("0", 32), *r, now); err == nil {
		t.Fatal("cross-key launch")
	}
	if _, err := OpenBrowserRequest(testHostSession, testHostKey, *r, now.Add(BrowserRequestLifetime)); err == nil {
		t.Fatal("expired launch")
	}
	for _, raw := range []string{"file:///tmp/other-host", "javascript:alert(1)", "https://user:password@login.example", "https://example.test/" + strings.Repeat("x", MaxBrowserURL)} {
		if _, err := SealBrowserRequest(testHostSession, testHostKey, raw, now); err == nil {
			t.Fatal("invalid browser request accepted")
		}
	}
}

func TestBrowserControlIsIndependentOfRecordingAndAcknowledgesOnlyExactRequest(t *testing.T) {
	hostFixture(t)
	paused := false
	_, err := updateHostState(testHostSession, testHostKey, false, func(s *HostState) error { s.BrowserControl = true; s.Recording = false; return nil })
	if err != nil {
		t.Fatal(err)
	}
	policy := func(key, ack string) RPCResponse {
		return HostRPC(context.Background(), RPCRequest{Protocol: 1, SessionID: testHostSession, Key: key, Operation: "policy", Recording: &paused, BrowserAck: ack})
	}
	if err := QueueBrowserRequest(testHostSession, testHostKey, "https://login.example/first-private-state"); err != nil {
		t.Fatal(err)
	}
	first := policy(testHostKey, "")
	if first.Error != "" || first.Recording || first.BrowserRequest == nil || len(first.Events) != 0 {
		t.Fatal(first)
	}
	encoded, _ := json.Marshal(hostSnapshot(t))
	if strings.Contains(string(encoded), "first-private-state") {
		t.Fatal("URL entered cached activity")
	}
	if err := QueueBrowserRequest(testHostSession, testHostKey, "https://login.example/replacement"); err != nil {
		t.Fatal(err)
	}
	if wrong := policy(strings.Repeat("0", 32), first.BrowserRequest.ID); wrong.Error == "" {
		t.Fatal("foreign key acknowledged request")
	}
	second := policy(testHostKey, first.BrowserRequest.ID)
	if second.Error != "" || second.BrowserRequest == nil || second.BrowserRequest.ID == first.BrowserRequest.ID {
		t.Fatal("old acknowledgement removed replacement", second)
	}
	if got, err := OpenBrowserRequest(testHostSession, testHostKey, *second.BrowserRequest, time.Now()); err != nil || got != "https://login.example/replacement" {
		t.Fatal(got, err)
	}
	if done := policy(testHostKey, second.BrowserRequest.ID); done.Error != "" || done.BrowserRequest != nil {
		t.Fatal("matching acknowledgement did not clear request", done)
	}
	if repeated := policy(testHostKey, second.BrowserRequest.ID); repeated.Error != "" || repeated.BrowserRequest != nil {
		t.Fatal("acknowledgement not idempotent", repeated)
	}
	state := hostSnapshot(t)
	if state.Recording || state.Command != nil || state.CLI != nil || state.Shell != nil {
		t.Fatal("launch enabled activity recording", state)
	}
}

func TestDisabledBrowserControlAndExpiredPendingRequests(t *testing.T) {
	hostFixture(t)
	if err := QueueBrowserRequest(testHostSession, testHostKey, "https://login.example/disabled"); err == nil {
		t.Fatal("default provider acquired control behavior")
	}
	old, err := SealBrowserRequest(testHostSession, testHostKey, "https://login.example/old", time.Now().Add(-BrowserRequestLifetime-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	_, err = updateHostState(testHostSession, testHostKey, false, func(s *HostState) error { s.BrowserControl = true; s.BrowserRequest = old; return nil })
	if err != nil {
		t.Fatal(err)
	}
	paused := false
	response := HostRPC(context.Background(), RPCRequest{Protocol: 1, SessionID: testHostSession, Key: testHostKey, Operation: "policy", Recording: &paused})
	if response.Error != "" || response.BrowserRequest != nil || hostSnapshot(t).BrowserRequest != nil {
		t.Fatal("expired queue not pruned", response)
	}
}
