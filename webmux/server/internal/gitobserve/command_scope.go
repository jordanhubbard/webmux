package gitobserve

import (
	"context"
	"os/exec"
	"path/filepath"
	"sync"
)

type commandScopeKey struct{}

// Only executable resolution is shared. Every command retains its own context,
// arguments, environment, output limits and cancellation. Never cache globally.
type commandScope struct {
	once sync.Once
	path string
	err  error
}

// WithCommandScope starts a fresh, lazily resolved observation. Call it after
// account selection and after starting the observation deadline.
func WithCommandScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, commandScopeKey{}, &commandScope{})
}

// EnsureCommandScope lets a repository witness inherit its source observation's
// executable, or creates a private scope for a standalone witness. No lookup
// occurs here, so the witness deadline still includes its first resolution.
func EnsureCommandScope(ctx context.Context) context.Context {
	if _, ok := ctx.Value(commandScopeKey{}).(*commandScope); ok {
		return ctx
	}
	return WithCommandScope(ctx)
}

type Preparation struct{ resolution string }

// CommandContext preserves Git's original argv[0] and constructor errors while
// avoiding another PATH search for every metadata command in one observation.
func CommandContext(ctx context.Context, args ...string) (*exec.Cmd, Preparation) {
	scope, ok := ctx.Value(commandScopeKey{}).(*commandScope)
	if !ok {
		return exec.CommandContext(ctx, "git", args...), Preparation{}
	}
	var first *exec.Cmd
	scope.once.Do(func() {
		first = exec.CommandContext(ctx, "git", args...)
		scope.path, scope.err = first.Path, first.Err
		if scope.err == nil && !filepath.IsAbs(scope.path) {
			if absolute, err := filepath.Abs(scope.path); err == nil {
				scope.path = absolute
			} else if filepath.Base(scope.path) == scope.path {
				scope.path = "." + string(filepath.Separator) + scope.path
			}
		}
	})
	if first != nil {
		return first, Preparation{resolution: "initial"}
	}
	if scope.err != nil {
		// Start returns this same constructor error without retrying resolution.
		return &exec.Cmd{Path: scope.path, Args: append([]string{"git"}, args...), Err: scope.err}, Preparation{resolution: "reused"}
	}
	command := exec.CommandContext(ctx, scope.path, args...)
	command.Args[0] = "git"
	return command, Preparation{resolution: "reused"}
}
