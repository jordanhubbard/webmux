package terminal

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/recovery"

	"github.com/jordanhubbard/webmux/server/internal/nativeprovider"
)

type providerTerminal struct{ *Process }

func (p providerTerminal) Wait() (int, error) { exit := p.Process.Wait(); return exit.Code, exit.Err }
func (p providerTerminal) PID() int           { return p.native.(*windowsProcess).process.Pid }

// RunNativeProviderHelper is the private framed-stdin entry point used by
// the independent owner of a Windows local terminal.
func RunNativeProviderHelper(ctx context.Context, mode string, input io.Reader, output io.Writer) error {
	if mode == "--native-provider-bootstrap" {
		return nativeprovider.Bootstrap(ctx, input, output)
	}
	if mode != "--native-provider-owner" {
		return errors.New("unknown native provider helper mode")
	}
	return nativeprovider.Serve(ctx, input, output, func(spec nativeprovider.Launch) (nativeprovider.Terminal, error) {
		p, err := Start(Command{Path: spec.Program, Args: spec.Args, Dir: spec.Directory, Env: spec.Environment, Cols: spec.Cols, Rows: spec.Rows})
		if err != nil {
			return nil, err
		}
		return providerTerminal{p}, nil
	})
}

// A local viewer owns this connection, not the provider's process tree.
// Close, including controller shutdown, detaches without stopping the job.
type nativeViewer struct {
	*nativeprovider.Client
	output io.Reader
}

func (v *nativeViewer) Read(p []byte) (int, error)  { return v.output.Read(p) }
func (v *nativeViewer) resize(cols, rows int) error { return v.Client.Resize(cols, rows) }
func (v *nativeViewer) kill() error                 { return v.Client.Close() }
func (v *nativeViewer) wait() Exit                  { code, err := v.Client.Wait(); return Exit{Code: code, Err: err} }

func startLaunch(request LaunchRequest, command Command) (*Process, error) {
	if request.Transport != "local" {
		return Start(command)
	}
	if err := recovery.InitializeHostEvidence(request.SessionID, request.RecoveryKey, false, request.RecoverySeed); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	identity, event, err := recovery.EnsureNativeProvider(ctx, request.SessionID, request.RecoveryKey, command.Path, nativeprovider.Launch{Directory: command.Dir, Environment: command.Env, Cols: command.Cols, Rows: command.Rows})
	if err != nil {
		return nil, err
	}
	client, err := nativeprovider.Attach(ctx, identity, request.RecoveryKey, command.Cols, command.Rows)
	if err != nil {
		return nil, err
	}
	// This fresh signed event starts the broker's authenticated host-state
	// polling. Cached shell/CLI events must not be replayed as live events:
	// the snapshot reconciler uses the control state to determine readiness.
	packet, err := recovery.SignedPacket(event, request.RecoveryKey)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return newProcess(&nativeViewer{Client: client, output: io.MultiReader(strings.NewReader(packet), client)}), nil
}
