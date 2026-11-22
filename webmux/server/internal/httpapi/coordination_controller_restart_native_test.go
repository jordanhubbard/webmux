package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

// Separate production processes, real SSH and one retained native CLI/MCP pair.
// Opt-in paths must identify new, private qualification artifacts only.
func TestCoordinationProductionControllerRestart(t *testing.T) {
	if os.Getenv("WEBMUX_TEST_PRODUCTION_COORDINATION_RESTART") != "1" {
		t.Skip("explicit production executable and native SSH qualification only")
	}
	host, user := os.Getenv("WEBMUX_SSH_COORD_HOST"), os.Getenv("WEBMUX_SSH_COORD_USER")
	root, helper := os.Getenv("WEBMUX_SSH_COORD_ROOT"), os.Getenv("WEBMUX_SSH_COORD_HELPER")
	dir, source := os.Getenv("WEBMUX_SSH_COORD_DIR"), os.Getenv("WEBMUX_SSH_COORD_SOURCE_HELPER")
	if host == "" || user == "" || !filepath.IsAbs(root) || !filepath.IsAbs(helper) || !filepath.IsAbs(dir) || !filepath.IsAbs(source) {
		t.Fatal("explicit SSH identity and absolute owned fixture paths required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "controller-home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name string, value any) { t.Helper(); nativePairWrite(t, filepath.Join(dir, name), value) }
	realSSH, err := exec.LookPath("ssh")
	if err != nil {
		t.Fatal(err)
	}
	quote := nativePairQuote
	ssh := func(code string, input any) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, realSSH, "-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=5", "-l", user, "--", host, code)
		if input != nil {
			data, _ := json.Marshal(input)
			cmd.Stdin = bytes.NewReader(data)
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("owned SSH fixture command failed: %v %s", err, out)
		}
		return out
	}
	var remote struct {
		Prefix, Tmux, Name, Project string
		SocketCache                 string `json:"socket_cache"`
	}
	if err := json.Unmarshal(ssh("python3 -c "+quote(nativeSSHSetup), map[string]any{"root": root, "helper": helper, "native": true}), &remote); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		code := `import json,pathlib,shutil,subprocess,sys;i=json.load(sys.stdin);subprocess.run([i['tmux'],'-L',i['name'],'kill-server'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL);p=pathlib.Path(i['cache']);assert p.parent==pathlib.Path('/tmp') and p.name.startswith('wmcc-');shutil.rmtree(p)`
		ssh("python3 -c "+quote(code), map[string]string{"tmux": remote.Tmux, "name": remote.Name, "cache": remote.SocketCache})
	})
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		code := `import json,pathlib,sys;p=pathlib.Path(sys.argv[1]);print(json.dumps({name:(p/name).read_text() for name in ['native.jsonl','native.stderr'] if (p/name).exists()}))`
		var files map[string]string
		if err := json.Unmarshal(ssh("python3 -c "+quote(code)+" "+quote(root), nil), &files); err != nil {
			t.Error(err)
			return
		}
		for name, data := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
				t.Error(err)
			}
		}
	})
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	wrapper := fmt.Sprintf("#!/usr/bin/env python3\nimport os,sys\na=sys.argv[1:]\nif a and a[-1].startswith('webmux '):a[-1]=%q+' '+a[-1][7:]\nos.execv(%q,[%q,*a])\n", remote.Prefix, realSSH, realSSH)
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(wrapper), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config", "app.yaml"), []byte("app:\n  name: native-restart-fixture\n  listen_host: 127.0.0.1\n  http_port: 18080\n  https_port: 18443\n  secure_mode: false\n  session_logging:\n    enabled: false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 8 * time.Second}
	var address, bearer string
	api := func(method, path string, input, output any) {
		t.Helper()
		var data []byte
		if input != nil {
			data, _ = json.Marshal(input)
		}
		req, _ := http.NewRequest(method, address+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			t.Fatalf("fixture API %s %s status %d", method, path, resp.StatusCode)
		}
		if output != nil {
			if err := json.NewDecoder(resp.Body).Decode(output); err != nil {
				t.Fatal(err)
			}
		}
	}
	start := func(label string) (int, func()) {
		t.Helper()
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		bind := listener.Addr().String()
		_ = listener.Close()
		address = "http://" + bind
		log, err := os.OpenFile(filepath.Join(dir, label+".log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(source, "-home", home, "-listen", bind)
		cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		cmd.Stdout, cmd.Stderr = log, log
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait(); _ = log.Close() }()
		stopped := false
		stop := func() {
			t.Helper()
			if stopped {
				return
			}
			stopped = true
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("production controller exit: %v", err)
				}
			case <-time.After(12 * time.Second):
				_ = cmd.Process.Kill()
				<-done
				t.Error("production controller did not shut down within original bound")
			}
		}
		t.Cleanup(stop)
		ready := false
		for deadline := time.Now().Add(40 * time.Second); time.Now().Before(deadline); {
			resp, err := client.Get(address + "/api/health")
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == 200 {
					ready = true
					break
				}
			}
			select {
			case err := <-done:
				stopped = true
				t.Fatalf("production controller exited before ready: %v; see private log", err)
			default:
			}
			time.Sleep(50 * time.Millisecond)
		}
		if !ready {
			t.Fatal("production controller startup deadline expired")
		}
		return cmd.Process.Pid, stop
	}
	pidBefore, stop := start("controller-before")
	var login struct {
		Token string `json:"token"`
	}
	api("POST", "/api/auth/bootstrap", map[string]string{"username": "owner", "password": "NativeFixtureOnly-630-Password"}, &login)
	bearer = login.Token
	var created session.Session
	api("POST", "/api/sessions", session.CreateRequest{Transport: "ssh", Hostname: host, Username: user, Cols: 120, Rows: 40}, &created)
	write("created.json", created)
	view := func() plane.View {
		t.Helper()
		var v plane.View
		api("GET", "/api/session-coordination", nil, &v)
		return v
	}
	wait := func(task string, claims int) plane.View {
		t.Helper()
		for deadline := time.Now().Add(120 * time.Second); time.Now().Before(deadline); {
			v := view()
			write("last-view.json", v)
			for _, m := range v.Members {
				if m.Ref.Session == created.ID && m.Active && m.Task == task && len(v.Claims) == claims && ((task == "" && strings.Contains(m.Interface, "ssh-stream-local-configured")) || (task != "" && m.ProjectStatus == "verified-git")) {
					return v
				}
			}
			time.Sleep(time.Second)
		}
		t.Fatal("production coordination observation deadline expired; private controller/native artifacts retained")
		return plane.View{}
	}
	initial := wait("", 0)
	var ref plane.Ref
	for _, m := range initial.Members {
		if m.Ref.Session == created.ID {
			ref = m.Ref
		}
	}
	sum := sha256.Sum256([]byte(ref.Key()))
	prefix := "mcp__webmux_" + hex.EncodeToString(sum[:6]) + "__webmux_coordination_"
	tools := []string{prefix + "view", prefix + "act"}
	write("expected.json", map[string]any{"subject": ref, "tools": tools, "task": "native SSH restart qualification"})
	firstPrompt := `Inspect your collaboration interface. Register a work action with text "native SSH restart qualification" and request_id "restart-task-1". Claim file "restart-owned.txt" for 600 seconds with request_id "restart-claim-1". Do not read/edit files, run shell commands, delegate, or launch agents. Use the available collaboration interface, report confirmed results, then stop.`
	driver := `import json,pathlib,sys,time
p=pathlib.Path(sys.argv[1]);print((p/'first.json').read_text(),flush=True);deadline=time.monotonic()+240
while not (p/'second.json').exists() and time.monotonic()<deadline:time.sleep(.05)
if (p/'second.json').exists():
 print((p/'second.json').read_text(),flush=True)
 while not (p/'finish').exists() and time.monotonic()<deadline:time.sleep(.05)`
	putMessage := func(file, prompt string) {
		t.Helper()
		code := `import json,pathlib,sys;i=json.load(sys.stdin);pathlib.Path(i['file']).write_text(json.dumps({'type':'user','message':{'role':'user','content':i['prompt']}}))`
		ssh("python3 -c "+quote(code), map[string]string{"file": root + "/" + file, "prompt": prompt})
	}
	putMessage("first.json", firstPrompt)
	ssh("python3 -c "+quote(`import json,pathlib,sys;i=json.load(sys.stdin);pathlib.Path(i['file']).write_text(i['data'])`), map[string]string{"file": root + "/driver.py", "data": driver})
	args := []string{"claude", "--print", "--output-format", "stream-json", "--input-format", "stream-json", "--replay-user-messages", "--verbose", "--max-turns", "16", "--allowedTools", tools[0], tools[1]}
	for i, arg := range args {
		args[i] = quote(arg)
	}
	command := "cd " + quote(remote.Project) + " && { python3 " + quote(root+"/driver.py") + " " + quote(root) + " | " + strings.Join(args, " ") + " > " + quote(root+"/native.jsonl") + " 2> " + quote(root+"/native.stderr") + " & job=$!; printf '%s' \"$job\" > " + quote(root+"/native-launch.pid") + "; wait \"$job\"; printf '%s' \"$?\" > " + quote(root+"/native.exit") + "; }\r"
	ws, _, err := websocket.DefaultDialer.Dial(strings.Replace(address, "http://", "ws://", 1)+"/api/term/"+created.ID+"?token="+url.QueryEscape(bearer), nil)
	if err != nil {
		t.Fatal(err)
	}
	terminalLog, err := os.OpenFile(filepath.Join(dir, "terminal.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	drained := make(chan struct{})
	t.Cleanup(func() { _ = ws.Close(); <-drained; _ = terminalLog.Close() })
	go func() {
		defer close(drained)
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			_, _ = terminalLog.Write(append(data, '\n'))
			if bytes.Contains(data, []byte("wmc1.")) {
				t.Error("capability entered terminal output")
			}
		}
	}()
	if err := ws.WriteJSON(map[string]string{"type": "input", "data": command}); err != nil {
		t.Fatal(err)
	}
	before := wait("native SSH restart qualification", 1)
	write("view.json", before)
	waitNativeTurns := func(count int) {
		t.Helper()
		turnReady := `import json,pathlib,time,sys;p=pathlib.Path(` + fmt.Sprintf("%q", root+"/native.jsonl") + `);deadline=time.monotonic()+10
while time.monotonic()<deadline:
 events=[]
 for line in p.read_text().splitlines():
  try:events.append(json.loads(line))
  except ValueError:pass
 if sum(e.get('type')=='result' and e.get('subtype')=='success' and e.get('is_error') is False for e in events)==int(sys.argv[1]):break
 time.sleep(.05)
else:raise RuntimeError('native turn did not complete')`
		ssh("python3 -c "+quote(turnReady)+" "+fmt.Sprint(count), nil)
	}
	waitNativeTurns(1)
	identity := func() json.RawMessage {
		t.Helper()
		return json.RawMessage(ssh("python3 -c "+quote(nativeRestartIdentity)+" "+quote(root), nil))
	}
	nativeBefore := identity()
	pane := func() string {
		return strings.TrimSpace(string(ssh(quote(remote.Tmux)+" -L "+quote(remote.Name)+" display-message -p -t "+quote("=webmux-"+created.ID+":")+" "+quote("#{pane_pid}"), nil)))
	}
	paneBefore := pane()
	_ = ws.Close()
	stop()
	pidAfter, _ := start("controller-after")
	restored := wait("native SSH restart qualification", 1)
	write("view-restored.json", restored)
	if restored.HistoryEpoch != before.HistoryEpoch || restored.Claims[0].ID != before.Claims[0].ID || restored.Claims[0].Expires != before.Claims[0].Expires {
		t.Fatal("controller restart replaced durable claim/history")
	}
	nativeAfter, paneAfter := identity(), pane()
	if string(nativeBefore) != string(nativeAfter) || paneBefore != paneAfter {
		t.Fatal("controller restart replaced native process or pane")
	}
	continuity := map[string]any{"controller_before": pidBefore, "controller_after": pidAfter, "native_before": nativeBefore, "native_after": nativeAfter, "pane_before": paneBefore, "pane_after": paneAfter}
	write("restart.json", continuity)
	putMessage("second.json", `Recheck your collaboration interface now. Confirm the existing task and file claim. Register a work action with text "native SSH restart resumed" and request_id "restart-task-2". Release your original "restart-owned.txt" claim using its existing record ID and request_id "restart-release-1". Do not create another claim, read/edit files, run commands, delegate or launch agents. Report only confirmed results and stop.`)
	after := wait("native SSH restart resumed", 0)
	write("view-after.json", after)
	waitNativeTurns(2)
	nativeCompleted, paneCompleted := identity(), pane()
	if string(nativeBefore) != string(nativeCompleted) || paneBefore != paneCompleted {
		t.Fatal("completed post-restart turn replaced native processes or pane")
	}
	continuity["native_completed"], continuity["pane_completed"] = nativeCompleted, paneCompleted
	write("restart.json", continuity)
	ssh("python3 -c "+quote(`import pathlib,sys;pathlib.Path(sys.argv[1]).write_text('done')`)+" "+quote(root+"/finish"), nil)
	capture := `import json,pathlib,time;p=pathlib.Path(` + fmt.Sprintf("%q", root) + `);deadline=time.monotonic()+10
while not (p/'native.exit').exists() and time.monotonic()<deadline:time.sleep(.05)
print(json.dumps({key:(p/name).read_text() if (p/name).exists() else '' for key,name in [('exit','native.exit'),('stdout','native.jsonl'),('stderr','native.stderr')]}))`
	var result struct{ Exit, Stdout, Stderr string }
	if err := json.Unmarshal(ssh("python3 -c "+quote(capture), nil), &result); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"native.jsonl": result.Stdout, "native.stderr": result.Stderr} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if result.Exit != "0" || strings.Contains(result.Stdout, "wmc1.") {
		t.Fatal("native process failed or exposed capability")
	}
	write("expected.json", map[string]any{"subject": ref, "tools": tools, "task": "native SSH restart qualification"})
	t.Log("two separate production controller processes preserve one native Claude/MCP pair through completed post-restart work and durable task/claim state; audit actual native calls before accepting")
}

const nativeRestartIdentity = `import json,pathlib,sys
p=pathlib.Path(sys.argv[1]);parent=int((p/'native-launch.pid').read_text());rows={}
for d in pathlib.Path('/proc').iterdir():
 if not d.name.isdigit():continue
 try:
  stat=(d/'stat').read_text();tail=stat[stat.rindex(')')+2:].split();rows[int(d.name)]=(int(tail[1]),tail[19],(d/'comm').read_text().strip())
 except (OSError,ValueError):pass
owned={parent}
for _ in range(20):owned.update(pid for pid,row in rows.items() if row[0] in owned)
mcp=[]
for pid in sorted(owned):
 try:
  args=(pathlib.Path('/proc')/str(pid)/'cmdline').read_bytes().split(b'\x00')
  if args[1:3]==[b'coordinate',b'mcp']:mcp.append(pid)
 except OSError:pass
assert len(mcp)==1;native=rows[mcp[0]][0];assert rows[native][2].startswith('claude')
print(json.dumps({k:{'pid':pid,'start':rows[pid][1]} for k,pid in [('launcher',parent),('cli',native),('mcp',mcp[0])]},sort_keys=True))`
