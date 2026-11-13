//go:build windows

package recovery

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/jordanhubbard/webmux/server/internal/nativeprovider"
)

// Local Windows viewers attach directly, without a second console. The
// legacy remote --session-attach path still requires a qualified native viewer.
func EnsureProvider(context.Context, string, string) (Event, error) {
	return Event{}, errors.New("Windows durable recovery is available through a local terminal on this WebMux server; remote native attachment remains unsupported")
}

// EnsureNativeProvider shares the durable execution-host state with the RPC
// worker. A surviving owner is authoritative; only provider loss starts a new
// shell, which waits for an explicit choice when prior activity is recorded.
func EnsureNativeProvider(ctx context.Context, id, key, helper string, launch nativeprovider.Launch) (nativeprovider.Identity, Event, error) {
	cache, err := HostCacheRoot()
	if err != nil {
		return nativeprovider.Identity{}, Event{}, err
	}
	var identity nativeprovider.Identity
	event := Event{Kind: "provider", Mode: "ready", Platform: "windows", Helper: helper}
	_, err = updateHostState(id, key, false, func(state *HostState) error {
		env := make([]string, 0, len(launch.Environment)+4)
		for _, item := range launch.Environment {
			name, _, _ := strings.Cut(item, "=")
			if !strings.HasPrefix(strings.ToUpper(name), "WEBMUX_RECOVERY_") {
				env = append(env, item)
			}
		}
		wait := "0"
		if state.Command != nil || state.CLI != nil {
			wait = "1"
			event.Mode = "waiting"
		}
		env = append(env, "WEBMUX_RECOVERY_SESSION="+id, "WEBMUX_RECOVERY_KEY="+key, "WEBMUX_RECOVERY_CACHE_HOME="+cache, "WEBMUX_RECOVERY_WAIT="+wait)
		launch.Program, launch.Args, launch.Environment = helper, []string{"--browser-shell"}, env
		var survived bool
		var err error
		identity, survived, err = nativeprovider.Ensure(ctx, nativeprovider.Config{Root: filepath.Join(cache, "webmux", "native-providers", id), Session: id, Key: key, Executable: helper, Launch: launch})
		if err != nil {
			return err
		}
		event.Survived = survived
		if survived {
			event.Mode = "ready"
			if state.Control != nil && state.Control.Mode == "waiting" {
				event.Mode = "waiting"
			}
		} else {
			state.Control, state.Replay = nil, nil
		}
		state.Provider = &event
		return nil
	})
	return identity, event, err
}
