package httpapi

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/jordanhubbard/webmux/server/internal/terminal"
	"golang.org/x/term"
)

// Only the native federation test enables these child modes. The provider and
// ConPTY are production implementations; the controlled terminal program is not
// a coding CLI or an authenticated recovery qualification.
func federationRendererHelper() (bool, error) {
	if os.Getenv("WEBMUX_TEST_FEDERATION_RENDERER") != "1" || len(os.Args) != 2 {
		return false, nil
	}
	switch os.Args[1] {
	case "--native-provider-bootstrap", "--native-provider-owner":
		return true, terminal.RunNativeProviderHelper(context.Background(), os.Args[1], os.Stdin, os.Stdout)
	case "--browser-shell":
		state, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return true, err
		}
		defer term.Restore(int(os.Stdin.Fd()), state)
		if err := os.WriteFile(os.Getenv("WEBMUX_TEST_FEDERATION_START"), []byte(fmt.Sprint(os.Getpid())), 0600); err != nil {
			return true, err
		}
		fmt.Print("READY;")
		var input [1]byte
		for {
			if _, err := io.ReadFull(os.Stdin, input[:]); err != nil {
				return true, err
			}
			switch input[0] {
			case 'R':
				fmt.Print("\x1bc\x1b[?7l\x1b[2;7H日Z")
			case 'C':
				fmt.Print("\x1b[2b")
			case 'P':
				fmt.Printf("\r\nP:%d;", os.Getpid())
			case 'Q':
				return true, nil
			}
		}
	}
	return false, nil
}
