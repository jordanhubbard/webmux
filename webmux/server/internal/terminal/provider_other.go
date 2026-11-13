//go:build !windows

package terminal

import (
	"context"
	"errors"
	"io"
)

func RunNativeProviderHelper(context.Context, string, io.Reader, io.Writer) error {
	return errors.New("native ConPTY provider helpers require Windows")
}
