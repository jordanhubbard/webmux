package terminal

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/jordanhubbard/webmux/server/internal/config"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

// LaunchRequest is internal session state, never an unvalidated HTTP request.
// Passwords are separate so they cannot accidentally enter persisted sessions.
type LaunchRequest struct {
	Hostname, Username, Transport, KeyID string
	Port, Cols, Rows                     int
	ExecCommand                          string
	ExecArgv                             []string
	ExecCwd                              string
}

type Launcher struct{ Store *storage.Store }

func (l Launcher) Launch(request LaunchRequest, password string) (*Process, error) {
	keyPath, moshServer := "", ""
	if request.Transport != "exec" && request.KeyID != "" {
		var document struct {
			Keys []struct {
				ID   string `yaml:"id"`
				Path string `yaml:"private_key_path"`
			} `yaml:"keys"`
		}
		if err := l.Store.ReadConfig("keys.yaml", &document); err != nil {
			return nil, fmt.Errorf("load SSH keys: %w", err)
		}
		for _, key := range document.Keys {
			if key.ID == request.KeyID {
				keyPath = key.Path
				break
			}
		}
	}
	if request.Transport == "mosh" {
		var document struct {
			App struct {
				Transport struct {
					Server string `yaml:"mosh_server_path"`
				} `yaml:"transport"`
			} `yaml:"app"`
		}
		if err := l.Store.ReadConfig("app.yaml", &document); err != nil {
			return nil, fmt.Errorf("load mosh configuration: %w", err)
		}
		moshServer = document.App.Transport.Server
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	currentUser := ""
	if u, err := user.Current(); err == nil {
		currentUser = u.Username
	}
	command, err := (planner{platform: runtime.GOOS, home: home, environment: os.Environ(), lookup: exec.LookPath, currentUser: currentUser}).build(request, password, keyPath, moshServer)
	if err != nil {
		return nil, err
	}
	return Start(command)
}

type planner struct {
	platform, home, currentUser string
	environment                 []string
	lookup                      func(string) (string, error)
}

func isLoopbackHost(hostname string) bool {
	switch strings.ToLower(hostname) {
	case "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	}
	return false
}

var hostname = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9.])?$|^\[[0-9a-fA-F:]+\]$`)
var username = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func (p planner) env(name string) string {
	for i := len(p.environment) - 1; i >= 0; i-- {
		key, value, ok := strings.Cut(p.environment[i], "=")
		if ok && (key == name || p.platform == "windows" && strings.EqualFold(key, name)) {
			return value
		}
	}
	return ""
}

func (p planner) build(request LaunchRequest, password, keyPath, moshServer string) (Command, error) {
	var command Command
	if request.Hostname == "" {
		return command, errors.New("Hostname is required")
	}
	if len(request.Hostname) > 255 {
		return command, errors.New("Hostname too long")
	}
	if !hostname.MatchString(request.Hostname) {
		return command, errors.New("Invalid hostname: " + request.Hostname)
	}
	if len(request.Username) > 64 {
		return command, errors.New("Username too long")
	}
	if request.Username != "" && !username.MatchString(request.Username) {
		return command, errors.New("Invalid username: " + request.Username)
	}
	if request.Port == 0 {
		request.Port = 22
	}
	if request.Port < 1 || request.Port > 65535 {
		return command, errors.New("port must be an integer between 1 and 65535")
	}
	if err := dimensions(request.Cols, request.Rows); err != nil {
		return command, err
	}
	command = Command{Dir: p.home, Cols: request.Cols, Rows: request.Rows, Env: append([]string{}, p.environment...)}
	// Remove inherited TERM rather than relying on duplicate-variable behavior.
	for i := len(command.Env) - 1; i >= 0; i-- {
		key, _, _ := strings.Cut(command.Env[i], "=")
		if key == "TERM" || p.platform == "windows" && strings.EqualFold(key, "TERM") {
			command.Env = append(command.Env[:i], command.Env[i+1:]...)
		}
	}
	command.Env = append(command.Env, "TERM=xterm-256color")
	resolve := func(name string) (string, error) {
		path, err := p.lookup(name)
		if err != nil {
			return "", fmt.Errorf("%s is not installed or is not available on PATH", name)
		}
		return path, nil
	}
	var err error
	switch request.Transport {
	case "exec":
		if request.ExecCwd != "" {
			command.Dir = request.ExecCwd
		}
		if len(request.ExecArgv) > 0 {
			command.Path, err = resolve(request.ExecArgv[0])
			command.Args = append([]string{}, request.ExecArgv[1:]...)
			break
		}
		template := request.ExecCommand
		if template == "" {
			template = p.env("WEBMUX_EXEC_COMMAND")
		}
		if template == "" {
			return Command{}, errors.New("exec transport requires exec_command or WEBMUX_EXEC_COMMAND env var")
		}
		line := strings.NewReplacer("{host}", request.Hostname, "{port}", strconv.Itoa(request.Port), "{user}", request.Username).Replace(template)
		if p.platform == "windows" {
			shell := p.env("COMSPEC")
			if shell == "" {
				shell = "cmd.exe"
			}
			command.Path, err = resolve(shell)
			command.ShellCommand = line
		} else {
			shell := strings.TrimSpace(p.env("SHELL"))
			if shell == "" {
				shell = "/bin/sh"
			}
			command.Path, err = resolve(shell)
			command.Args = []string{"-c", line}
		}
	case "mosh":
		command.Path, err = resolve("mosh")
		if err != nil {
			return Command{}, errors.New("mosh is not installed on this system")
		}
		ssh := "ssh -o StrictHostKeyChecking=accept-new -p " + strconv.Itoa(request.Port)
		if keyPath != "" {
			ssh += " -i " + shellQuote(keyPath)
		}
		command.Args = []string{"--ssh=" + ssh}
		if moshServer != "" {
			if err := config.ValidateMoshServerPath(moshServer); err != nil {
				return Command{}, err
			}
			command.Args = append(command.Args, "--server="+moshServer)
		}
		destination := request.Hostname
		if request.Username != "" {
			destination = request.Username + "@" + destination
		}
		command.Args = append(command.Args, destination)
	case "ssh", "":
		// On macOS, an ssh hop to the local machine lands in a fresh sshd
		// session with no Aqua/keychain attachment, breaking keyring-backed
		// CLIs (e.g. jira-cli) even though the webmux server itself runs in
		// the user's unlocked GUI session. Skip the unnecessary ssh hop for
		// same-user loopback connections so the spawned shell inherits that
		// session directly, same as any other locally spawned PTY.
		if p.platform == "darwin" && p.currentUser != "" && isLoopbackHost(request.Hostname) &&
			request.Port == 22 && password == "" && keyPath == "" &&
			(request.Username == "" || request.Username == p.currentUser) {
			shell := strings.TrimSpace(p.env("SHELL"))
			if shell == "" {
				shell = "/bin/sh"
			}
			command.Path, err = resolve(shell)
			command.Args = []string{"-l"}
			break
		}
		command.Path, err = resolve("ssh")
		command.Args = []string{"-tt", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3", "-o", "ConnectTimeout=10", "-o", "TCPKeepAlive=yes", "-o", "StrictHostKeyChecking=accept-new", "-p", strconv.Itoa(request.Port)}
		if keyPath != "" {
			command.Args = append(command.Args, "-i", keyPath)
		}
		if request.Username != "" {
			command.Args = append(command.Args, "-l", request.Username)
		}
		command.Args = append(command.Args, request.Hostname)
		if err == nil && password != "" {
			ssh := command.Path
			command.Path, err = resolve("sshpass")
			if err != nil {
				return Command{}, errors.New("Password authentication requires sshpass to be installed on the jump box")
			}
			command.Args = append([]string{"-e", ssh}, command.Args...)
			command.Env = append(command.Env, "SSHPASS="+password)
		}
	default:
		return Command{}, errors.New("unsupported terminal transport")
	}
	if err != nil {
		return Command{}, err
	}
	return command, nil
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'" }
