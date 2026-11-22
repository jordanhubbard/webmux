package httpapi

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

type nativePairManifest struct {
	URL         string `json:"url"`
	Node        string `json:"node_id"`
	Certificate string `json:"certificate"`
	Role        string `json:"role"`
}

func nativePairWrite(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path+".tmp", append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(path+".tmp", path); err != nil {
		t.Fatal(err)
	}
}
func nativePairQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'" }

// Two explicitly invoked native host roles, launched by the fixture operator.
// The operator enrolls nodes and types one ordinary task command per agent. All
// subsequent work/claims/messages/leases come from native agents or their guarded
// verification program; the harness never supplies coordination terminal input.
func TestCoordinationNativePairHost(t *testing.T) {
	role := os.Getenv("WEBMUX_NATIVE_PAIR_ROLE")
	if role != "reviewer" && role != "verifier" {
		t.Skip("explicit two-host native coding-agent qualification only")
	}
	dir, bind, helper := os.Getenv("WEBMUX_NATIVE_PAIR_DIR"), os.Getenv("WEBMUX_NATIVE_PAIR_BIND"), os.Getenv("WEBMUX_NATIVE_PAIR_HELPER")
	if !filepath.IsAbs(dir) || !filepath.IsAbs(helper) || net.ParseIP(bind) == nil || net.ParseIP(bind).IsLoopback() {
		t.Fatal("explicit private evidence directory, helper and non-loopback bind required")
	}
	secret := nodeNetworkSecret(t, dir)
	deterministicCoordinationGit(t)
	s, h, _, records, _ := coordinationAPI(t)
	repository := records[0].ExecCwd
	if err := os.WriteFile(filepath.Join(repository, "task.py"), []byte("def score(x):\n    return x + 7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	jobPath := filepath.Join(repository, "verify.py")
	if err := os.WriteFile(jobPath, []byte(nativePairJob), 0600); err != nil {
		t.Fatal(err)
	}
	jobHash := sha256.Sum256([]byte(nativePairJob))
	node, err := s.coordinationNode()
	if err != nil {
		t.Fatal(err)
	}
	federationConfig(t, s, serverDirectory{Version: 1, Grants: []serverGrant{{ID: "native-peer", Owner: "owner", Credential: secret}}})
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	cert := x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "WebMux isolated native pair"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(15 * time.Minute), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP(bind)}}
	der, err := x509.CreateCertificate(rand.Reader, &cert, &cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	listener, err := net.Listen("tcp", net.JoinHostPort(bind, "0"))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_qualification/stop" {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+secret)) != 1 {
				w.WriteHeader(401)
				return
			}
			select {
			case <-done:
			default:
				close(done)
			}
			w.WriteHeader(204)
			return
		}
		h.ServeHTTP(w, r)
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	t.Cleanup(func() { _ = server.Close() })
	go func() {
		_ = server.Serve(tls.NewListener(listener, &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}))
	}()
	nativePairWrite(t, filepath.Join(dir, "manifest.json"), nativePairManifest{URL: "https://" + listener.Addr().String(), Node: node, Certificate: certificate, Role: role})
	var peer nativePairManifest
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(filepath.Join(dir, "peer.json"))
		if err == nil {
			if json.Unmarshal(data, &peer) != nil {
				t.Fatal("invalid peer manifest")
			}
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if peer.Node == "" || peer.Node == node || peer.Role == role {
		t.Fatal("missing distinct peer manifest")
	}
	s.peerRoots = x509.NewCertPool()
	if !s.peerRoots.AppendCertsFromPEM([]byte(peer.Certificate)) {
		t.Fatal("invalid isolated peer certificate")
	}
	federationConfig(t, s, serverDirectory{Version: 1, Servers: []serverPeer{{ID: "native-peer", Name: peer.Role, URL: peer.URL, Credential: secret}}, Grants: []serverGrant{{ID: "native-peer", Owner: "owner", Credential: secret, Coordination: true, CoordinationMessages: true, CoordinationReservations: true, SourceNode: peer.Node, SourceOwner: "owner"}}})
	requireStatus(t, request(h, "POST", "/api/servers/native-peer/verify-identity", "{}", token(t, s, "owner")), 204)
	requireStatus(t, request(h, "PUT", "/api/servers/native-peer", directoryJSON(map[string]any{"name": peer.Role, "url": peer.URL, "coordination_owner": "owner", "coordination_messages": true, "coordination_reservations": true}), token(t, s, "owner")), 204)
	label := "native-pair-" + role
	prompt := nativePairReviewerTask
	cli := "claude"
	if role == "verifier" {
		prompt = nativePairVerifierTask
	}
	command := "cd " + nativePairQuote(repository) + " && exec /usr/bin/env SHELL=/bin/bash WEBMUX_RECOVERY_CACHE_HOME=" + nativePairQuote(t.TempDir()) + " WEBMUX_NATIVE_PAIR_HELPER=" + nativePairQuote(helper) + " WEBMUX_NATIVE_PAIR_JOB_EVIDENCE=" + nativePairQuote(filepath.Join(dir, "job.json")) + " " + nativePairQuote(helper) + " --browser-shell"
	created, err := s.sessions.Create("owner", session.CreateRequest{Hostname: "localhost", Username: "owner", Transport: "exec", Cols: 140, Rows: 50, ExecCommand: command})
	if err != nil {
		t.Fatal(err)
	}
	ready := time.Now().Add(15 * time.Second)
	for {
		current, err := s.sessions.Get("owner", created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.CoordinationActivity != nil && current.CoordinationActivity.Directory == repository {
			break
		}
		if time.Now().After(ready) {
			t.Fatal("ordinary native shell bootstrap unavailable")
		}
		time.Sleep(50 * time.Millisecond)
	}
	ref := coordinationRef(node, created)
	hash := sha256.Sum256([]byte(ref.Key()))
	mcpName := "mcp__webmux_" + hex.EncodeToString(hash[:6]) + "__"
	args := []string{cli, "--print", "--output-format", "stream-json", "--verbose", "--max-turns", "60", "--allowedTools", mcpName + "webmux_coordination_view", mcpName + "webmux_coordination_act"}
	if role == "verifier" {
		args = append(args, "Read", "Edit", "Bash(python3 verify.py *)")
	}
	args = append(args, "--", prompt)
	words := make([]string, len(args))
	for i, arg := range args {
		words[i] = nativePairQuote(arg)
	}
	nativePairWrite(t, filepath.Join(dir, "started.json"), map[string]any{"role": role, "session": coordinationRef(node, created), "cli": cli, "task": label, "job_driver_sha256": hex.EncodeToString(jobHash[:])})
	if err = s.sessions.Input("owner", created.ID, strings.Join(words, " ")+"; exit\r"); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(360 * time.Second)
	exited := false
	retainedOutput := ""
	for time.Now().Before(deadline) {
		current, err := s.sessions.Get("owner", created.ID)
		if err != nil {
			t.Fatal(err)
		}
		output, err := s.sessions.Scrollback("owner", created.ID)
		if err != nil {
			t.Fatal(err)
		}
		var complete bool
		retainedOutput, complete = retainNativeOutput(retainedOutput, output)
		if !complete {
			t.Fatal("native output exceeded capture overlap; retain the selected native transcript before acceptance")
		}
		if err = os.WriteFile(filepath.Join(dir, "native-output.log"), []byte(retainedOutput), 0600); err != nil {
			t.Fatal(err)
		}
		var view plane.View
		err = s.sessions.WithCoordinationSnapshot(func(all []session.Session) error {
			var e error
			view, e = s.sessionCoord.service.OwnerView(node, "owner", s.coordinationInputs(node, all))
			return e
		})
		if err != nil {
			t.Fatal(err)
		}
		nativePairWrite(t, filepath.Join(dir, "state.json"), map[string]any{"role": role, "native_session": coordinationRef(node, created), "state": current.State, "activity": current.CoordinationActivity, "view": view})
		if current.State == "disconnected" {
			exited = true
			break
		}
		select {
		case <-done:
			t.Fatal("qualification stopped before native task completed")
		case <-time.After(300 * time.Millisecond):
		}
	}
	if !exited {
		t.Fatal("native task did not finish within bounded qualification")
	}
	actual, err := os.ReadFile(jobPath)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(actual) != jobHash {
		t.Fatal("native task changed the guarded verification driver")
	}
	task, err := os.ReadFile(filepath.Join(repository, "task.py"))
	if err != nil {
		t.Fatal(err)
	}
	nativePairWrite(t, filepath.Join(dir, "finished.json"), map[string]any{"role": role, "session": coordinationRef(node, created), "task_source": string(task), "job_driver_unchanged": true, "native_process_exited": true})
	// Keep the authority live while its peer finishes and for operator readback.
	select {
	case <-done:
	case <-time.After(90 * time.Second):
	}
	t.Log("native host task exited; audit peer/claim/handoff/job artifacts before claiming cross-host acceptance")
}

const nativePairReviewerTask = `You are the reviewer in a two-person verification exercise for this project. Your task title is native-pair-reviewer. The other person's title is native-pair-verifier, on the other host. The target is task.py: score(x) must return x*x+7, but currently returns x+7. Do not edit files, run commands, delegate or create other agents. Use the automatically available collaboration interface. Register your task and a file ownership claim for task.py, find the verifier, and reserve the verifier host's heavy worker while you review. Tell the verifier to claim the same target and try acquiring that same worker while you hold it; ask them to confirm the ownership conflict and busy result before proceeding. Keep your lease current while waiting. Once their confirmation arrives, release your original worker record/fence and your file claim, then send an acknowledged handoff asking them to fix the function and run the guarded verification program. Wait for completion, explicitly record delivered/acknowledged/completed for messages you process, and stop when the verifier confirms the guarded job passed and their worker lease is released. Keep each action's request ID unchanged on uncertain retries. Do not infer completion from prose without the corresponding interface receipts. Recheck your inbox at work boundaries; allow the other person up to three minutes to respond.`
const nativePairVerifierTask = `You are the verifier in a two-person coding exercise. Your task title is native-pair-verifier. The reviewer title is native-pair-reviewer on the other host. Use the automatically available collaboration interface to register your task and find them. Do not delegate or launch agents. The target task.py must implement score(x)=x*x+7. Wait until the reviewer says they hold your host's worker, then claim task.py and attempt to reserve your local heavy worker. Confirm the overlapping ownership and busy result to the reviewer. Do not edit or run verification while they own the file/worker. After their explicit handoff and release, acknowledge it, claim your work with a fresh intent, reserve the local worker with a fresh intent, require a current unexpired lease, fix task.py using Edit, and run exactly python3 verify.py RECORD FENCE EPOCH using the identifiers from your own grant. The verification driver must remain unchanged; it rechecks and renews your original lease around a bounded real computation and records evidence. Stop work if verification or lease checks fail. After the job passes, release the original worker and your claim, complete the handoff, and report completion to the reviewer through the collaboration inbox. Explicitly record delivered/acknowledged/completed for messages you process. Preserve request IDs for exact uncertain retries; new work needs a new ID. Allow the reviewer up to three minutes; do not invent success if the interface or permissions refuse.`
const nativePairJob = `import datetime, hashlib, json, os, subprocess, sys, time
from task import score
record, fence, epoch = sys.argv[1], int(sys.argv[2]), sys.argv[3]
helper = os.environ['WEBMUX_NATIVE_PAIR_HELPER']
def current(stage):
    view = json.loads(subprocess.check_output([helper, 'coordinate', 'view'], text=True))
    ref = view['self']['ref']
    resource = 'node:' + ref['node_id'] + ':heavy'
    assert view['history_epoch'] == epoch
    held = next(r for r in view['resources'] if r['id'] == resource)
    assert held['occupied'] and held['holder'] == ref and held['fence'] == fence
    request = dict(request_id='native-pair-job-' + stage + '-' + record, action='renew-reservation', resource=resource, record_id=record, fence=fence, lease_seconds=120)
    result = subprocess.run([helper, 'coordinate', 'act'], input=json.dumps(request), capture_output=True, text=True, check=True)
    lease = json.loads(result.stdout)
    assert lease['status'] == 'renewed' and lease['record_id'] == record and lease['fence'] == fence and lease['lease']['current']
    return dict(at=datetime.datetime.now(datetime.timezone.utc).isoformat(), ref=ref, resource=resource, fence=fence, record=record, epoch=epoch, lease=lease)
before = current('before')
for x in range(-100,101):
    assert score(x) == x*x+7
value = b'webmux-native-pair-job'
for _ in range(20000):
    value = hashlib.sha256(value).digest()
time.sleep(2)
after = current('after')
assert before['ref'] == after['ref'] and before['fence'] == after['fence']
with open(os.environ['WEBMUX_NATIVE_PAIR_JOB_EVIDENCE'], 'x') as out:
    json.dump(dict(before=before, after=after, cases=201, computation_sha256=value.hex(), task_sha256=hashlib.sha256(open('task.py','rb').read()).hexdigest()), out, indent=2)
print('GUARDED_NATIVE_JOB_PASSED')
`

// The broker's 64 KiB scrollback is a window, not a complete native event log.
// Join overlapping windows in linear time and refuse gaps instead of silently
// dropping early grants/receipts or inventing model success from the final text.
func retainNativeOutput(retained, window string) (string, bool) {
	if retained == "" {
		return window, true
	}
	if window == "" {
		return retained, true
	}
	prefix := make([]int, len(window))
	for i, j := 1, 0; i < len(window); i++ {
		for j > 0 && window[i] != window[j] {
			j = prefix[j-1]
		}
		if window[i] == window[j] {
			j++
		}
		prefix[i] = j
	}
	tail := retained
	if len(tail) > len(window) {
		tail = tail[len(tail)-len(window):]
	}
	matched := 0
	for i := 0; i < len(tail); i++ {
		for matched > 0 && (matched == len(window) || tail[i] != window[matched]) {
			matched = prefix[matched-1]
		}
		if tail[i] == window[matched] {
			matched++
		}
	}
	if matched == 0 {
		return retained, false
	}
	return retained + window[matched:], true
}
func TestNativeOutputCapturePreservesRollingWindowsAndRefusesGaps(t *testing.T) {
	value, ok := retainNativeOutput("", "first grant\nsecond")
	if !ok {
		t.Fatal("initial capture failed")
	}
	value, ok = retainNativeOutput(value, "grant\nsecond receipt\nthird")
	if !ok || value != "first grant\nsecond receipt\nthird" {
		t.Fatal(value, ok)
	}
	repeated, ok := retainNativeOutput(value, "second receipt\nthird")
	if !ok || repeated != value {
		t.Fatal("repeated tail duplicated events")
	}
	if _, ok = retainNativeOutput(value, "unrelated missing prefix"); ok {
		t.Fatal("capture gap silently accepted")
	}
}
