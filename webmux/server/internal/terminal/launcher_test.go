package terminal

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func testPlanner() planner {
	return planner{platform: "linux", home: "/home/test", environment: []string{"TERM=old", "SHELL=/bin/sh", "KEEP=value"}, lookup: func(name string) (string, error) { return "/resolved/" + name, nil }}
}
func testRequest() LaunchRequest {
	return LaunchRequest{Hostname: "host.example", Username: "alice", Port: 2222, Cols: 80, Rows: 24, Transport: "ssh"}
}

func TestSSHCommandCompatibility(t *testing.T) {
	p := testPlanner()
	command, err := p.build(testRequest(), "", "/keys/key with spaces", "")
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{"-tt", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3", "-o", "ConnectTimeout=10", "-o", "TCPKeepAlive=yes", "-o", "StrictHostKeyChecking=accept-new", "-p", "2222", "-i", "/keys/key with spaces", "-l", "alice", "host.example"}
	if command.Path != "/resolved/ssh" || !reflect.DeepEqual(command.Args, expected) {
		t.Fatalf("command: %+v", command)
	}
	if !reflect.DeepEqual(command.Env, []string{"SHELL=/bin/sh", "KEEP=value", "TERM=xterm-256color"}) {
		t.Fatalf("environment: %v", command.Env)
	}
	if p.environment[0] != "TERM=old" {
		t.Fatal("modified parent environment")
	}
	withPassword, err := p.build(testRequest(), "fixture-password", "/keys/key with spaces", "")
	if err != nil {
		t.Fatal(err)
	}
	if withPassword.Path != "/resolved/sshpass" || !reflect.DeepEqual(withPassword.Args, append([]string{"-e", "/resolved/ssh"}, expected...)) {
		t.Fatal("wrong password transport arguments")
	}
	if strings.Contains(strings.Join(withPassword.Args, " "), "fixture-password") {
		t.Fatal("password leaked into argv")
	}
	if withPassword.Env[len(withPassword.Env)-1] != "SSHPASS=fixture-password" {
		t.Fatal("password was not passed through environment")
	}
}

func TestExecCommandCompatibility(t *testing.T) {
	p := testPlanner()
	request := testRequest()
	request.Transport = "exec"
	request.ExecCommand = "printf '%s' '{user}@{host}:{port}'"
	command, err := p.build(request, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(command.Args, []string{"-c", "printf '%s' 'alice@host.example:2222'"}) {
		t.Fatalf("shell argv: %v", command.Args)
	}
	p.platform = "windows"
	p.environment = []string{`ComSpec=C:\Windows\System32\cmd.exe`}
	command, err = p.build(request, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if command.ShellCommand != "printf '%s' 'alice@host.example:2222'" || len(command.Args) != 0 {
		t.Fatal("Windows shell line must bypass CRT quoting")
	}
	request.ExecArgv = []string{"helper", "argument with spaces", "literal \"quotes\""}
	request.ExecCwd = "/workspace"
	command, err = p.build(request, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(command.Args, request.ExecArgv[1:]) || command.ShellCommand != "" || command.Dir != "/workspace" {
		t.Fatalf("explicit argv changed: %+v", command)
	}
	request.ExecArgv = nil
	request.ExecCommand = ""
	p.environment = []string{"WEBMUX_EXEC_COMMAND=echo {host}"}
	command, err = p.build(request, "", "", "")
	if err != nil || command.ShellCommand != "echo host.example" {
		t.Fatalf("environment template: %+v, %v", command, err)
	}
}

func TestMoshCommandCompatibility(t *testing.T) {
	request := testRequest()
	request.Transport = "mosh"
	command, err := testPlanner().build(request, "", "/keys/alice's key", "/usr/local/bin/mosh-server")
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{`--ssh=ssh -o StrictHostKeyChecking=accept-new -p 2222 -i '/keys/alice'"'"'s key'`, "--server=/usr/local/bin/mosh-server", "alice@host.example"}
	if !reflect.DeepEqual(command.Args, expected) {
		t.Fatalf("mosh args: %v", command.Args)
	}
}

func TestLauncherRejectsInvalidInputsAndMissingTools(t *testing.T) {
	for _, change := range []func(*LaunchRequest){
		func(r *LaunchRequest) { r.Hostname = "" },
		func(r *LaunchRequest) { r.Hostname = "host with spaces" },
		func(r *LaunchRequest) { r.Username = "user with spaces" },
		func(r *LaunchRequest) { r.Port = -1 },
		func(r *LaunchRequest) { r.Port = 65536 },
		func(r *LaunchRequest) { r.Transport = "unknown" },
		func(r *LaunchRequest) { r.Transport = "exec" },
	} {
		request := testRequest()
		change(&request)
		if _, err := testPlanner().build(request, "", "", ""); err == nil {
			t.Fatalf("accepted invalid request: %+v", request)
		}
	}
	p := testPlanner()
	p.lookup = func(name string) (string, error) {
		if name == "ssh" {
			return "/ssh", nil
		}
		return "", errors.New("missing")
	}
	if _, err := p.build(testRequest(), "password", "", ""); err == nil || err.Error() != "Password authentication requires sshpass to be installed on the jump box" {
		t.Fatalf("missing sshpass: %v", err)
	}
	request := testRequest()
	request.Transport = "mosh"
	if _, err := p.build(request, "", "", ""); err == nil || err.Error() != "mosh is not installed on this system" {
		t.Fatalf("missing mosh: %v", err)
	}
	if _, err := testPlanner().build(request, "", "", "relative/path"); err == nil {
		t.Fatal("accepted invalid server path")
	}
}
