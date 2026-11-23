//go:build windows

package recovery

import (
	"context"
	"errors"
)

func coordinationProviderDirectory(context.Context, string) (string, error) {
	return "", errors.New("live SSH provider directory query is unavailable on this platform")
}
