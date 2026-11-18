package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/session"
)

func TestPublicNativeRigRecoveryKeepsTypedUncertaintyAndExactOriginalCompletion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native Rig uses POSIX tmux transport")
	}
	t.Setenv("WEBMUX_COORDINATION_DISABLED", "")
	s, handler := fixture(t, "none")
	bins := t.TempDir()
	log := filepath.Join(t.TempDir(), "operations.jsonl")
	t.Setenv("WEBMUX_NATIVE_ORIGINAL_TEST_LOG", log)
	script := `#!/usr/bin/python3
import sys,json,os
if sys.argv[2]!='native-recovery-resume':sys.exit(6)
p=json.load(sys.stdin)
with open(os.environ['WEBMUX_NATIVE_ORIGINAL_TEST_LOG'],'a') as out:out.write(json.dumps(p)+'\n')
i=p['identity'];r=json.loads(i['native_rig_recovery']);v={'operationId':p['operationId'],'nativeSessionId':r['nativeSessionId'],'logicalId':r['pod']+'.'+r['member']}
if p.get('prepare'):v.update(state='prepared',code='native_resume_prepared',posted=False)
elif not p.get('inspect'):v.update(state='uncertain',code='native_resume_response_unavailable',posted=True)
else:v.update(state='confirmed',code='already_running_original',posted=True,attachment={'state':'attached','nodeId':r['nodeId'],'target':r['sessionName'],'tmuxServerFlag':i['tmux_server_flag'],'tmuxServerValue':i['tmux_server_value']})
print(json.dumps(v))
`
	for name, body := range map[string]string{"node": script, "rig": "#!/bin/sh\nexit 0\n", "tmux": "#!/bin/sh\nexit 0\n"} {
		if err := os.WriteFile(filepath.Join(bins, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bins+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := s.store.WriteConfig("app.yaml", map[string]any{"app": map[string]any{"session_logging": map[string]any{"enabled": true}, "coordination": map[string]any{"enabled_providers": []string{"rig"}}}}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	ref := session.NativeRigRecoveryReference{Contract: "webmux-rig-native-codex-recovery/v1", RigID: "01JQPWV4E7QP26M799M3S0MR6E", NodeID: "01JQPWV4E7QP26M799M3S0MR6F", Pod: "agents", Member: "alpha", Cwd: root, SessionName: "agents-alpha@qualified", SnapshotID: "01JQPWV4E7QP26M799M3S0MR6G", HistorySessionID: "01JQPWV4E7QP26M799M3S0MR6H", NativeSessionID: "01a1eff2-f58a-7082-be6b-69f9eca4a07c", NativeStateFile: filepath.Join(root, "original.jsonl"), NativeRecordIdentity: strings.Repeat("a", 64), SnapshotSHA256: strings.Repeat("b", 64), PlanFingerprint: strings.Repeat("c", 64)}
	bytes, _ := json.Marshal(ref)
	identity := session.RigIdentity{Target: ref.SessionName, AttachmentTarget: ref.SessionName, SeatID: localRigSeatID(ref.RigID, "agents.alpha"), Host: "localhost", Runtime: "codex", Workspace: root, TmuxServerFlag: "-S", TmuxServerValue: filepath.Join(root, "tmux.sock"), NativeRigRecovery: string(bytes), NativeSessionID: ref.NativeSessionID, NativeStateFile: ref.NativeStateFile, NativeSource: "rig-native-codex"}
	blank := identity
	blank.NativeRigRecovery, blank.NativeSessionID, blank.NativeStateFile, blank.NativeSource = "", "", "", ""
	saved, err := s.sessions.EnsureRigTerminal("anonymous", identity.Target, "coder", []string{"/bin/sh", "-c", "read line"}, blank)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.sessions.CaptureManagedRecovery(context.Background(), "anonymous", saved.ID, func(context.Context, session.RigIdentity) (session.RigIdentity, error) { return identity, nil }); err != nil {
		t.Fatal(err)
	}
	view, err := s.sessions.Recovery("anonymous", saved.ID)
	if err != nil || !view.ManagedOriginalContract || !view.CanResumeManaged {
		t.Fatal("native capture not offered", view, err)
	}
	input := session.RecoveryRequest{ID: "9aa1753a-1d61-4420-af42-e6b6fa591c47", Kind: "managed", Confirm: true, Revision: view.State.Revision, Instance: view.Instance}
	endpoint := "/api/session-recovery/" + saved.ID
	body, _ := json.Marshal(input)
	first := request(handler, "POST", endpoint, string(body), "")
	requireStatus(t, first, 202)
	var result session.RecoveryResult
	if json.Unmarshal(first.Body.Bytes(), &result) != nil || result.Outcome != "uncertain" {
		t.Fatal(first.Body.String())
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := request(handler, "POST", endpoint, string(body), "")
	requireStatus(t, duplicate, 202)
	after, _ := os.ReadFile(log)
	if string(after) != string(raw) {
		t.Fatal("ordinary duplicate reached native helper")
	}
	preview := request(handler, "GET", endpoint, "", "")
	requireStatus(t, preview, 200)
	if json.Unmarshal(preview.Body.Bytes(), &view) != nil || view.ManagedOriginalRequest == nil || view.CanResumeManaged {
		t.Fatal("lost original completion", preview.Body.String())
	}
	if view.ManagedOriginalRequest.ID != input.ID || view.ManagedOriginalRequest.Revision != input.Revision || view.ManagedOriginalRequest.Instance != input.Instance || !view.ManagedOriginalRequest.Inspect {
		t.Fatal("refreshed preview changed original request")
	}
	complete, _ := json.Marshal(view.ManagedOriginalRequest)
	requireStatus(t, request(handler, "POST", endpoint, string(complete), ""), 200)
	requireStatus(t, request(handler, "POST", endpoint, string(complete), ""), 200)
	final, _ := os.ReadFile(log)
	lines := strings.Split(strings.TrimSpace(string(final)), "\n")
	if len(lines) != 3 {
		t.Fatalf("unexpected native phases: %s", final)
	}
	for index, line := range lines {
		var payload struct {
			OperationID string              `json:"operationId"`
			Prepare     bool                `json:"prepare"`
			Inspect     bool                `json:"inspect"`
			Identity    session.RigIdentity `json:"identity"`
		}
		if json.Unmarshal([]byte(line), &payload) != nil || payload.OperationID != input.ID || payload.Prepare != (index == 0) || payload.Inspect != (index == 2) || payload.Identity.NativeRigRecovery != identity.NativeRigRecovery {
			t.Fatal("wrong frozen native operation", line)
		}
	}
}
