package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

// Real SSH, provider bootstrap and scoped client calls. The fixture changes only
// helper resolution and tmux namespace; no installed helper/configuration or
// existing provider is changed. Optional native Claude mode requires a separate
// transcript audit; shell-only mode is not coding-agent adoption evidence.
func TestCoordinationSSHNativeBootstrap(t *testing.T) {
	if os.Getenv("WEBMUX_TEST_SSH_COORDINATION") != "1" {
		t.Skip("opt-in native SSH execution-host fixture")
	}
	host, user, root, helper, dir := os.Getenv("WEBMUX_SSH_COORD_HOST"), os.Getenv("WEBMUX_SSH_COORD_USER"), os.Getenv("WEBMUX_SSH_COORD_ROOT"), os.Getenv("WEBMUX_SSH_COORD_HELPER"), os.Getenv("WEBMUX_SSH_COORD_DIR")
	native := os.Getenv("WEBMUX_SSH_COORD_NATIVE_CLAUDE") == "1"
	if native && (os.Getenv("WEBMUX_SSH_COORD_REFUSE") == "1" || os.Getenv("WEBMUX_SSH_COORD_RECONNECT") == "1") {
		t.Fatal("qualify native adoption separately from shell refusal/reconnect controls")
	}
	if host == "" || user == "" || !filepath.IsAbs(root) || !filepath.IsAbs(helper) || !filepath.IsAbs(dir) {
		t.Fatal("explicit SSH fixture identity and absolute owned paths required")
	}
	realSSH, err := exec.LookPath("ssh")
	if err != nil {
		t.Fatal(err)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	runSSH := func(command string, input any) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, realSSH, "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=5", "-l", user, "--", host, command)
		if input != nil {
			data, _ := json.Marshal(input)
			cmd.Stdin = strings.NewReader(string(data))
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("owned SSH fixture command failed: %v %s", err, out)
		}
		return out
	}
	setup := `import json,os,pathlib,secrets,shlex,shutil,subprocess,sys,tempfile
i=json.load(sys.stdin);root=pathlib.Path(i['root']);root.mkdir(mode=0o700)
(root/'home').mkdir();(root/'cache').mkdir(mode=0o700);(root/'bin').mkdir()
socket_cache=tempfile.mkdtemp(prefix='wmcc-'); tmux=shutil.which('tmux'); assert tmux
name='wmcc-'+secrets.token_hex(8)
(root/'bin/tmux').write_text('#!/bin/sh\nexec '+shlex.quote(tmux)+' -L '+shlex.quote(name)+' "$@"\n')
(root/'bin/tmux').chmod(0o700)
repo=root/'project';repo.mkdir()
env={k:v for k,v in os.environ.items() if not k.upper().startswith('GIT_')};env.update(GIT_CONFIG_GLOBAL='/dev/null',GIT_CONFIG_NOSYSTEM='1')
def git(*args):subprocess.run(['git','-c','core.hooksPath=/dev/null','-C',str(repo),*args],env=env,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,check=True)
git('init');git('-c','user.name=Fixture','-c','user.email=fixture@example.test','commit','--allow-empty','-m','SSH coordination fixture');git('remote','add','origin','https://example.test/team/native-ssh-channel.git')
values={'HOME':str(root/'home'),'SHELL':'/bin/bash','WEBMUX_RECOVERY_CACHE_HOME':str(root/'cache'),'WEBMUX_COORDINATION_CACHE_HOME':socket_cache,'PATH':str(root/'bin')+':'+os.environ['PATH']}
if i.get('native'):values['CLAUDE_CONFIG_DIR']=os.environ.get('CLAUDE_CONFIG_DIR',str(pathlib.Path.home()/'.claude'))
prefix='env '+' '.join(shlex.quote(k+'='+v) for k,v in values.items())+' '+shlex.quote(i['helper'])
print(json.dumps({'prefix':prefix,'tmux':tmux,'name':name,'socket_cache':socket_cache,'project':str(repo)}))`
	var remote struct {
		Prefix, Tmux, Name, Project string
		SocketCache                 string `json:"socket_cache"`
	}
	if err := json.Unmarshal(runSSH("python3 -c "+quote(setup), map[string]any{"root": root, "helper": helper, "native": native}), &remote); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		code := `import json,pathlib,shutil,subprocess,sys;i=json.load(sys.stdin);subprocess.run([i['tmux'],'-L',i['name'],'kill-server'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL);p=pathlib.Path(i['cache']);assert str(p).startswith('/tmp/wmcc-') and p.parent==pathlib.Path('/tmp');shutil.rmtree(p)`
		data, _ := json.Marshal(map[string]string{"tmux": remote.Tmux, "name": remote.Name, "cache": remote.SocketCache})
		cmd := exec.CommandContext(ctx, realSSH, "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "-l", user, "--", host, "python3 -c "+quote(code))
		cmd.Stdin = strings.NewReader(string(data))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("owned fixture cleanup failed: %v %s", err, out)
		}
	}
	t.Cleanup(cleanup)
	bin := t.TempDir()
	refused := os.Getenv("WEBMUX_SSH_COORD_REFUSE") == "1"
	// Preserve encrypted stdin and actual SSH authentication. Rewrite only the
	// fixed WebMux helper command to the transferred candidate in its fixture.
	wrapper := fmt.Sprintf("#!/usr/bin/env python3\nimport os,sys\na=sys.argv[1:]\nif os.environ.get('WEBMUX_SSH_COORD_REFUSE')=='1' and '-O' in a and a[a.index('-O')+1]=='forward': sys.exit(99)\nif a and a[-1].startswith('webmux '):\n a[-1]=%q+' '+a[-1][7:]\nos.execv(%q,[%q,*a])\n", remote.Prefix, realSSH, realSSH)
	if os.Getenv("WEBMUX_SSH_COORD_FIRST_USE") == "1" {
		insertion := fmt.Sprintf("\na=['-o',%q,'-o','GlobalKnownHostsFile=/dev/null',*a]\n", "UserKnownHostsFile="+filepath.Join(bin, "first-known-hosts"))
		wrapper = strings.Replace(wrapper, "\nos.execv(", insertion+"os.execv(", 1)
	}
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(wrapper), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	s, _ := fixture(t, "local")
	federationAccounts(t, s)
	if err := s.store.WriteConfig("app.yaml", map[string]any{"app": map[string]any{"session_logging": map[string]any{"enabled": false}}}); err != nil {
		t.Fatal(err)
	}
	agent := httptest.NewServer(s.CoordinationAgentHandler())
	t.Cleanup(agent.Close)
	s.EnableSessionCoordination(agent.URL)
	s.StartSessionCoordination()
	created, err := s.sessions.Create("owner", session.CreateRequest{Transport: "ssh", Hostname: host, Username: user, Cols: 120, Rows: 40})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	deadline := time.Now().Add(35 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		v, _ := s.sessions.Get("owner", created.ID)
		channelReady := v.CoordinationChannel == "ssh-stream-local-configured"
		if v.CoordinationActivity != nil && ((refused && !channelReady) || (!refused && channelReady)) {
			ready = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		text, _ := s.sessions.Scrollback("owner", created.ID)
		t.Fatalf("normal SSH bootstrap did not configure its private channel: %s", text)
	}
	command := "cd " + quote(remote.Project) + " && \"$WEBMUX_COORDINATION_CLI\" coordinate view && printf '%s\\n' " + quote(`{"request_id":"native-ssh-work-1","action":"work","text":"native SSH channel qualification"}`) + " | \"$WEBMUX_COORDINATION_CLI\" coordinate act && printf '\\nSSH_COORDINATION_ACTION_DONE\\n'\r"
	var nativeTools []string
	if native {
		node, e := s.coordinationNode()
		if e != nil {
			t.Fatal(e)
		}
		ref := coordinationRef(node, created)
		sum := sha256.Sum256([]byte(ref.Key()))
		prefix := "mcp__webmux_" + hex.EncodeToString(sum[:6]) + "__webmux_coordination_"
		nativeTools = []string{prefix + "view", prefix + "act"}
		prompt := "Inspect the current project context and available collaboration interface before work. Register your current task with text exactly: native SSH channel qualification. Do not read or edit files, run shell commands, delegate, or launch another agent. Report only what the available interface confirms and then stop."
		args := []string{"claude", "--print", "--output-format", "stream-json", "--verbose", "--max-turns", "12", "--allowedTools", nativeTools[0], nativeTools[1], "--", prompt}
		words := make([]string, len(args))
		for i, arg := range args {
			words[i] = quote(arg)
		}
		command = "cd " + quote(remote.Project) + " && " + strings.Join(words, " ") + " > " + quote(root+"/native.jsonl") + " 2> " + quote(root+"/native.stderr") + "; printf '%s' \"$?\" > " + quote(root+"/native.exit") + "\r"
	}
	if refused {
		command = "if \"$WEBMUX_COORDINATION_CLI\" coordinate view >/dev/null 2>&1; then printf unexpected; else printf unavailable; fi > " + quote(root+"/shell-result") + "\r"
	}
	if err := s.sessions.Input("owner", created.ID, command); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(25 * time.Second)
	if native {
		deadline = time.Now().Add(120 * time.Second)
	}
	var view plane.View
	passed := false
	if refused {
		code := `import pathlib,time;p=pathlib.Path(` + fmt.Sprintf("%q", root+"/shell-result") + `);deadline=time.monotonic()+5
while not p.exists() and time.monotonic()<deadline:time.sleep(.02)
assert p.read_text()=='unavailable'`
		runSSH("python3 -c "+quote(code), nil)
		passed = true
	}
	for !passed && time.Now().Before(deadline) {
		text, _ := s.sessions.Scrollback("owner", created.ID)
		if strings.Contains(text, "wmc1.") {
			t.Fatal("capability entered terminal output")
		}
		node, _ := s.coordinationNode()
		snapshots, _ := s.coordinationSnapshots()
		var e error
		view, e = s.sessionCoord.service.OwnerView(node, "owner", s.coordinationInputs(node, snapshots))
		if e == nil {
			for _, member := range view.Members {
				if member.Ref.Session == created.ID && member.Task == "native SSH channel qualification" && member.Project != nil && member.Project.Repository == "example.test/team/native-ssh-channel" {
					passed = true
				}
			}
		}
		if passed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if native {
		code := `import json,pathlib,time;p=pathlib.Path(` + fmt.Sprintf("%q", root) + `);deadline=time.monotonic()+10
while not (p/'native.exit').exists() and time.monotonic()<deadline:time.sleep(.05)
print(json.dumps({key:(p/name).read_text() if (p/name).exists() else '' for key,name in [('exit','native.exit'),('stdout','native.jsonl'),('stderr','native.stderr')]}))`
		var result struct{ Exit, Stdout, Stderr string }
		if err := json.Unmarshal(runSSH("python3 -c "+quote(code), nil), &result); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "native.jsonl"), []byte(result.Stdout), 0600); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(dir, "native.stderr"), []byte(result.Stderr), 0600)
		if result.Exit != "0" || strings.Contains(result.Stdout, "wmc1.") {
			t.Fatal("native CLI failed or exposed a capability; private transcript retained")
		}
		node, _ := s.coordinationNode()
		data, _ := json.MarshalIndent(map[string]any{"subject": coordinationRef(node, created), "tools": nativeTools, "task": "native SSH channel qualification"}, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, "expected.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if !passed {
		text, _ := s.sessions.Scrollback("owner", created.ID)
		t.Fatalf("remote scoped CLI did not update its broker subject: %s", text)
	}
	if refused {
		node, _ := s.coordinationNode()
		snapshots, _ := s.coordinationSnapshots()
		view, err = s.sessionCoord.service.OwnerView(node, "owner", s.coordinationInputs(node, snapshots))
		if err != nil {
			t.Fatal(err)
		}
		for _, member := range view.Members {
			if member.Ref.Session == created.ID && (member.Interface != "execution-host-onboarding-pending" || member.Task != "") {
				t.Fatal("refused channel advertised successful onboarding")
			}
		}
	}
	if !refused && os.Getenv("WEBMUX_SSH_COORD_RECONNECT") == "1" {
		paneCommand := quote(remote.Tmux) + " -L " + quote(remote.Name) + " display-message -p -t " + quote("=webmux-"+created.ID+":") + " " + quote("#{pane_pid}")
		before := strings.TrimSpace(string(runSSH(paneCommand, nil)))
		oldControl := s.sessions.SSHControlPath(created.ID)
		if _, err := s.sessions.Reconnect("owner", created.ID, ""); err != nil {
			t.Fatal(err)
		}
		deadline = time.Now().Add(20 * time.Second)
		configured := false
		for time.Now().Before(deadline) {
			v, _ := s.sessions.Get("owner", created.ID)
			if v.CoordinationChannel == "ssh-stream-local-configured" {
				configured = true
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		if !configured || oldControl == s.sessions.SSHControlPath(created.ID) {
			t.Fatal("reconnect did not establish its own verified channel/run")
		}
		after := strings.TrimSpace(string(runSSH(paneCommand, nil)))
		if before == "" || before != after {
			t.Fatal("reconnect replaced the existing remote provider")
		}
		second := strings.ReplaceAll(strings.ReplaceAll(command, "native-ssh-work-1", "native-ssh-work-2"), "native SSH channel qualification", "native SSH channel reconnected")
		if err := s.sessions.Input("owner", created.ID, second); err != nil {
			t.Fatal(err)
		}
		deadline = time.Now().Add(20 * time.Second)
		updated := false
		for time.Now().Before(deadline) {
			node, _ := s.coordinationNode()
			snapshots, _ := s.coordinationSnapshots()
			view, err = s.sessionCoord.service.OwnerView(node, "owner", s.coordinationInputs(node, snapshots))
			if err != nil {
				t.Fatal(err)
			}
			for _, member := range view.Members {
				if member.Ref.Session == created.ID && member.Task == "native SSH channel reconnected" {
					updated = true
				}
			}
			if updated {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if !updated {
			text, _ := s.sessions.Scrollback("owner", created.ID)
			t.Fatalf("retained shell did not use its refreshed access: %s", text)
		}
		t.Log("reconnect replaced only the private SSH channel, preserved the exact remote pane PID, and the existing shell used its new locator")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.MarshalIndent(view, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "view.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	text, _ := s.sessions.Scrollback("owner", created.ID)
	_ = os.WriteFile(filepath.Join(dir, "terminal.log"), []byte(text), 0600)
	check := exec.Command(realSSH, "-S", s.sessions.SSHControlPath(created.ID), "-O", "check", "--", host)
	master, err := check.CombinedOutput()
	if err != nil {
		t.Fatalf("private SSH master was not observable before close: %v %s", err, master)
	}
	pid := regexp.MustCompile(`pid=([0-9]+)`).FindSubmatch(master)
	if len(pid) != 2 {
		t.Fatal("master identity missing")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state, err := exec.Command("/bin/ps", "-o", "stat=", "-p", string(pid[1])).Output()
		if err != nil || strings.TrimSpace(string(state)) == "" || strings.HasPrefix(strings.TrimSpace(string(state)), "Z") {
			stopped = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !stopped {
		t.Fatal("source close left its private coordination SSH master running")
	}
	if refused {
		t.Log("forward refusal preserved a usable ordinary SSH shell, no scoped interface, pending onboarding and private master cleanup")
	} else {
		if native {
			t.Log("real remote Claude launched through the ordinary shell alias and updated its verified SSH member; audit the retained native transcript before accepting MCP adoption")
		} else {
			t.Log("real SSH startup provisioned a private authenticated channel before provider start; remote ordinary CLI read its view and updated its verified execution-host project membership; private master exits on close; no native coding-agent or restart acceptance")
		}
	}
}
