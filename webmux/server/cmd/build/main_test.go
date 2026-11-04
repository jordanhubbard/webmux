package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/buildinfo"
)

func TestObserveSourcePreservesArchiveUnknownsAndGitDirtyState(t *testing.T) {
	root := t.TempDir()
	module := filepath.Join(root, "server")
	if err := os.Mkdir(module, 0755); err != nil {
		t.Fatal(err)
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(module, "go.mod"), "module fixture\n")
	value, err := observeSource(module)
	if err != nil || value != (sourceObservation{}) {
		t.Fatal("module copy invented metadata", value, err)
	}
	write(filepath.Join(root, "package.json"), `{"version":"1.4.2"}`)
	value, err = observeSource(module)
	if err != nil || value != (sourceObservation{version: "1.4.2"}) {
		t.Fatal("archive invented Git facts", value, err)
	}
	git := func(arguments ...string) string {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture Git failed: %v: %s", err, output)
		}
		return strings.TrimSpace(string(output))
	}
	git("init", "-b", "main")
	git("add", ".")
	git("-c", "user.name=Build fixture", "-c", "user.email=build@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath="+t.TempDir(), "commit", "-m", "Source fixture")
	// An inherited alternate Git directory must not override the source checkout.
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "unrelated"))
	value, err = observeSource(module)
	if err != nil || value.version != "1.4.2" || !buildinfo.ValidRevision(value.revision) || value.modified != "false" {
		t.Fatal("clean source observation failed", value, err)
	}
	write(filepath.Join(module, "untracked.go"), "package fixture\n")
	dirty, err := observeSource(module)
	if err != nil || dirty.revision != value.revision || dirty.modified != "true" {
		t.Fatal("untracked compiled input hidden", dirty, err)
	}
	write(filepath.Join(root, "package.json"), `{"version":"1.4.2 -X injected"}`)
	if _, err := observeSource(module); err == nil {
		t.Fatal("unsafe linker version accepted")
	}
}

func TestCanonicalBuilderLinksMetadataIntoRunningServer(t *testing.T) {
	module, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(module)
	expected, err := observeSource(module)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	defaults := filepath.Join(root, "defaults")
	if err := copyTree(filepath.Join(module, "..", "config.defaults"), defaults); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defaults, "auth.yaml"), []byte("auth:\n  mode: none\n  users: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	web := filepath.Join(root, "web")
	if err := os.Mkdir(web, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "index.html"), []byte("<html>owned builder fixture</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "webmux")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	if err := build(executable, web, defaults, runtime.GOOS, runtime.GOARCH); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable, "--home", filepath.Join(root, "home"), "--listen", "127.0.0.1:0")
	for _, variable := range os.Environ() {
		key, _, _ := strings.Cut(variable, "=")
		upper := strings.ToUpper(key)
		if !strings.HasPrefix(upper, "WEBMUX_") && upper != "HTTP_PORT" && upper != "HTTPS_PORT" && upper != "JWT_SECRET" {
			command.Env = append(command.Env, variable)
		}
	}
	command.Env = append(command.Env, "WEBMUX_COORDINATION_DISABLED=1")
	output, err := command.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	address := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		pattern := regexp.MustCompile(`url=(http://127\.0\.0\.1:[0-9]+)`)
		for scanner.Scan() {
			if match := pattern.FindStringSubmatch(scanner.Text()); len(match) == 2 {
				select {
				case address <- match[1]:
				default:
				}
			}
		}
	}()
	waited := make(chan struct{})
	var waitErr error
	go func() { waitErr = command.Wait(); close(waited) }()
	t.Cleanup(func() {
		_ = command.Process.Kill()
		select {
		case <-waited:
		case <-time.After(5 * time.Second):
			t.Error("owned fixture server did not reap")
		}
	})
	var origin string
	select {
	case origin = <-address:
	case <-waited:
		t.Fatalf("owned fixture server exited before listening: %v", waitErr)
	case <-time.After(15 * time.Second):
		t.Fatal("owned fixture server did not listen")
	}
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(origin + "/api/server-info")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var value struct {
		Contract string
		Build    buildinfo.Info
	}
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || value.Contract != "webmux-server-info/v1" || value.Build.Attested || value.Build.Provenance != "build-producer-self-report" || value.Build.Version == nil || *value.Build.Version != expected.version {
		t.Fatalf("producer metadata not linked: %#v", value)
	}
	if expected.revision == "" {
		if value.Build.Revision != nil || value.Build.Modified != nil {
			t.Fatal("archive build invented Git facts", value.Build)
		}
	} else if value.Build.Revision == nil || *value.Build.Revision != expected.revision || value.Build.Modified == nil || *value.Build.Modified != (expected.modified == "true") {
		t.Fatal("staged build lost original Git observations", value.Build)
	}
}
