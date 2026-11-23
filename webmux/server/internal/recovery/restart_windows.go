//go:build windows

package recovery

import (
	"context"
	"errors"
)

func liveRestartSelection(context.Context, string) (string, error) {
	return "", errors.New("use the native provider lifecycle controls on this host")
}
func restartLiveProvider(context.Context, RPCRequest) (RPCResponse, error) {
	return RPCResponse{}, errors.New("use the native provider lifecycle controls on this host")
}
