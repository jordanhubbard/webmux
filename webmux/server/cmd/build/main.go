// Command build creates a standalone server from a prebuilt frontend.
// Run from webmux/server: go run ./cmd/build -o ../bin/webmux
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/buildinfo"
)

func main() {
	binary := "webmux"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	output := flag.String("o", filepath.Join("..", "bin", binary), "output executable")
	web := flag.String("web", "../web", "production frontend directory")
	defaults := flag.String("defaults", "../config.defaults", "configuration defaults directory")
	goos := flag.String("goos", runtime.GOOS, "target operating system")
	goarch := flag.String("goarch", runtime.GOARCH, "target architecture")
	flag.Parse()
	if err := build(*output, *web, *defaults, *goos, *goarch); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func build(output, web, defaults, goos, goarch string) error {
	module, err := os.Getwd()
	if err != nil {
		return err
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return err
	}
	observed, err := observeSource(module)
	if err != nil {
		return err
	}
	// Fail before compiling rather than ship a binary missing its UI or defaults.
	for _, name := range []string{filepath.Join(web, "index.html"), filepath.Join(defaults, "app.yaml"), filepath.Join(defaults, "auth.yaml"), filepath.Join(defaults, "hosts.yaml"), filepath.Join(defaults, "clients.yaml"), filepath.Join(defaults, "keys.yaml"), filepath.Join(defaults, "layout.yaml")} {
		if info, err := os.Lstat(name); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("missing or invalid build input %s (run the frontend build first)", name)
		}
	}
	stage, err := os.MkdirTemp("", "webmux-build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	// Each build owns its source and assets, so concurrent platform builds cannot
	// mix bundles and generated JavaScript never enters the source tree or git.
	for _, name := range []string{"go.mod", "go.sum", "cmd", "internal"} {
		if err := copyTree(filepath.Join(module, name), filepath.Join(stage, name)); err != nil {
			return err
		}
	}
	for name, source := range map[string]string{"web": web, "config.defaults": defaults} {
		if err := copyTree(source, filepath.Join(stage, "internal", "assets", "bundle", name)); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	// A moved HEAD, changed package version or changed dirty observation must
	// not silently become the staged binary's reported source identity.
	after, err := observeSource(module)
	if err != nil {
		return err
	}
	if after != observed {
		return errors.New("build metadata changed while staging source")
	}
	command := exec.Command("go", "build", "-trimpath", "-tags=embedded", "-ldflags", observed.linkerFlags(), "-o", output, "./cmd/webmux")
	command.Dir = stage
	for _, variable := range os.Environ() {
		key, _, _ := strings.Cut(variable, "=")
		switch strings.ToUpper(key) {
		case "GOOS", "GOARCH", "CGO_ENABLED", "GOWORK":
			continue
		}
		command.Env = append(command.Env, variable)
	}
	command.Env = append(command.Env, "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0", "GOWORK=off")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command.Run()
}

type sourceObservation struct{ version, revision, modified string }

func observeSource(module string) (sourceObservation, error) {
	var result sourceObservation
	data, err := os.ReadFile(filepath.Join(module, "..", "package.json"))
	if err == nil {
		var metadata struct {
			Version string `json:"version"`
		}
		if len(data) > 1<<20 || json.Unmarshal(data, &metadata) != nil || !buildinfo.ValidVersion(metadata.Version) {
			return result, errors.New("invalid build package version")
		}
		result.version = metadata.Version
	} else if !errors.Is(err, os.ErrNotExist) {
		return result, errors.New("cannot read build package version")
	}
	// Source archives and module-only copies do not invent a revision. Require
	// the module itself to be tracked, rather than accepting an unrelated outer
	// repository or caller-supplied GIT_DIR/GIT_WORK_TREE.
	if _, ok := observeGit(module, "ls-files", "--error-unmatch", "--", "go.mod"); !ok {
		return result, nil
	}
	if revision, ok := observeGit(module, "rev-parse", "HEAD"); ok && buildinfo.ValidRevision(revision) {
		result.revision = revision
		if status, ok := observeGit(module, "status", "--porcelain", "--untracked-files=normal"); ok {
			result.modified = "false"
			if status != "" {
				result.modified = "true"
			}
		}
	}
	return result, nil
}

func observeGit(module string, arguments ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks", "-c", "core.fsmonitor=false", "-C", module}, arguments...)...)
	for _, variable := range os.Environ() {
		key, _, _ := strings.Cut(variable, "=")
		if !strings.HasPrefix(strings.ToUpper(key), "GIT_") {
			command.Env = append(command.Env, variable)
		}
	}
	output, err := command.Output()
	if err != nil || len(output) > 1<<20 {
		return "", false
	}
	return strings.TrimSpace(string(output)), true
}

func (s sourceObservation) linkerFlags() string {
	const prefix = "github.com/jordanhubbard/webmux/server/internal/buildinfo."
	var flags []string
	for _, field := range []struct{ name, value string }{{"version", s.version}, {"revision", s.revision}, {"modified", s.modified}} {
		if field.value != "" {
			flags = append(flags, "-X", prefix+field.name+"="+field.value)
		}
	}
	return strings.Join(flags, " ")
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, name)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("cannot package non-regular file %s", name)
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = file.Write(data)
		return errors.Join(err, file.Close())
	})
}
