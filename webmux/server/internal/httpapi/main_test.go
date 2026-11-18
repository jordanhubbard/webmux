package httpapi

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/agentdeckattach"
)

// Real terminal restoration executes the same child-mode entry point as the
// standalone server, before Go's test flag parser handles the parent process.
func TestMain(m *testing.M) {
	if handled, err := federationRendererHelper(); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if len(os.Args) >= 2 && os.Args[1] == agentdeckattach.Helper {
		if err := agentdeckattach.Run(context.Background(), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
