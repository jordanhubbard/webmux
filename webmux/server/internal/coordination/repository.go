package coordination

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrRepository = errors.New("verified repository identity is unavailable; an explicit execution-host mapping is required")

// InspectRepository reads local Git metadata and object history. It never
// contacts a remote, fetches, runs a hook, or uses a folder basename as identity.
// Shallow/unborn/remote-less or ambiguous repositories require an explicit map.
func InspectRepository(parent context.Context, directory string) (*Binding, error) {
	if !filepath.IsAbs(directory) {
		return nil, ErrRepository
	}
	directory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return nil, ErrRepository
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	env := []string{}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(strings.ToUpper(name), "GIT_") {
			env = append(env, entry)
		}
	}
	null := "/dev/null"
	if os.PathSeparator == '\\' {
		null = "NUL"
	}
	env = append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+null, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	run := func(args ...string) (string, error) {
		fixed := []string{"--no-optional-locks", "-c", "core.fsmonitor=false", "-C", directory}
		fixed = append(fixed, args...)
		command := exec.CommandContext(ctx, "git", fixed...)
		command.Env = env
		command.WaitDelay = 200 * time.Millisecond
		var out bytes.Buffer
		command.Stdout = &limitedRepositoryOutput{buffer: &out}
		command.Stderr = &bytes.Buffer{}
		if err := command.Run(); err != nil {
			return "", ErrRepository
		}
		return strings.TrimSpace(out.String()), nil
	}
	shallow, err := run("rev-parse", "--is-shallow-repository")
	if err != nil || shallow != "false" {
		return nil, ErrRepository
	}
	root, err := run("rev-parse", "--show-toplevel")
	if err != nil || !filepath.IsAbs(root) {
		return nil, ErrRepository
	}
	remote, err := run("config", "--local", "--get", "remote.origin.url")
	if err != nil {
		return nil, ErrRepository
	}
	canonical, ok := canonicalRepositoryRemote(remote)
	if !ok {
		return nil, ErrRepository
	}
	roots, err := run("rev-list", "--max-parents=0", "--max-count=33", "HEAD")
	if err != nil {
		return nil, ErrRepository
	}
	witnesses := strings.Fields(roots)
	if len(witnesses) == 0 || len(witnesses) > 32 {
		return nil, ErrRepository
	}
	for _, oid := range witnesses {
		if !validHex(oid, 20) && !validHex(oid, 32) {
			return nil, ErrRepository
		}
	}
	sort.Strings(witnesses)
	branch, err := run("symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		branch = "(detached)"
	}
	folded, _ := run("config", "--local", "--get", "core.ignorecase")
	binding := &Binding{ID: digest([]byte("webmux-repository/v1\n" + canonical + "\n" + strings.Join(witnesses, "\n"))), Repository: canonical, Worktree: root, Branch: branch, Source: "verified-git", CaseFold: folded == "true"}
	if !validBinding(binding) {
		return nil, ErrRepository
	}
	return binding, nil
}

type limitedRepositoryOutput struct{ buffer *bytes.Buffer }

func (w *limitedRepositoryOutput) Write(p []byte) (int, error) {
	if w.buffer.Len()+len(p) > 8192 {
		return 0, ErrRepository
	}
	return w.buffer.Write(p)
}
func canonicalRepositoryRemote(raw string) (string, bool) {
	if !validText(raw, 4096) {
		return "", false
	}
	// Normalize the common SCP-style SSH and HTTPS spellings of the same remote.
	if !strings.Contains(raw, "://") {
		colon := strings.Index(raw, ":")
		if colon <= 0 || strings.ContainsAny(raw[:colon], "/\\") || strings.Contains(raw, "\\") {
			return "", false
		}
		host := raw[:colon]
		if at := strings.LastIndex(host, "@"); at >= 0 {
			host = host[at+1:]
		}
		if host == "" {
			return "", false
		}
		raw = "ssh://" + host + "/" + strings.TrimPrefix(raw[colon+1:], "/")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && parsed.Scheme != "ssh" && parsed.Scheme != "http") {
		return "", false
	}
	repository := strings.TrimSuffix(strings.Trim(parsed.Path, "/"), ".git")
	if repository == "" || strings.Contains(repository, "//") {
		return "", false
	}
	for _, part := range strings.Split(repository, "/") {
		if part == "." || part == ".." || part == "" {
			return "", false
		}
	}
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	defaultPort := map[string]string{"ssh": "22", "https": "443", "http": "80"}[parsed.Scheme]
	if port != "" && port != defaultPort {
		host = strings.ToLower(parsed.Host)
	}
	return host + "/" + repository, true
}
