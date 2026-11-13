package terminal

import (
	"context"
	"errors"
	"io"

	"github.com/jordanhubbard/webmux/server/internal/nativeprovider"
)

type providerTerminal struct{ *Process }

func (p providerTerminal) Wait() (int, error) { exit := p.Process.Wait(); return exit.Code, exit.Err }
func (p providerTerminal) PID() int           { return p.native.(*windowsProcess).process.Pid }

// RunNativeProviderHelper is a private, framed-stdin entry point. Normal
// launchers do not use it until durable recovery/shell integration is ready.
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
