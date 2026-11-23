//go:build !windows

package recovery

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/coordinationclient"
)

func TestCoordinationSocketRPCAuthenticatesWithoutStartingAProvider(t *testing.T) {
	hostFixture(t)
	root, err := os.MkdirTemp("/tmp", "wmr-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	t.Setenv("WEBMUX_COORDINATION_CACHE_HOME", root)
	r := RPCRequest{Protocol: 1, SessionID: testHostSession, Key: testHostKey, Operation: "coordination-prepare", CoordinationChannel: strings.Repeat("d", 32)}
	wrong := r
	wrong.Key = strings.Repeat("a", 32)
	if response := HostRPC(context.Background(), wrong); response.Error == "" || response.CoordinationSocket != "" {
		t.Fatal("foreign authentication prepared a channel")
	}
	prepared := HostRPC(context.Background(), r)
	if prepared.Error != "" || prepared.CoordinationSocket == "" {
		t.Fatal(prepared)
	}
	state := hostSnapshot(t)
	if state.Provider != nil || !state.Recording {
		t.Fatal("channel preparation started/replaced a provider or changed capture policy")
	}
	l, err := net.Listen("unix", prepared.CoordinationSocket)
	if err != nil {
		t.Fatal(err)
	}
	access := coordinationclient.Access{Contract: coordination.Contract, Endpoint: "http://127.0.0.1:12345", Capability: "wmc1.PRIVATE_FIXTURE_TOKEN", Subject: coordination.Ref{Node: strings.Repeat("b", 32), Owner: "owner", Session: testHostSession, Incarnation: strings.Repeat("c", 32)}}
	h := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+access.Capability {
			t.Error("forwarded source capability missing")
		}
		_ = json.NewEncoder(w).Encode(coordination.View{Contract: coordination.Contract, Enabled: true, Self: &coordination.Member{MemberInput: coordination.MemberInput{Ref: access.Subject}}})
	})}
	go h.Serve(l)
	t.Cleanup(func() { _ = h.Close() })
	r.Operation, r.CoordinationAccess = "coordination-install", &access
	response := HostRPC(context.Background(), r)
	if response.Error != "" || response.Outcome != "coordination-installed" {
		t.Fatal(response)
	}
	data, _ := json.Marshal(response)
	if strings.Contains(string(data), access.Capability) || strings.Contains(string(data), testHostKey) {
		t.Fatal("RPC response exported credentials")
	}
	locator, _ := coordinationclient.Locator(testHostSession, testHostKey)
	t.Setenv("WEBMUX_COORDINATION_FILE", locator)
	t.Setenv("WEBMUX_COORDINATION_EXPECTED_SESSION", testHostSession)
	installed, err := coordinationclient.Load()
	if err != nil || installed.Socket != prepared.CoordinationSocket {
		t.Fatal(installed.Socket, err)
	}
}
