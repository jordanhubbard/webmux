package recovery

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

// Initializes only an isolated qualification cache. Native qualification then
// sends requests through the actual executable's --recovery-rpc SSH stdin path.
// No tmux provider, shell, installed configuration or server is started here.
func TestCoordinationProjectNativeInitialize(t *testing.T) {
	if os.Getenv("WEBMUX_TEST_COORDINATION_PROJECT_INITIALIZE") != "1" {
		t.Skip("isolated native SSH qualification setup only")
	}
	if os.Getenv("WEBMUX_RECOVERY_CACHE_HOME") == "" {
		t.Fatal("an explicit private qualification cache is required")
	}
	var request RPCRequest
	if err := readRPC(os.Stdin, &request); err != nil {
		t.Fatal(err)
	}
	if err := InitializeHostEvidence(request.SessionID, request.Key, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(RPCResponse{Protocol: 1, Outcome: "initialized"}); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

func TestCoordinationProjectRPCAuthenticatesAndInspectsWithoutRecording(t *testing.T) {
	hostFixture(t)
	directory := t.TempDir()
	for _, args := range [][]string{{"init"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit", "--allow-empty", "-m", "root"}, {"remote", "add", "origin", "https://example.test/team/ssh-project.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", directory}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("Git fixture: %v %s", err, out)
		}
	}
	paused := false
	policy := HostRPC(context.Background(), RPCRequest{Protocol: 1, SessionID: testHostSession, Key: testHostKey, Operation: "policy", Recording: &paused})
	if policy.Error != "" {
		t.Fatal(policy.Error)
	}
	before := hostSnapshot(t)
	request := RPCRequest{Protocol: 1, SessionID: testHostSession, Key: testHostKey, Operation: "coordination-project", CoordinationDirectory: directory}
	expected, err := coordination.InspectRepository(context.Background(), directory)
	if err != nil {
		t.Fatal(err)
	}
	response := HostRPC(context.Background(), request)
	if response.Error != "" || response.CoordinationProject == nil || *response.CoordinationProject != *expected || len(response.Events) != 0 || response.Recording {
		t.Fatalf("fixed execution-host inspection failed: %+v", response)
	}
	after := hostSnapshot(t)
	if after.Revision != before.Revision || after.Command != nil || after.CLI != nil || after.Shell != nil || after.Recording {
		t.Fatal("read-only project inspection changed capture state or retained directory history")
	}
	for name, change := range map[string]func(*RPCRequest){
		"foreign-key":     func(r *RPCRequest) { r.Key = strings.Repeat("a", 32) },
		"foreign-session": func(r *RPCRequest) { r.SessionID = "f00d0000-0000-4000-8000-000000000001" },
		"relative-path":   func(r *RPCRequest) { r.CoordinationDirectory = "relative" },
		"missing-path":    func(r *RPCRequest) { r.CoordinationDirectory = "" },
		"protocol":        func(r *RPCRequest) { r.Protocol = 2 },
	} {
		t.Run(name, func(t *testing.T) {
			copy := request
			change(&copy)
			response := HostRPC(context.Background(), copy)
			if response.Error == "" || response.CoordinationProject != nil {
				t.Fatal("unverified project escaped execution-host admission", response)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if response := HostRPC(ctx, request); response.Error == "" || response.CoordinationProject != nil {
		t.Fatal("cancelled Git inspection returned a binding", response)
	}
}
