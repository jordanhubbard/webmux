package httpapi

import (
	"fmt"
	"os"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

// Real terminal restoration executes the same child-mode entry point as the
// standalone server, before Go's test flag parser handles the parent process.
func TestMain(m *testing.M) {
	if os.Getenv("WEBMUX_TEST_SSH_COORDINATION") == "1" && len(os.Args) == 3 && os.Args[1] == "--browser-connect" {
		if err := terminal.ConnectBrowser(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if handled, err := federationRendererHelper(); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
