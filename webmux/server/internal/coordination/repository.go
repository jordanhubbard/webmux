package coordination

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/gitobserve"
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
	ctx = gitobserve.EnsureCommandScope(ctx)
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
	run := func(phase string, args ...string) (string, error) {
		fixed := []string{"--no-optional-locks", "-c", "core.fsmonitor=false", "-C", directory}
		fixed = append(fixed, args...)
		preparedAt := time.Now()
		command, preparation := gitobserve.CommandContext(ctx, fixed...)
		command.Env = env
		command.WaitDelay = 200 * time.Millisecond
		var out bytes.Buffer
		command.Stdout = &limitedRepositoryOutput{buffer: &out}
		command.Stderr = &bytes.Buffer{}
		if err := gitobserve.Run(ctx, phase, preparedAt, command, preparation); err != nil {
			return "", ErrRepository
		}
		return strings.TrimSpace(out.String()), nil
	}
	// Read the same six witnesses with three processes. Keep all preparation and
	// Git work inside the existing shared two-second budget.
	layout, err := run("repository.layout", "rev-parse", "--is-shallow-repository", "--show-toplevel", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, ErrRepository
	}
	parts := strings.Split(layout, "\n")
	if len(parts) != 3 || strings.TrimSpace(parts[0]) != "false" {
		return nil, ErrRepository
	}
	root, branch := strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
	if !filepath.IsAbs(root) || branch == "" {
		return nil, ErrRepository
	}
	if branch == "HEAD" {
		branch = "(detached)"
	}
	config, err := run("repository.config", "config", "--local", "--null", "--get-regexp", `^(remote\.origin\.url|core\.ignorecase)$`)
	if err != nil {
		return nil, ErrRepository
	}
	remote, folded, ok := repositoryConfigValues(config)
	if !ok {
		return nil, ErrRepository
	}
	canonical, ok := canonicalRepositoryRemote(remote)
	if !ok {
		return nil, ErrRepository
	}
	roots, err := run("repository.roots", "rev-list", "--max-parents=0", "--max-count=33", "HEAD")
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
	binding := &Binding{ID: digest([]byte("webmux-repository/v1\n" + canonical + "\n" + strings.Join(witnesses, "\n"))), Repository: canonical, Worktree: root, Branch: branch, Source: "verified-git", CaseFold: folded == "true"}
	// A completed final command must not turn cancellation into an identity.
	if ctx.Err() != nil || !validBinding(binding) {
		return nil, ErrRepository
	}
	return binding, nil
}

// --null --get-regexp separates each key from its value with a newline, and
// terminates each record with NUL. As with --get, the last local value wins.
func repositoryConfigValues(data string) (remote, folded string, ok bool) {
	if !strings.HasSuffix(data, "\x00") {
		return "", "", false
	}
	for _, record := range strings.Split(strings.TrimSuffix(data, "\x00"), "\x00") {
		// A valueless entry is emitted as just its key. Preserve --get's
		// empty-string interpretation rather than treating it as "true".
		key, value, _ := strings.Cut(record, "\n")
		switch key {
		case "remote.origin.url":
			remote, ok = strings.TrimSpace(value), true
		case "core.ignorecase":
			folded = strings.TrimSpace(value)
		default:
			return "", "", false
		}
	}
	return remote, folded, ok
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
